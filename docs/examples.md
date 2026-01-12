# Examples

Real-world deployment patterns and configuration examples for Frappe Operator.

> **Note**: All example YAML files are available in the [`examples/`](https://github.com/vyogotech/frappe-operator/tree/main/examples) directory of the repository.

## Table of Contents

- [Quick Start](#quick-start)
- [Development Environment](#development-environment)
- [Production Deployment](#production-deployment)
- [Multi-Tenant SaaS](#multi-tenant-saas)
- [Enterprise Setup](#enterprise-setup)
- [External Database and Redis](#external-database-and-redis) **📊 Support Status**
- [Custom Domains](#custom-domains)
- [High Availability](#high-availability)
- [Worker Autoscaling](#worker-autoscaling) **⚡ NEW**
- [Resource Scaling](#resource-scaling)
- [Using Example Files](#using-example-files)

---

## Quick Start

The fastest way to get started is using the minimal example:

```bash
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/examples/minimal-bench-and-site.yaml
```

Or clone the repository:

```bash
git clone https://github.com/vyogotech/frappe-operator.git
cd frappe-operator/examples
kubectl apply -f minimal-bench-and-site.yaml
```

---

## Development Environment

### Minimal Local Setup

Perfect for local development and testing.

```yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: dev-bench
  namespace: default
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext"]'
  domainConfig:
    suffix: ".local"
    autoDetect: false

---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: mysite
  namespace: default
spec:
  benchRef:
    name: dev-bench
  siteName: "mysite"
  dbConfig:
    mode: shared
  ingress:
    enabled: true
    className: "nginx"
```

**Access:**
```bash
kubectl port-forward service/dev-bench-nginx 8080:8080
# Add to /etc/hosts: 127.0.0.1 mysite.local
# Access: http://mysite.local:8080
```

---

## Production Deployment

### Single-Site Production Setup

Production-ready configuration with proper resources and TLS.

```yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: prod-bench
  namespace: production
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms"]'
  
  imageConfig:
    repository: frappe/erpnext
    tag: v15.0.0
    pullPolicy: IfNotPresent
  
  componentReplicas:
    gunicorn: 3
    socketio: 2
    workerDefault: 2
    workerLong: 2
    workerShort: 1
  
  componentResources:
    gunicorn:
      requests:
        cpu: "1"
        memory: "2Gi"
      limits:
        cpu: "2"
        memory: "4Gi"
    socketio:
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "1"
        memory: "1Gi"
    scheduler:
      requests:
        cpu: "250m"
        memory: "512Mi"
      limits:
        cpu: "500m"
        memory: "1Gi"
    workerDefault:
      requests:
        cpu: "500m"
        memory: "1Gi"
      limits:
        cpu: "1"
        memory: "2Gi"
    workerLong:
      requests:
        cpu: "500m"
        memory: "1Gi"
      limits:
        cpu: "1"
        memory: "2Gi"
    workerShort:
      requests:
        cpu: "250m"
        memory: "512Mi"
      limits:
        cpu: "500m"
        memory: "1Gi"
  
  redisConfig:
    type: dragonfly
    maxMemory: "4gb"
    resources:
      requests:
        cpu: "500m"
        memory: "4Gi"
      limits:
        cpu: "1"
        memory: "6Gi"

---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: erp-site
  namespace: production
spec:
  benchRef:
    name: prod-bench
  
  siteName: "erp.example.com"
  domain: "erp.example.com"
  
  adminPasswordSecretRef:
    name: erp-admin-password
  
  dbConfig:
    mode: dedicated
    storageSize: "100Gi"
    resources:
      requests:
        cpu: "1"
        memory: "4Gi"
      limits:
        cpu: "2"
        memory: "8Gi"
  
  ingress:
    enabled: true
    className: "nginx"
    annotations:
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
      nginx.ingress.kubernetes.io/proxy-body-size: "100m"
      nginx.ingress.kubernetes.io/proxy-read-timeout: "600"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"

---
# Admin password secret
apiVersion: v1
kind: Secret
metadata:
  name: erp-admin-password
  namespace: production
type: Opaque
stringData:
  password: "YourSecurePassword123!"
```

---

## Multi-Tenant SaaS

### Shared Bench with Multiple Customer Sites

One bench serving multiple customer sites - cost-effective SaaS model.

```yaml
---
# Shared bench for all customers
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: saas-bench
  namespace: saas-platform
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms"]'
  
  # Automatic domain assignment
  domainConfig:
    suffix: ".myplatform.com"
  
  componentReplicas:
    gunicorn: 5
    socketio: 3
    workerDefault: 5
    workerLong: 3
    workerShort: 2
  
  componentResources:
    gunicorn:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
    workerDefault:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "1", memory: "2Gi"}
  
  redisConfig:
    type: dragonfly
    maxMemory: "8gb"

---
# Customer 1 - Shared database
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: customer1
  namespace: saas-platform
spec:
  benchRef:
    name: saas-bench
  siteName: "customer1"  # Results in: customer1.myplatform.com
  dbConfig:
    mode: shared
    mariadbRef:
      name: shared-mariadb
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"

---
# Customer 2 - Shared database
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: customer2
  namespace: saas-platform
spec:
  benchRef:
    name: saas-bench
  siteName: "customer2"  # Results in: customer2.myplatform.com
  dbConfig:
    mode: shared
    mariadbRef:
      name: shared-mariadb
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"

---
# Customer 3 - Dedicated database (enterprise tier)
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: enterprise-customer
  namespace: saas-platform
spec:
  benchRef:
    name: saas-bench
  siteName: "enterprise-customer"
  dbConfig:
    mode: dedicated
    storageSize: "200Gi"
    resources:
      requests: {cpu: "2", memory: "8Gi"}
      limits: {cpu: "4", memory: "16Gi"}
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"
```

---

## Enterprise Setup

### Dedicated Bench for Enterprise Customer

Complete isolation with dedicated resources.

```yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: acme-corp-bench
  namespace: acme-corp
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms", "custom_app"]'
  
  imageConfig:
    repository: acmecorp.azurecr.io/frappe-custom
    tag: v15-acme-1.0.0
    pullPolicy: Always
    pullSecrets:
      - name: acr-credentials
  
  componentReplicas:
    gunicorn: 10
    socketio: 5
    workerDefault: 10
    workerLong: 5
    workerShort: 3
  
  componentResources:
    gunicorn:
      requests: {cpu: "2", memory: "4Gi"}
      limits: {cpu: "4", memory: "8Gi"}
    workerDefault:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
  
  redisConfig:
    type: dragonfly
    maxMemory: "16gb"
    storageSize: "50Gi"

---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: acme-erp
  namespace: acme-corp
spec:
  benchRef:
    name: acme-corp-bench
  
  siteName: "erp.acme.com"
  domain: "erp.acme.com"
  
  dbConfig:
    mode: external  # Using Azure Database for MySQL
    connectionSecretRef:
      name: azure-mysql-credentials
  
  ingress:
    enabled: true
    className: "nginx"
    annotations:
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
      nginx.ingress.kubernetes.io/proxy-body-size: "500m"
      nginx.ingress.kubernetes.io/proxy-read-timeout: "1800"
      nginx.ingress.kubernetes.io/rate-limit: "100"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"

---
# External database credentials
apiVersion: v1
kind: Secret
metadata:
  name: azure-mysql-credentials
  namespace: acme-corp
type: Opaque
stringData:
  host: "acme-mysql.mysql.database.azure.com"
  port: "3306"
  database: "acme_erp"
  username: "acme_admin@acme-mysql"
  password: "YourAzureMySQLPassword"
```

---

## External Database and Redis

### Support Status and Configuration

The Frappe Operator provides varying levels of support for external databases and Redis instances.

#### External MariaDB/MySQL Support

**Status:** ✅ **SUPPORTED** (v1.0.0+)

The operator supports connecting to external MariaDB or MySQL databases (e.g., AWS RDS, Azure Database for MySQL, Google Cloud SQL).

**Configuration Example:**

```yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: external-db-site
  namespace: production
spec:
  benchRef:
    name: prod-bench
  siteName: "erp.example.com"
  
  dbConfig:
    mode: external  # Use external database
    connectionSecretRef:
      name: external-db-credentials

---
# External database credentials secret
apiVersion: v1
kind: Secret
metadata:
  name: external-db-credentials
  namespace: production
type: Opaque
stringData:
  host: "mysql.rds.amazonaws.com"        # Database hostname
  port: "3306"                            # Database port
  database: "erp_production"              # Database name
  username: "frappe_user"                 # Database user
  password: "your-secure-password-here"   # Database password
```

**Use Cases:**
- ✅ AWS RDS for MariaDB/MySQL
- ✅ Azure Database for MySQL
- ✅ Google Cloud SQL for MySQL
- ✅ Self-managed external MariaDB/MySQL instances
- ✅ High-availability managed database services

**Requirements:**
- The external database must be accessible from the Kubernetes cluster
- The database user must have permissions to create and manage databases
- Network connectivity must be properly configured (security groups, firewall rules)

**Notes:**
- The operator will use the provided database connection details
- Database provisioning (creating database, user, grants) must be handled externally
- The site will connect directly to the external database instance

---

#### External Redis Support

**Status:** ⚠️ **PLANNED** (Not Yet Implemented)

External Redis support is defined in the API but **not yet implemented** in the current version (v1.0.0).

**API Definition (Available but not functional):**

```yaml
# This configuration is defined in the API but NOT yet implemented
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: external-redis-bench
spec:
  frappeVersion: "version-15"
  apps:
    - name: erpnext
      source: image
  
  redisConfig:
    type: redis
    # ConnectionSecretRef is defined but not yet implemented
    connectionSecretRef:
      name: external-redis-credentials  # ⚠️ Not yet functional
```

**Current Status:**
- ❌ The `ConnectionSecretRef` field exists in the API (`api/v1alpha1/shared_types.go:240`)
- ❌ Implementation is marked as TODO in the codebase (`controllers/frappebench_resources.go:1504`)
- ❌ The operator currently only supports **in-cluster Redis** deployments
- ✅ In-cluster Redis (managed by operator) works perfectly

**Current Behavior:**
The operator automatically deploys Redis instances within the cluster for:
- **Redis Cache** - Session storage and caching
- **Redis Queue** - Background job queue management

You can configure the in-cluster Redis with:
```yaml
redisConfig:
  type: redis          # or 'dragonfly' for better performance
  maxMemory: "4gb"     # Memory limit
  storageSize: "10Gi"  # Persistent storage size
  resources:
    requests:
      cpu: "500m"
      memory: "4Gi"
```

**Roadmap:**
External Redis support is planned for a future release (v1.1+). When implemented, it will support:
- AWS ElastiCache for Redis
- Azure Cache for Redis
- Google Cloud Memorystore
- Self-managed external Redis instances

---

#### Summary Table

| Feature | Status | Version | Use Cases |
|---------|--------|---------|-----------|
| **External MariaDB/MySQL** | ✅ Supported | v1.0.0+ | AWS RDS, Azure Database, Cloud SQL, Self-managed |
| **Shared MariaDB** | ✅ Supported | v1.0.0+ | Multi-tenant, cost-effective |
| **Dedicated MariaDB** | ✅ Supported | v1.0.0+ | Enterprise, isolated databases |
| **In-cluster Redis** | ✅ Supported | v1.0.0+ | Default deployment, fully managed |
| **In-cluster Dragonfly** | ✅ Supported | v1.0.0+ | High-performance alternative to Redis |
| **External Redis** | ⚠️ Planned | Future (v1.1+) | ElastiCache, Azure Cache, Memorystore |
| **PostgreSQL** | ⚠️ Planned | v1.1.0+ | PostgreSQL provider |
| **SQLite** | ✅ Supported | v1.0.0+ | Frappe v16+ sites |

---

## Custom Domains

### Custom Domain per Site

Each site with its own custom domain.

```yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: multi-domain-bench
  namespace: default
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext"]'

---
# Site 1: Custom domain
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: site-company-a
  namespace: default
spec:
  benchRef:
    name: multi-domain-bench
  siteName: "erp.company-a.com"
  domain: "erp.company-a.com"
  dbConfig:
    mode: dedicated
    storageSize: "50Gi"
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"

---
# Site 2: Different custom domain
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: site-company-b
  namespace: default
spec:
  benchRef:
    name: multi-domain-bench
  siteName: "system.company-b.net"
  domain: "system.company-b.net"
  dbConfig:
    mode: dedicated
    storageSize: "50Gi"
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"
```

---

## High Availability

### HA Setup with Auto-Scaling

High-availability configuration with horizontal pod autoscaling.

```yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: ha-bench
  namespace: production
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms"]'
  
  # Start with moderate replicas
  componentReplicas:
    gunicorn: 5
    socketio: 3
    workerDefault: 3
    workerLong: 2
    workerShort: 2
  
  componentResources:
    gunicorn:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
    socketio:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "1", memory: "2Gi"}
    workerDefault:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "1", memory: "2Gi"}
  
  redisConfig:
    type: dragonfly
    maxMemory: "8gb"

---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: ha-site
  namespace: production
spec:
  benchRef:
    name: ha-bench
  siteName: "erp.example.com"
  
  dbConfig:
    mode: external
    connectionSecretRef:
      name: rds-mariadb-galera  # AWS RDS with multi-AZ
  
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true

---
# HPA for gunicorn
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ha-bench-gunicorn-hpa
  namespace: production
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ha-bench-gunicorn
  minReplicas: 5
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80

---
# HPA for workers
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ha-bench-worker-default-hpa
  namespace: production
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ha-bench-worker-default
  minReplicas: 3
  maxReplicas: 15
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 75
```

---

## Worker Autoscaling

### KEDA-Based Autoscaling with Scale-to-Zero

**⚡ NEW in v1.1.0**: Automatically scale background workers based on Redis queue length, with scale-to-zero support for cost savings.

#### Prerequisites

KEDA is automatically installed by `install.sh`. For manual installation:

```bash
kubectl apply -f https://github.com/kedacore/keda/releases/download/v2.16.1/keda-2.16.1.yaml
```

#### Production Setup with Autoscaling

```yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: autoscaling-bench
  namespace: production
spec:
  frappeVersion: "version-15"
  apps:
    - name: erpnext
      source: image
    - name: hrms
      source: image
  
  redisConfig:
    type: redis
  
  # Worker autoscaling configuration
  workerAutoscaling:
    # Short queue - scale to zero when idle
    short:
      enabled: true
      minReplicas: 0        # Scale to zero to save costs
      maxReplicas: 10       # Scale up to 10 workers under load
      queueLength: 2        # Trigger: 2 jobs per worker
      pollingInterval: 10   # Check queue every 10 seconds
      cooldownPeriod: 30    # Wait 30s before scaling down
    
    # Long queue - maintain minimum workers
    long:
      enabled: true
      minReplicas: 1        # Always have 1 worker available
      maxReplicas: 5        # Maximum 5 workers
      queueLength: 5        # Trigger: 5 jobs per worker
      pollingInterval: 30   # Check queue every 30 seconds
      cooldownPeriod: 60    # Wait 60s before scaling down
    
    # Default queue - static replicas (no autoscaling)
    default:
      enabled: false        # Disable autoscaling
      staticReplicas: 2     # Always maintain 2 workers
  
  # Resources for autoscaled workers
  componentResources:
    workerShort:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "1", memory: "2Gi"}
    workerLong:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
    workerDefault:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "1", memory: "2Gi"}

---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: autoscaling-site
  namespace: production
spec:
  benchRef:
    name: autoscaling-bench
  siteName: "app.example.com"
  dbConfig:
    provider: mariadb
    mode: shared
  domain: "app.example.com"
```

#### Development Setup with Aggressive Scale-to-Zero

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: dev-autoscaling
  namespace: development
spec:
  frappeVersion: "version-15"
  apps:
    - name: erpnext
      source: image
  
  # All workers scale to zero when idle
  workerAutoscaling:
    short:
      enabled: true
      minReplicas: 0
      maxReplicas: 3
      queueLength: 1        # Scale up quickly
      pollingInterval: 5    # Check frequently
      cooldownPeriod: 10    # Scale down fast
    long:
      enabled: true
      minReplicas: 0        # Also scale to zero
      maxReplicas: 2
      queueLength: 1
      pollingInterval: 10
      cooldownPeriod: 20
    default:
      enabled: true
      minReplicas: 0
      maxReplicas: 2
      queueLength: 1
      pollingInterval: 5
      cooldownPeriod: 10
```

#### Monitoring Autoscaling

```bash
# Check ScaledObjects created by KEDA
kubectl get scaledobjects -n production

# Check worker scaling status
kubectl get frappebench autoscaling-bench -o jsonpath='{.status.workerScaling}' | jq

# View current HPA status (created by KEDA)
kubectl get hpa -n production

# Check queue lengths
kubectl exec -n production deployment/autoscaling-bench-redis-queue -- \
  redis-cli LLEN "rq:queue:short"

kubectl exec -n production deployment/autoscaling-bench-redis-queue -- \
  redis-cli LLEN "rq:queue:long"

# Watch worker pods scaling
kubectl get pods -n production -l component=worker-short -w
```

#### Benefits

- 💰 **Cost Savings**: Workers scale to zero when idle (especially useful for dev/staging)
- 📈 **Auto-scaling**: Automatically handles traffic spikes
- 🎯 **Queue-based**: Scales based on actual job queue length, not CPU/memory
- ⚙️ **Configurable**: Fine-tune scaling behavior per queue type
- 🚀 **Production-ready**: Tested end-to-end in production environments

#### Configuration Parameters

| Parameter | Description | Default | Recommended |
|-----------|-------------|---------|-------------|
| `enabled` | Enable KEDA autoscaling | `false` | `true` for production |
| `minReplicas` | Minimum workers (0 for scale-to-zero) | `1` | `0` for dev, `1+` for prod |
| `maxReplicas` | Maximum workers | `10` | Based on load |
| `queueLength` | Jobs per worker threshold | `5` | `2-5` for short, `5-10` for long |
| `pollingInterval` | Queue check frequency (seconds) | `30` | `10-30` |
| `cooldownPeriod` | Wait before scale down (seconds) | `300` | `30-60` for short, `60-300` for long |

> **Note**: For traditional CPU/memory-based HPA, see the [High Availability](#high-availability) section above.

---

## Resource Scaling

### Three-Tier Resource Configuration

Small, medium, and large resource tiers.

#### Small Tier (Development/Testing)

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: small-bench
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext"]'
  
  componentReplicas:
    gunicorn: 1
    socketio: 1
    workerDefault: 1
    workerLong: 1
    workerShort: 1
  
  componentResources:
    gunicorn:
      requests: {cpu: "200m", memory: "256Mi"}
      limits: {cpu: "500m", memory: "512Mi"}
    socketio:
      requests: {cpu: "100m", memory: "128Mi"}
      limits: {cpu: "200m", memory: "256Mi"}
    scheduler:
      requests: {cpu: "100m", memory: "128Mi"}
      limits: {cpu: "200m", memory: "256Mi"}
    workerDefault:
      requests: {cpu: "200m", memory: "256Mi"}
      limits: {cpu: "400m", memory: "512Mi"}
```

#### Medium Tier (Small Production)

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: medium-bench
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms"]'
  
  componentReplicas:
    gunicorn: 3
    socketio: 2
    workerDefault: 2
    workerLong: 1
    workerShort: 1
  
  componentResources:
    gunicorn:
      requests: {cpu: "500m", memory: "512Mi"}
      limits: {cpu: "1", memory: "1Gi"}
    socketio:
      requests: {cpu: "250m", memory: "256Mi"}
      limits: {cpu: "500m", memory: "512Mi"}
    scheduler:
      requests: {cpu: "250m", memory: "256Mi"}
      limits: {cpu: "500m", memory: "512Mi"}
    workerDefault:
      requests: {cpu: "500m", memory: "512Mi"}
      limits: {cpu: "1", memory: "1Gi"}
```

#### Large Tier (Production)

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: large-bench
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms"]'
  
  componentReplicas:
    gunicorn: 5
    socketio: 3
    workerDefault: 5
    workerLong: 3
    workerShort: 2
  
  componentResources:
    gunicorn:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
    socketio:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "1", memory: "2Gi"}
    scheduler:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "1", memory: "2Gi"}
    workerDefault:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
```

---

## Using Example Files

All examples are available in the repository under `examples/`:

| File | Description |
|------|-------------|
| `minimal-bench-and-site.yaml` | Minimal setup for quick testing |
| `autoscaling-bench.yaml` | **⚡ NEW**: KEDA-based worker autoscaling with scale-to-zero |
| `production-bench.yaml` | Production-ready bench configuration |
| `production-site.yaml` | Production site with TLS |
| `multi-tenant-bench.yaml` | Bench for multiple customer sites |
| `multi-tenant-sites.yaml` | Multiple sites on shared bench |
| `enterprise-setup.yaml` | Complete enterprise configuration |
| `high-availability-bench.yaml` | HA setup with multiple replicas |
| `dedicated-db-site.yaml` | Site with dedicated database |
| `custom-domain-site.yaml` | Custom domain configuration |
| `custom-image-bench.yaml` | Using custom container images |
| `resource-tiers.yaml` | Small/Medium/Large resource tiers |

### Applying Examples

```bash
# Clone the repository
git clone https://github.com/vyogotech/frappe-operator.git
cd frappe-operator

# Apply a specific example
kubectl apply -f examples/minimal-bench-and-site.yaml

# Or apply all examples (not recommended)
kubectl apply -f examples/

# Apply from remote URL
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/examples/production-bench.yaml
```

---

## Quick Reference

### Command Cheat Sheet

```bash
# Create resources
kubectl apply -f examples/minimal-bench-and-site.yaml

# Check status
kubectl get frappebench,frappesite

# Watch for changes
kubectl get frappebench,frappesite -w

# Get details
kubectl describe frappebench <name>
kubectl describe frappesite <name>

# Check pods
kubectl get pods -l bench=<bench-name>
kubectl get pods -l site=<site-name>

# View logs
kubectl logs -l app=<component> -f

# Scale replicas
kubectl patch frappebench <name> --type=merge -p '{
  "spec": {"componentReplicas": {"gunicorn": 5}}
}'

# Delete resources
kubectl delete frappesite <name>
kubectl delete frappebench <name>
```

---

## Next Steps

- **[Operations Guide](operations.md)** - Production operations and maintenance
- **[Troubleshooting](troubleshooting.md)** - Common issues and solutions
- **[API Reference](api-reference.md)** - Complete field specifications
- **[Browse All Examples](https://github.com/vyogotech/frappe-operator/tree/main/examples)** - View all example files

