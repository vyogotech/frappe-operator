# Migrating to Hybrid App Installation with FPM Support

## Overview

The Frappe Operator now supports three methods of app installation:
1. **FPM Packages** - Versioned packages from FPM repositories
2. **Git Repositories** - Traditional `bench get-app` (can be disabled)
3. **Pre-built Images** - Apps baked into container images

This guide will help you migrate from the old `appsJSON` format to the new structured `apps` format.

## Why Migrate?

### Current Limitations (appsJSON)
- Apps come only from container images
- No version control
- Must rebuild image for every app change
- No enterprise-friendly offline deployment

### New Capabilities (apps + FPM)
- Install apps from FPM repositories without rebuilding images
- Version-specific app deployments
- Enterprise mode: disable Git access
- Hybrid approach: combine all three methods
- Reproducible deployments
- Faster iteration

## Migration Path

### Step 1: Understand Your Current Setup

**Old Format:**
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms", "custom_app"]'
```

**What happens:** Apps are expected to be pre-installed in the container image.

### Step 2: Choose Your Strategy

#### Strategy A: Pure FPM (Recommended for Enterprise)
Best for production environments without Git access.

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  frappeVersion: "version-15"
  
  # New structured format
  apps:
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
    
    - name: hrms
      source: fpm
      org: frappe
      version: "15.0.0"
    
    - name: custom_app
      source: fpm
      org: mycompany
      version: "1.2.3"
  
  # FPM configuration
  fpmConfig:
    repositories:
      - name: company-repo
        url: https://fpm.company.com
        priority: 10
        authSecretRef:
          name: fpm-credentials
  
  # Disable Git for security
  gitConfig:
    enabled: false
```

#### Strategy B: Hybrid (Flexible)
Combine image, FPM, and Git for maximum flexibility.

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  frappeVersion: "version-15"
  
  apps:
    # Base framework in image
    - name: frappe
      source: image
    
    # Stable apps from FPM
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
    
    # Development apps from Git
    - name: custom_app
      source: git
      gitUrl: https://github.com/mycompany/custom_app.git
      gitBranch: main
  
  fpmConfig:
    repositories:
      - name: company-repo
        url: https://fpm.company.com
        priority: 10
  
  # Allow Git for development
  gitConfig:
    enabled: true
```

#### Strategy C: Image-Only (Backward Compatible)
Keep using pre-built images (existing behavior).

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  frappeVersion: "version-15"
  
  apps:
    - name: erpnext
      source: image
    - name: hrms
      source: image
    - name: custom_app
      source: image
  
  imageConfig:
    repository: myregistry.com/frappe-all-apps
    tag: v1.0.0
```

### Step 3: Prepare FPM Repository (if using FPM)

#### Install FPM CLI

```bash
# Download FPM CLI
curl -LO https://github.com/yourusername/fpm/releases/latest/download/fpm-linux-amd64
chmod +x fpm-linux-amd64
sudo mv fpm-linux-amd64 /usr/local/bin/fpm
```

#### Package Your Apps

```bash
# Package each app
cd /path/to/your-app
fpm package --version 1.0.0 --org mycompany

# This creates: mycompany/your-app-1.0.0.fpm
```

#### Set Up FPM Repository

