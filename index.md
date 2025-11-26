# Frappe Operator

A Kubernetes Operator for managing Frappe Framework deployments on Kubernetes.

## What is Frappe Operator?

Frappe Operator simplifies the lifecycle management of Frappe Sites on Kubernetes that are currently managed by Helm charts. It provides a declarative way to create and manage Frappe benches and sites using Kubernetes Custom Resources.

## Features

- **Declarative Management**: Define Frappe benches and sites using Kubernetes manifests
- **Multi-Tenancy**: Support for multiple sites sharing a single bench infrastructure
- **Flexible Database Options**: Shared, dedicated, or external database configurations
- **Auto-Scaling**: Built-in support for Horizontal Pod Autoscaling
- **Production-Ready**: TLS, ingress management, and resource optimization
- **Operator Integration**: Works with MariaDB Operator and cert-manager
- **Component Management**: Automatic management of all Frappe components (gunicorn, workers, scheduler, socketio)

## Custom Resource Definitions

### Core Resources

1. **FrappeBench** - Defines and manages a Frappe bench with shared infrastructure
2. **FrappeSite** - Defines and manages individual Frappe sites

### Additional Resources

3. **SiteUser** - Manages users of a particular site and their permissions
4. **SiteWorkspace** - Creates workspaces on a site declaratively
5. **SiteDashboard** & **SiteDashboardChart** - Creates dashboards and dashboard charts
6. **SiteBackup** - Manages site backups
7. **SiteJob** - Executes custom jobs on sites

## Quick Start

Get started with Frappe Operator in minutes:

```bash
# Install the operator
kubectl apply -f https://raw.githubusercontent.com/vyogo-tech/frappe-operator/main/config/install.yaml

# Create a minimal bench and site
kubectl apply -f https://raw.githubusercontent.com/vyogo-tech/frappe-operator/main/examples/minimal-bench-and-site.yaml

# Check status
kubectl get frappebench,frappesite
```

## Architecture

The Frappe Operator follows the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) and uses controllers to manage the lifecycle of Frappe deployments:

```
┌─────────────────────────────────────────────────────────┐
│                   Frappe Operator                       │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌────────────┐         ┌────────────┐                │
│  │FrappeBench │◄────────┤FrappeSite  │                │
│  │Controller  │         │Controller  │                │
│  └─────┬──────┘         └─────┬──────┘                │
│        │                      │                        │
│        ▼                      ▼                        │
│  ┌─────────────────────────────────────┐              │
│  │   Kubernetes Resources              │              │
│  │  • Deployments                      │              │
│  │  • Services                         │              │
│  │  • Ingresses                        │              │
│  │  • Jobs (Init, Migrate, Backup)     │              │
│  │  • ConfigMaps & Secrets             │              │
│  │  • PersistentVolumeClaims           │              │
│  └─────────────────────────────────────┘              │
│                                                         │
└─────────────────────────────────────────────────────────┘
           │                           │
           ▼                           ▼
    ┌──────────┐              ┌─────────────┐
    │ MariaDB  │              │   Redis/    │
    │ Operator │              │ DragonFly   │
    └──────────┘              └─────────────┘
```

## Use Cases

### 1. **Development Environment**
Quick setup for local development and testing with minimal resource requirements.

### 2. **SaaS Platform**
Multi-tenant deployment where multiple customer sites share bench infrastructure for efficiency.

### 3. **Enterprise Deployment**
High-availability production deployment with dedicated resources and auto-scaling.

### 4. **Multi-Environment Setup**
Manage dev, staging, and production environments in separate namespaces.

## Why Use Frappe Operator?

### Before (Helm Charts)
- Manual configuration management
- Complex upgrade procedures
- Limited automation
- Difficult multi-tenancy

### After (Frappe Operator)
- Declarative configuration
- Automated lifecycle management
- Built-in multi-tenancy
- Self-healing capabilities
- GitOps-ready

## Documentation Structure

- **[Getting Started](getting-started.md)** - Installation and first deployment
- **[Concepts](concepts.md)** - Understanding benches, sites, and architecture
- **[API Reference](api-reference.md)** - Complete CRD specification
- **[Examples](examples.md)** - Common deployment patterns
- **[Operations](operations.md)** - Day-2 operations guide
- **[Troubleshooting](troubleshooting.md)** - Common issues and solutions

## Requirements

- Kubernetes 1.19+
- kubectl configured to access your cluster
- (Optional) MariaDB Operator for database management
- (Optional) Ingress Controller for external access
- (Optional) cert-manager for TLS certificates

## Community and Support

- **GitHub**: [vyogo-tech/frappe-operator](https://github.com/vyogo-tech/frappe-operator)
- **Issues**: [GitHub Issues](https://github.com/vyogo-tech/frappe-operator/issues)
- **License**: Apache 2.0

## Next Steps

1. [Install and configure the operator](getting-started.md#installation)
2. [Deploy your first site](getting-started.md#deploying-your-first-site)
3. [Explore deployment patterns](examples.md)
4. [Learn about production best practices](operations.md)

---

*Frappe Operator is maintained by Vyogo Technologies and the community.*

