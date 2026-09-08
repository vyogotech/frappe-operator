---
layout: default
title: Frappe Operator Documentation
nav_order: 1
description: "Complete guide for deploying and managing Frappe Framework applications on Kubernetes & OpenShift"
permalink: /
has_toc: true
---

# Frappe Operator Documentation

Welcome to the comprehensive documentation for the **Frappe Operator** by **Vyogo Technologies** — an enterprise-grade Kubernetes operator that automates the deployment, scaling, multi-database provisioning, and lifecycle management of [Frappe Framework](https://frappeframework.com/) applications (including ERPNext) on Kubernetes and OpenShift.

## What is Frappe Operator?

Frappe Operator brings cloud-native Kubernetes orchestration to Frappe deployments, making it easy to:

- <i data-lucide="rocket"></i> **Declarative Deployment** — Deploy Frappe & ERPNext applications with declarative YAML manifests
- <i data-lucide="layers"></i> **Multi-Tenancy** — Run hundreds of isolated sites on shared bench infrastructure
- <i data-lucide="database"></i> **Polymorphic Databases** — Seamlessly provision PostgreSQL (StackGres & Percona) or MariaDB
- <i data-lucide="shield-check"></i> **Enterprise Security** — Fully compatible with OpenShift `restricted-v2` SCCs out-of-the-box
- <i data-lucide="lock"></i> **HTTPS-Only Ingress** — Automatic TLS certificate mounting and edge redirection
- <i data-lucide="trending-up"></i> **Provider-Agnostic Autoscaling** — Scale workers and web tiers dynamically using HPA or KEDA
- <i data-lucide="refresh-cw"></i> **Zero-Downtime Lifecycles** — Rolling updates and declarative schema migrations

## Quick Navigation

### <i data-lucide="book-open"></i> Core Guides
- **[Getting Started](getting-started.md)** - 5-minute quickstart on Kubernetes or OpenShift
- **[Comprehensive Guide](COMPREHENSIVE_GUIDE.md)** - Complete reference guide and production best practices
- **[Architecture Overview](ARCHITECTURE.md)** - Internal controller architecture, CRDs, and lifecycle loops

### <i data-lucide="server"></i> Platform & Installation
- **[OpenShift Production Guide](INSTALL_OPENSHIFT.md)** - Complete OpenShift guide & `restricted-v2` SCC compliance
- **[OpenShift Console Plugin & FPM Store](CONSOLE_PLUGIN.md)** - Dynamic web bundle, dashboards, and air-gapped FPM package catalog
- **[Helm Installation Guide](INSTALLATION_HELM.md)** - Deploy via official Vyogo Helm chart
- **[Upgrade Guide](upgrade-guide.md)** - Seamlessly upgrade to v5.2.0

### <i data-lucide="database"></i> Database Management
- **[PostgreSQL Integration Guide](POSTGRESQL_INTEGRATION.md)** - Dedicated (StackGres & Percona) & shared PostgreSQL
- **[MariaDB Integration Guide](MARIADB_INTEGRATION.md)** - Dedicated & shared MariaDB Operator integration
- **[External Resources](external-resources.md)** - Connect to external AWS RDS, Cloud SQL, and Redis

### <i data-lucide="activity"></i> Day-2 Operations & Reference
- **[Operations Guide](operations.md)** - Site provisioning, updates, and maintenance
- **[Site App Installation](SITE_APP_INSTALLATION.md)** - Declarative per-site app installation
- **[API Reference](api-reference.md)** - Complete CRD specification
- **[Examples](examples.md)** - Ready-to-use production manifests
- **[Troubleshooting Guide](troubleshooting.md)** - Common issues, diagnostics, and debugging
- **[Monitoring & Metrics](monitoring.md)** - Prometheus metrics and Grafana dashboards

## Quick Start

```bash
# 1. Install Frappe Operator using official Helm chart
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm repo update
helm install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace

# 2. Deploy a bench
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/examples/basic-bench.yaml

# 3. Deploy a site
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/examples/basic-site.yaml
```

## Version Information

- **Current Version**: v5.2.0
- **Helm Chart**: 5.2.0
- **API Group**: `vyogo.tech/v1`
- **Kubernetes**: 1.22+
- **OpenShift**: 4.10+ (`restricted-v2` SCC compliant)
- **Go Version**: 1.22+
- **License**: Elastic License 2.0 (ELv2)

## What's New in v5.2.0

- ✅ **Polymorphic Database Architecture**: Dedicated and shared PostgreSQL support alongside MariaDB.
- ✅ **StackGres Integration**: Fully declarative per-site `SGCluster` and `SGScript` templates for dedicated PostgreSQL.
- ✅ **Percona PostgreSQL Toggle**: Dedicated `PerconaPGCluster` option with backward-compatibility preservation for existing clusters.
- ✅ **HTTPS-Only Ingress & Routes**: Enforced TLS termination and edge redirect policy (`tls.insecureEdgeTerminationPolicy: Redirect`) across all sites and custom domains.
- ✅ **OpenShift `restricted-v2` SCC Compliance**: Out-of-the-box support for strict security contexts (dynamic non-root UIDs, platform-allocated fsGroup, no privileged permissions).
- ✅ **Provider-Agnostic Autoscaling**: Unified `componentAutoscaling` supporting both Kubernetes HPA and KEDA.

See [CHANGELOG](https://github.com/vyogotech/frappe-operator/blob/main/CHANGELOG.md) for full release history.
