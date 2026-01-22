# Frappe Operator: Release & Installation Guide

This document provides a comprehensive guide to installing the Frappe Operator and deploying Frappe Benches/Sites. It includes the latest working images and detailed steps for both Kubernetes and OpenShift environments.

### Latest Working Images (as of v0.0.5)
- **Frappe Operator**: `ghcr.io/rmallam/frappe-operator:v0.0.5`
- **Frappe/ERPNext**: `ghcr.io/rmallam/frappe_docker:sha-c39d75f`

> [!IMPORTANT]
> The **Frappe Operator v0.0.5** image is built for `linux/amd64`. If you are running on a different architecture (e.g., ARM64), ensure your nodes support AMD64 emulation or request an architecture-specific build.

---

## 1. Prerequisites

- **Kubernetes Cluster**: v1.23+ or **OpenShift** (v4.10+).
- **Helm 3.x**: Required for installing the operator and its dependencies.
- **Storage**: A dynamic ReadWriteMany (RWX) storage class is required for production Frappe sites (e.g., CephFS, OCS).

---

## 2. Installation Steps

### Step 1: Install Dependencies
The Frappe Operator requires `cert-manager` and the `mariadb-operator`.

```bash
# 1. Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.2/cert-manager.yaml

# 2. Install MariaDB Operator
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo update
helm install mariadb-operator mariadb-operator/mariadb-operator \
  -n mariadb-operator-system --create-namespace --wait
```

### Step 2: Install Frappe Operator
Install the latest version (`v0.0.3`) using Helm.

```bash
helm install frappe-operator ./helm/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace \
  --set operator.image.repository=ghcr.io/rmallam/frappe-operator \
  --set operator.image.tag=v0.0.3 \
  --set mariadb-operator.enabled=false \
  --set cert-manager.enabled=false
```

---

## 3. Deploying your first Bench and Site

### Step 3: Create a FrappeBench
This resource defines the Frappe framework version and the base image to use.

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
  namespace: my-frappe-app
spec:
  frappeVersion: "15.0.0"
  imageConfig:
    repository: ghcr.io/rmallam/frappe_docker
    tag: sha-c39d75f
    pullPolicy: Always
  storageClassName: ocs-external-storagecluster-cephfs # Use your RWX storage class
  storageSize: 10Gi
```

### Step 4: Create a FrappeSite
This resource triggers the creation of the database and initializes the Frappe site on the PV.

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: my-site
  namespace: my-frappe-app
spec:
  siteName: my-site.example.com
  benchRef:
    name: production-bench
  dbConfig:
    mode: shared # Uses the MariaDB instance managed by the operator
```

---

## 4. How it Works (Operator Mechanics) - Release Notes

- **v0.0.5**:
    - **OpenShift Deletion Fix**: Fixed `KeyError: 'getpwuid()'` in the site deletion script.
- **v0.0.4**:
    - **OpenShift Initialization Compatibility**: Fixed `KeyError: 'getpwuid()'` in site and bench initialization by mocking `USER` environment variables for arbitrary UIDs.
    - **PVC Permission Fixes**: Improved robustness of `chmod`/`chgrp` logic during site initialization to handle restricted mount points on OpenShift.
- **v0.0.3**:
    - **Automatic Deployment Updates**: Changing the image tag in `FrappeBench` now triggers a rollout for all associated deployments (Gunicorn, Nginix, etc.).
    - **Self-Healing Site Configuration**: Sites automatically ensure `site_config.json` contains correct DB credentials and `logs` directory.
    - **RBAC fixes**: Added missing `leases` permissions for leader election.
    - **Architecture**: AMD64 cross-compiled build.

---

## 5. Troubleshooting

- **502 Bad Gateway**: Usually caused by missing database credentials in `site_config.json`. The v0.0.3 operator fixes this automatically during reconciliation.
- **ErrImagePull**: Verify the image tag exists in `ghcr.io` and that your nodes have internet access to pull from the registry.
- **Pending Pods**: Check for resource pressure (CPU/Memory). Scale down existing pods if the cluster is out of capacity.
