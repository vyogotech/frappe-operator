# Frappe Operator

A Kubernetes Operator for managing Frappe Framework deployments on Kubernetes.

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.19+-blue.svg)](https://kubernetes.io/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vyogo-tech/frappe-operator)](go.mod)

## Overview

Frappe Operator simplifies the lifecycle management of Frappe Sites on Kubernetes. It provides a declarative way to create and manage Frappe benches and sites using Kubernetes Custom Resources.

**Key Features:**
- 🚀 Declarative management of Frappe benches and sites
- 🏢 Multi-tenancy support with shared or dedicated resources
- 📊 Auto-scaling and high availability
- 🔒 Production-ready with TLS and security best practices
- 🔄 Automated updates and migrations
- 📦 Integration with MariaDB Operator and cert-manager

## Documentation

**📚 [Complete Documentation](https://vyogo-tech.github.io/frappe-operator/)**

- [Getting Started](https://vyogo-tech.github.io/frappe-operator/getting-started) - Installation and first deployment
- [Concepts](https://vyogo-tech.github.io/frappe-operator/concepts) - Understanding benches and sites
- [API Reference](https://vyogo-tech.github.io/frappe-operator/api-reference) - Complete CRD specification
- [Examples](https://vyogo-tech.github.io/frappe-operator/examples) - Common deployment patterns
- [Operations](https://vyogo-tech.github.io/frappe-operator/operations) - Production operations guide
- [Troubleshooting](https://vyogo-tech.github.io/frappe-operator/troubleshooting) - Common issues and solutions

## Quick Start

### Install the Operator

```bash
kubectl apply -f https://raw.githubusercontent.com/vyogo-tech/frappe-operator/main/config/install.yaml
```

### Deploy Your First Site

```bash
kubectl apply -f https://raw.githubusercontent.com/vyogo-tech/frappe-operator/main/examples/minimal-bench-and-site.yaml
```

### Check Status

```bash
kubectl get frappebench,frappesite
```

For detailed instructions, see the [Getting Started Guide](https://vyogo-tech.github.io/frappe-operator/getting-started).

## Custom Resources

The operator provides the following Custom Resource Definitions:

### Core Resources

- **FrappeBench** - Defines shared infrastructure for multiple sites
- **FrappeSite** - Manages individual Frappe sites

### Additional Resources

- **SiteUser** - Manages site users and permissions
- **SiteWorkspace** - Creates workspaces declaratively
- **SiteDashboard** & **SiteDashboardChart** - Manages dashboards
- **SiteBackup** - Automates site backups
- **SiteJob** - Executes custom jobs on sites

## Architecture

```
┌─────────────────────────────────────────┐
│         FrappeBench (Shared)            │
│  ┌──────┐  ┌───────┐  ┌──────────┐    │
│  │NGINX │  │ Redis │  │  Common  │    │
│  └───┬──┘  └───────┘  │  Storage │    │
│      │                 └──────────┘    │
└──────┼──────────────────────────────────┘
       │
       ├────────┬────────┬────────┐
       │        │        │        │
   ┌───▼──┐ ┌──▼───┐ ┌──▼───┐ ┌──▼───┐
   │Site 1│ │Site 2│ │Site 3│ │Site N│
   │      │ │      │ │      │ │      │
   │ Web  │ │ Web  │ │ Web  │ │ Web  │
   │Workers│ │Workers│ │Workers│ │Workers│
   │  DB  │ │  DB  │ │  DB  │ │  DB  │
   └──────┘ └──────┘ └──────┘ └──────┘
```

## Examples

Browse the [`examples/`](examples/) directory for common deployment patterns:

- **[minimal-bench-and-site.yaml](examples/minimal-bench-and-site.yaml)** - Quick start
- **[production-bench.yaml](examples/production-bench.yaml)** - Production configuration
- **[multi-tenant-bench.yaml](examples/multi-tenant-bench.yaml)** - Multi-tenant SaaS
- **[enterprise-setup.yaml](examples/enterprise-setup.yaml)** - Enterprise deployment
- **[high-availability-bench.yaml](examples/high-availability-bench.yaml)** - HA setup

See the [Examples Documentation](https://vyogo-tech.github.io/frappe-operator/examples) for detailed explanations.

## Requirements

- Kubernetes 1.19+
- kubectl configured to access your cluster
- (Optional) MariaDB Operator for database management
- (Optional) Ingress Controller for external access
- (Optional) cert-manager for TLS certificates

## Development

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- kind or minikube (for local testing)

### Build and Run Locally

```bash
# Install CRDs
make install

# Run controller locally
make run

# Build docker image
make docker-build IMG=<your-registry>/frappe-operator:tag

# Deploy to cluster
make deploy IMG=<your-registry>/frappe-operator:tag
```

### Testing

```bash
# Run tests
make test

# Run with coverage
make test-coverage
```

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

### How to Contribute

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Community

- **Issues**: [GitHub Issues](https://github.com/vyogo-tech/frappe-operator/issues)
- **Discussions**: [GitHub Discussions](https://github.com/vyogo-tech/frappe-operator/discussions)
- **Frappe Forum**: [discuss.frappe.io](https://discuss.frappe.io/)

## License

Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

---

**Built with ❤️ using [Kubebuilder](https://book.kubebuilder.io/)**
