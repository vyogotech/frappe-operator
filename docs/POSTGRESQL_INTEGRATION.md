# PostgreSQL Integration

The Frappe operator supports PostgreSQL as a first-class database provider
alongside MariaDB. Choose per-site with `spec.dbConfig.provider: postgres`.

> **Frappe version requirement.** Frappe's PostgreSQL support ships in the
> `develop` branch (→ v17). The default stable v15 bench image does **not**
> support Postgres. You must run an operator-compatible Frappe **develop/v17**
> bench image on any bench whose sites use `provider: postgres`. See
> [Building a Postgres bench image](#building-a-postgres-bench-image).

## Modes

| Mode | What the operator does | Backing service |
|------|------------------------|-----------------|
| `shared` (default) | Runs a `pg-provision` Job that `CREATE ROLE` + `CREATE DATABASE` on an **existing** PostgreSQL server | Any Postgres (a shared Percona cluster, a managed RDS/CloudSQL, etc.) |
| `dedicated` | Provisions a **per-site** `PerconaPGCluster` (one Postgres instance per site) | [Percona PostgreSQL Operator](https://docs.percona.com/percona-operator-for-postgresql/2.0/) v2 |

Both modes honour `spec.deletionPolicy`:

- **`Retain`** (default): the database, role, and credential Secret (shared) or
  the whole `PerconaPGCluster` (dedicated) are **kept** when the `FrappeSite` is
  deleted. GitOps-safe — an accidental CR delete or an ArgoCD prune never drops
  tenant data.
- **`Delete`**: the operator runs a `pg-delete` Job (shared, `DROP DATABASE` /
  `DROP ROLE`) or deletes the `PerconaPGCluster` (dedicated).

## Prerequisites

Install the Percona PostgreSQL Operator (needed for dedicated mode, and for the
shared-cluster example below):

```bash
kubectl apply --server-side \
  -f https://raw.githubusercontent.com/percona/percona-postgresql-operator/v2.3.1/deploy/bundle.yaml
```

CRDs only (e.g. for manifest validation without running the operator):

```bash
kubectl apply --server-side \
  -f https://raw.githubusercontent.com/percona/percona-postgresql-operator/v2.3.1/deploy/crd.yaml
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

The operator creates a complete, per-site `PerconaPGCluster`:

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
    storageSize: 5Gi        # data + pgBackRest repo volume size (default 2Gi)
  deletionPolicy: Delete
```

The generated cluster is `PerconaPGCluster/<site>-postgres` with:

- `postgresVersion: 16`, one instance (`instance1`)
- a pgBouncer proxy (Frappe connects through `<site>-postgres-pgbouncer`)
- a PVC-backed pgBackRest repo (`repo1`) so backups work out of the box
- a role whose credentials the Percona operator writes to
  `Secret/<site>-postgres-pguser-<user>`

> The Percona CRD constrains the role name to a DNS label, so the operator uses
> a stable label-safe name (`u<hash>`) derived from the site — distinct from the
> underscore-form database identifier used in shared mode.

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

`provider: postgres` needs a Frappe build from `develop` (→ v17). Build an
operator-compatible image the same way as the base image (so the entrypoint is
compatible with the operator's PVC mount), from Frappe `develop`, and point the
bench at it:

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: pg-bench
spec:
  frappeVersion: "develop"
  imageConfig:
    repository: ghcr.io/vyogotech/frappe-postgres
    tag: develop
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
