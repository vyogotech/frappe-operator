# <i data-lucide="folder-code"></i> Frappe Operator Deployment Manifests

This directory contains production-ready and development example manifests for deploying Frappe and ERPNext using the **Frappe Operator** by **Vyogo Technologies**.

---

## <i data-lucide="layers"></i> Architecture & Database Options

Frappe Operator v5.2.0 supports polymorphic database backends and strict enterprise security constraints:

1. **PostgreSQL**: Declaratively managed via the StackGres Operator or Percona Operator.
2. **MariaDB**: Automated database and user provisioning via the MariaDB Operator.
3. **External Databases**: Direct connectivity to AWS RDS, Google Cloud SQL, or managed PostgreSQL/MySQL instances.
4. **OpenShift `restricted-v2`**: Compliant with hardened enterprise OpenShift clusters using dynamic UIDs and automatic Edge TLS Routes.

---

## <i data-lucide="play-circle"></i> Quick Starts

### 1. PostgreSQL with StackGres (Recommended for Enterprise)

```bash
# Deploy PostgreSQL cluster and schema using StackGres
kubectl apply -f stackgres-postgres.yaml

# Create the FrappeBench
kubectl apply -f basic-bench.yaml

# Wait for bench initialization to complete
kubectl wait --for=condition=Ready frappebench/dev-bench --timeout=300s

# Create the FrappeSite with dedicated StackGres PostgreSQL
kubectl apply -f kind-e2e-postgres-manifests.yaml
```

### 2. Shared MariaDB Setup (High-Density Multi-Tenancy)

```bash
# Deploy shared MariaDB instance
kubectl apply -f mariadb-shared-instance.yaml

# Wait for MariaDB cluster readiness
kubectl wait --for=condition=Ready mariadb/frappe-mariadb --timeout=300s

# Deploy Bench and Site
kubectl apply -f basic-bench.yaml
kubectl apply -f site-shared-mariadb.yaml
```

### 3. OpenShift Enterprise Deployment (`restricted-v2`)

```bash
# Apply OpenShift-compliant bench with dynamic UID allocation
oc apply -f ocp-restricted-bench.yaml

# Apply OpenShift-compliant site with automatic Edge TLS Route
oc apply -f ocp-restricted-site.yaml
```

---

## <i data-lucide="list"></i> Manifest Catalog

### <i data-lucide="server"></i> Infrastructure & Benches
- `basic-bench.yaml`: Simple bench running Frappe v15 with default resource sizing.
- `autoscaling-bench.yaml`: Bench configured with KEDA (Redis queue triggers) and HPA (CPU/Memory).
- `ocp-restricted-bench.yaml`: Certified for OpenShift `restricted-v2` SCC (no `fsGroup: 0`, dynamic UIDs).
- `advanced-pod-config.yaml`: Bench with custom node affinity, tolerations, and `geoTag` scheduling.
- `hybrid-bench.yaml`: Bench combining pre-compiled images, Git repos, and FPM packages.

### <i data-lucide="globe"></i> Sites & Database Engines
- `stackgres-postgres.yaml`: Enterprise PostgreSQL provisioning via StackGres CRDs.
- `site-dedicated-mariadb.yaml`: Site with dedicated, auto-provisioned MariaDB instance.
- `site-shared-mariadb.yaml`: Site running against a shared MariaDB cluster.
- `site-external-db.yaml`: Site connected to an external cloud database (AWS RDS / Cloud SQL).
- `site-with-apps.yaml`: Site pre-configured with ERPNext and HRMS applications.
- `ocp-restricted-site.yaml`: Site configured with OpenShift Route Edge TLS redirection.

### <i data-lucide="cpu"></i> Operational & Management CRDs
- `basic-sitebackup.yaml`: Manual backup manifest for site databases and files.
- `scheduled-sitebackup.yaml`: Cron-based scheduled automated backup.
- `siteuser.yaml`: Declarative Frappe user and role management.
- `sitedomain.yaml`: Custom secondary domains and host header routing.
- `sitemigration.yaml`: Automated schema migration execution job.

---

## <i data-lucide="activity"></i> Diagnostics & Verification

### Check Cluster Status
```bash
# Verify all Benches and Sites
kubectl get frappebench,frappesite -A

# Check detailed status and conditions
kubectl describe frappesite <site-name>
```

### Stream Initialization Logs
```bash
# Inspect bench initialization
kubectl logs job/<bench-name>-init -f

# Inspect site creation and database migration
kubectl logs job/<site-name>-init -f
```

---

## <i data-lucide="arrow-right-circle"></i> Related Documentation

- **[Architecture Guide](../docs/ARCHITECTURE.md)** - Operator control loops and lifecycle
- **[PostgreSQL Integration](../docs/POSTGRESQL_INTEGRATION.md)** - StackGres & Percona setup guide
- **[OpenShift Enterprise Guide](../docs/INSTALL_OPENSHIFT.md)** - Hardened security and SCC compliance
- **[API Reference](../docs/api-reference.md)** - Complete CRD schema definitions
