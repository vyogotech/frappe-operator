# Frappe Operator

[![Release](https://img.shields.io/github/v/release/vyogotech/frappe-operator)](https://github.com/vyogotech/frappe-operator/releases)
[![License](https://img.shields.io/badge/License-Elastic%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.19+-blue.svg)](https://kubernetes.io/)
[![Production Ready](https://img.shields.io/badge/Production-Ready-green.svg)](https://vyogotech.github.io/frappe-operator/)

> [!IMPORTANT]
> **⚡ Now licensed under the Elastic License 2.0**
>
> Frappe Operator has moved to the **[Elastic License 2.0 (ELv2)](LICENSE)** — a
> **source-available** license. In short: **free for general use, not free to run as a managed cloud service.**
>
> - ✅ **Free** to use, self-host, modify, and redistribute — including in
>   production for your own organization or your own dedicated customer instances.
> - 💼 **Requires a commercial license** if you offer Frappe Operator to third
>   parties as a **hosted or managed service** (a Frappe/ERPNext cloud, managed
>   hosting, or multi-tenant SaaS) built on its functionality.
>
> Releases up to and including `v4.x` remain available under Apache 2.0; this
> change applies going forward. See **[LICENSING.md](LICENSING.md)** for the full
> plain-English breakdown, including how this relates to Frappe (MIT) and ERPNext (GPLv3).
>
> Building a managed Frappe/ERPNext hosting business on this? Talk to us first.
> 📬 **Commercial / hosting inquiries:** dev@vyogo.tech | 🌐 [vyogo.tech](https://vyogo.tech)

A production-ready Kubernetes operator that automates deployment, scaling, and management of [Frappe Framework](https://frappeframework.com/) applications (including ERPNext) on Kubernetes.

**📚 [Complete Documentation](https://vyogotech.github.io/frappe-operator/)** | **🚀 [Examples](examples/)** | **💬 [Discussions](https://github.com/vyogotech/frappe-operator/discussions)**

## Features

- **One-Command Deployment** - Deploy Frappe/ERPNext with a single kubectl command
- **Multi-Tenancy** - Run hundreds of sites on shared infrastructure
- **Site-Specific Apps** - Install different apps per site with graceful degradation
- **Secure by Default** - Auto-generated credentials, per-site DB isolation
- **Production-Ready** - Provider-agnostic auto-scaling (KEDA/HPA), zero-downtime updates, automated backups
- **Multi-Platform** - ARM64/AMD64 support
- **Enterprise-Grade** - Fully compatible with OpenShift `restricted-v2` SCCs

## Quick Start

### Prerequisites

- Kubernetes cluster (v1.19+)
- `kubectl` configured
- `helm` (recommended)

### Install

```bash
# Install via Operator Lifecycle Manager (OLM) / OperatorHub (Recommended)
kubectl create -f https://operatorhub.io/install/frappe-operator.yaml

# Or install with Helm
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
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/release/examples/mariadb-shared-instance.yaml

# 2. Deploy a basic site
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/release/examples/basic-bench.yaml
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/release/examples/basic-site.yaml

# 3. Monitor deployment
kubectl get frappebench,frappesite -w

# 4. Get admin password
kubectl get secret basic-site-admin -o jsonpath='{.data.password}' | base64 -d

# 5. Access (local testing)
kubectl port-forward svc/basic-bench-nginx 8080:8080
# Open http://localhost:8080
```

### Triggering a Site Update for a New Image

If you have updated the image in your `FrappeBench` (e.g. pushed a new tag to the registry) and need the operator to re-run the initialization job to pick it up, simply update or increment the `frappe.io/site-version` annotation on your `FrappeSite`:

```bash
kubectl annotate frappesite basic-site frappe.io/site-version="sha-1db505941e7bbe6c47f79ca805e007e20a638aa2" --overwrite
```
This signals the operator to delete the old `bench-init` job and spin up a new one using the updated `ImageConfig`.

**That's it!** You now have a running Frappe site.

### Uninstalling the Operator

If you are just testing and want to clean up your cluster, delete your site and bench resources first, then clean up the CRDs and operator resources.

**1. Clean up Frappe resources and CRDs:**

**Bash / Zsh**:
```bash
kubectl delete CustomResourceDefinition $(kubectl get CustomResourceDefinition | grep -F ".vyogo.tech" | awk '{print $1}')
```

**PowerShell**:
```powershell
kubectl get crd | Select-String -SimpleMatch ".vyogo.tech" | ForEach-Object {
    ($_.ToString().Split()[0])
} | ForEach-Object {
    kubectl delete crd $_
}
```

**2. Uninstall the Operator itself:**

Depending on how you installed the operator, use one of the following methods to remove the controller manager, RBAC, and namespace:

**If installed via OLM:**
```bash
kubectl delete subscription my-frappe-operator -n operators
kubectl delete clusterserviceversion $(kubectl get csv -n operators | grep frappe-operator | awk '{print $1}') -n operators
```

**If installed via Helm:**
```bash
helm uninstall frappe-operator --namespace frappe-operator-system
kubectl delete namespace frappe-operator-system
```

**If installed via kubectl (install.yaml):**
```bash
kubectl delete -f https://github.com/vyogotech/frappe-operator/releases/latest/download/install.yaml
```

## Documentation

For detailed guides, visit **[vyogotech.github.io/frappe-operator](https://vyogotech.github.io/frappe-operator/)**:

- **[Getting Started](https://vyogotech.github.io/frappe-operator/getting-started)** - Comprehensive installation guide
- **[Concepts](https://vyogotech.github.io/frappe-operator/concepts)** - Understand benches, sites, and architecture
- **[Examples](https://vyogotech.github.io/frappe-operator/examples)** - Production-ready deployment patterns
- **[Operations Guide](https://vyogotech.github.io/frappe-operator/operations)** - Scaling, backups, updates, monitoring
- **[API Reference](https://vyogotech.github.io/frappe-operator/api-reference)** - Complete CRD specifications
- **[Troubleshooting](https://vyogotech.github.io/frappe-operator/troubleshooting)** - Common issues and solutions
- **[Site App Installation](docs/SITE_APP_INSTALLATION.md)** - Install specific apps per site
- **[OpenShift Installation](docs/INSTALL_OPENSHIFT.md)** - Step-by-step OpenShift guide
- **[OpenShift Technical Guide](docs/openshift.md)** - Deep dive into compatibility & SCCs
- **[MariaDB Integration Guide](docs/MARIADB_INTEGRATION.md)** - Database isolation & credentials

## Examples

Check the [`examples/`](examples/) directory for ready-to-use configurations:

- **[basic-bench.yaml](examples/basic-bench.yaml)** - Simple development setup
- **[basic-site.yaml](examples/basic-site.yaml)** - Basic site configuration
- **[site-with-apps.yaml](examples/site-with-apps.yaml)** - Site with specific apps installed
- **[hybrid-bench.yaml](examples/hybrid-bench.yaml)** - FPM + Git + Image sources
- **[worker-autoscaling.yaml](examples/worker-autoscaling.yaml)** - KEDA-based autoscaling
- **[scheduled-sitebackup.yaml](examples/scheduled-sitebackup.yaml)** - Automated backups
- **[advanced-pod-config.yaml](examples/advanced-pod-config.yaml)** - Custom labels, geo-tagging, and affinity
- **[test-keda-bench.yaml](examples/test-keda-bench.yaml)** - Generated KEDA test manifest
- **[test-hpa-bench.yaml](examples/test-hpa-bench.yaml)** - Generated HPA test manifest
- And [many more](examples/)...

> **Note**: Test manifests are generated from configuration. See [CONFIGURATION.md](CONFIGURATION.md) for customization.

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

## Development & Testing

### Quick Test Setup

```bash
# Run unit tests
make test

# Run integration tests  
make integration-test

# Test autoscaling functionality (configurable)
./test-autoscaling-helm.sh all

# Customize test configuration
cp test-config.conf my-config.conf
./test-autoscaling-helm.sh --config my-config.conf keda
```

### Configuration System

The project uses a flexible configuration system for testing that eliminates hardcoded values. See [CONFIGURATION.md](CONFIGURATION.md) for complete documentation.

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

> [!NOTE]
> Frappe Operator is licensed under the Elastic License 2.0. Contributions are accepted under the same license, and contributors may be asked to sign a Contributor License Agreement (CLA). See [LICENSING.md](LICENSING.md).

## License

Frappe Operator is licensed under the **Elastic License 2.0 (ELv2)** — a
**source-available** license. See [LICENSE](LICENSE) for the full text and
[LICENSING.md](LICENSING.md) for a plain-English breakdown.

- ✅ **Free** to use, self-host, modify, and redistribute — including in
  production for your own organization and your own dedicated customer instances.
- 💼 **A commercial license is required** to offer Frappe Operator to third
  parties as a **hosted or managed service** (e.g. a Frappe/ERPNext cloud,
  managed hosting, or multi-tenant SaaS built on its functionality).

Releases up to and including `v4.x` were published under the Apache License 2.0
and remain available under those terms; this change applies going forward.

Frappe Operator orchestrates Frappe (MIT) and ERPNext (GPLv3) at arm's length as
separate processes/containers — see [LICENSING.md](LICENSING.md#relationship-to-frappe-and-erpnext).

📬 **Commercial / managed-hosting inquiries:** [dev@vyogo.tech](mailto:dev@vyogo.tech) · 🌐 [vyogo.tech](https://vyogo.tech)

## Trademarks

**Vyogo™** and **Vyogo Cloud™** are trademarks of Vyogo. The Elastic License 2.0
grants rights to the **software**, not to the **marks** — you may refer to the
project by name, but you may not use the Vyogo marks to brand your own product
or a managed service, or to imply endorsement. See **[TRADEMARK.md](TRADEMARK.md)**
for the full policy. "Frappe" and "ERPNext" are trademarks of Frappe Technologies
Pvt. Ltd.; this project is not affiliated with or endorsed by them.

---

⭐ **[Star this project](https://github.com/vyogotech/frappe-operator)** if you find it useful!