Follow the [FPM Repository Setup Guide](https://github.com/yourusername/fpm/blob/main/fpm-repo-README.md):

```bash
# Deploy repository using docker-compose
cd fpm-repository
./scripts/setup.sh
podman-compose up -d
```

#### Publish Packages

```bash
# Add repository
fpm repo add company-repo https://fpm.company.com --priority 10

# Publish packages
fpm publish mycompany/your-app==1.0.0 --repo company-repo
```

### Step 4: Create Kubernetes Secrets

#### For FPM Authentication

```bash
kubectl create secret generic fpm-credentials \
  --from-literal=username=admin \
  --from-literal=password=your-secure-password
```

#### For Private Registry (if needed)

```bash
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.com \
  --docker-username=user \
  --docker-password=pass
```

### Step 5: Update FrappeBench Manifest

Replace the old manifest with your chosen strategy from Step 2.

### Step 6: Deploy

```bash
# Apply the updated manifest
kubectl apply -f my-bench.yaml

# Watch the initialization
kubectl get pods -l bench=my-bench -w

# Check init job logs
kubectl logs job/my-bench-init
```

### Step 7: Verify

```bash
# Check bench status
kubectl describe frappebench my-bench

# Verify installed apps
kubectl exec deployment/my-bench-gunicorn -- cat /home/frappe/frappe-bench/sites/apps.txt
```

## Comparison Matrix

| Feature | appsJSON (Old) | apps + FPM (New) |
|---------|---------------|------------------|
| App source | Image only | Image, FPM, Git |
| Versioning | No | Yes (FPM) |
| Git access | Required for build | Optional |
| Enterprise mode | No | Yes (disable Git) |
| Update apps | Rebuild image | Update version in FPM |
| Deployment speed | Fast (if cached) | Fast (no rebuild) |
| Reproducibility | Medium | High (versions locked) |
| Offline capable | Yes (with image) | Yes (with FPM cache) |

## Common Migration Patterns

### Pattern 1: Dev → Staging → Production

**Development:**
```yaml
apps:
  - name: custom_app
    source: git  # Rapid iteration
    gitUrl: https://github.com/company/custom_app.git
    gitBranch: develop

gitConfig:
  enabled: true  # Git allowed
```

**Staging:**
```yaml
apps:
  - name: custom_app
    source: fpm  # Version-locked testing
    org: company
    version: "1.2.3-beta"

gitConfig:
  enabled: true  # Git allowed for debugging
```

**Production:**
```yaml
apps:
  - name: custom_app
    source: fpm  # Stable, versioned
    org: company
    version: "1.2.3"

gitConfig:
  enabled: false  # Git disabled (enterprise)
```

### Pattern 2: Mixed Apps

```yaml
apps:
  # Framework: pre-built in image (fastest)
  - name: frappe
    source: image
  
  # Community apps: from FPM (versioned)
  - name: erpnext
    source: fpm
    org: frappe
    version: "15.0.0"
  
  - name: hrms
    source: fpm
    org: frappe
    version: "15.0.0"
  
  # Custom apps: from company FPM repo
  - name: company_customization
    source: fpm
    org: mycompany
    version: "2.1.0"
```

### Pattern 3: Air-Gapped Deployment

For completely offline environments:

```yaml
apps:
  - name: erpnext
    source: fpm
    org: frappe
    version: "15.0.0"

fpmConfig:
  repositories:
    # Internal-only FPM repository
    - name: internal
      url: http://fpm.internal.company.com
      priority: 10

gitConfig:
  enabled: false  # No external access
```

## Operator Configuration

### Global Git Disable

Set operator-level default in `config/manager/operator-config.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: frappe-operator-config
  namespace: frappe-operator-system
data:
  gitEnabled: "false"  # Disable Git by default
  fpmRepositories: |
    [
      {
        "name": "company-default",
        "url": "https://fpm.company.com",
        "priority": 10
      }
    ]
```

Individual benches can override:

```yaml
spec:
  gitConfig:
    enabled: true  # Override for this bench only
```

## Troubleshooting

### Issue: FPM Package Not Found

```bash
# Check FPM repositories
kubectl exec deployment/my-bench-gunicorn -- fpm repo list

# Search for package
kubectl exec deployment/my-bench-gunicorn -- fpm search myorg/myapp

# Check init job logs
kubectl logs job/my-bench-init
```

### Issue: Git App Failed

```bash
# Check if Git is enabled
kubectl describe frappebench my-bench | grep "Git Enabled"

# Check init job for Git errors
kubectl logs job/my-bench-init | grep -A 10 "git"
```

### Issue: Authentication Failed

```bash
# Verify FPM credentials secret exists
kubectl get secret fpm-credentials

# Check credentials
kubectl get secret fpm-credentials -o yaml

# Test authentication manually
kubectl run -it --rm debug --image=frappe/erpnext:latest --restart=Never -- bash
# Inside pod:
# fpm repo add test https://fpm.company.com
# fpm search  # Should work if auth is correct
```

## Best Practices

1. **Use FPM for Production**: Versioned, reproducible deployments
2. **Disable Git in Production**: Enterprise security requirement
3. **Version Lock**: Always specify exact versions in production
4. **Test Pipeline**: Dev (Git) → Staging (FPM beta) → Prod (FPM release)
5. **Private Repository**: Host your own FPM repository for custom apps
6. **Backup Strategy**: Backup FPM repository and packages
7. **CI/CD Integration**: Automate packaging and publishing

## Rollback Strategy

If migration fails, you can roll back:

```yaml
# Revert to old format (temporarily supported)
spec:
  appsJSON: '["erpnext", "hrms"]'
  
  # Old format is deprecated but still works
  # with image-based apps
```

Or use image source:

```yaml
apps:
  - name: erpnext
    source: image
  - name: hrms
    source: image
```

## Examples

See complete examples in the `examples/` directory:
- [`fpm-bench.yaml`](fpm-bench.yaml) - Pure FPM deployment
- [`hybrid-bench.yaml`](hybrid-bench.yaml) - All three sources

## Support

- **FPM Documentation**: [FPM README](https://github.com/yourusername/fpm)
- **Repository Setup**: [FPM Repository Guide](https://github.com/yourusername/fpm/blob/main/fpm-repo-README.md)
- **Operator Issues**: [GitHub Issues](https://github.com/vyogotech/frappe-operator/issues)

---

**Ready to migrate? Start with Strategy B (Hybrid) for maximum flexibility!**

