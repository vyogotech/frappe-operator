# <i data-lucide="map"></i> Product Roadmap & Architecture Vision

This document tracks completed milestones, architectural capabilities, and the future engineering roadmap for **Frappe Operator** by **Vyogo Technologies**.

---

## <i data-lucide="check-circle-2"></i> Delivered in v5.2.0

### <i data-lucide="database"></i> Polymorphic Database Architecture
- **Status**: **Completed** <i data-lucide="check" style="color: #00BC86;"></i>
- **Implementation**: Decoupled the controller reconciliation loops from MariaDB by introducing the `DatabaseProvider` Go interface. Native support for:
  - **StackGres PostgreSQL**: Automated `SGCluster` and `SGScript` provisioning over JDBC without CLI dependencies.
  - **Percona PostgreSQL**: Declarative `PerconaPGCluster` management.
  - **MariaDB Operator**: Isolated databases and dedicated grants.
  - **External Cloud Databases**: Seamless connectivity to AWS RDS, Google Cloud SQL, and Azure PostgreSQL.

### <i data-lucide="shield-check"></i> Red Hat OpenShift `restricted-v2` Compliance
- **Status**: **Completed** <i data-lucide="check" style="color: #00BC86;"></i>
- **Implementation**: Full compatibility with OpenShift's strictest Security Context Constraints:
  - Eliminated hardcoded UIDs and disallowed `fsGroup: 0` privilege escalation.
  - Runtime pods drop all Linux capabilities (`drop: ["ALL"]`).
  - Automatic OpenShift Route generation with enforced Edge TLS redirection.

---

## <i data-lucide="compass"></i> Future Engineering Roadmap

### <i data-lucide="hard-drive"></i> 1. Storage Optimization & Object Storage Offload
- **Target**: v5.3.0
- **Problem**: Frappe stores static assets and user files under `/sites/<site>/public/files` and `/private/files`, requiring ReadWriteMany (RWX) volumes (e.g., AWS EFS, GCP Filestore, CephFS). High tenant volume can cause I/O latency.
- **Planned Solution**:
  - Native S3-compatible object storage provider abstraction (AWS S3, Cloudflare R2, MinIO).
  - Background asset sync and offloading sidecar to eliminate heavy RWX dependency for media and attachments.

### <i data-lucide="git-merge"></i> 2. Automated Canary & Pre-Migration Rollback
- **Target**: v5.4.0
- **Problem**: In-place `bench migrate` modifies database schemas irreversibly if an upgrade patch fails.
- **Planned Solution**:
  - Pre-migration snapshot hooks integrated with VolumeSnapshots and StackGres/Percona backup engines.
  - Automated canary deployment pattern for multi-replica benches before promoting new image tags.
  - Automatic traffic shifting and rollback reconciliation if migration jobs exit with non-zero status.

### <i data-lucide="network"></i> 3. Kubernetes Gateway API & Dynamic Host Routing
- **Target**: v5.5.0
- **Problem**: High-density multi-tenant clusters running thousands of custom domains generate thousands of individual Ingress or Route resources, causing control plane bloat.
- **Planned Solution**:
  - Implement Gateway API HTTPRoute controllers.
  - Integrate dynamic host-header lookup via Envoy Gateway or Traefik, eliminating the need for separate Ingress objects per tenant site.

### <i data-lucide="activity"></i> 4. Native OpenTelemetry APM & Frappe ORM Observability
- **Target**: v5.6.0
- **Problem**: Standard Kubernetes monitoring metrics capture container CPU/Memory and RQ lengths, but lack deep Frappe application insights.
- **Planned Solution**:
  - Automated OpenTelemetry sidecar injection for Gunicorn and RQ workers.
  - Export distributed traces for slow Frappe ORM queries, database connection pool exhaustion, and background job deadlocks to Prometheus and Grafana.