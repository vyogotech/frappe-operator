# Installing via OLM (OperatorHub / Installed Operators)

The Helm chart installs the operator as an ordinary Deployment. That works, but
the operator will **not** appear under *Operators → Installed Operators* in the
OpenShift console, because that page lists `ClusterServiceVersion` objects and
only operators installed through the Operator Lifecycle Manager have one.

Installing through OLM gives you the console entry, the Provided APIs list, and
managed upgrades through a `Subscription`.

```bash
./install-olm.sh
```

## Before you start

**Stop the Helm-managed operator first.** Two copies cannot run together: both
install in `AllNamespaces` mode and would reconcile the same `FrappeSite`
resources against each other. The script refuses to run until you do this, and
will not touch your deployment on its own.

```bash
oc scale deployment/frappe-operator-controller-manager -n frappe-operator-system --replicas=0
```

Scaling to zero is deliberate, and much safer than uninstalling:

- Your CRDs live in the chart's `crds/` directory, which Helm never deletes, so
  every `FrappeSite` and `FrappeBench` survives regardless.
- The chart installs the **MariaDB operator as a subchart**, and the OLM bundle
  does not include it. A `helm uninstall` would take MariaDB management away
  from any site using it. Scaling only the frappe deployment leaves it running.

To go back to Helm at any point:

```bash
./install-olm.sh --uninstall
oc scale deployment/frappe-operator-controller-manager -n frappe-operator-system --replicas=1
```

## How it decides what to do

The script picks sensible defaults and prints them before doing anything.

| Setting | Default | Meaning |
|---|---|---|
| `REGISTRY_MODE` | `internal` on OpenShift | `internal` builds the bundle in-cluster with a BuildConfig and pushes to the integrated registry, so **no registry credentials are needed**. `external` builds locally and pushes to `IMAGE_TAG_BASE`. |
| `INSTALL_MODE` | `dev` with `internal` | `dev` uses `operator-sdk run bundle`, which serves the bundle from a temporary in-cluster pod. `catalog` builds a real catalog image with `opm` and installs via a `CatalogSource`, which is what you want for a durable or production install. |
| `NAMESPACE` | `frappe-operator-system` | Where the operator and its OperatorGroup are created. |
| `CHANNEL` | `alpha` | Subscription channel, matching the bundle's `annotations.yaml`. |
| `VERSION` | read from `Makefile` | Bundle version. |
| `ALLOW_HELM_COEXIST` | `false` | Set to `true` only if you have already stopped the Helm operator yourself. |

`internal` mode defaults to `dev` because the integrated registry usually has no
route reachable from a laptop, so `opm` cannot push a catalog image into it.

## Durable install with a real catalog

For a repeatable install backed by a catalog image, push to a registry your
cluster can pull from:

```bash
REGISTRY_MODE=external \
INSTALL_MODE=catalog \
IMAGE_TAG_BASE=ghcr.io/vyogotech/frappe-operator \
./install-olm.sh
```

This builds and pushes `…-bundle:v<version>` and `…-catalog:v<version>`, then
creates a `CatalogSource` and `Subscription`.

## Requirements

- OLM on the cluster. Built into OpenShift; elsewhere run `operator-sdk olm install`.
- A bundle whose CSV points at a **real, pullable operator image**. The script
  refuses to continue if it still contains the `controller:latest` placeholder.
  Regenerate it with:

  ```bash
  make bundle IMG=ghcr.io/vyogotech/frappe-operator:v5.2.0
  ```

- `operator-sdk` for `dev` mode, or `opm` (via `make opm`) for `catalog` mode.

## After installing

```bash
oc get csv -n frappe-operator-system
oc get pods -n frappe-operator-system
```

The console entry appears under *Operators → Installed Operators* in that
project.

**Install the MariaDB operator separately if your sites use MariaDB**, since the
bundle does not declare it as a dependency:

```bash
kubectl create -f https://operatorhub.io/install/mariadb-operator.yaml
```

## Dependencies and compatibility

**The bundle declares no OLM dependencies, by design.** You install the database
provider yourself.

MariaDB is the default provider: a `FrappeSite` that does not set
`spec.dbConfig.provider` resolves to MariaDB, so install the MariaDB Operator
unless every site names a different provider.

Declaring it as a hard `olm.package` dependency was tried and reverted. OLM
resolves dependencies eagerly and fails the whole install when it cannot satisfy
one, which had two unacceptable effects: PostgreSQL-only and SQLite-only users
were forced to install MariaDB, and the bundle became impossible to install on
any cluster whose catalogs do not carry the package, including bare kind
clusters and air-gapped environments. It broke this project's own bundle E2E
job, where the operator previously installed cleanly.

Install whichever of these matches your configuration:

| Configuration | Also install |
|---|---|
| dedicated PostgreSQL, `postgresEngine: stackgres` | `stackgres-community` |
| dedicated PostgreSQL, `postgresEngine: percona` | `percona-postgresql-operator` |
| shared PostgreSQL | any reachable PostgreSQL, e.g. CloudNativePG |
| worker autoscaling | `keda` |
| admission webhooks enabled | cert-manager |

**Compatibility** is declared as OpenShift `v4.14` and later
(`com.redhat.openshift.versions`, open-ended) with `minKubeVersion: 1.27.0`,
which is the Kubernetes version shipped by OpenShift 4.14.

To change the supported floor, edit the version in the `bundle` target of the
`Makefile` and in `config/manifests/bases/frappe-operator.clusterserviceversion.yaml`,
then re-run `make bundle`.

### A validator warning you can ignore

`operator-sdk bundle validate --select-optional suite=operatorframework` reports
that the bundle "is using APIs which were deprecated and removed in v1.25" for
`cronjobs` and `horizontalpodautoscalers`. This is a false positive. RBAC rules
name API groups and resources but never versions, so the validator flags the
resource names on sight. The operator only ever uses the stable `batch/v1` and
`autoscaling/v2` APIs; there are no beta API references in the code.
