# Frappe Operator Helm Chart

A Helm chart for deploying the Frappe Operator with all required dependencies on Kubernetes.

## Features

- 🚀 **One-Command Installation** - Deploy operator with all dependencies
- 🔐 **Secure by Default** - MariaDB Operator integration with auto-generated credentials
- 📦 **Batteries Included** - Includes MariaDB Operator and optional shared MariaDB instance
- ⚙️ **Highly Configurable** - Extensive values for customization
- 🏢 **Production Ready** - Designed for enterprise deployments

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- At least 4GB RAM available in cluster
- **NGINX Ingress Controller** (optional, but recommended for site access via Ingress)

## Installation

### Quick Start (One Command)

The Helm chart automatically installs MariaDB Operator CRDs via a pre-install hook:

```bash
helm install frappe-operator oci://ghcr.io/vyogotech/charts/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace
```

**What gets installed:**
- Frappe Operator and all CRDs
- MariaDB Operator and its CRDs (automatic via hook)
- A shared MariaDB instance (optional, enabled by default)

### Optional: Install NGINX Ingress Controller

If you want sites to be accessible via Ingress (recommended for production):

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/cloud/deploy.yaml

# Wait for Ingress controller to be ready
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s
```

### Install from Source

```bash
# Clone the repository
git clone https://github.com/vyogotech/frappe-operator.git
cd frappe-operator/helm/frappe-operator

# Install the chart
helm install frappe-operator . \
  --namespace frappe-operator-system \
  --create-namespace
```

### Custom Installation

Create a `custom-values.yaml` file:

```yaml
# Custom operator configuration
operator:
  replicaCount: 2
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi

# MariaDB configuration
mariadb:
  enabled: true
  storage:
    size: 100Gi
  resources:
    requests:
      cpu: 1000m
      memory: 2Gi
    limits:
      cpu: 4000m
      memory: 8Gi

# Create example resources
examples:
  createBench: true
  createSite: true
```

Install with custom values:

```bash
helm install frappe-operator . \
  --namespace frappe-operator-system \
  --create-namespace \
  -f custom-values.yaml
```

## Configuration

### Operator Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `operator.replicaCount` | Number of operator replicas | `1` |
| `operator.image.repository` | Operator image repository | `ghcr.io/vyogotech/frappe-operator` |
| `operator.image.tag` | Operator image tag | `v1.0.0` |
| `operator.resources.limits.cpu` | CPU limit | `500m` |
| `operator.resources.limits.memory` | Memory limit | `256Mi` |
| `operator.serviceAccount.create` | Create service account | `true` |

### MariaDB Operator

| Parameter | Description | Default |
|-----------|-------------|---------|
| `mariadb-operator.enabled` | Install MariaDB Operator | `true` |

### MariaDB Instance

| Parameter | Description | Default |
|-----------|-------------|---------|
| `mariadb.enabled` | Create shared MariaDB instance | `true` |
| `mariadb.name` | MariaDB instance name | `frappe-mariadb` |
| `mariadb.image.tag` | MariaDB version | `10.11` |
| `mariadb.storage.size` | Storage size | `50Gi` |
| `mariadb.replicas` | Number of replicas | `1` |
| `mariadb.galera.enabled` | Enable Galera cluster | `false` |
| `mariadb.rootPasswordSecretRef.generate` | Auto-generate root password | `true` |

### Examples

| Parameter | Description | Default |
|-----------|-------------|---------|
| `examples.createBench` | Create example FrappeBench | `false` |
| `examples.createSite` | Create example FrappeSite | `false` |

## Usage Examples

### Deploy Operator Only (No MariaDB)

If you have an existing database:

```bash
helm install frappe-operator . \
  --set mariadb-operator.enabled=false \
  --set mariadb.enabled=false
```

### High Availability Setup

```bash
helm install frappe-operator . \
  --set operator.replicaCount=3 \
  --set mariadb.replicas=3 \
  --set mariadb.galera.enabled=true
```

### Development Setup with Examples

```bash
helm install frappe-operator . \
  --set examples.createBench=true \
  --set examples.createSite=true \
  --set mariadb.storage.size=10Gi
```

## Upgrading

### Upgrade to Latest Version

```bash
helm upgrade frappe-operator . \
  --namespace frappe-operator-system
