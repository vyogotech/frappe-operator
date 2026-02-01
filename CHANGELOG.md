# Changelog

All notable changes to the Frappe Operator project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- **FrappeSite stability tests**: Fixed fake client not finding shared MariaDB CR by creating the MariaDB via `fakeClient.Create()` in test setup (matching `frappesite_jobs_test.go`), so reconciliation tests no longer fail with "shared MariaDB instance 'frappe-mariadb' not found".
- **Security context test (non-OpenShift)**: Made the test deterministic by using explicit `bench.Spec.Security` overrides instead of env vars (`FRAPPE_DEFAULT_UID`/`FRAPPE_DEFAULT_GID`), avoiding flakiness from test order or environment.
- **Integration Test Tags**: Corrected `FrappeVersion` tags from `v15` to `version-15` in integration tests to match official Docker images.
- **Webhook Validation in Tests**: Added required `Apps` to `FrappeBench` and `FrappeSite` resources in integration tests to satisfy newer webhook validation rules.

### Added
- **Dynamic `envtest` Detection**: Improved test suites to automatically search for `etcd` and `kube-apiserver` in the project-local `bin/k8s` directory. This enables `TestAPIs` and E2E tests to run without manual `KUBEBUILDER_ASSETS` configuration.
- **E2E Bootstrap Configuration**: Enabled E2E tests to attempt execution even when local `envtest` binaries are missing, provided an existing cluster is available.

### Changed
- **CI**: Unit test job runs on push/PR to `main`, `master`, `develop`, and `feature/**`; Docker build depends on test job. Go version in workflows aligned to 1.22.
- **E2E workflow**: Added "Run Integration Tests" step that runs `./test/integration/...` with `INTEGRATION_TEST=true` in the Kind cluster after installing the operator. Go version set to 1.22.
- **CONTRIBUTING.md**: Updated Testing section to match Makefile targets (`make test`, `make coverage`, `make integration-test`), Go prerequisite (1.22+), and described CI/E2E test integration.

---

## [2.6.3] - 2026-01-28

### Fixed
- **Site Deletion Failure**: Fixed an issue where the site deletion job failed to find `apps.txt` by adding the same symlinking logic and volume mounting used in the initialization job.
- **Volume Mount Structure**: Removed incorrect `SubPath: "frappe-sites"` from all volume mounts. Frappe expects the PVC to be mounted directly at `/home/frappe/frappe-bench/sites/` without subdirectory nesting.
- **GitHub Workflow Security**: Fixed PAT token security issue in publish-helm-chart workflow by using x-access-token URL method instead of credential helper store.

### Changed
- **BREAKING: Database Hash Format**: Changed hash format from 4-character (`%x`) to 8-character (`%08x`) for database username generation in MariaDB provider. **This is a breaking change that affects existing deployments.** See Migration Notes below for upgrade instructions.

### Migration Notes for v2.6.3

#### Database Hash Format Change

The database username hash format has changed from 4 characters to 8 characters. This affects how database usernames are generated for MariaDB sites.

**Impact:**
- Existing sites will not be able to connect to their databases after upgrading unless you take action
- Database usernames will change from format `site_a1b2` to `site_a1b2c3d4`

**Migration Options:**

**Option 1: Keep Existing Sites (Recommended for Production)**
1. Do NOT upgrade existing sites immediately
2. Only apply v2.6.3 to NEW sites
3. Mark existing FrappeSite resources with annotation to prevent upgrades:
   ```yaml
   metadata:
     annotations:
       frappe.tech/skip-upgrade: "true"
   ```

**Option 2: Manual Migration (For Advanced Users)**
1. Before upgrading, note down all existing database usernames
2. Upgrade the operator
3. Manually update MariaDB Users and Grants to use new hash format
4. Update site configs with new database credentials

**Option 3: Fresh Deployment**
- If testing or development environment, delete all sites and recreate them

**Verification:**
After upgrade, check that new sites create database users with 8-character hashes:
```bash
kubectl get mariadbuser -A
# Should show usernames like: site_a1b2c3d4 (8 chars after underscore)
```

---

## [2.6.2] - 2026-01-28

### Fixed
- **Missing Namespace RBAC**: Added missing `namespaces` list/watch permissions to the operator's `ClusterRole`, resolving `forbidden` errors during platform detection and SELinux MCS label retrieval on OpenShift.

---

## [2.6.1] - 2026-01-28

### Added
- **Route Support for FrappeBench**: Added ownership tracking for OpenShift Routes at the bench level.
- **Improved Platform Detection**: Refactored platform detection to use a robust Discovery Client, reducing API overhead and improving reliability.
- **Custom-Host Route Support**: Added RBAC permissions for managing Route custom hosts.

### Fixed
- **OpenShift SCC Violations (Redis)**: Fixed `forbidden` errors for Redis pods by removing hardcoded UID/GID 999 and GID 0 in security context specifications.
- **OpenShift SCC Violations (Init Jobs)**: Resolved security standard violations in `new-bench-init` by defaulting restricted IDs (UID/GID/FSGroup) to `nil`, allowing OpenShift to manage their values.
- **Redis Reconciliation**: Fixed an issue where existing Redis StatefulSets were not being updated with new security contexts due to missing `ResourceVersion` handling.
- **Security Context Refactor**: Synchronized security context helpers across all components (Bench, Site, Redis) to use a consistent, platform-aware logic.
- **Bench Init Path Fix**: Fixed `apps.txt` creation path and symlinking for production images.

---

## [2.5.0] - 2026-01-13

### Added
- **OpenShift Compatibility**: Verified support for OpenShift restricted security contexts.
- **Pod Security Policies**: Secure defaults applied across all managed resources.
    - `runAsNonRoot: true` enabled by default.
    - `allowPrivilegeEscalation: false` enabled by default.
    - `seccompProfile` set to `RuntimeDefault` for all pods.
- **Customizable SecurityContext**: Added `spec.security` to `FrappeBench` for granular security control.

## [2.4.0] - 2026-01-13

### Added

#### External Database Support
- **External Provider:** Support for using databases managed outside of the Kubernetes cluster (e.g., AWS RDS, Google Cloud SQL).
- **Direct Connection:** Un-deprecated `Host` and `Port` fields in `DatabaseConfig` for direct external connections.
- **Secret-based Credentials:** Added `ConnectionSecretRef` to source database credentials from existing Kubernetes Secrets.
- **Bench-level DB Defaults:** Added `DBConfig` to `FrappeBench` for shared database architecture across multiple sites.
- **Auto-Detection:** Automatic detection of the `external` provider when `ConnectionSecretRef` is provided.

#### Operator Robustness
- **Sequential Initialization:** `FrappeSite` now explicitly waits for `FrappeBench` to reach the `Ready` phase before attempting site initialization.
- **Bench Readiness:** `FrappeBench` status now accurately reflects the completion of its mandatory initialization job (building assets, creating `apps.txt`).
- **Dynamic CLI Compatibility:** Initialization scripts now dynamically detect `bench` CLI features (like `--db-user` support), ensuring compatibility with both Frappe v15 and v16 images.

### Fixed
- Fixed race conditions on shared storage during concurrent bench and site initialization.
- Improved error handling for external database credential extraction.

---

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
| 2.6.3   | 2026-01-28   | Site deletion apps.txt fix |
| 2.6.2   | 2026-01-28   | Missing Namespace RBAC fix |
| 2.6.1   | 2026-01-28   | OpenShift SCC fixes, Redis reconciliation refactor, Robust platform detection |
| 2.4.0   | 2026-01-13   | External database support, Robustness improvements, CLI compatibility |
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

