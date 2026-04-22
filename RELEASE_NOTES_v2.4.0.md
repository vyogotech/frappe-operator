# Frappe Operator v2.4.0 - External Database Support & Robustness 🚀

**Release Date:** January 13, 2026

## Overview

This release introduces support for **External Database Connections** and significantly improves the **Operational Robustness** of site deployments. Users can now easily connect Frappe sites to externally managed databases (like AWS RDS, Google Cloud SQL, or Azure Database for MariaDB) while the operator handles the complex initialization and runtime configuration.

## 🚀 Major Features

### External Database Support
You can now use databases managed outside of your Kubernetes cluster. This is essential for enterprise deployments requiring high availability, automated backups, and managed database services.

- **External Provider**: A new `external` database provider that bypasses on-cluster database provisioning.
- **Secret Integration**: Securely consume database credentials from existing Kubernetes Secrets.
- **Flexible Connection**: Configure `host`, `port`, and `database` names directly or source them from Secrets.
- **Auto-Detection**: The operator automatically selects the `external` provider if a `connectionSecretRef` is provided but no provider is specified.

### Shared Database Architecture (Bench-level Defaults)
Simplified management of multiple sites on a shared external database server.
- Define a default `DBConfig` at the `FrappeBench` level.
- Individual `FrappeSite` objects automatically merge their configuration with bench defaults.
- Allows for a "Shared Host, Unique Database" pattern across multiple sites.

### Operational Robustness
We've re-engineered the initialization flow to eliminate race conditions and improve compatibility.

- **Sequential Initialization**: 
  - `FrappeBench` now waits for its mandatory `init` job (building assets, internal config) to succeed before marked as `Ready`.
  - `FrappeSite` deployments are now delayed until the referenced `FrappeBench` is `Ready`, preventing storage race conditions and "file not found" errors.
- **Dynamic CLI Compatibility**:
  - The site initialization job now automatically detects the features of the installed `bench` CLI.
  - Fixes compatibility issues with `frappe/erpnext:version-15` images where the `--db-user` flag was causing crashes in older `bench` versions.

## 📦 Changes

### API Changes
- Added `DBConfig` field to `FrappeBench` spec.
- Un-deprecated `Host`, `Port`, and `ConnectionSecretRef` in `DatabaseConfig`.
- Added `external` as a valid value for `Provider`.

### Controller Enhancements
- Implemented `ExternalProvider` logic for credential extraction and connectivity mapping.
- Added readiness state tracking in `FrappeBench` status.
- Added dependency checking in `FrappeSite` reconciler.

## 🛠️ Usage Example

### 1. Create a Secret with External DB Credentials
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rds-creds
type: Opaque
stringData:
  username: frappe_root
  password: secure_password
  database: site1_db # Optional
```

### 2. Define FrappeSite with External DB
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: site1
spec:
  benchRef:
    name: dev-bench
  siteName: site1.example.com
  dbConfig:
    provider: external
    host: rds-instance.abcdef.us-east-1.rds.amazonaws.com
    port: "3306"
    connectionSecretRef:
      name: rds-creds
```

## 🧪 Testing

Tested and verified on:
- ✅ **Kind** clusters using Podman.
- ✅ **Frappe v15** images (verified CLI compatibility).
- ✅ **Frappe v16** images (verified sequential init).
- ✅ **External MariaDB** connection handling.

## 🔄 Upgrade Path

Upgrading from v2.0.0 is seamless. Existing `FrappeBench` and `FrappeSite` resources will continue to work as intended. New features can be adopted by updating your manifests.

---

**Full Changelog**: https://github.com/vyogotech/frappe-operator/commits/v2.4.0
