---
layout: default
title: Frappe Operator Documentation
nav_order: 1
description: "Complete guide for deploying and managing Frappe Framework applications on Kubernetes"
permalink: /
has_toc: true
---

# Frappe Operator Documentation

Welcome to the comprehensive documentation for the **Frappe Operator** - a Kubernetes operator that automates the deployment, scaling, and management of Frappe Framework applications (including ERPNext) on Kubernetes clusters.

## What is Frappe Operator?

Frappe Operator brings the power of Kubernetes orchestration to Frappe deployments, making it easy to:

- 🚀 **Deploy** Frappe applications with a single command
- 📈 **Scale** automatically based on traffic and resource usage
- 🏢 **Manage** multiple sites efficiently on shared infrastructure
- 🔄 **Update** with zero-downtime rolling updates
- 🔐 **Secure** with auto-generated credentials and RBAC

## Quick Navigation

### 📘 Comprehensive Guide

- **[Complete Reference Guide](COMPREHENSIVE_GUIDE.md)** - Everything you need to know in one place

### For Platform Operators

- **[Installation Guide](getting-started.md)** - Get started in 5 minutes
- **[Configuration Guide](COMPREHENSIVE_GUIDE.md#configuration)** - Configure operator defaults
- **[Image Configuration](COMPREHENSIVE_GUIDE.md#image-configuration)** - Set up custom registries
- **[Operations Guide](operations.md)** - Day-to-day operations and maintenance
- **[Troubleshooting](troubleshooting.md)** - Common issues and solutions

### For Developers

- **[Getting Started](getting-started.md)** - Quick start guide
- **[Concepts](concepts.md)** - Understanding Frappe Operator architecture
- **[API Reference](api-reference.md)** - Complete CRD documentation
- **[Examples](examples.md)** - Real-world deployment examples
- **[Site App Installation](SITE_APP_INSTALLATION.md)** - Install specific apps per site
- **[Backup Management](examples.md#site-backup-management)** - Automated site backups
- **[Best Practices](COMPREHENSIVE_GUIDE.md#best-practices)** - Production deployment patterns

## Key Features

### 🎯 Core Capabilities

- **Declarative Configuration** - Define infrastructure as YAML
- **Multi-Tenancy** - Run hundreds of sites on shared infrastructure
- **Auto-Scaling** - Scale workers based on queue length (KEDA integration)
- **Database Management** - Automatic MariaDB/PostgreSQL provisioning
- **External Database Support** - Connect to RDS, Cloud SQL, or any external DB
- **OpenShift Ready** - Optimized for restricted security contexts
- **GitOps Compatible** - Manage infrastructure as code

### 🔧 Advanced Features

- **Site-Specific App Installation** - Install different apps per site with graceful degradation
- **Hybrid App Installation** - Install from FPM packages, Git, or images
- **Worker Autoscaling** - Scale-to-zero for cost optimization
- **Site reconciliation concurrency** - Tune concurrent site reconciles for 100+ sites (operator config or per-bench)
- **Backup Management** - Automated backups with retention policies
- **Observability** - Built-in Prometheus metrics and logging
- **Multi-Platform** - ARM64 and AMD64 compatible

## Quick Start

```bash
# Install operator
curl -fsSL https://raw.githubusercontent.com/vyogotech/frappe-operator/main/install.sh | bash

# Create a bench
kubectl apply -f examples/basic-bench.yaml

# Create a site
kubectl apply -f examples/basic-site.yaml
```

## Documentation Structure

### 📚 Getting Started
- [Installation](getting-started.md) - Step-by-step installation guide
- [Quick Start](getting-started.md#quick-start) - Deploy your first bench and site
- [Prerequisites](getting-started.md#prerequisites) - What you need before starting

### 🏗️ Architecture & Concepts
- [Concepts](concepts.md) - Understanding how Frappe Operator works
- [Architecture Overview](concepts.md#architecture) - Component interactions
- [Resource Model](concepts.md#resource-model) - CRDs and their relationships

### ⚙️ Configuration & Operations
- [Operations Guide](operations.md) - Day-to-day operations
- [Image Configuration](operations.md#image-configuration) - Custom registries and images
- [Database Configuration](operations.md#database-configuration) - DB setup and management
- [Scaling Configuration](operations.md#scaling) - Auto-scaling setup

### 📖 API Reference
- [API Reference](api-reference.md) - Complete CRD documentation
- [FrappeBench Spec](api-reference.md#frappebench) - Bench configuration options
- [FrappeSite Spec](api-reference.md#frappesite) - Site configuration options

### 💡 Examples
- [Examples](examples.md) - Real-world deployment scenarios
- [Basic Deployment](examples.md#basic-deployment) - Simple bench and site
- [Production Deployment](examples.md#production-deployment) - High-availability setup
- [OpenShift Deployment](examples.md#openshift-deployment) - OpenShift-specific examples

### 🔧 Troubleshooting
- [Troubleshooting Guide](troubleshooting.md) - Common issues and solutions
- [Debugging](troubleshooting.md#debugging) - How to debug issues
- [Logs and Metrics](troubleshooting.md#logs-and-metrics) - Observability tools

## Version Information

- **Current Version**: v2.5.0
- **Kubernetes**: 1.19+
- **Go Version**: 1.24+
- **License**: Apache 2.0

## Support & Community

- **GitHub**: [vyogotech/frappe-operator](https://github.com/vyogotech/frappe-operator)
- **Issues**: [GitHub Issues](https://github.com/vyogotech/frappe-operator/issues)
- **Releases**: [GitHub Releases](https://github.com/vyogotech/frappe-operator/releases)

## What's New

### v2.6.0 (Upcoming)
- ✅ **Site-Specific App Installation**: Install different apps per site with graceful degradation
- ✅ **SiteBackup CRD**: Automated site backups with `bench backup`
- ✅ Full backup options support (files, compression, selective DocTypes)
- ✅ Scheduled backups via CronJob and one-time via Job
- ✅ Custom backup paths and filtering capabilities

### v2.5.0
- ✅ OpenShift Route support
- ✅ Configurable image defaults via ConfigMap
- ✅ Enhanced Conditions and Events
- ✅ Improved finalizer cleanup logic
- ✅ Exponential backoff for retries

### v2.4.0
- ✅ External database support (RDS, Cloud SQL)
- ✅ Production-ready features
- ✅ Enhanced security contexts

See [Release Notes](RELEASE_NOTES_v2.5.0.md) for complete changelog.

---

**Ready to get started?** Head to the [Installation Guide](getting-started.md)!
