# Frappe Operator Installation Guide (Helm)

This guide describes how to install the **Frappe Operator** using Helm, the recommended approach for production environments.

---

## Prerequisites

- **Kubernetes Cluster** (v1.22+) or **OpenShift** (v4.10+)
- **Helm 3.x** installed
- `kubectl` or `oc` configured to your cluster

---

## Installation Steps

### 1. Add the Helm Repository

Add the official Vyogo Technologies Helm repository:

```bash
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm repo update
```

---

### 2. (Optional) Database Dependencies

If you plan to use operator-managed databases:

- **PostgreSQL via StackGres (Recommended)**:
  ```bash
  helm repo add stackgres https://stackgres.io/downloads/stackgres-k8s/stackgres/helm
  helm repo update
  helm install stackgres-operator stackgres/stackgres-operator -n stackgres --create-namespace
  ```
- **MariaDB Operator**:
  ```bash
  helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
  helm repo update
  helm install mariadb-operator mariadb-operator/mariadb-operator -n mariadb-operator-system --create-namespace --wait
  ```

---

### 3. Install Frappe Operator

Install the operator chart:

```bash
helm upgrade --install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace \
  --set operator.image.repository=ghcr.io/vyogotech/frappe-operator \
  --set operator.image.tag=v5.2.0 \
  --set operator.image.pullPolicy=IfNotPresent \
  --set operatorConfig.enforceHTTPS=true \
  --wait
```

#### Key Chart Values:
- `operator.image.repository`: Container image repository (`ghcr.io/vyogotech/frappe-operator`).
- `operator.image.tag`: Operator release tag (`v5.2.0`).
- `operatorConfig.enforceHTTPS`: Enforce HTTPS and automatic TLS redirects across all sites (`true` recommended).
- `keda.enabled`: Set to `true` if you wish to bundle KEDA for worker queue autoscaling (or leave `false` to use standard Kubernetes HPA).
- `mariadb-operator.enabled`: Set to `false` when managing databases independently.

---

### 4. Verify Installation

```bash
kubectl get pods -n frappe-operator-system
```

You should see the `frappe-operator-controller-manager` pod in a `Running` state.

---

## Configuration Options

Refer to the official [values.yaml](../helm/frappe-operator/values.yaml) for a full list of configuration options, including:
- Resource limits and requests
- Platform overrides (Kubernetes vs. OpenShift)
- Global ImageConfig defaults (custom container registries)
- Autoscaling provider defaults (`hpa` or `keda`)
- Webhook certificate configurations
