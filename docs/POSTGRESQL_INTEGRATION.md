# PostgreSQL Integration

The Frappe operator supports PostgreSQL as a first-class database provider
alongside MariaDB. Choose per-site with `spec.dbConfig.provider: postgres`.

> **Frappe version requirement.** Frappe's PostgreSQL support ships in the
> `develop` branch (→ v17). The default stable v15 bench image does **not**
> support Postgres. You must run an operator-compatible Frappe **develop/v17**
> bench image on any bench whose sites use `provider: postgres`. See
> [Building a Postgres bench image](#building-a-postgres-bench-image).

## Modes & Engines

| Mode | Engine | What the operator does | Backing operator |
|------|--------|------------------------|------------------|
| `shared` (default) | *Agnostic* | Runs a `pg-provision` Job that `CREATE ROLE` + `CREATE DATABASE` on an **existing** PostgreSQL server | Any Postgres (StackGres, Percona, CloudNativePG, RDS, CloudSQL, etc.) |
| `dedicated` | `stackgres` (**default**) | Provisions a **per-site** `SGCluster` with shared configuration and sizing profiles | [StackGres Operator](https://stackgres.io) |
| `dedicated` | `percona` (toggle) | Provisions a **per-site** `PerconaPGCluster` (one Postgres instance per site) | [Percona PostgreSQL Operator](https://docs.percona.com/percona-operator-for-postgresql/2.0/) v2 |

Both modes honour `spec.deletionPolicy`:

- **`Retain`** (default): the database, role, and credential Secret (shared) or
  the whole database cluster (dedicated) are **kept** when the `FrappeSite` is
  deleted. GitOps-safe — an accidental CR delete or an ArgoCD prune never drops
  tenant data.
- **`Delete`**: the operator runs a `pg-delete` Job (shared, `DROP DATABASE` /
  `DROP ROLE`) or deletes the cluster CR (`SGCluster` or `PerconaPGCluster`).

### Dedicated Engine Toggle (`postgresEngine`)

Choose the dedicated PostgreSQL operator via `spec.dbConfig.postgresEngine`:
- `stackgres` (**default for new dedicated sites**): provisions via StackGres (`SGCluster`). Certified on OpenShift.
- `percona`: provisions via Percona (`PerconaPGCluster`).

```yaml
spec:
  dbConfig:
    provider: postgres
    mode: dedicated
    postgresEngine: stackgres  # "stackgres" (default) or "percona"
```

> **Backward Compatibility Guarantee**: Existing dedicated-mode sites created before `postgresEngine` existed
> probe for `<site>-postgres` `PerconaPGCluster`. If found, they continue managing the Percona cluster indefinitely.
> Only new dedicated sites default to StackGres.
> Once provisioned, `dbConfig.postgresEngine` is immutable.

## Prerequisites

### Option A: StackGres (Default)
Install StackGres via OpenShift OperatorHub (`stackgres-community` package from `community-operators`) or upstream Helm chart:

```bash
helm repo add stackgres https://stackgres.io/downloads/stackgres-k8s/stackgres/helm
helm repo update
helm install stackgres-operator stackgres/stackgres-operator -n stackgres --create-namespace
```

Or enable `INSTALL_STACKGRES=true ./install.sh`.

### Option B: Percona (Optional)
Install the Percona PostgreSQL Operator:

```bash
kubectl apply --server-side \
  -f https://raw.githubusercontent.com/percona/percona-postgresql-operator/v2.3.1/deploy/bundle.yaml
```

## Shared mode

The operator provisions a database + role on an existing server via a Job. It
needs superuser credentials to do so, supplied in a Secret with keys `user` and
`password`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: frappe-postgres-provisioner   # default name; override with dbConfig.postgresRef.name
  namespace: my-namespace
type: Opaque
stringData:
  user: postgres
  password: <superuser-password>
---
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: my-site
spec:
  benchRef: { name: pg-bench }
  siteName: my-site.example.com
  dbConfig:
    provider: postgres
    mode: shared
    # Optional. Defaults to host `frappe-postgres-pgbouncer` in the site
    # namespace. When set, the host becomes `<name>-pgbouncer` and the
    # provisioner Secret name becomes `<name>`.
    postgresRef:
      name: frappe-postgres
  deletionPolicy: Retain
```

Host resolution:

- No `postgresRef` → `frappe-postgres-pgbouncer.<namespace>.svc.cluster.local:5432`
- `postgresRef: {name: X, namespace: Y}` → `X-pgbouncer.Y.svc.cluster.local:5432`

The per-site password is stored in `Secret/<site>-db-password` (no owner
reference, so `Retain` survives site deletion).

## Dedicated mode

Dedicated mode provisions a full, isolated PostgreSQL cluster per site.

### StackGres (Default)

The operator creates a per-site `SGCluster` alongside shared configuration profiles:

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: my-site
spec:
  benchRef: { name: pg-bench }
  siteName: my-site.example.com
  dbConfig:
    provider: postgres
    mode: dedicated
    postgresEngine: stackgres  # default for new sites; can be omitted
    storageSize: 5Gi           # persistent volume size (default 2Gi)
    resources:                 # optional: generates bespoke <site>-postgres-profile
      requests:
        cpu: "1"
        memory: 2Gi
  deletionPolicy: Delete
```

The generated StackGres resources:
- `SGCluster/<site>-postgres`: runs PostgreSQL 16 with PgBouncer sidecar pooling.
- `SGPostgresConfig/frappe-postgres-dedicated-defaults`: shared namespace configuration (`postgresql.conf`).
- `SGPoolingConfig/frappe-postgres-dedicated-pooling`: shared namespace configuration (`pgbouncer.ini` in transaction pooling mode).
- `SGInstanceProfile/frappe-postgres-dedicated-default` (or `<site>-postgres-profile` if custom resources are set).
- `SGScript/<site>-postgres-script`: declarative database, role, and schema permissions script executed by StackGres controller.
- `Secret/<site>-postgres-script-secret`: script contents mounted by `SGScript`.
- Credentials stored in `Secret/<site>-db-password`.
- Note: Site-level backups via `SiteBackup` (`bench backup`) are fully supported. Cluster-level continuous PITR via `SGObjectStorage` is an optional upcoming enhancement.

### Percona (Toggle)

To provision via Percona Operator instead:

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: my-site
spec:
  benchRef: { name: pg-bench }
  siteName: my-site.example.com
  dbConfig:
    provider: postgres
    mode: dedicated
    postgresEngine: percona
    storageSize: 5Gi
  deletionPolicy: Delete
```

The generated cluster is `PerconaPGCluster/<site>-postgres` with:
- `postgresVersion: 16`, one instance (`instance1`)
- Frappe connects to the direct `<site>-postgres-primary` service
- a PVC-backed pgBackRest repo (`repo1`) for cluster-level PITR
- credentials stored in `Secret/<site>-postgres-pguser-<user>`

### Schema Ownership & Initialization

On PostgreSQL 15+, the `public` schema is locked to the database owner. Frappe requires `public` access:
- **StackGres**: The operator declaratively provisions user creation, database creation, and `ALTER SCHEMA public OWNER TO ...` using native `SGScript` and `spec.managedSql.scripts`. The site checks script completion status before reporting `DatabaseReady`.
- **Percona**: The operator connects as the cluster superuser and runs a one-time idempotent configure Job that hands the database to the app user and sets its `search_path` to `public`.

> The Percona CRD constrains the role name to a DNS label, so the operator uses
> a stable label-safe name (`u<hash>`) derived from the site for Percona. StackGres
> uses standard database user identifiers matching the site name.

### Image overrides

Dedicated-cluster component images default to Percona v2.3.1 (PostgreSQL 16) and
can be pinned/mirrored via Helm (or the equivalent operator env vars):

```yaml
# values.yaml
postgres:
  percona:
    postgresImage:   "myregistry/percona-postgresql-operator:2.3.1-ppg16-postgres"
    pgBouncerImage:  "myregistry/percona-postgresql-operator:2.3.1-ppg16-pgbouncer"
    pgBackRestImage: "myregistry/percona-postgresql-operator:2.3.1-ppg16-pgbackrest"
```

Env vars: `FRAPPE_PERCONA_POSTGRES_IMAGE`, `FRAPPE_PERCONA_PGBOUNCER_IMAGE`,
`FRAPPE_PERCONA_PGBACKREST_IMAGE`.

## Building a Postgres bench image

`provider: postgres` needs a Frappe build from `develop` (→ v17). The
`frappe-erpnext-images-for-operator` repo publishes an operator-compatible
develop image; its `develop` base already ships psycopg2 + libpq + psql, so it is
Postgres-capable out of the box. Build/publish it by running that repo's
`container-build.yml` workflow with `frappe_version=develop` (tag `v17`), then
point the bench at it:

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: pg-bench
spec:
  frappeVersion: "develop"
  imageConfig:
    repository: ghcr.io/vyogotech/erpnext-for-operator
    tag: v17
    pullPolicy: IfNotPresent
```

## End-to-end example

A full both-modes manifest is in
[`examples/kind-e2e-postgres-manifests.yaml`](../examples/kind-e2e-postgres-manifests.yaml).

## Validation & selection summary

- `dbConfig.provider`: `mariadb` (default) | `postgres` | `sqlite` | `external`
- `postgresRef` is only valid when `provider: postgres`; `mariadbRef` only when
  `provider: mariadb` (the admission webhook rejects mismatches).
- `dedicated` mode is supported for `mariadb` and `postgres` only.
- Switching an existing site's provider is **not** supported — provider is fixed
  at creation.
