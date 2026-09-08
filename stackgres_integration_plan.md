# Implementation Plan: StackGres as Default PostgreSQL Engine (with Percona Toggle)

## Context Established From Code

- `controllers/database/postgres_provider.go`: `PostgresProvider.EnsureDatabase` branches only on `site.Spec.DBConfig.Mode` (`shared`/`dedicated`). Shared mode (`ensureSharedPostgres` + `getSharedHostPort`) is already provider-agnostic as of commit `d809214` — `getSharedHostPort` honors `dbConfig.host` verbatim and only falls back to the Percona-specific `<postgresRef>-pgbouncer` guess when `host` is unset. Dedicated mode (`ensureDedicatedPostgres`, `ensureDedicatedConfigured`) hardcodes `PerconaPGClusterGVK` (`pgv2.percona.com/v2`, kind `PerconaPGCluster`) — this is the only remaining Percona-specific code path.
- `api/v1/shared_types.go`: `DatabaseConfig` (aliased as `dbConfig` on both `FrappeSite` and `FrappeBench`) already carries `Provider`, `Mode`, `PostgresRef`, `Host`, `Port`, `StorageSize`, `Resources` — but **no engine field**. This struct is shared by both CRDs, so one new field lights up on both.
- `controllers/site_lifecycle.go:resolveDBConfig` merges site-level `dbConfig` over bench-level `dbConfig` field-by-field (`if config.X == "" { config.X = bench...X }`) — any new field must be added to this merge or it silently won't inherit from the bench.
- `api/v1/frappesite_webhook.go:validateSite` enforces provider/mode/ref combinations, but **`ValidateUpdate` never compares `oldObj` to the new object** — it just calls `r.validateSite()` on the newly-decoded object. So the docs' claim ("switching provider is not supported") is aspirational only; nothing today actually blocks it.
- `examples/stackgres-postgres.yaml` and its comments are the most detailed existing knowledge of StackGres's shape: `SGInstanceProfile` (sizing), `SGPostgresConfig` (postgresql.conf), `SGPoolingConfig` (pgbouncer.ini, sidecar not a separate Service), `SGCluster` (references the three by name), and a superuser Secret named after the cluster. It explicitly states dedicated mode is unsupported today: *"Only shared mode is supported on StackGres... a per-site StackGres cluster would mean teaching the provider to emit SGCluster CRs, which this does not do."*
- RBAC for `pgv2.percona.com`/`perconapgclusters` exists in `config/rbac/role.yaml` (kubebuilder-generated) but is **absent from `helm/frappe-operator/templates/rbac/clusterrole.yaml`** (hand-maintained, per its own comment: *"Keep in step with config/rbac/role.yaml; this chart's rules are maintained by hand"*) and **absent from `bundle/manifests/frappe-operator.clusterserviceversion.yaml`**'s `clusterPermissions`. Since `install.sh` deploys the Helm chart, every Helm-based install today is missing RBAC for the CRD it needs for dedicated-mode Percona. This is a pre-existing gap this plan must close for both engines, not just StackGres.
- `install.sh` does not install Percona's operator at all (`docs/POSTGRESQL_INTEGRATION.md` tells the user to `kubectl apply` Percona's bundle manually) and uses no OLM `Subscription` anywhere — its only OpenShift-specific step is `INSTALL_POSTGRES_SCC` (a hand-authored `SecurityContextConstraints` for Percona's uid/gid‑26, no-fsGroup containers).
- `controllers/database/provider.go:NewPostgresProvider(client, scheme)` takes no config — every method reads `site.Spec.DBConfig` fresh per call. An engine toggle fits this pattern exactly: read from `site.Spec.DBConfig` inside the dedicated-mode methods, same as `mode` is today.
- `api/v1/sitebackup_types.go` / `controllers/sitebackup_controller.go`: `SiteBackup` is a `bench backup`-based, app-level mechanism entirely independent of the database engine's own cluster-level backup story. It has nothing to do with Percona's `pgbackrest` block in `ensureDedicatedPostgres`. That block is purely "does the per-site Postgres cluster get free PITR out of the box," which is the actual StackGres gap to design around.
- `api/v1/shared_types.go` already defines a reusable `S3Config` (endpoint/bucket/region/access+secret key selectors, used by `SiteBackup`'s `BackupStorageConfig.S3`) — this is structurally exactly what an `SGObjectStorage` needs, so it doesn't need to be invented from scratch when that phase comes.
- `controllers/database/postgres_provider_test.go` / `postgres_names_test.go` establish the existing unit-test idiom (fake client + scheme, `pgTestSetup()`/`pgSite()` helpers, asserting on `unstructured.Unstructured` shape, a `dnsLabel` regexp helper) that new tests should extend rather than duplicate.

---

## 1. The Engine Toggle

### Field design

Add to `DatabaseConfig` in `api/v1/shared_types.go`, next to `Provider`:

```go
// PostgresEngine selects which operator provisions dedicated-mode PostgreSQL
// clusters. Only meaningful when Provider is "postgres"; shared mode is
// already engine-agnostic (it just needs a reachable host:port; see
// dbConfig.host). Empty is resolved dynamically at reconcile time rather than
// defaulted in the CRD schema — see the backward-compatibility note below.
// +kubebuilder:validation:Enum=stackgres;percona
// +optional
PostgresEngine string `json:"postgresEngine,omitempty"`
```

**Naming**: `postgresEngine`, not a bare `engine` — `DatabaseConfig` is shared with MariaDB, which has no engine choice, so a generic name would be misleading on a `FrappeBench`/`FrappeSite` using `provider: mariadb`. Not `dedicatedPostgresProvider` — verbose, and the field is validated identically to how `postgresRef`/`mariadbRef` are already provider-scoped, so `postgresEngine` reads consistently next to `provider`.

**Critically: do NOT put `+kubebuilder:default=stackgres` on this field.** CRD structural-schema defaults in Kubernetes are applied at *decode time*, including for objects already sitting in etcd that predate the field — not just on admission of new objects. If this field carries a schema default of `stackgres`, then the instant the CRD is updated, every existing dedicated-mode `FrappeSite` (today implicitly Percona, since it's the only engine that ever existed) would start reporting `postgresEngine: stackgres` the next time the controller `Get`s it — flipping `ensureDedicatedPostgres` over to emit an `SGCluster` for a site that already has a live `PerconaPGCluster` and real data. That is a correctness bug, not just a documentation nuance, and it is the single biggest risk in this whole plan.

Instead, resolve the effective engine in code, in a new helper alongside the existing `mode` defaulting in `postgres_provider.go`:

```go
// resolvePostgresEngine returns the dedicated-mode engine for site. Explicit
// dbConfig.postgresEngine always wins. Otherwise: if a PerconaPGCluster
// already exists for this site (pre-upgrade dedicated-mode sites created
// before this field existed), grandfather it in as "percona" so upgrading the
// operator cannot silently swap a live cluster out from under a site. Only a
// genuinely new dedicated-mode site with no existing cluster defaults to the
// new "stackgres" default.
func (p *PostgresProvider) resolvePostgresEngine(ctx context.Context, site *vyogotechv1.FrappeSite) (string, error) {
    if e := site.Spec.DBConfig.PostgresEngine; e != "" {
        return e, nil
    }
    existing := &unstructured.Unstructured{}
    existing.SetGroupVersionKind(PerconaPGClusterGVK)
    err := p.client.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-postgres", site.Name), Namespace: site.Namespace}, existing)
    if err == nil {
        return "percona", nil // grandfather an already-provisioned cluster
    }
    if !errors.IsNotFound(err) {
        return "", err
    }
    return "stackgres", nil // new site, new default
}
```

This is called from `EnsureDatabase`, `IsReady`, `GetCredentials`, and `Cleanup` wherever `mode == "dedicated"` today, exactly parallel to how `mode` itself is defaulted inline (`if mode == "" { mode = "shared" }`).

As a belt-and-suspenders complement (not a substitute): ship a short migration note in `docs/POSTGRESQL_INTEGRATION.md` recommending operators explicitly set `dbConfig.postgresEngine: percona` on existing dedicated-mode `FrappeSite`/`FrappeBench` specs at their convenience post-upgrade, so the behavior stops depending on a live cluster-existence probe and becomes visible/declarative in the spec. The probe is the safety net; the explicit field is the long-term source of truth.

### Where it lives in the CRD

Since `DatabaseConfig` is the type behind both `FrappeSiteSpec.DBConfig` (`api/v1/frappesite_types.go:50`) and `FrappeBenchSpec.DBConfig` (a `*DatabaseConfig`, per `site_lifecycle.go:314`), the field is automatically available at both bench and site level with no separate type needed. It must be added to `resolveDBConfig` in `controllers/site_lifecycle.go` so a site can inherit the bench's choice:

```go
if config.PostgresEngine == "" {
    config.PostgresEngine = bench.Spec.DBConfig.PostgresEngine
}
```

### Backward compatibility for existing Percona dedicated-mode users

Covered above via the probe-based resolver. Concretely, this means:
- Existing `FrappeSite`s with `mode: dedicated, provider: postgres` and no `postgresEngine` set continue to resolve to Percona forever, as long as their `PerconaPGCluster` still exists.
- Brand-new dedicated-mode sites created after this ships default to StackGres with zero spec changes required — satisfying "StackGres as the default... with a toggle to keep Percona available."
- Users who want Percona for a *new* site set `postgresEngine: percona` explicitly.
- `Cleanup`, `IsReady`, and `GetCredentials` must all resolve the engine the same way (via the same helper) — a mismatch between what `EnsureDatabase` created and what `Cleanup`/`IsReady` believes exists would be its own class of bug, so this needs one shared code path, not four ad hoc re-derivations.

### Validation webhook changes (`api/v1/frappesite_webhook.go`)

In `validateSite()`, alongside the existing provider/mode-ref checks:

```go
if r.Spec.DBConfig.PostgresEngine != "" {
    if provider != "postgres" {
        return fmt.Errorf("dbConfig.postgresEngine is only valid when dbConfig.provider is 'postgres'")
    }
    switch r.Spec.DBConfig.PostgresEngine {
    case "stackgres", "percona":
    default:
        return fmt.Errorf("dbConfig.postgresEngine must be one of 'stackgres', 'percona'")
    }
}
```

(The `+kubebuilder:validation:Enum` on the field gives API-server-level enforcement for free; the webhook check above is mainly for the provider-scoping rule, mirroring the existing `postgresRef`/`mariadbRef` scoping checks a few lines below it.)

Separately — and this is new, not currently enforced for *anything* on this type — add real `ValidateUpdate` immutability checks, since `ValidateUpdate` today silently ignores `oldObj`:

```go
func (r *FrappeSite) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
    if err := r.validateSite(); err != nil {
        return nil, err
    }
    old, ok := oldObj.(*FrappeSite)
    if ok && old.Spec.DBConfig.Mode == "dedicated" && old.Spec.DBConfig.PostgresEngine != "" &&
        r.Spec.DBConfig.PostgresEngine != "" && r.Spec.DBConfig.PostgresEngine != old.Spec.DBConfig.PostgresEngine {
        return nil, fmt.Errorf("dbConfig.postgresEngine cannot be changed after a dedicated cluster has been provisioned")
    }
    return nil, nil
}
```

This closes a real gap (provider-switching was already undocumented-but-unenforced) at the exact moment a second axis of "which cluster owns my data" is introduced, where the blast radius of silently ignoring it is much worse (orphaning a live database cluster, not just switching an already-empty shared connection).

Add table-driven cases to `api/v1/webhook_test.go` next to the existing `"postgres dedicated is valid without a ref"` cases: `postgresEngine` valid on `provider: postgres`, rejected on `provider: mariadb`, rejected on invalid enum value, and a `ValidateUpdate` test asserting the immutability rule.

---

## 2. Dedicated-Mode StackGres Support

### CRs the provider must emit

Minimum viable per-site set, mirroring `ensureDedicatedPostgres`'s structure:

- **`SGCluster`** (`stackgres.io/v1`) — one per site, named `<site>-postgres` (same naming convention as `PerconaPGCluster` today), in `site.Namespace`. `spec.postgres.version`, `spec.instances: 1`, `spec.pods.persistentVolume.size` from `dbConfig.StorageSize` (fallback `defaultDedicatedStorageSize`), `spec.sgInstanceProfile`/`spec.configurations.sgPostgresConfig`/`spec.configurations.sgPoolingConfig` referencing the supporting CRs below.
- **`SGInstanceProfile`, `SGPostgresConfig`, `SGPoolingConfig`** — these must exist before/alongside the `SGCluster` (StackGres CRs reference them by name; there is no inline equivalent in `SGCluster.spec`, unlike Percona which embeds everything in one `PerconaPGCluster.spec`). Decision: **default to one shared, lazily-created set per namespace** rather than one bespoke triple per site:
  - There is currently no per-site knob in `DatabaseConfig` for arbitrary `postgresql.conf`/pgbouncer tuning, so a bespoke `SGPostgresConfig`/`SGPoolingConfig` per site would just be N identical copies of the same operator-chosen defaults — pure CR sprawl with no configurability behind it. Ensure one `frappe-postgres-dedicated-defaults` `SGPostgresConfig` and one `frappe-postgres-dedicated-pooling` `SGPoolingConfig` per namespace (create-if-absent, same "no owner reference so Retain works" pattern used elsewhere in this file), and reference them from every dedicated `SGCluster` in that namespace.
  - `SGInstanceProfile` (cpu/memory sizing), by contrast, already has a per-site analog in the API: `dbConfig.Resources` (`*ResourceRequirements`, already on `DatabaseConfig`, currently unused by the postgres provider at all). Default to a shared `frappe-postgres-dedicated-default` profile sized from `defaultDedicatedStorageSize`-equivalent constants, but emit a bespoke `<site>-postgres-profile` **only when `dbConfig.Resources` is explicitly set** — same "override only if the user actually asked" philosophy as `storageSize`.
  - This keeps the common case (no per-site tuning) to exactly one new CR (`SGCluster`) per site plus three namespace-shared CRs total, instead of four CRs per site.

### Database/user provisioning (no `users[].databases[]` in StackGres)

`SGCluster.spec` has no equivalent of Percona's `spec.users[].databases[]` — confirmed by `examples/stackgres-postgres.yaml`'s `SGCluster` block, which has no `users` key at all. The plan reuses the **existing shared-mode SQL-provisioning-Job pattern** (`ensureSharedPostgres`'s `psql ... CREATE ROLE ... CREATE DATABASE ...` container) rather than inventing a second mechanism:

1. Once the `SGCluster` reports ready, look up its auto-generated superuser Secret (StackGres names this after the cluster, e.g. `<cluster-name>` per the example's comment — **exact key names must be confirmed against a live install before hardcoding**, flagged as an open risk below).
2. Run a provisioning Job, structurally identical to `ensureSharedPostgres`'s script, connecting as that superuser to `CREATE ROLE`/`CREATE DATABASE` for the site — using `generatePGUserName` for the role name (StackGres roles are not constrained to a DNS label the way Percona's are, since there's no derived-Secret-name coupling, but keeping the same label-safe generator avoids a second identifier-format rule to maintain).
3. Reuse `ensureDedicatedConfigured` **as-is, generalized** rather than duplicated: the "public schema is locked to the DB owner on PG15+" behavior it fixes is a PostgreSQL server behavior, not a Percona behavior — it will bite StackGres-backed clusters running PG15/16 identically. Parameterize `ensureDedicatedConfigured` by cluster host/superuser-secret-name instead of hardcoding the `<site>-postgres` / `<site>-postgres-pguser-postgres` Percona names, and call it from both engine paths. This turns what looked like "new Percona-specific code to port" into "one existing function made engine-agnostic," which is the same shape of change already made to `getSharedHostPort` in the current branch.

### Backup differences

- Percona's dedicated mode gets a PVC-backed `pgbackrest` repo (`ensureDedicatedPostgres`'s `backups.pgbackrest.repos` block) for free, purely as a field in the CR spec — continuous WAL archiving/PITR with no external dependency beyond a PVC.
- StackGres's cluster-level backup (`SGCluster.spec.configurations.backups[]`) requires an `SGObjectStorage` CR, which is **S3-only** — there is no "just give me a PVC" equivalent in StackGres's backup model.
- Decision for this plan: **do not block the default-engine switch on backup parity.** Ship dedicated-mode StackGres without an automatic cluster-level backup CR in the first phase; sites still get the existing, engine-independent `SiteBackup`/`bench backup` mechanism (`api/v1/sitebackup_types.go`), which was always separate from Percona's `pgbackrest` block and unaffected either way. Document explicitly (in `docs/POSTGRESQL_INTEGRATION.md`) that choosing StackGres dedicated mode without additional configuration means no cluster-level PITR — a real, named capability gap versus Percona, not a silently-dropped feature.
- As an optional, deferred enhancement: reuse the existing `S3Config` type (`api/v1/shared_types.go`) as the shape for a new opt-in `dbConfig.postgresBackup *S3Config`-style field, which the provider would translate into an `SGObjectStorage` + a `backups[]` entry on the `SGCluster`. Because `S3Config` already exists and is already used by `SiteBackup.Storage.S3`, this is additive, not a new design problem, when/if prioritized.

### Connection routing

Percona's `ensureDedicatedPostgres` has this comment: *"Connect Frappe to the PRIMARY service, not pgbouncer. Percona's pgBouncer defaults to transaction pooling, which breaks `bench new-site`/`bench migrate`... The `<cluster>-primary` service is a stable, direct session connection."* That comment is now **empirically outdated** per the already-established finding that a real PgBouncer in transaction-pooling mode does *not* break `bench new-site`/`bench migrate` (validated via CNPG's Pooler as a stand-in).

StackGres structurally can't do the Percona-style bypass anyway: it runs PgBouncer as a sidecar reachable through the *same* cluster Service (`examples/stackgres-postgres.yaml`'s header comment: *"StackGres publishes its primary as `<cluster>` and runs PgBouncer as a sidecar behind that same Service"*) rather than exposing a separate `-primary` Service the way Percona does. There is no clean "just point at the other Service" escape hatch to mirror.

Combining both facts, the design for StackGres dedicated mode is: **connect through the SGCluster's regular Service, pooled, with no primary-bypass logic.** This simplifies dedicated-mode routing relative to Percona (no second host string to derive, no dependency on a `-primary` Service naming convention that may not even exist for StackGres) and is consistent with the already-validated shared-mode behavior. Flag as the phase's top open risk: this has only been validated against CNPG's Pooler, not StackGres's actual PgBouncer sidecar — re-validate `bench new-site`/`bench migrate` against a real StackGres pooled connection before relying on it (see Testing Plan). If it turns out StackGres's pooler behaves differently, the fallback is to expose the cluster's non-pooled port explicitly (StackGres does allow disabling pooling per-service/port in newer versions) — keep that as a documented fallback, not a default.

---

## 3. install.sh / install.yaml Changes

Today `install.sh` installs zero PostgreSQL operator (Percona is entirely BYO per `docs/POSTGRESQL_INTEGRATION.md`), and uses no OLM `Subscription` pattern anywhere — its closest analog is the `INSTALL_MARIADB_CRDS` step (`kubectl apply --server-side -k github.com/.../config/crd?ref=...`, with a URL-list fallback) and `INSTALL_POSTGRES_SCC` (opt-in, OCP-only, detected via `kubectl api-resources --api-group=security.openshift.io`).

Add a new **opt-in, OCP-aware** step following the same idiom:

```bash
INSTALL_STACKGRES="${INSTALL_STACKGRES:-false}"
STACKGRES_NAMESPACE="${STACKGRES_NAMESPACE:-stackgres}"

# Step 2c: StackGres PostgreSQL Operator (optional)
if [ "$INSTALL_STACKGRES" = "true" ]; then
    if kubectl api-resources --api-group=security.openshift.io 2>/dev/null | grep -q securitycontextconstraints; then
        # OpenShift: OLM Subscription against the community catalog.
        kubectl create namespace "$STACKGRES_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
        cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: stackgres-og
  namespace: $STACKGRES_NAMESPACE
spec:
  targetNamespaces:
  - $STACKGRES_NAMESPACE
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: stackgres-community
  namespace: $STACKGRES_NAMESPACE
spec:
  channel: stable
  name: stackgres-community
  source: community-operators
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
EOF
        kubectl wait --for condition=established --timeout=120s crd sgclusters.stackgres.io || true
    else
        # Non-OpenShift: upstream Helm chart (see examples/stackgres-postgres.yaml header).
        helm repo add stackgres https://stackgres.io/downloads/stackgres-k8s/stackgres/helm
        helm repo update
        helm install stackgres-operator stackgres/stackgres-operator \
            -n "$STACKGRES_NAMESPACE" --create-namespace
    fi
fi
```

Notes:
- **Opt-in, defaulting to `false`**, matching `INSTALL_POSTGRES_SCC`'s existing pattern — most `install.sh` runs don't provision Postgres at all, and this step needs cluster-scoped OLM permissions (`OperatorGroup`/`Subscription` creation) that not every caller of this script has.
- `source: community-operators` / `sourceNamespace: openshift-marketplace` and the exact package's supported `installModes` (`AllNamespaces` vs `OwnNamespace`) need confirming against a real cluster's `PackageManifest` before this is finalized (`oc get packagemanifest stackgres-community -n openshift-marketplace -o yaml`) — the channel name `stable` and package name `stackgres-community` are already confirmed per the example's header comment, but the `OperatorGroup`/namespace shape is not yet live-tested. Track as a testing-phase task, not a blocker to writing the script.
- `install.yaml` is a generated/rendered artifact (contains the rendered CRDs/RBAC currently, e.g. the `pgv2.percona.com` RBAC rule and `defaultPostgresImage` seen at line 7523) — it should be regenerated from the Helm chart/kustomize sources after those are updated (Section 4), not hand-edited.
- Also extend the existing `INSTALL_POSTGRES_SCC` step's doc/behavior: it's currently named/scoped around Percona's known uid/gid‑26-no-fsGroup requirement. Whether StackGres needs an equivalent custom SCC is **unconfirmed** (StackGres has not been installed on a real cluster in this effort) — default to *not* creating one for StackGres and only add a StackGres-specific SCC if live testing shows pods failing to start under the default `restricted-v2` SCC (most modern operators designed for OpenShift compatibility run under arbitrary UIDs without needing this). Flag explicitly in the testing checklist.

---

## 4. RBAC Changes (`config/rbac/role.yaml`)

Add a new `stackgres.io` block (`config/rbac/role.yaml` is `controller-gen`-generated via `make manifests`, so the actual source of truth is a new kubebuilder marker, likely alongside the existing Percona marker in `controllers/frappesite_controller.go:69`):

```go
//+kubebuilder:rbac:groups=stackgres.io,resources=sgclusters;sginstanceprofiles;sgpostgresconfigs;sgpoolingconfigs,verbs=get;list;watch;create;update;patch;delete
```

After `make manifests`, this lands in `config/rbac/role.yaml` as:

```yaml
- apiGroups:
  - stackgres.io
  resources:
  - sgclusters
  - sginstanceprofiles
  - sgpostgresconfigs
  - sgpoolingconfigs
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
```

(If the backup phase (Section 2) is pursued later, add `sgobjectstorages`/`sgbackups` at that time rather than up front.)

**Two things this plan must fix that are pre-existing, not new**, because they directly undermine "keep Percona available" and the OCP-certification goal:

1. `helm/frappe-operator/templates/rbac/clusterrole.yaml` has **no `pgv2.percona.com` rule at all** today, despite its own comment promising to stay in sync with `config/rbac/role.yaml`. Since `install.sh` deploys this chart, dedicated-mode Percona is effectively RBAC-broken on every Helm-based install today. Add both the `pgv2.percona.com`/`perconapgclusters` block (fixing the existing gap) and the new `stackgres.io` block to this hand-maintained file in the same change.
2. `bundle/manifests/frappe-operator.clusterserviceversion.yaml`'s `install.spec.clusterPermissions` is similarly missing the Percona rule — and this is the artifact OpenShift's certification pipeline actually scans. Regenerate it via `make bundle` after `config/rbac/role.yaml` is updated (Makefile target `bundle: manifests kustomize`), verifying both `pgv2.percona.com` and `stackgres.io` appear in the regenerated CSV, and diff it against the currently-committed (stale) bundle to confirm no unrelated drift slipped in unnoticed.

---

## 5. Testing Plan

### Unit tests (`controllers/database/`)

Extend `postgres_provider_test.go` / add a new `postgres_provider_stackgres_test.go`, following the existing `pgTestSetup()`/`pgSite()`/fake-client idiom:

- `TestPostgresProvider_DedicatedEngineDefaultsToStackGres` — new dedicated-mode site, no `postgresEngine` set, no existing `PerconaPGCluster` → `EnsureDatabase` emits an `SGCluster`, not a `PerconaPGCluster`.
- `TestPostgresProvider_DedicatedEngineGrandfathersExistingPercona` — pre-seed a `PerconaPGCluster` named `<site>-postgres` in the fake client (simulating a pre-upgrade site), then call `EnsureDatabase` with no `postgresEngine` set → asserts it still manages the Percona cluster, never creates an `SGCluster`. This is the regression test for the backward-compatibility design in Section 1 and should be treated as load-bearing.
- `TestPostgresProvider_DedicatedEngineExplicitPercona` / `...ExplicitStackGres` — explicit `postgresEngine` always wins regardless of what exists.
- `TestPostgresProvider_StackGresClusterShape` — mirrors `TestPostgresProvider_DedicatedClusterShape`: assert the emitted `SGCluster` has `spec.postgres.version`, `spec.instances`, `spec.pods.persistentVolume.size`, and references to the (namespace-shared or per-site) `SGInstanceProfile`/`SGPostgresConfig`/`SGPoolingConfig`; assert the supporting CRs are created when absent and reused when present (idempotency, matching the existing "no OwnerReference, adopt existing" pattern).
- `TestPostgresProvider_StackGresConfiguredJob` — asserts the generalized `ensureDedicatedConfigured` runs against StackGres's superuser secret name/host, reusing the same `-postgres-configure` Job assertions as the Percona test.
- `api/v1/webhook_test.go` additions per Section 1 (enum validation, provider-scoping, `ValidateUpdate` immutability).
- `TestPostgresProvider_GenerateDBName`/`postgres_names_test.go`-style hash tests need no changes (engine choice doesn't affect naming), but worth a quick regression run to confirm nothing in the new code paths reintroduces the `[:8]` slice panic class of bug for leading-zero hashes.

### Live-cluster validation checklist (real StackGres, not the CNPG stand-in)

1. Install real StackGres via the `install.sh` OLM path (or upstream Helm) on the same OCP cluster used for the earlier CNPG/Percona testing; confirm `SGCluster`/`SGInstanceProfile`/`SGPostgresConfig`/`SGPoolingConfig` CRDs are `Established`.
2. Confirm the actual key names in StackGres's auto-generated superuser Secret (the example yaml's comment says "confirm the key names against your StackGres version before relying on them" — resolve this before hardcoding in the provisioning Job).
3. Deploy a dedicated-mode `FrappeBench`/`FrappeSite` (`provider: postgres, mode: dedicated`, default engine) and confirm: `SGCluster` reaches ready, the provisioning Job creates the role/database, `ensureDedicatedConfigured` succeeds (public-schema ownership fix), and the site reaches `Ready`.
4. Re-run the same test already validated against CNPG's Pooler — `bench new-site` and `bench migrate` against StackGres's actual pgbouncer sidecar, pooled, through the regular cluster Service (the routing decision in Section 2) — this is the single most important live check, since it was only proven against a stand-in, not the real component.
5. Backup/restore: since dedicated-mode StackGres ships without automatic cluster-level backup in phase 1, verify the engine-independent `SiteBackup`/`bench backup` path still works unchanged against a StackGres-backed dedicated site (it should, since it goes through Frappe's own dump path, not the DB engine's snapshot mechanism) — this closes the "verify backup/restore" requirement for phase 1 scope; a separate SGObjectStorage-based check is only relevant once/if that optional phase ships.
6. Confirm pod startup under OpenShift's default `restricted-v2` SCC with no custom SCC added; only author a StackGres-specific SCC (mirroring `percona-pg-fsgroup`) if this fails.
7. Delete a `Delete`-policy dedicated StackGres site and confirm `Cleanup` removes the `SGCluster` (and does not touch the namespace-shared `SGInstanceProfile`/`SGPostgresConfig`/`SGPoolingConfig`, which other sites may still reference).
8. Upgrade scenario dry run: take an existing dedicated-mode Percona `FrappeSite` (no `postgresEngine` set), upgrade the operator to the version with this change, and confirm the reconcile loop leaves the `PerconaPGCluster` alone (exercises the grandfather-probe logic against a real API server, not just the fake client).

---

## 6. Migration/Rollout Considerations for Existing Percona Users

- No spec changes are required from existing users at upgrade time — covered by the probe-based engine resolution (Section 1). This is the load-bearing guarantee and is why it's called out twice.
- Two RBAC fixes ship in the same release as this feature (Section 4): the Helm chart and OLM bundle both gain the `pgv2.percona.com` rule they were silently missing. Existing Percona dedicated-mode users on Helm installs may currently be tolerating denied-but-non-fatal RBAC errors (or have manually patched their `ClusterRole`) — call this out in release notes as "this release also fixes Percona RBAC that Helm/OLM installs were previously missing," since some users may have their own out-of-band patch that would now be redundant.
- Recommend (not require) that operators explicitly set `dbConfig.postgresEngine: percona` on their existing dedicated-mode specs post-upgrade, turning an implicit (probe-based) guarantee into an explicit, GitOps-visible one. Document this as a one-time, low-urgency cleanup step in `docs/POSTGRESQL_INTEGRATION.md`, not a required migration.
- `docs/POSTGRESQL_INTEGRATION.md` needs a rewrite pass: today it presents Percona as the only dedicated-mode option and states shared-mode host defaults (`frappe-postgres-pgbouncer.<namespace>...`) that are already stale relative to the `d809214` commit on this branch. This is a pre-existing doc/code drift independent of this feature, but should be fixed in the same PR since the doc is being substantially rewritten anyway (add the "Modes" table's third dimension: engine; document the StackGres backup gap from Section 2; correct the shared-mode host-resolution section to match current `getSharedHostPort` behavior).
- No data migration path (Percona cluster → StackGres cluster) is in scope for this plan — if a user wants to move an existing dedicated-mode site from Percona to StackGres, that's a manual dump/restore via `SiteBackup`/`SiteRestore` today, called out explicitly as out of scope rather than silently unaddressed.

---

## 7. Phased Milestones

### Phase 0 — Engine toggle + backward-compat plumbing (Small)
- `DatabaseConfig.PostgresEngine` field, `resolveDBConfig` merge, `resolvePostgresEngine` probe helper, webhook validation + immutability check.
- No new provisioning behavior yet — dedicated mode still only knows how to build a `PerconaPGCluster`; the field is plumbed but `EnsureDatabase` errors clearly if `postgresEngine == "stackgres"` is resolved (temporary `fmt.Errorf("stackgres dedicated-mode support ships in a later phase")`), so this phase is safely mergeable/shippable on its own.
- **Biggest risk**: getting the grandfather-probe semantics exactly right — an off-by-one in "does a PerconaPGCluster exist" logic is the difference between "seamless upgrade" and "silently orphaned production database." Mitigate with the dedicated regression test in Section 5 before this phase is considered done.

### Phase 1 — RBAC + install.sh fixes (Small)
- `stackgres.io` kubebuilder marker, regenerate `config/rbac/role.yaml`, hand-patch `helm/.../clusterrole.yaml` (both the new StackGres rule and the missing Percona rule), regenerate `bundle/manifests/...clusterserviceversion.yaml` via `make bundle`, add the opt-in `INSTALL_STACKGRES` step to `install.sh`.
- Can ship independently of Phase 2/3 (it's pure RBAC/install-tooling, doesn't touch the provider's runtime behavior) and unblocks live-cluster testing for the phases after it.
- **Biggest risk**: the exact OLM `Subscription`/`OperatorGroup` shape for `stackgres-community` on a real OCP cluster hasn't been confirmed (only the channel/package name are known from the example yaml's comment) — get this wrong and `install.sh`'s new step silently fails or half-installs. Mitigate by treating the live-cluster StackGres install (Testing Plan step 1) as a gate before this phase is called "done," not just "merged."

### Phase 2 — Dedicated-mode StackGres provisioning (Large)
- `SGCluster` + shared `SGInstanceProfile`/`SGPostgresConfig`/`SGPoolingConfig` emission, generalized `ensureDedicatedConfigured`, superuser-secret-based provisioning Job reusing the shared-mode SQL pattern, connection routing through the pooled Service (Section 2).
- Depends on Phase 0 (toggle) and Phase 1 (RBAC, so the operator is even permitted to create these CRs) being merged first.
- **Biggest risk**: this is the only genuinely untested-on-real-infrastructure part of the whole plan. Two specific unknowns compound here: (a) the exact superuser Secret key names StackGres generates, and (b) whether real StackGres PgBouncer behaves like the CNPG stand-in already validated for `bench new-site`/`bench migrate`. Either could force a design change (e.g., needing a non-pooled port after all) mid-phase — budget for a live-cluster spike before committing to the final code shape, not just at the end.

### Phase 3 — Documentation + phase-1 backup gap disclosure (Small)
- Rewrite `docs/POSTGRESQL_INTEGRATION.md` (three-mode table: shared/dedicated × mariadb/postgres × stackgres/percona engine; corrected shared-mode host resolution; explicit "no automatic cluster-level backup for StackGres dedicated mode yet" callout).
- Ships alongside or immediately after Phase 2, gated on Phase 2's real behavior (don't document StackGres dedicated mode before it's been live-tested).
- **Biggest risk**: low — mostly a communication risk (users assuming backup parity with Percona that doesn't exist yet) rather than a technical one; mitigated entirely by being explicit in the doc.

### Phase 4 — Optional: StackGres backup parity via `SGObjectStorage` (Medium, deferred)
- New opt-in `dbConfig.postgresBackup` (reusing the existing `S3Config` shape) → `SGObjectStorage` + `SGCluster.spec.configurations.backups[]`.
- Explicitly deferred: not required for the OCP-certification motivation (which is about the *engine*, not backup features), and StackGres's S3-only backup model is a genuine scope increase (credentials management, bucket lifecycle) that shouldn't block shipping the default-engine switch.
- **Biggest risk**: scope creep — this is the phase most likely to be requested by a customer prematurely; keep it clearly separated from Phases 0–3 so "ship StackGres as default" isn't blocked waiting on it.

### Critical Files for Implementation

- controllers/database/postgres_provider.go
- api/v1/shared_types.go
- api/v1/frappesite_webhook.go
- controllers/site_lifecycle.go
- helm/frappe-operator/templates/rbac/clusterrole.yaml
- install.sh
- docs/POSTGRESQL_INTEGRATION.md
