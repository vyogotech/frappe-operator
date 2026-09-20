# Upgrade Guide

This guide covers upgrading the Frappe Operator to **v5.2.0**.

---

## Table of Contents

- [General Upgrade Process](#general-upgrade-process)
- [Upgrading to v5.2.0](#upgrading-to-v520)
- [Rollback Procedures](#rollback-procedures)
- [Troubleshooting](#troubleshooting)

---

## General Upgrade Process

### Pre-Upgrade Checklist

1. **Backup your Custom Resources**
   ```bash
   # Backup all FrappeSite and FrappeBench definitions
   kubectl get frappesites -A -o yaml > frappesites-backup.yaml
   kubectl get frappebenches -A -o yaml > frappebenches-backup.yaml
   ```

2. **Check current operator version**
   ```bash
   kubectl get deployment frappe-operator-controller-manager -n frappe-operator-system \
     -o jsonpath='{.spec.template.spec.containers[0].image}'
   ```

3. **Review Changelog**
   - Review [CHANGELOG.md](https://github.com/vyogotech/frappe-operator/blob/main/CHANGELOG.md) for breaking changes and release highlights.

---

### Upgrade Methods

#### Method 1: Helm Upgrade (Recommended)

```bash
# Update Helm repository
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm repo update

# Dry-run to inspect changes
helm upgrade frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --version 5.2.0 \
  --dry-run

# Perform upgrade
helm upgrade frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --version 5.2.0 \
  --wait
```

#### Method 2: Direct Manifest Upgrade

```bash
kubectl apply -f https://github.com/vyogotech/frappe-operator/releases/latest/download/install.yaml --server-side --force-conflicts
```

---

### Post-Upgrade Verification

1. **Verify controller manager health**
   ```bash
   kubectl get pods -n frappe-operator-system
   kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager --tail=50
   ```

2. **Verify CRD registration**
   ```bash
   kubectl get crd | grep vyogo.tech
   ```

3. **Verify site reconciliation**
   ```bash
   kubectl get frappesites -A
   kubectl get frappebenches -A
   ```

---

## Upgrading to v5.2.0

### Key Changes & Migration Notes

1. **Polymorphic Database Architecture**:
   - The operator now supports PostgreSQL alongside MariaDB.
   - For dedicated PostgreSQL sites, the default engine is `stackgres` (`SGCluster`). If you are running existing Percona clusters, the operator automatically detects them and preserves management via Percona.
   - Once provisioned, `dbConfig.postgresEngine` is immutable.

2. **HTTPS-Only Ingress Enforcement**:
   - Sites and custom domains now mandate TLS. Ingress resources automatically configure `spec.tls` (defaulting to `<site-name>-tls`) and set `ssl-redirect: "true"`.
   - On OpenShift, routes are admitted with `tls.termination: edge` and `tls.insecureEdgeTerminationPolicy: Redirect`.
   - The validating webhook rejects any annotations attempting to disable SSL redirection (`ssl-redirect: "false"` or `"0"`).

3. **OpenShift `restricted-v2` SCC Compliance**:
   - Workloads strictly adhere to `restricted-v2` security constraints.
   - If your bench manifests specify custom security contexts, verify they do not hardcode `fsGroup: 0` or privileged root UIDs.

4. **Licensing**:
   - Releases v4.2.0+ are published under the **Elastic License 2.0 (ELv2)** — free for internal production and self-hosting, commercial license required for managed SaaS hosting. See [LICENSING.md](../LICENSING.md).

---

## Rollback Procedures

### Helm Rollback

```bash
# View release revision history
helm history frappe-operator -n frappe-operator-system

# Roll back to the previous revision
helm rollback frappe-operator -n frappe-operator-system
```

---

## Troubleshooting

### CRD Conflicts During Upgrade
**Symptom:** `metadata.resourceVersion: Invalid value`  
**Resolution:** Apply CRDs with server-side apply:
```bash
kubectl apply -f https://github.com/vyogotech/frappe-operator/releases/latest/download/install.yaml --server-side --force-conflicts
```

### Webhook Validation Blocking Update
**Symptom:** `failed calling webhook: ...`  
**Resolution:** Ensure your updated site manifest has valid `apps` specified and does not attempt to mutate immutable fields such as `dbConfig.postgresEngine`.
