# Examples

Real-world deployment patterns and configuration examples for Frappe Operator.

> **Note**: All example YAML files are available in the [`examples/`](https://github.com/vyogotech/frappe-operator/tree/main/examples) directory of the repository.

## Table of Contents

- [Quick Start](#quick-start)
- [Development Environment](#development-environment)
- [Production Deployment](#production-deployment)
- [Multi-Tenant SaaS](#multi-tenant-saas)
- [Enterprise Setup](#enterprise-setup)
- [Custom Domains](#custom-domains)
- [High Availability](#high-availability)
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

