# Changelog

All notable changes to the Frappe Operator project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2024-11-27

### Added

#### Hybrid App Installation System
- **Three app installation sources:** FPM packages, Git repositories, and pre-built images
- **`AppSource` type** for structured app definitions with `source`, `version`, `org`, `gitUrl`, and `gitBranch` fields
- **FPM package support** for installing apps from Frappe Package Manager repositories
- **Git repository support** with branch/tag specification
- **Pre-built image support** for fastest startup times
- **Hybrid combinations** allowing mixing of all three sources in a single bench

#### Enterprise Features
- **Cluster-wide Git disable** for security compliance in enterprise environments
- **Per-bench Git override** allowing selective Git enablement
- **FPM repository authentication** via Kubernetes secrets
- **Priority-based repository resolution** for multi-repository setups
- **Air-gapped deployment support** with internal FPM repositories

#### FrappeBench CRD
- **Formal FrappeBench CRD** with comprehensive configuration options
- **`FPMConfig` type** for FPM repository management
- **`FPMRepository` type** for individual repository configuration
- **`GitConfig` type** for Git access control
- **`ImageConfig` type** for custom image configuration
- **`ComponentReplicas` type** for replica configuration
- **`ComponentResources` type** for resource requirements
- **`RedisConfig` type** for Redis/Dragonfly configuration
- **`DomainConfig` type** for domain management
- **Status reporting** with `InstalledApps`, `GitEnabled`, and `FPMRepositories` fields

#### Controllers
- **`FrappeBenchReconciler`** for bench lifecycle management
- **`FPMManager`** for FPM CLI integration
- **Git enable/disable resolution** with priority: bench > operator > default
- **FPM repository merging** combining operator-level and bench-level repos
- **App installation script generation** for hybrid sources
- **Status updates** with comprehensive bench state

#### Configuration
- **Operator ConfigMap** (`config/manager/operator-config.yaml`) for cluster-wide defaults
- **Git enabled/disabled** operator-level setting
- **FPM CLI path** configuration
- **Default FPM repositories** in operator config

#### Documentation
- **`FPM_MIGRATION.md`** - Complete migration guide with three strategies
- **`HYBRID_FPM_IMPLEMENTATION.md`** - Technical implementation details
- **`APPS_INSTALLATION_ISSUE.md`** - Updated with new implementation
- **`examples/fpm-bench.yaml`** - Pure FPM deployment example
- **`examples/hybrid-bench.yaml`** - Hybrid deployment example
- **`Dockerfile.frappe-fpm`** - Dockerfile for Frappe+FPM images
- **`scripts/build-frappe-fpm-image.sh`** - Build script for FPM images
- **Updated README.md** with hybrid app installation section

#### Installation
- **`INSTALLATION.md`** - Comprehensive installation guide
- **`install.yaml`** - All-in-one installation manifest
- **`install.sh`** - Interactive installation script

#### Development
- **`.gitignore`** - Comprehensive gitignore file
- **`.gitignore.md`** - Documentation for gitignore
- **`hack/boilerplate.go.txt`** - Boilerplate header for generated files

### Changed

#### API
- **`FrappeBench.spec.appsJSON`** deprecated in favor of structured `apps` field
- **DeepCopy methods** manually added for all new types (workaround for controller-gen issue)
- **Enhanced status fields** for better observability

#### Controllers
- **`main.go`** updated to register `FrappeBenchReconciler`
- **Removed unused imports** in controllers
- **Improved error handling** throughout

### Fixed

- **Controller-gen panic** resolved by manual DeepCopy generation
- **FrappeBench controller registration** in main.go
- **Unused import** in `frappebench_controller.go`
- **Operator image build** process
- **CRD manifest generation** with manual creation

### Security

- **Git disable feature** for enterprise security compliance
- **FPM authentication** via Kubernetes secrets
- **No hardcoded credentials** in any configuration

### Deprecated

- **`FrappeBench.spec.appsJSON`** - Use `apps` field instead (will be removed in v3.0)

### Testing

- Tested FrappeBench creation and initialization
- Tested operator deployment on Kind cluster
- Tested CRD application and validation
- Tested controller registration and startup
- Verified status reporting

---

## [1.0.0] - 2024-11-20

### Added

- Initial release of Frappe Operator
- `FrappeSite` CRD for individual site management
- `SiteUser` CRD for user management
- `SiteBackup` CRD for backup management
- `SiteJob` CRD for running bench commands
- `SiteWorkspace` CRD for workspace management
- `SiteDashboard` CRD for dashboard management
- MariaDB Operator integration
- Redis support
- NGINX deployment for static assets
- Ingress management
- Domain resolution and configuration
- Multi-tenancy support
- Resource tier configurations
- Comprehensive examples
- Complete documentation

### Initial Controllers

- `FrappeSiteReconciler` - Site lifecycle management
- `SiteUserReconciler` - User management
- `SiteBackupReconciler` - Backup automation
- `SiteJobReconciler` - Job execution
- `SiteWorkspaceReconciler` - Workspace management
- `SiteDashboardReconciler` - Dashboard management

### Initial Features

- Automated site provisioning
- Database management via MariaDB Operator
- Redis cache management
- NGINX configuration
- Ingress creation with TLS support
- Domain resolution logic
- Resource management
- Status reporting

---

## [Unreleased]

### Planned for v2.1

- Enhanced FrappeBench resource creation logic
- Complete bench component lifecycle management
- Horizontal Pod Autoscaling support
- Built-in monitoring dashboards
- Automated migration testing

### Planned for v3.0

- Blue-green deployment support
- Multi-cluster federation
- Helm chart support
- GitOps integration (ArgoCD/Flux)
- Removal of deprecated `appsJSON` field

---

## Version History

| Version | Release Date | Major Features |
|---------|--------------|----------------|
| 2.0.0   | 2024-11-27   | Hybrid app installation, Enterprise Git control, FPM support |
| 1.0.0   | 2024-11-20   | Initial release, Core CRDs, Site management |

---

## Migration Notes

### Migrating from v1.x to v2.0

**No breaking changes!** v2.0 is fully backward compatible.

#### Optional Migration

To use new features, update your `FrappeBench` manifests:

```yaml
# v1.x (still works)
spec:
  appsJSON: '["erpnext", "hrms"]'

# v2.0 (recommended)
spec:
  apps:
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
    - name: hrms
      source: fpm
      org: frappe
      version: "15.0.0"
```

See [FPM_MIGRATION.md](FPM_MIGRATION.md) for complete guide.

---

## Links

- **Repository:** https://github.com/vyogotech/frappe-operator
- **Documentation:** https://vyogotech.github.io/frappe-operator/
- **Issues:** https://github.com/vyogotech/frappe-operator/issues
- **Discussions:** https://github.com/vyogotech/frappe-operator/discussions

---

**Note:** For detailed release notes, see [RELEASE_NOTES.md](RELEASE_NOTES.md)

