# Frappe Operator Examples

This directory contains example manifests for deploying Frappe using the operator with MariaDB Operator integration.

## Prerequisites

### 1. Install MariaDB Operator

The Frappe Operator uses MariaDB Operator for secure database provisioning:

```bash
# Install MariaDB Operator CRDs
kubectl apply -f https://github.com/mariadb-operator/mariadb-operator/releases/latest/download/crds.yaml

# Install MariaDB Operator
kubectl apply -f https://github.com/mariadb-operator/mariadb-operator/releases/latest/download/mariadb-operator.yaml
```

### 2. Install Frappe Operator

```bash
kubectl apply -f https://github.com/vyogotech/frappe-operator/releases/latest/download/install.yaml
```

## Quick Start

### 1. Shared MariaDB Setup (Recommended for Multiple Sites)

```bash
# Step 1: Create shared MariaDB instance (one time)
kubectl apply -f mariadb-shared-instance.yaml

# Wait for MariaDB to be ready
kubectl wait --for=condition=Ready mariadb/frappe-mariadb --timeout=300s

# Step 2: Create a bench
kubectl apply -f basic-bench.yaml

# Wait for bench to be ready
kubectl wait --for=condition=Ready frappebench/dev-bench --timeout=300s

# Step 3: Create a site
kubectl apply -f basic-site.yaml

# Wait for site database to be provisioned
kubectl wait --for=condition=Ready database/dev-site-db --timeout=120s

# Wait for site to be ready
kubectl wait --for=condition=Ready frappesite/dev-site --timeout=300s

# Get auto-generated admin password
kubectl get secret dev-site-admin -o jsonpath='{.data.password}' | base64 -d
```

### 2. Dedicated MariaDB per Site (Enterprise/Isolated)

```bash
# Create bench (if not already created)
kubectl apply -f basic-bench.yaml

# Create site with dedicated MariaDB
kubectl apply -f site-dedicated-mariadb.yaml

# The operator automatically creates:
# - Dedicated MariaDB instance
# - Database and user
# - All necessary credentials
```

### 3. Production Deployment

```bash
# Step 1: Deploy shared MariaDB with HA
kubectl apply -f mariadb-shared-instance.yaml

# Step 2: Deploy sites with TLS
kubectl apply -f site-shared-mariadb.yaml
```

## Examples

### Infrastructure
- `mariadb-shared-instance.yaml` - Shared MariaDB for multiple sites (cost-effective)

### Basic Examples
- `basic-bench.yaml` - Simple bench with default settings
- `basic-site.yaml` - Site using shared MariaDB (development)

### Database Modes
- `site-shared-mariadb.yaml` - Site with shared MariaDB (production)
- `site-dedicated-mariadb.yaml` - Site with dedicated MariaDB (enterprise)

### Advanced Examples  
- `autoscaling-bench.yaml` - **NEW**: Bench with KEDA-based worker autoscaling (scale-to-zero)
- `hybrid-bench.yaml` - Bench with hybrid app installation
- `fpm-bench.yaml` - Bench using FPM packages

### Legacy Examples (for reference)
- `mariadb-connection-secret.yaml` - Legacy secret-based DB connection

## Configuration Options

### FrappeBench

Key configuration options:
- `frappeVersion` - Frappe version (e.g., "version-15", "version-14")
- `imageConfig` - Custom container images
- `apps` - List of apps to install
- `componentReplicas` - Replica counts for each component (static)
- `workerAutoscaling` - **NEW**: KEDA-based autoscaling for background workers
  - `short` - Short-running tasks (scale-to-zero capable)
  - `long` - Long-running tasks (minimum replicas configurable)
  - `default` - Default queue workers (static or autoscaled)
- `componentResources` - CPU/memory for each component
- `storageSize` - PVC size (default: 10Gi)
- `storageClassName` - Storage class to use
- `domainConfig` - Domain resolution settings

### FrappeSite

Key configuration options:
- `benchRef` - Reference to FrappeBench
- `siteName` - Site domain name
- `adminPasswordSecretRef` - Admin password secret (optional, auto-generates if not provided)
- `dbConfig` - Database configuration
- `domain` - External domain
- `tls` - TLS configuration
- `ingress` - Ingress settings

## Best Practices

### Development
- Use `.localhost` or `.local` domains
- Use auto-generated passwords
- Use default resource limits
- Use local storage (RWO)
- Enable worker autoscaling to save resources

### Production
- Use proper domain names
- Store credentials in Secrets
- Configure appropriate resource limits
- Use RWX storage (NFS, EFS, etc.)
- Enable TLS with cert-manager
- Configure worker autoscaling for cost optimization
  - Set appropriate `queueLength` based on workload
  - Use `minReplicas: 0` for scale-to-zero on bursty workloads
  - Use `minReplicas: 1+` for consistent background processing

## Troubleshooting

### Check Bench Status
```bash
kubectl get frappebench -A
kubectl describe frappebench <name>
```

### Check Site Status
```bash
kubectl get frappesite -A
kubectl describe frappesite <name>
```

### Check Logs
```bash
# Operator logs
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager -c manager

# Site init job logs
kubectl logs job/<site-name>-init

# Application logs
kubectl logs deployment/<bench-name>-gunicorn

# Worker autoscaling logs
kubectl logs deployment/<bench-name>-worker-short
kubectl logs deployment/<bench-name>-worker-long
```

### Check Worker Autoscaling
```bash
# Check ScaledObjects (KEDA)
kubectl get scaledobjects

# Check worker scaling status
kubectl get frappebench <bench-name> -o jsonpath='{.status.workerScaling}' | jq

# Check HPA created by KEDA
kubectl get hpa

# Check queue length
kubectl exec deployment/<bench-name>-redis-queue -- redis-cli LLEN "rq:queue:short"
```

### Common Issues

1. **Site stuck in Provisioning**
   - Check init job: `kubectl get job <site-name>-init`
   - Check job logs: `kubectl logs job/<site-name>-init`

2. **Database connection errors**
   - Verify database secret exists
   - Check credentials in secret
   - Verify database is accessible

3. **Storage issues**
   - Check PVC status: `kubectl get pvc`
   - Verify storage class supports required access mode
   - Check storage class: `kubectl get storageclass`

## More Information

- [Main Documentation](../README.md)
- [Operations Guide](../operations.md)
- [Troubleshooting Guide](../troubleshooting.md)
- [API Reference](../api-reference.md)


