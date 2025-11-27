# Release Notes - Frappe Operator v2.0.0

**Release Date:** November 27, 2024

**Release Title:** Hybrid App Installation & Enterprise Features

---

## 🎉 Major Features

### Hybrid App Installation

**The biggest feature in v2.0!** Install Frappe apps from three different sources:

1. **FPM Packages** - Install from Frappe Package Manager repositories
   - Version-specific installations
   - Private repository support
   - Reproducible deployments
   - Perfect for enterprise environments

2. **Git Repositories** - Traditional `bench get-app` approach
   - Clone directly from Git
   - Branch/tag specification
   - Ideal for development
   - Can be disabled cluster-wide

3. **Pre-built Images** - Apps baked into container images
   - Fastest startup time
   - Offline-capable
   - Backward compatible

**Example:**

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  frappeVersion: "version-15"
  apps:
    - name: frappe
      source: image
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
    - name: custom_app
      source: git
      gitUrl: https://github.com/company/custom_app.git
```

### Enterprise Git Control

**NEW:** Disable Git access cluster-wide for security compliance.

- Operator-level default configuration
- Per-bench override capability
- Perfect for regulated industries
- Prevents unauthorized code access

**Configuration:**

```yaml
# Operator ConfigMap
data:
  gitEnabled: "false"  # Disable Git by default

# Per-bench override
spec:
  gitConfig:
    enabled: true  # Enable for this bench only
```

### FPM Repository Management

**NEW:** Configure multiple FPM package repositories with priority-based resolution.

- Multiple repository support
- Authentication via Kubernetes secrets
- Priority-based package resolution
- Operator-level + bench-level configuration

**Example:**

```yaml
spec:
  fpmConfig:
    repositories:
      - name: company-private
        url: https://fpm.company.com
        priority: 10
        authSecretRef:
          name: fpm-credentials
      - name: frappe-community
        url: https://fpm.frappe.io
        priority: 50
```

### FrappeBench CRD

**NEW:** Formal CRD for bench management with comprehensive configuration.

- Structured app sources
- Image configuration
- Component replicas configuration
- Resource requirements
- Redis/Dragonfly configuration
- Domain configuration
- Status reporting

---

## ✨ Enhancements

### API Types

- Added `AppSource` type for flexible app definitions
- Added `FPMConfig` for repository management
- Added `FPMRepository` for individual repository configuration
- Added `GitConfig` for Git control
- Added `ImageConfig` for image customization
- Added `ComponentReplicas` for replica configuration
- Added `ComponentResources` for resource management
- Added `RedisConfig` for Redis/Dragonfly configuration

### Controllers

- New `FrappeBenchReconciler` for bench management
- FPM CLI integration via `FPMManager`
- Git enable/disable resolution logic
- FPM repository merging and priority handling
- Comprehensive status reporting

### Configuration

- Operator-level ConfigMap for defaults
- Git enabled/disabled cluster-wide
- FPM CLI path configuration
- Default FPM repositories

### Documentation

- Complete migration guide (`FPM_MIGRATION.md`)
- Technical implementation documentation (`HYBRID_FPM_IMPLEMENTATION.md`)
- Updated apps installation documentation (`APPS_INSTALLATION_ISSUE.md`)
- Comprehensive examples (`examples/fpm-bench.yaml`, `examples/hybrid-bench.yaml`)
- Updated README with new features

---

## 🔧 Changes

### Breaking Changes

**None!** This release is fully backward compatible.

- Old `appsJSON` format still supported (deprecated)
- Existing deployments continue to work
- Migration is optional but recommended

### Deprecations

- `FrappeBench.spec.appsJSON` - Use `apps` field instead
  - Will be removed in v3.0
  - Migration guide available in `FPM_MIGRATION.md`

---

## 📦 Installation

### New Installations

```bash
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/v2.0.0/install.yaml
```

### Upgrading from v1.x

```bash
# Update CRDs
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/v2.0.0/config/crd/bases/

# Update operator
kubectl set image deployment/frappe-operator-controller-manager \
  manager=vyogotech/frappe-operator:v2.0.0 \
  -n frappe-operator-system