```

### Upgrade with Value Changes

```bash
helm upgrade frappe-operator . \
  --namespace frappe-operator-system \
  -f custom-values.yaml
```

## Uninstallation

```bash
# Delete the Helm release
helm uninstall frappe-operator --namespace frappe-operator-system

# Delete the namespace (optional)
kubectl delete namespace frappe-operator-system

# CRDs are not automatically deleted by Helm
# Delete them manually if needed:
kubectl delete crd frappebenchs.vyogo.tech
kubectl delete crd frappesites.vyogo.tech
kubectl delete crd frappeworkpaces.vyogo.tech
kubectl delete crd sitebackups.vyogo.tech
kubectl delete crd sitedashboards.vyogo.tech
kubectl delete crd sitedashboardcharts.vyogo.tech
kubectl delete crd sitejobs.vyogo.tech
kubectl delete crd siteusers.vyogo.tech
kubectl delete crd siteworkspaces.vyogo.tech
```

## Post-Installation

After installation, verify the deployment:

```bash
# Check operator pod
kubectl get pods -n frappe-operator-system

# Check CRDs
kubectl get crds | grep vyogo.tech

# Check MariaDB (if enabled)
kubectl get mariadb frappe-mariadb

# View operator logs
kubectl logs -n frappe-operator-system -l control-plane=controller-manager -f
```

## Creating Your First Site

After the operator is installed:

```yaml
# my-site.yaml
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
  namespace: default
spec:
  frappeVersion: "version-15"
  apps:
    - name: erpnext
      source: image
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: my-company-site
  namespace: default
spec:
  benchRef:
    name: production-bench
    namespace: default
  siteName: mycompany.example.com
  dbConfig:
    provider: mariadb
    mode: shared
    mariadbRef:
      name: frappe-mariadb  # Uses the MariaDB installed by this chart
      namespace: frappe-operator-system
  domain: mycompany.example.com
  ingress:
    enabled: true
  ingressClassName: nginx
```

Apply:

```bash
kubectl apply -f my-site.yaml
```

## Troubleshooting

### Operator Not Starting

```bash
# Check operator logs
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager

# Check events
kubectl get events -n frappe-operator-system
```

### MariaDB Not Ready

```bash
# Check MariaDB status
kubectl get mariadb frappe-mariadb -o yaml

# Check MariaDB pod
kubectl get pods -l app.kubernetes.io/instance=frappe-mariadb
```

### CRDs Not Installing

MariaDB Operator CRDs are installed automatically via a pre-install hook. If installation fails:

```bash
# Check the CRD installer job logs
kubectl logs -n frappe-operator-system -l app.kubernetes.io/component=crd-installer

# Manual CRD installation
kubectl apply --server-side -k "github.com/mariadb-operator/mariadb-operator/config/crd?ref=v0.34.0"

# Verify CRDs are installed
kubectl get crd | grep mariadb
```

### Ingress Not Working

Ingress is created automatically for all FrappeSites by default. If it's not working:

**1. Check if Ingress was created:**
```bash
kubectl get ingress -n <namespace>
```

**2. Verify NGINX Ingress Controller is installed:**
```bash
kubectl get pods -n ingress-nginx
```

If not installed, install it:
```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/cloud/deploy.yaml
```

**3. Disable Ingress creation (if not needed):**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: my-site
spec:
  # ... other fields ...
  ingress:
    enabled: false  # Explicitly disable Ingress
```

### Operator Logs Show Leader Election Errors

Fixed in v1.0.0+ via automatic RBAC configuration. If you still see errors:

```bash
# Verify Role and RoleBinding exist
kubectl get role -n frappe-operator-system | grep leader-election
kubectl get rolebinding -n frappe-operator-system | grep leader-election

# Check if leases are working
kubectl get leases -n frappe-operator-system
```

## Values Reference

See [values.yaml](values.yaml) for complete configuration options.

## Support

- **Documentation**: https://vyogotech.github.io/frappe-operator/
- **GitHub**: https://github.com/vyogotech/frappe-operator
- **Issues**: https://github.com/vyogotech/frappe-operator/issues

## License

Apache 2.0 - See [LICENSE](../../LICENSE) for details.

---

**Maintained by** [Vyogo Technologies](https://vyogo.tech)

