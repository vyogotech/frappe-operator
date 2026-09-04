#!/bin/bash
set -e
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
export KIND_EXPERIMENTAL_PROVIDER=podman

CLUSTER_NAME="test-upgrade-cluster"

echo "=== 1. Starting Kind cluster ==="
kind create cluster --name $CLUSTER_NAME

echo "=== 2. Building operator image ==="
make docker-build IMG=vyogotech/frappe-operator:test-upgrade

echo "=== 3. Loading image into Kind ==="
podman save -o operator.tar localhost/vyogotech/frappe-operator:test-upgrade
kind load image-archive operator.tar --name $CLUSTER_NAME

echo "=== 4. Installing Operator via Helm ==="
helm dependency update ./helm/frappe-operator
helm upgrade --install frappe-operator ./helm/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace \
  --set mariadb-operator.enabled=true \
  --set keda.enabled=true \
  --set operator.image.pullPolicy=Never \
  --set operator.image.repository=vyogotech/frappe-operator \
  --set operator.image.tag=test-upgrade

echo "Waiting for Operator to be ready..."
kubectl rollout status deployment/frappe-operator-controller-manager -n frappe-operator-system --timeout=2m

echo "=== 5. Running the Upgrade Test Script ==="
./scripts/test-apps-upgrade.sh

echo "=== 6. Cleanup ==="
kind delete cluster --name $CLUSTER_NAME
echo "✅ All tests passed and cluster cleaned up successfully!"
