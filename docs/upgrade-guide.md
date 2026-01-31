# Upgrade Guide

This guide covers upgrading the Frappe Operator between versions.

## Table of Contents

- [General Upgrade Process](#general-upgrade-process)
- [Version-Specific Guides](#version-specific-guides)
- [Rollback Procedures](#rollback-procedures)
- [Troubleshooting](#troubleshooting)

## General Upgrade Process

### Pre-Upgrade Checklist

1. **Backup your data**
   ```bash
   # Backup all FrappeSite data
   kubectl get frappesites -A -o yaml > frappesites-backup.yaml
   kubectl get frappebenches -A -o yaml > frappebenches-backup.yaml
   
   # Trigger backups for all sites
   for site in $(kubectl get frappesites -A -o jsonpath='{.items[*].metadata.name}'); do
     kubectl create -f - <<EOF
   apiVersion: vyogo.tech/v1alpha1
   kind: SiteBackup
   metadata:
     name: pre-upgrade-backup-${site}
     namespace: frappe
   spec:
     siteRef:
       name: ${site}
     includeFiles: true
   EOF
   done
   ```

2. **Check current version**
   ```bash
   kubectl get deployment frappe-operator-controller-manager -n frappe-operator-system \
     -o jsonpath='{.spec.template.spec.containers[0].image}'
   ```

3. **Review release notes**
   - Check [releases](https://github.com/vyogotech/frappe-operator/releases) for breaking changes
   - Review API deprecations

4. **Test in staging**
   - Always test upgrades in a non-production environment first

### Upgrade Methods

#### Method 1: Helm Upgrade (Recommended)

```bash
# Update Helm repository
helm repo update vyogotech

# View available versions
helm search repo vyogotech/frappe-operator --versions

# Dry-run to see changes
helm upgrade frappe-operator vyogotech/frappe-operator \
  --namespace frappe-operator-system \
  --version X.Y.Z \
  --dry-run

# Perform upgrade
helm upgrade frappe-operator vyogotech/frappe-operator \
  --namespace frappe-operator-system \
  --version X.Y.Z \
  --wait
```

#### Method 2: Kustomize

```bash
# Update kustomization.yaml with new image tag
cd config/manager
kustomize edit set image controller=ghcr.io/vyogotech/frappe-operator:vX.Y.Z

# Apply changes
kubectl apply -k config/default
```

#### Method 3: Direct YAML

```bash
# Download new install manifest
curl -LO https://github.com/vyogotech/frappe-operator/releases/download/vX.Y.Z/install.yaml

# Apply with replacement
kubectl apply -f install.yaml --server-side --force-conflicts
```

### Post-Upgrade Verification

1. **Check operator health**
   ```bash
   kubectl get pods -n frappe-operator-system
   kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager
   ```

2. **Verify CRD updates**
   ```bash
   kubectl get crd frappebenches.vyogo.tech -o jsonpath='{.spec.versions[*].name}'
   kubectl get crd frappesites.vyogo.tech -o jsonpath='{.spec.versions[*].name}'
   ```

3. **Check resource reconciliation**
   ```bash
   kubectl get frappebenches -A
   kubectl get frappesites -A
   ```

## Version-Specific Guides

### Upgrading to v2.5.0

**New Features:**
- Job TTL cleanup (3600s default)
- Prometheus metrics
- Validation webhooks
- Resource builders

**Migration Steps:**
1. No breaking changes
2. Existing jobs will have TTL applied on next reconciliation
3. Enable metrics endpoint for monitoring

### Upgrading to v2.4.0

**New Features:**
- OpenShift Route support
- Enhanced security contexts

**Migration Steps:**
1. Review security context changes
2. OpenShift Routes auto-created if Route API detected

### Upgrading from v1.x to v2.x

**Breaking Changes:**
- API version changed from `v1alpha1` to `v1alpha1` (same, but schema changes)
- `benchName` field renamed to `benchRef` (object reference)
- Database configuration restructured

**Migration Steps:**

1. **Backup existing resources**
   ```bash
   kubectl get frappebenches -A -o yaml > benches-v1.yaml
   kubectl get frappesites -A -o yaml > sites-v1.yaml
   ```

2. **Convert resources**
   ```yaml
   # Old v1.x FrappeSite
   spec:
     benchName: my-bench
     dbPassword: secret123
   
   # New v2.x FrappeSite
   spec:
     benchRef:
       name: my-bench
       namespace: frappe
     dbConfig:
       mode: shared
       mariadbRef:
         name: frappe-mariadb
   ```

3. **Apply conversion tool** (if available)
   ```bash
   go run hack/convert-v1-to-v2.go < sites-v1.yaml > sites-v2.yaml
   ```

## Rollback Procedures

### Quick Rollback

```bash
# Helm rollback
helm rollback frappe-operator -n frappe-operator-system

# Or specify revision
helm rollback frappe-operator 1 -n frappe-operator-system
```

### Manual Rollback

1. **Identify previous version**
   ```bash
   helm history frappe-operator -n frappe-operator-system
   ```

2. **Apply previous manifests**
   ```bash
   kubectl apply -f install-vX.Y.Z.yaml
   ```

3. **Restore CRDs if needed**
   ```bash
   kubectl apply -f crds-vX.Y.Z.yaml
   ```

## Troubleshooting

### Common Issues

#### CRD Conflicts

**Symptom:** `metadata.resourceVersion: Invalid value`

**Solution:**
```bash
kubectl apply -f install.yaml --server-side --force-conflicts
```

#### Webhook Errors

**Symptom:** `failed calling webhook`

**Solution:**
```bash
# Temporarily disable webhooks
kubectl delete validatingwebhookconfiguration frappe-operator-validating-webhook-configuration

# Upgrade operator
helm upgrade frappe-operator vyogotech/frappe-operator ...

# Webhooks will be recreated
```

#### Stuck Resources

**Symptom:** Resources stuck in terminating state

**Solution:**
```bash
# Remove finalizers
kubectl patch frappesite mysite -p '{"metadata":{"finalizers":[]}}' --type=merge
```

### Getting Help

- [GitHub Issues](https://github.com/vyogotech/frappe-operator/issues)
- [Discussions](https://github.com/vyogotech/frappe-operator/discussions)
- Check operator logs: `kubectl logs -n frappe-operator-system -l control-plane=controller-manager`
