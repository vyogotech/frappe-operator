# Frappe Operator - Comprehensive Guide

A complete reference guide for developers and platform operators deploying and managing Frappe Framework applications on Kubernetes.

## Table of Contents

1. [Introduction](#introduction)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Core Concepts](#core-concepts)
5. [Resource Management](#resource-management)
6. [Image Configuration](#image-configuration)
7. [Database Configuration](#database-configuration)
8. [Scaling and Performance](#scaling-and-performance)
9. [Security](#security)
10. [Operations](#operations)
11. [Troubleshooting](#troubleshooting)
12. [API Reference](#api-reference)
13. [Examples](#examples)

---

## Introduction

### What is Frappe Operator?

Frappe Operator is a Kubernetes operator that automates the lifecycle management of Frappe Framework applications. It provides:

- **Declarative Configuration**: Define benches and sites as Kubernetes resources
- **Automated Provisioning**: Handles database, storage, and networking setup
- **Multi-Tenancy**: Efficiently manage multiple sites on shared infrastructure
- **Auto-Scaling**: Scale workers based on queue length using KEDA
- **Production Ready**: High availability, security, and observability built-in

### Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                     │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │ Frappe Operator  │──────▶│  FrappeBench     │         │
│  │   Controller     │      │   (CRD)          │         │
│  └──────────────────┘      └──────────────────┘         │
│         │                           │                    │
│         │                           │                    │
│         │                           ▼                    │
│         │                  ┌──────────────────┐         │
│         │                  │   FrappeSite     │         │
│         │                  │   (CRD)          │         │
│         │                  └──────────────────┘         │
│         │                           │                    │
│         │                           │                    │
│         ▼                           ▼                    │
│  ┌──────────────────────────────────────────┐           │
│  │  Kubernetes Resources                    │           │
│  │  - Deployments (gunicorn, nginx, etc.)   │           │
│  │  - Services                             │           │
│  │  - PersistentVolumeClaims                 │           │
│  │  - Ingress/Routes                         │           │
│  │  - Jobs (site init, backup)              │           │
│  └──────────────────────────────────────────┘           │
│                                                           │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │  MariaDB         │      │  Redis            │         │
│  │  Operator        │      │  (Cache + Queue)  │         │
│  └──────────────────┘      └──────────────────┘         │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

---

## Installation

### Prerequisites

- Kubernetes cluster (v1.19+)
- `kubectl` configured and connected
- `helm` v3.x (for Helm installation)
- Sufficient cluster resources:
  - 2 CPU cores
  - 4GB RAM
  - 20GB storage

### Quick Installation

```bash
# One-command installation
curl -fsSL https://raw.githubusercontent.com/vyogotech/frappe-operator/main/install.sh | bash
```

### Manual Installation

#### Step 1: Install MariaDB Operator CRDs

```bash
kubectl apply --server-side -k "github.com/mariadb-operator/mariadb-operator/config/crd?ref=v0.34.0"
```

#### Step 2: Install Frappe Operator

```bash
# Add Helm repository
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator
helm repo update

# Install operator
helm install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace
```

#### Step 3: Verify Installation

```bash
# Check operator pod
kubectl get pods -n frappe-operator-system

# Check CRDs
kubectl get crd | grep frappe

# Check operator logs
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager
```

### Installation Options

#### Custom Namespace

```bash
helm install frappe-operator frappe-operator/frappe-operator \
  --namespace my-frappe-system \
  --create-namespace
```

#### Custom Image Registry

```bash
helm install frappe-operator frappe-operator/frappe-operator \
  --set operator.image.repository=myregistry.com/frappe-operator \
  --set operator.image.tag=v2.5.0
```

---

## Configuration

### Operator-Level Configuration

The operator reads configuration from the `frappe-operator-config` ConfigMap in the operator namespace.

#### ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: frappe-operator-config
  namespace: frappe-operator-system
data:
  # Domain configuration
  defaultDomainSuffix: ".myplatform.com"
  ingressControllerService: "ingress-nginx-controller"
  ingressControllerNamespace: "ingress-nginx"
  
  # Git configuration
  gitEnabled: "false"
  
  # Max concurrent FrappeSite reconciles (default: 10). Tune for 100s of sites.
  # Can be overridden per-bench via spec.siteReconcileConcurrency (operator uses max).
  maxConcurrentSiteReconciles: "10"
  
  # FPM configuration
  fpmCliPath: "/usr/local/bin/fpm"
  fpmRepositories: |
    [
      {
        "name": "frappe-community",
        "url": "https://fpm.frappe.io",
        "priority": 100
      }
    ]
  
  # Image defaults
  defaultFrappeImage: "docker.io/frappe/erpnext:latest"
  defaultMariaDBImage: "docker.io/library/mariadb:10.6"
  defaultPostgresImage: "docker.io/library/postgres:15-alpine"
  defaultRedisImage: "docker.io/library/redis:7-alpine"
  defaultNginxImage: "docker.io/library/nginx:1.25-alpine"
```

#### Updating Configuration

**Via Helm:**
```bash
helm upgrade frappe-operator frappe-operator/frappe-operator \
  --set operatorConfig.defaultFrappeImage="myregistry.com/frappe/erpnext:latest"
```

**Direct ConfigMap Edit:**
```bash
kubectl edit configmap frappe-operator-config -n frappe-operator-system
```

---

## Core Concepts

### FrappeBench

A `FrappeBench` represents a shared infrastructure for multiple Frappe sites. It includes:

- **Storage**: Persistent volume for site data
- **Components**: Gunicorn, Nginx, Socket.IO, Scheduler, Workers
- **Apps**: Installed Frappe applications (ERPNext, etc.)
- **Database**: Shared or dedicated database instances

### FrappeSite

A `FrappeSite` represents a single Frappe site running on a bench. Each site:

- Has its own database (isolated)
- Shares bench infrastructure (cost-efficient)
- Can have custom domain and TLS configuration
- Supports backup and restore operations

### Resource Relationships

```
FrappeBench (1)
    │
    ├── PersistentVolumeClaim (1)
    ├── Deployment: gunicorn (N)
    ├── Deployment: nginx (1)
    ├── Deployment: socketio (1)
    ├── Deployment: scheduler (1)
    ├── Deployment: worker-* (N)
    └── FrappeSite (N)
            │
            ├── Database (1)
            ├── Secret: db-credentials (1)
            ├── Secret: admin-password (1)
            ├── Ingress/Route (1)
            └── Job: site-init (1)
```

---

## Resource Management

### Creating a FrappeBench

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
  namespace: default
spec:
  frappeVersion: "15"
  apps:
    - name: erpnext
      source: image
  imageConfig:
    repository: "myregistry.com/frappe/erpnext"
    tag: "v15.41.2"
    pullPolicy: Always
  componentReplicas:
    gunicorn: 2
    nginx: 1
    socketio: 1
    scheduler: 1
  storageClassName: "fast-ssd"
```

### Creating a FrappeSite

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: customer-site
  namespace: default
spec:
  siteName: customer.example.com
  benchRef:
    name: production-bench
    namespace: default
  adminPasswordSecretRef:
    name: site-admin-password
    namespace: default
  ingress:
    enabled: true
  tls:
    enabled: true
    secretName: customer-tls-cert
```

### Monitoring Resources

```bash
# Check bench status
kubectl get frappebench production-bench -o yaml

# Check site status
kubectl get frappesite customer-site -o yaml

# Watch bench events
kubectl get events --field-selector involvedObject.name=production-bench --sort-by='.lastTimestamp'

# Check component pods
kubectl get pods -l app=frappe,bench=production-bench
```

---

## Image Configuration

### Configuration Priority

Image selection follows this priority:

1. **Bench-level override** (`spec.imageConfig`)
2. **Operator ConfigMap defaults** (`frappe-operator-config`)
3. **Hardcoded constants** (fallback)

### Operator-Level Defaults

Set defaults in the operator ConfigMap:

```yaml
# In frappe-operator-config ConfigMap
data:
  defaultFrappeImage: "myregistry.com/frappe/erpnext:latest"
  defaultMariaDBImage: "myregistry.com/library/mariadb:10.6"
  defaultPostgresImage: "myregistry.com/library/postgres:15-alpine"
  defaultRedisImage: "myregistry.com/library/redis:7-alpine"
  defaultNginxImage: "myregistry.com/library/nginx:1.25-alpine"
```

### Bench-Level Override

Override defaults per bench:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: custom-bench
spec:
  frappeVersion: "15"
  imageConfig:
    repository: "production-registry.com/frappe/erpnext"
    tag: "v15.41.2"
    pullPolicy: Always
    pullSecrets:
      - name: registry-credentials
```

### Version Handling

When `frappeVersion` is specified:

- If `imageConfig.repository` is set but `tag` is not → version is used as tag
- If using ConfigMap defaults → version replaces tag in default image
- If no defaults → `docker.io/frappe/erpnext:{version}` is used

### Air-Gapped Environments

For air-gapped deployments, configure all images in the operator ConfigMap:

```yaml
data:
  defaultFrappeImage: "internal-registry.company.com/frappe/erpnext:latest"
  defaultMariaDBImage: "internal-registry.company.com/library/mariadb:10.6"
  defaultPostgresImage: "internal-registry.company.com/library/postgres:15-alpine"
  defaultRedisImage: "internal-registry.company.com/library/redis:7-alpine"
  defaultNginxImage: "internal-registry.company.com/library/nginx:1.25-alpine"
```

---

## Database Configuration

### Shared Database Mode

Multiple sites share one MariaDB instance:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: shared-bench
spec:
  frappeVersion: "15"
  dbConfig:
    provider: mariadb
    mode: shared
    # Optional: reference existing MariaDB
    mariadbRef:
      name: shared-mariadb
      namespace: default
```

### Dedicated Database Mode

Each site gets its own MariaDB instance:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: isolated-site
spec:
  siteName: isolated.example.com
  benchRef:
    name: production-bench
  dbConfig:
    provider: mariadb
    mode: dedicated
```

### External Database

Connect to RDS, Cloud SQL, or any external database:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: external-db-site
spec:
  siteName: external.example.com
  benchRef:
    name: production-bench
  dbConfig:
    provider: external
    host: "rds-instance.region.rds.amazonaws.com"
    port: "3306"
    connectionSecretRef:
      name: rds-credentials
      namespace: default
```

**Connection Secret:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rds-credentials
  namespace: default
type: Opaque
stringData:
  username: "frappe_user"
  password: "secure_password"
```

### PostgreSQL Support

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: postgres-site
spec:
  siteName: postgres.example.com
  benchRef:
    name: production-bench
  dbConfig:
    provider: postgres
    host: "postgres.example.com"
    port: "5432"
    connectionSecretRef:
      name: postgres-credentials
```

### Database Security and Privilege Model

The Frappe Operator implements a **principle of least privilege** security model for database access to protect production data.

#### Privilege Separation

There are **two distinct sets of database credentials** with different privilege levels:

1. **Site User Credentials** (Limited Privileges)
   - Used by running application pods (gunicorn, workers, scheduler, socketio)
   - Stored in site-specific secret: `{site-name}-db-password`
   - Can perform table-level operations only
   - **Cannot drop databases or users** (protection against accidental data loss)

2. **MariaDB Root Credentials** (Administrative Privileges)
   - Used only in deletion jobs (never in runtime pods)
   - Stored in: `{site-name}-mariadb-root` (dedicated mode) or MariaDB CR's root secret (shared mode)
   - Can perform database-level operations including DROP DATABASE
   - **Never exposed to application containers**

#### Site User Privileges

Site users are granted these privileges (via MariaDB Operator Grant CR):

| Privilege | Purpose | Risk Level |
|-----------|---------|------------|
| `SELECT`, `INSERT`, `UPDATE`, `DELETE` | Basic data operations | Low |
| `CREATE`, `ALTER`, `INDEX` | Schema management (migrations, DocType creation) | Low |
| `DROP` (table-level only) | Remove tables during migrations | Medium |
| `REFERENCES` | Foreign key constraints | Low |
| `CREATE TEMPORARY TABLES`, `LOCK TABLES` | Complex queries and transactions | Low |
| `EXECUTE` | Stored procedures and functions | Low |
| `CREATE VIEW`, `SHOW VIEW` | View management | Low |
| `CREATE ROUTINE`, `ALTER ROUTINE` | Function management | Low |
| `EVENT`, `TRIGGER` | Event and trigger management | Low |

**Site users CANNOT:**
- Drop databases (`DROP DATABASE`) - prevents accidental site destruction
- Drop users (`DROP USER`) - prevents credential tampering  
- Grant privileges to others (`GRANT OPTION` is false) - prevents privilege escalation

#### Security Rationale

This design protects against several scenarios:

1. **Developer Access**: If a developer gains pod access (via `kubectl exec`), they can query data but cannot accidentally drop the entire database
2. **Compromised Credentials**: If site credentials leak, attackers can modify data but cannot destroy the database
3. **Application Bugs**: Bugs in Frappe code or custom apps cannot execute `DROP DATABASE` commands
4. **Audit Trail**: Database deletions only occur through operator-managed jobs with proper logging

#### Site Deletion Process

When a FrappeSite resource is deleted:

1. Operator creates a deletion job
2. Job retrieves **MariaDB root credentials** (not site user credentials)
3. Runs `bench drop-site --db-root-username root --db-root-password <password>`
4. Drops database, user, and site files
5. Job completes and site is removed

This ensures only authorized Kubernetes operations can delete sites, not application code or developers.

#### Credential Storage

All credentials are stored as Kubernetes Secrets:

```bash
# View available secrets (values are base64-encoded)
kubectl get secrets -n <namespace>

# Site user credentials (used by runtime pods)
kubectl get secret <site-name>-db-password -o jsonpath='{.data.password}' | base64 -d

# Root credentials (dedicated mode only, used by deletion jobs)
kubectl get secret <site-name>-mariadb-root -o jsonpath='{.data.password}' | base64 -d
```

**Important:** Never mount root credentials in application pods. Root credentials should only be used in operator-managed jobs.

---

## Scaling and Performance

### Component Replicas

Configure replica counts for each component:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: scaled-bench
spec:
  frappeVersion: "15"
  componentReplicas:
    gunicorn: 3      # Web servers
    nginx: 2         # Load balancers
    socketio: 2      # WebSocket servers
    scheduler: 1     # Always 1
```

### Worker Autoscaling (KEDA)

Scale workers based on queue length:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: autoscaled-bench
spec:
  frappeVersion: "15"
  workerAutoscaling:
    default:
      enabled: true
      minReplicas: 0      # Scale to zero
      maxReplicas: 10
      targetQueueLength: 5
    long:
      enabled: true
      minReplicas: 0
      maxReplicas: 5
      targetQueueLength: 2
```

### Resource Limits

Set CPU and memory limits:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: resource-limited-bench
spec:
  frappeVersion: "15"
  componentResources:
    gunicorn:
      requests:
        cpu: "500m"
        memory: "1Gi"
      limits:
        cpu: "2"
        memory: "4Gi"
    worker-default:
      requests:
        cpu: "200m"
        memory: "512Mi"
      limits:
        cpu: "1"
        memory: "2Gi"
```

### Performance Tuning

**Gunicorn Workers:**
```yaml
componentReplicas:
  gunicorn: 4  # 2-4 workers per CPU core recommended
```

**Redis Configuration:**
```yaml
redisConfig:
  cache:
    memory: "2Gi"
  queue:
    memory: "1Gi"
```

---

## Security

### Security Contexts

Configure pod and container security:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: secure-bench
spec:
  frappeVersion: "15"
  security:
    podSecurityContext:
      runAsNonRoot: true
      runAsUser: 1001
      fsGroup: 1001
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: false
      capabilities:
        drop:
          - ALL
```

### Image Pull Secrets

For private registries:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: registry-credentials
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-encoded-docker-config>
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: private-registry-bench
spec:
  frappeVersion: "15"
  imageConfig:
    repository: "private-registry.com/frappe/erpnext"
    pullSecrets:
      - name: registry-credentials
```

### TLS Configuration

Enable TLS for sites:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: secure-site
spec:
  siteName: secure.example.com
  benchRef:
    name: production-bench
  tls:
    enabled: true
    secretName: tls-certificate
    issuer: letsencrypt-prod  # cert-manager issuer
```

### Network Policies

Restrict network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: frappe-bench-policy
spec:
  podSelector:
    matchLabels:
      app: frappe
      bench: production-bench
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              name: mariadb-operator
      ports:
        - protocol: TCP
          port: 3306
```

---

## Operations

### Updating a Bench

Update Frappe version or apps:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
spec:
  frappeVersion: "15"  # Update version
  apps:
    - name: erpnext
      source: image
    - name: custom-app
      source: git
      git:
        repository: "https://github.com/company/custom-app"
        branch: "main"
```

Apply changes:
```bash
kubectl apply -f bench.yaml
kubectl rollout status deployment/production-bench-gunicorn
```

### Backing Up a Site

Create a backup:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteBackup
metadata:
  name: daily-backup
spec:
  siteRef:
    name: customer-site
    namespace: default
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention:
    days: 30
  storage:
    s3:
      bucket: "frappe-backups"
      region: "us-east-1"
```

### Restoring from Backup

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteBackup
metadata:
  name: restore-job
spec:
  siteRef:
    name: customer-site
    namespace: default
  restore:
    backupName: "daily-backup-2024-01-15"
    restoreDatabase: true
    restoreFiles: true
```

### Monitoring and Observability

**Check Bench Status:**
```bash
kubectl get frappebench production-bench -o jsonpath='{.status.conditions[*]}'
```

**Check Site Status:**
```bash
kubectl get frappesite customer-site -o jsonpath='{.status.conditions[*]}'
```

**View Logs:**
```bash
# Operator logs
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager

# Bench component logs
kubectl logs -l app=frappe,bench=production-bench,component=gunicorn

# Site init job logs
kubectl logs job/customer-site-init
```

**Prometheus Metrics:**
The operator exposes metrics at `/metrics` endpoint:
```bash
kubectl port-forward -n frappe-operator-system deployment/frappe-operator-controller-manager 8080:8080
curl http://localhost:8080/metrics
```

### Health Checks

**Bench Health:**
```bash
kubectl get frappebench production-bench -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

**Component Health:**
```bash
kubectl get pods -l app=frappe,bench=production-bench
kubectl describe pod <pod-name>
```

---

## Troubleshooting

### Common Issues

#### Bench Not Ready

**Symptoms:**
- Bench status shows `Progressing` or `Degraded`
- Pods not starting

**Debugging:**
```bash
# Check bench conditions
kubectl describe frappebench production-bench

# Check events
kubectl get events --field-selector involvedObject.name=production-bench

# Check pod status
kubectl get pods -l app=frappe,bench=production-bench

# Check logs
kubectl logs -l app=frappe,bench=production-bench,component=gunicorn
```

**Common Causes:**
- Image pull errors (check image name and pull secrets)
- Storage issues (check PVC status)
- Resource constraints (check node resources)

#### Site Not Ready

**Symptoms:**
- Site status shows `Pending` or `Failed`
- Site init job failing

**Debugging:**
```bash
# Check site conditions
kubectl describe frappesite customer-site

# Check init job
kubectl get job customer-site-init
kubectl logs job/customer-site-init

# Check database connection
kubectl get secret customer-site-db-credentials -o yaml
```

**Common Causes:**
- Database connection issues
- Admin password secret missing
- Bench not ready

#### Image Pull Errors

**Symptoms:**
- Pods in `ImagePullBackOff` state

**Solutions:**
```bash
# Check image name
kubectl get frappebench production-bench -o jsonpath='{.spec.imageConfig}'

# Verify pull secrets
kubectl get secret registry-credentials

# Test image pull manually
kubectl run test-pull --image=myregistry.com/frappe/erpnext:latest --rm -it --restart=Never
```

#### Database Connection Issues

**Symptoms:**
- Site init job failing with database errors
- Sites unable to connect to database

**Debugging:**
```bash
# Check database credentials
kubectl get secret customer-site-db-credentials -o yaml

# Check database pod (if using MariaDB operator)
kubectl get mariadb -n default

# Test database connection
kubectl run db-test --image=mariadb:10.6 --rm -it --restart=Never -- \
  mysql -h <db-host> -u <user> -p<password>
```

#### Site Deletion Failures

**Symptoms:**
- Site deletion job fails with password prompt
- Error: "MySQL root password: Aborted!"
- Site resource stuck in deletion

**Root Cause:**
Site deletion requires MariaDB root credentials to drop the database. Site users have limited privileges and cannot drop databases (security feature).

**Debugging:**
```bash
# Check if deletion job exists
kubectl get job <site-name>-delete -n <namespace>

# View deletion job logs
kubectl logs job/<site-name>-delete -n <namespace>

# Verify root secret exists (dedicated mode)
kubectl get secret <site-name>-mariadb-root -n <namespace>

# For shared mode, check MariaDB CR's root secret
kubectl get mariadb <mariadb-name> -n <namespace> -o jsonpath='{.spec.rootPasswordSecretKeyRef}'
```

**Solutions:**

1. **Missing Root Secret**: If root secret is missing, recreate it or use MariaDB operator to regenerate:
   ```bash
   # For dedicated mode MariaDB instances
   kubectl get mariadb <site-name>-mariadb -o yaml
   # Check rootPasswordSecretKeyRef field
   ```

2. **Manual Cleanup** (if deletion job continues to fail):
   ```bash
   # Drop database manually
   kubectl exec -it <mariadb-pod> -- mysql -u root -p<password> \
     -e "DROP DATABASE IF EXISTS <database-name>;"
   
   # Drop user manually
   kubectl exec -it <mariadb-pod> -- mysql -u root -p<password> \
     -e "DROP USER IF EXISTS '<username>'@'%';"
   
   # Remove finalizer from site resource
   kubectl patch frappesite <site-name> -n <namespace> \
     --type json -p='[{"op": "remove", "path": "/metadata/finalizers"}]'
   ```

3. **Verify Privileges**: Confirm site user doesn't have DROP DATABASE privilege (expected):
   ```bash
   kubectl exec -it <mariadb-pod> -- mysql -u root -p<password> \
     -e "SHOW GRANTS FOR '<site-username>'@'%';"
   # Should NOT see "DROP" in database-level grants
   ```

**Prevention:**
- Ensure MariaDB Operator CRDs are installed: `kubectl get crd mariadbs.k8s.mariadb.com`
- Don't manually modify database secrets
- Let operator manage database lifecycle

### Debugging Tips

**Enable Verbose Logging:**
```bash
# Update operator deployment
kubectl edit deployment frappe-operator-controller-manager -n frappe-operator-system

# Add to args:
- --zap-log-level=debug
```

**Check Resource Events:**
```bash
kubectl get events --all-namespaces --sort-by='.lastTimestamp' | grep frappe
```

**Inspect Resource Status:**
```bash
kubectl get frappebench production-bench -o yaml | grep -A 20 status
kubectl get frappesite customer-site -o yaml | grep -A 20 status
```

---

## API Reference

### FrappeBench Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: example-bench
spec:
  # Required: Frappe version
  frappeVersion: "15"
  
  # Optional: Apps to install
  apps:
    - name: erpnext
      source: image  # or "git" or "fpm"
  
  # Optional: Image configuration
  imageConfig:
    repository: "docker.io/frappe/erpnext"
    tag: "latest"
    pullPolicy: Always
    pullSecrets:
      - name: registry-credentials
  
  # Optional: Component replicas
  componentReplicas:
    gunicorn: 2
    nginx: 1
    socketio: 1
    scheduler: 1
  
  # Optional: Resource limits
  componentResources:
    gunicorn:
      requests:
        cpu: "500m"
        memory: "1Gi"
      limits:
        cpu: "2"
        memory: "4Gi"
  
  # Optional: Worker autoscaling
  workerAutoscaling:
    default:
      enabled: true
      minReplicas: 0
      maxReplicas: 10
      targetQueueLength: 5
  
  # Optional: Storage class
  storageClassName: "fast-ssd"
  
  # Optional: Database config
  dbConfig:
    provider: mariadb
    mode: shared
  
  # Optional: Security context
  security:
    podSecurityContext:
      runAsNonRoot: true
      runAsUser: 1001
    securityContext:
      allowPrivilegeEscalation: false
```

### FrappeSite Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: example-site
spec:
  # Required: Site name (must match domain)
  siteName: "example.com"
  
  # Required: Bench reference
  benchRef:
    name: example-bench
    namespace: default
  
  # Optional: Admin password secret
  adminPasswordSecretRef:
    name: site-admin-password
    namespace: default
  
  # Optional: Database configuration
  dbConfig:
    provider: mariadb  # or "postgres" or "external"
    mode: shared  # or "dedicated"
    connectionSecretRef:
      name: db-credentials
  
  # Optional: Domain override
  domain: "custom.example.com"
  
  # Optional: Ingress configuration
  ingress:
    enabled: true
    className: "nginx"
    annotations:
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
  
  # Optional: TLS configuration
  tls:
    enabled: true
    secretName: tls-certificate
    issuer: "letsencrypt-prod"
  
  # Optional: OpenShift Route (for OpenShift clusters)
  routeConfig:
    enabled: true
    host: "example.com"
    tlsTermination: "edge"
```

### Status Fields

**FrappeBench Status:**
```yaml
status:
  phase: "Ready"  # Pending, Progressing, Ready, Failed
  conditions:
    - type: Ready
      status: "True"
      observedGeneration: 1
    - type: Progressing
      status: "False"
  installedApps:
    - erpnext
  observedGeneration: 1
```

**FrappeSite Status:**
```yaml
status:
  phase: "Ready"  # Pending, Provisioning, Ready, Failed
  conditions:
    - type: Ready
      status: "True"
      observedGeneration: 1
  benchReady: true
  databaseReady: true
  databaseName: "example_com"
  siteURL: "https://example.com"
  resolvedDomain: "example.com"
```

---

## Examples

### Basic Deployment

**Bench:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: basic-bench
spec:
  frappeVersion: "15"
  apps:
    - name: erpnext
      source: image
```

**Site:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: basic-site
spec:
  siteName: "basic.local"
  benchRef:
    name: basic-bench
  ingress:
    enabled: true
```

### Production Deployment

**High-Availability Bench:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: prod-bench
spec:
  frappeVersion: "15"
  apps:
    - name: erpnext
      source: image
  componentReplicas:
    gunicorn: 4
    nginx: 2
    socketio: 2
  componentResources:
    gunicorn:
      requests:
        cpu: "1"
        memory: "2Gi"
      limits:
        cpu: "4"
        memory: "8Gi"
  workerAutoscaling:
    default:
      enabled: true
      minReplicas: 2
      maxReplicas: 20
      targetQueueLength: 5
  storageClassName: "fast-ssd"
```

**Production Site:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: prod-site
spec:
  siteName: "production.example.com"
  benchRef:
    name: prod-bench
  ingress:
    enabled: true
    className: "nginx"
  tls:
    enabled: true
    issuer: "letsencrypt-prod"
```

### OpenShift Deployment

**OpenShift Bench:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: ocp-bench
spec:
  frappeVersion: "15"
  apps:
    - name: erpnext
      source: image
  imageConfig:
    repository: "image-registry.openshift-image-registry.svc:5000/frappe/erpnext"
    pullPolicy: Always
  security:
    podSecurityContext:
      runAsNonRoot: true
```

**OpenShift Site with Route:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: ocp-site
spec:
  siteName: "ocp.example.com"
  benchRef:
    name: ocp-bench
  routeConfig:
    enabled: true
    host: "ocp.example.com"
    tlsTermination: "edge"
    annotations:
      haproxy.router.openshift.io/timeout: "300s"
```

### External Database

**Site with RDS:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rds-credentials
type: Opaque
stringData:
  username: "frappe_user"
  password: "secure_password"
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: rds-site
spec:
  siteName: "rds.example.com"
  benchRef:
    name: production-bench
  dbConfig:
    provider: external
    host: "rds-instance.region.rds.amazonaws.com"
    port: "3306"
    connectionSecretRef:
      name: rds-credentials
```

---

## Best Practices

### Resource Planning

- **Gunicorn**: 2-4 replicas per CPU core
- **Workers**: Start with 2-3, scale based on queue length
- **Storage**: Use fast SSD storage for production
- **Memory**: Allocate 2-4GB per gunicorn pod

### Security

- Always use TLS for production sites
- Enable security contexts (runAsNonRoot)
- Use image pull secrets for private registries
- Regularly rotate database credentials

### Monitoring

- Monitor bench and site conditions
- Set up alerts for failed reconciliations
- Track resource usage and scaling metrics
- Monitor database connection pools

### Backup Strategy

- Enable automated backups for production sites
- Test restore procedures regularly
- Store backups in multiple locations
- Document recovery procedures

---

## Additional Resources

- **GitHub Repository**: [vyogotech/frappe-operator](https://github.com/vyogotech/frappe-operator)
- **Release Notes**: See `docs/RELEASE_NOTES_*.md`
- **Examples**: See `examples/` directory
- **Issues**: [GitHub Issues](https://github.com/vyogotech/frappe-operator/issues)

---

**Last Updated**: 2024-01-15  
**Operator Version**: v2.5.0