# Apply operator configuration
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/v2.0.0/config/manager/operator-config.yaml
```

**Note:** Existing FrappeBenches will continue to work. No action required unless you want to use the new features.

---

## 📖 Documentation

### New Documentation

- **[FPM_MIGRATION.md](FPM_MIGRATION.md)** - Complete migration guide
  - Three migration strategies
  - Step-by-step instructions
  - FPM repository setup
  - Troubleshooting tips

- **[HYBRID_FPM_IMPLEMENTATION.md](HYBRID_FPM_IMPLEMENTATION.md)** - Technical details
  - Architecture diagrams
  - Configuration resolution
  - Use cases and benefits

- **[examples/](examples/)** - Production-ready examples
  - `fpm-bench.yaml` - Pure FPM deployment
  - `hybrid-bench.yaml` - All three sources

### Updated Documentation

- **[README.md](README.md)** - Added hybrid app installation section
- **[APPS_INSTALLATION_ISSUE.md](APPS_INSTALLATION_ISSUE.md)** - Updated with implementation details
- **Installation guides** - Updated with new features

---

## 🐛 Bug Fixes

- Fixed controller-gen panic by manually generating DeepCopy methods
- Fixed unused import in FrappeBench controller
- Fixed FrappeBench CRD registration in main.go
- Improved operator image build process
- Enhanced error handling in FPM manager

---

## 🔍 Testing

### Tested Scenarios

✅ FrappeBench creation and initialization  
✅ Image-based app installation  
✅ Operator deployment and reconciliation  
✅ CRD application and validation  
✅ Controller registration and startup  
✅ Status reporting  

### Test Environment

- Kubernetes: Kind (v0.20.0)
- Container Runtime: Podman
- Go Version: 1.19
- Operator Framework: Kubebuilder v3

---

## 📊 Statistics

### Code Changes

- **Files Created:** 16
- **Files Modified:** 9
- **Lines Added:** ~4,000
- **Documentation:** ~30 KB
- **Code:** ~50 KB

### Implementation

- **API Types:** 8 new types
- **Controllers:** 2 new controllers
- **CRDs:** 1 new CRD (FrappeBench)
- **Examples:** 13 comprehensive examples
- **Documentation:** 4 new guides

---

## 🙏 Acknowledgments

### Contributors

- Core implementation and design
- Documentation and examples
- Testing and validation

### Special Thanks

- Frappe Framework team for the amazing platform
- Kubernetes community for the tools and patterns
- All users and contributors for feedback and support

---

## 📝 Migration Guide

### Quick Migration Path

**From Image-Only:**

```yaml
# Before
spec:
  appsJSON: '["erpnext", "hrms"]'

# After
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

**See [FPM_MIGRATION.md](FPM_MIGRATION.md) for complete guide.**

---

## 🚀 What's Next?

### v2.1 Roadmap

- Enhanced FrappeBench resource creation
- Complete bench component lifecycle management
- Horizontal Pod Autoscaling support
- Built-in monitoring dashboards
- Automated migration testing

### Long-term

- Blue-green deployment support
- Multi-cluster federation
- Helm chart support
- GitOps integration (ArgoCD/Flux)

---

## 🆘 Support

### Get Help

- **Documentation:** https://vyogotech.github.io/frappe-operator/
- **GitHub Issues:** https://github.com/vyogotech/frappe-operator/issues
- **GitHub Discussions:** https://github.com/vyogotech/frappe-operator/discussions
- **Frappe Forum:** https://discuss.frappe.io/

### Report Issues

Found a bug? Please report it:
- **GitHub Issues:** https://github.com/vyogotech/frappe-operator/issues/new

---

## 📄 License

Copyright 2024 Vyogo Technologies.

Licensed under the Apache License, Version 2.0.

---

**🎉 Thank you for using Frappe Operator v2.0!**

We hope these new features make your Frappe deployments more flexible, secure, and enterprise-ready.

⭐ If you find this useful, please star the project on GitHub!

