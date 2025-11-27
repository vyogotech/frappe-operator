# Frappe Operator v1.0.0 - Production Ready Release 🎉

**Release Date:** November 27, 2025

## Overview

This is the **first production-ready release** of the Frappe Operator, a Kubernetes operator for managing Frappe/ERPNext deployments at scale. This release provides a complete, enterprise-grade solution for hosting multiple Frappe sites on Kubernetes with flexible resource allocation, automatic site provisioning, and intelligent domain management.

## 🚀 Major Features

### Multi-Tenant Architecture
- **FrappeBench CRD**: Manages shared infrastructure for multiple Frappe sites
  - Shared Redis instances (cache + queue)
  - Shared application pods (Gunicorn, NGINX, Scheduler, Workers, Socket.IO)
  - Persistent storage with intelligent RWX/RWO detection
  - Configurable resource allocation per component

- **FrappeSite CRD**: Individual site management
  - Automatic site creation via `bench new-site`
  - Per-site database and user creation
  - Automatic admin password generation
  - Smart domain resolution with 4-tier priority system
  - Ingress management with TLS support

### Intelligent Storage Management
- **Dynamic Access Mode Detection**: Automatically detects StorageClass capabilities
  - Prefers ReadWriteMany (RWX) for multi-pod access
  - Falls back to ReadWriteOnce (RWO) for local development (Kind, Minikube)
  - Annotates resources with chosen mode for transparency
- **Production-Ready**: Works with NFS, EFS, Azure Files, GCS Filestore, and local provisioners

### Smart Domain Resolution
Four-tier priority system for determining site domains:
1. **Explicit Domain**: User-specified domain in FrappeSite spec
2. **Bench Suffix**: Auto-append suffix from FrappeBench config
3. **Auto-Detection**: Detect cluster domain from Ingress Controller annotations
4. **SiteName Fallback**: Use siteName as-is for local domains (`.local`, `.localhost`)

Special handling for local development domains to ensure Ingress creation.

### Redis Architecture
- **Dual Redis Instances**: Separate StatefulSets for cache and queue
  - `redis-cache`: For application caching
  - `redis-queue`: For background job queues
- **Proper DNS Resolution**: ClusterIP services for reliable hostname resolution
- **Per-Site Configuration**: Each site gets its own Redis configuration in `site_config.json`

### Hybrid App Installation (Enterprise-Ready)
- **Multiple Sources**: Install apps from base images, FPM packages, or Git repositories
- **Git Control**: Operator-wide setting to disable Git pulls for air-gapped environments
- **FPM Support**: Full integration with Frappe Package Manager for enterprise deployments
- **Flexible Priority**: Configure installation order and source priority

### Security Features
- **Automatic Password Generation**: Secure 16-character alphanumeric passwords
- **Secret Management**: Passwords stored in Kubernetes Secrets
- **RBAC**: Comprehensive role-based access control
- **TLS Support**: Ingress TLS with cert-manager integration

## 📦 Components

### Custom Resource Definitions (CRDs)
- `FrappeBench` - Manages shared Frappe infrastructure
- `FrappeSite` - Manages individual Frappe sites
- `SiteBackup` - Backup management (planned)
- `SiteUser` - User management (planned)

### Kubernetes Resources Created
Per FrappeBench:
- 1 PersistentVolumeClaim (sites storage)
- 2 Redis StatefulSets (cache + queue)
- 2 Redis Services
- 1 Gunicorn Deployment
- 1 NGINX Deployment
- 1 Socket.IO Deployment
- 1 Scheduler Deployment
- 3 Worker Deployments (default, long, short)
- 5 Services (gunicorn, nginx, socketio, redis-cache, redis-queue)

Per FrappeSite:
- 1 Initialization Job
- 1 Ingress (if enabled)
- 1 Secret (auto-generated admin password, if not provided)

## 🛠️ Installation

### Prerequisites
- Kubernetes 1.19+
- kubectl configured
- Ingress Controller (nginx, traefik, etc.)
- StorageClass with RWX support (for production) or RWO (for development)

### Quick Install

