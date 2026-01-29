# Frappe Operator

[![Release](https://img.shields.io/github/v/release/vyogotech/frappe-operator)](https://github.com/vyogotech/frappe-operator/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.19+-blue.svg)](https://kubernetes.io/)
[![Production Ready](https://img.shields.io/badge/Production-Ready-green.svg)](https://vyogotech.github.io/frappe-operator/)

A production-ready Kubernetes operator that automates deployment, scaling, and management of [Frappe Framework](https://frappeframework.com/) applications (including ERPNext) on Kubernetes.

**📚 [Complete Documentation](https://vyogotech.github.io/frappe-operator/)** | **🚀 [Examples](examples/)** | **💬 [Discussions](https://github.com/vyogotech/frappe-operator/discussions)**

## Features

- **One-Command Deployment** - Deploy Frappe/ERPNext with a single kubectl command
- **Multi-Tenancy** - Run hundreds of sites on shared infrastructure
- **Secure by Default** - Auto-generated credentials, per-site DB isolation
- **Production-Ready** - Auto-scaling, zero-downtime updates, automated backups
- **Multi-Platform** - ARM64/AMD64 support
- **Enterprise-Grade** - Fully compatible with OpenShift `restricted-v2` SCCs

## Quick Start

### Prerequisites

- Kubernetes cluster (v1.19+)
- `kubectl` configured
- `helm` (recommended)

### Install

```bash
# Install with Helm (recommended)
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace

# Or install with kubectl
kubectl apply -f https://github.com/vyogotech/frappe-operator/releases/latest/download/install.yaml
```

### Deploy Your First Site

```bash
# 1. Create MariaDB instance
kubectl apply -f examples/mariadb-shared-instance.yaml

# 2. Deploy a basic site
kubectl apply -f examples/basic-bench.yaml
kubectl apply -f examples/basic-site.yaml

# 3. Monitor deployment
kubectl get frappebench,frappesite -w

# 4. Get admin password
kubectl get secret basic-site-admin -o jsonpath='{.data.password}' | base64 -d

# 5. Access (local testing)
kubectl port-forward svc/basic-bench-nginx 8080:8080
# Open http://localhost:8080
```

**That's it!** You now have a running Frappe site.

## Documentation

For detailed guides, visit **[vyogotech.github.io/frappe-operator](https://vyogotech.github.io/frappe-operator/)**:

- **[Getting Started](https://vyogotech.github.io/frappe-operator/getting-started)** - Comprehensive installation guide
- **[Concepts](https://vyogotech.github.io/frappe-operator/concepts)** - Understand benches, sites, and architecture
- **[Examples](https://vyogotech.github.io/frappe-operator/examples)** - Production-ready deployment patterns
- **[Operations Guide](https://vyogotech.github.io/frappe-operator/operations)** - Scaling, backups, updates, monitoring
- **[API Reference](https://vyogotech.github.io/frappe-operator/api-reference)** - Complete CRD specifications
- **[Troubleshooting](https://vyogotech.github.io/frappe-operator/troubleshooting)** - Common issues and solutions
- **[OpenShift Installation](docs/INSTALL_OPENSHIFT.md)** - Step-by-step OpenShift guide
- **[OpenShift Technical Guide](docs/openshift.md)** - Deep dive into compatibility & SCCs
- **[MariaDB Integration Guide](docs/MARIADB_INTEGRATION.md)** - Database isolation & credentials

## Examples

Check the [`examples/`](examples/) directory for ready-to-use configurations:

- **[basic-bench.yaml](examples/basic-bench.yaml)** - Simple development setup
- **[basic-site.yaml](examples/basic-site.yaml)** - Basic site configuration
- **[hybrid-bench.yaml](examples/hybrid-bench.yaml)** - FPM + Git + Image sources
- **[worker-autoscaling.yaml](examples/worker-autoscaling.yaml)** - KEDA-based autoscaling
- **[scheduled-sitebackup.yaml](examples/scheduled-sitebackup.yaml)** - Automated backups
- And [many more](examples/)...

## Custom Resources

| Resource | Purpose | Documentation |
|----------|---------|---------------|
| **FrappeBench** | Shared infrastructure for sites | [API Docs](https://vyogotech.github.io/frappe-operator/api-reference#frappebench) |
| **FrappeSite** | Individual Frappe site | [API Docs](https://vyogotech.github.io/frappe-operator/api-reference#frappesite) |
| **SiteBackup** | Automated backups | [API Docs](https://vyogotech.github.io/frappe-operator/api-reference#sitebackup) |
| **SiteJob** | Run bench commands | [API Docs](https://vyogotech.github.io/frappe-operator/api-reference#sitejob) |

[See all resources →](https://vyogotech.github.io/frappe-operator/api-reference)

## Requirements

**Minimum:**
- Kubernetes 1.19+
- 2 CPU cores, 4GB RAM

**Recommended:**
- Kubernetes 1.24+
- MariaDB Operator or external database
- Ingress controller (nginx, Traefik)
- cert-manager for TLS
- Dynamic storage provisioning

[Full requirements →](https://vyogotech.github.io/frappe-operator/getting-started#prerequisites)

## Community & Support

- 💬 **[GitHub Discussions](https://github.com/vyogotech/frappe-operator/discussions)** - Ask questions
- 🐛 **[GitHub Issues](https://github.com/vyogotech/frappe-operator/issues)** - Report bugs
- 📖 **[Documentation](https://vyogotech.github.io/frappe-operator/)** - Complete guides
- 🌐 **[Frappe Forum](https://discuss.frappe.io/)** - Frappe community

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

---

**Built with ❤️ by [Vyogo Technologies](https://vyogo.tech)**

⭐ **[Star this project](https://github.com/vyogotech/frappe-operator)** if you find it useful!
