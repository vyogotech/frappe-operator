# Frappe Operator Installation Guide (Helm)

This guide describes how to install the Frappe Operator using Helm, which is the recommended approach for production and streamlined deployments.

## Prerequisites

- Kubernetes cluster (v1.23+) or OpenShift (v4.10+)
- Helm 3.x installed
- `kubectl` configured to your cluster

## Installation Steps

### 1. (Optional) Dependencies

If you haven't already installed the required dependencies (cert-manager and MariaDB operator), you can do so manually or let the Helm chart handle it.

To install dependencies manually:

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.2/cert-manager.yaml

# Install MariaDB Operator
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo update
helm install mariadb-operator mariadb-operator/mariadb-operator -n mariadb-operator-system --create-namespace --wait
```

### 2. Install Frappe Operator

From the root of the `frappe-operator` repository:

```bash
helm install frappe-operator ./helm/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace \
  --set operator.image.repository=ghcr.io/rmallam/frappe-operator \
  --set operator.image.tag=latest \
  --set operator.image.pullPolicy=IfNotPresent \
  --set mariadb-operator.enabled=false \
  --set keda.enabled=false \
  --set cert-manager.enabled=false
```

#### Key Parameters:
- `mariadb-operator.enabled`: Set to `false` if already installed.
- `cert-manager.enabled`: Set to `false` if already installed.
- `keda.enabled`: Set to `true` if you want automatic worker pod autoscaling.

### 3. Verify Installation

```bash
kubectl get pods -n frappe-operator-system
```

You should see the `frappe-operator-controller-manager` pod in a `Running` state.

## Configuration Options

Refer to the [values.yaml](file:///Users/rakeshkumarmallam/Rakesh-work/frappe-operator/helm/frappe-operator/values.yaml) for a full list of configuration options, including:
- Resource limits/requests
- Image pull secrets
- Default domain suffixes
- External database configurations