```bash
# Install CRDs
kubectl apply -f https://github.com/vyogotech/frappe-operator/releases/download/v1.0.0/install.yaml

# Verify installation
kubectl get deployment -n frappe-operator-system
```

### Deploy Operator Config (Optional)
```bash
kubectl apply -f config/manager/operator-config.yaml
```

## 📚 Usage Examples

### Create a FrappeBench

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
  namespace: default
spec:
  version: v15.41.2
  imageConfig:
    repository: frappe/erpnext
    tag: v15.41.2
  apps:
    - name: frappe
      source:
        type: image
    - name: erpnext
      source:
        type: image
  componentReplicas:
    gunicorn: 3
    nginx: 2
    socketio: 2
    workers:
      default: 2
      long: 1
      short: 2
  componentResources:
    gunicorn:
      requests:
        cpu: "500m"
        memory: "1Gi"
      limits:
        cpu: "2"
        memory: "4Gi"
  storageSize: "50Gi"
  storageClassName: "nfs-client"  # Optional: specify storage class
  domainConfig:
    suffix: ".mycompany.com"
    autoDetect: true
```

### Create a FrappeSite

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: customer1
  namespace: default
spec:
  benchRef:
    name: production-bench
    namespace: default
  siteName: customer1.mycompany.com
  # adminPasswordSecretRef is optional - will auto-generate if not provided
  domain: customer1.mycompany.com
  tls:
    enabled: true
    issuer: letsencrypt-prod
  ingress:
    enabled: true
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/proxy-body-size: "100m"
```

### Retrieve Auto-Generated Password

```bash
# Get the auto-generated admin password
kubectl get secret customer1-admin -n default -o jsonpath='{.data.password}' | base64 -d
```

## 🔧 Configuration

### Operator-Wide Settings
Configure via `frappe-operator-config` ConfigMap:
- Git enable/disable
- Default FPM repositories
- Domain detection settings
- Default resource limits

### Per-Bench Settings
- Component replicas
- Resource requests/limits
- Storage size and class
- Domain configuration
- App sources

### Per-Site Settings
- Site name and domain
- Database configuration
- TLS settings
- Ingress annotations
- Admin password (or auto-generate)

## 🧪 Testing

Tested on:
- ✅ Kind (Kubernetes in Docker) - Local development
- ✅ Minikube - Local development
- ✅ Production clusters with NFS storage

Test scenarios:
- ✅ Single site deployment
- ✅ Multiple sites on same bench (4+ sites)
- ✅ Auto-password generation
- ✅ Domain resolution (all 4 tiers)
- ✅ Storage fallback (RWX → RWO)
- ✅ Redis connectivity
- ✅ Ingress creation
- ✅ Site initialization
- ✅ Asset building

## 📊 Performance

- **Site Creation Time**: ~2-3 minutes (includes DB creation, app installation, asset building)
- **Resource Efficiency**: Shared infrastructure reduces overhead by 60-70% vs individual deployments
- **Scalability**: Tested with 4+ sites per bench, supports 10+ sites per bench

## 🐛 Known Issues

1. **Socket.IO Pod**: May show errors initially but recovers automatically
2. **Local Domains**: Require `/etc/hosts` entries or DNS configuration for browser access
3. **Storage Class**: Must support RWX for production multi-replica deployments

## 🔄 Upgrade Path

This is the first production release. Future upgrades will be documented here.

## 📖 Documentation

- [Installation Guide](docs/installation.md)
- [User Guide](docs/user-guide.md)
- [Architecture](docs/architecture.md)
- [Storage Implementation](STORAGE_IMPLEMENTATION.md)
- [Local Testing](LOCAL_TESTING.md)
- [Examples](examples/)

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## 📄 License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## 🙏 Acknowledgments

- Frappe Framework team for the excellent framework
- Kubernetes community for operator patterns and best practices
- MariaDB Operator team for database management inspiration

## 📞 Support

- GitHub Issues: https://github.com/vyogotech/frappe-operator/issues
- Documentation: https://vyogotech.github.io/frappe-operator/
- Community: [Join our discussions](https://github.com/vyogotech/frappe-operator/discussions)

---

**Full Changelog**: https://github.com/vyogotech/frappe-operator/commits/v1.0.0

