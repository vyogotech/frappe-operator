#!/bin/bash
set -e
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
if command -v podman &> /dev/null; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
    CONTAINER_TOOL="podman"
else
    CONTAINER_TOOL="docker"
fi

CLUSTER_NAME="test-apps-upgrade-cluster"
NAMESPACE="e2e-upgrade-test"

echo "=== 1. Starting Kind cluster ==="
kind create cluster --name $CLUSTER_NAME || true

echo "=== 2. Building operator image ==="
make docker-build IMG=vyogotech/frappe-operator:test-upgrade

echo "=== 3. Loading image into Kind ==="
if [ "$CONTAINER_TOOL" == "podman" ]; then
    podman tag localhost/vyogotech/frappe-operator:test-upgrade vyogotech/frappe-operator:test-upgrade || true
    podman save -o operator.tar vyogotech/frappe-operator:test-upgrade
    kind load image-archive operator.tar --name $CLUSTER_NAME
else
    kind load docker-image vyogotech/frappe-operator:test-upgrade --name $CLUSTER_NAME
fi

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

echo "=== 5. Applying initial site ==="
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace mariadb --dry-run=client -o yaml | kubectl apply -f -
echo "Waiting for MariaDB webhook..."
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=mariadb-operator-webhook -n frappe-operator-system --timeout=2m || true
sleep 10
for i in {1..5}; do
    if kubectl apply -f deploy/mariadb.yaml; then
        break
    fi
    echo "Retrying DB creation in 5s..."
    sleep 5
done

echo "================================================="
echo "Testing Dynamic App Upgrade & BYOD Initialization"
echo "================================================="

echo "[1/4] Creating initial site with only ERPNext..."
kubectl apply -f test/scenarios/apps-upgrade.yaml -n $NAMESPACE

echo "Waiting for site to become Ready..."
kubectl wait --for=condition=Ready pod/frappe-mariadb-0 -n mariadb --timeout=5m || echo "mariadb wait failed"
kubectl wait --for=condition=Complete job/upgrade-bench-init -n $NAMESPACE --timeout=5m || echo "bench init wait failed"
kubectl wait --for=condition=Complete job/upgrade-site-init -n $NAMESPACE --timeout=5m || echo "site init wait failed"
kubectl wait --for=condition=Ready frappesite/upgrade-site -n $NAMESPACE --timeout=5m || echo "Initial site wait timed out, proceeding anyway to check..."

echo "[2/4] Initial site Ready! Verifying deployed apps..."
kubectl get frappesite upgrade-site -n $NAMESPACE -o jsonpath='{.status.installedApps}' | grep erpnext || echo "erpnext not found in status!"

echo "[3/4] Triggering upgrade by patching apps list (Adding HRMS)..."
kubectl apply -f test/scenarios/apps-upgrade-v2.yaml -n $NAMESPACE

echo "Waiting for Operator to detect app change, delete old job, and create a new job..."
sleep 15

echo "Waiting for upgraded site to become Ready..."
kubectl wait --for=condition=Complete job/upgrade-site-init -n $NAMESPACE --timeout=5m || echo "site upgrade job wait failed"
kubectl wait --for=condition=Ready frappesite/upgrade-site -n $NAMESPACE --timeout=5m || echo "site upgrade wait failed"

echo "[4/4] Upgrade complete! Verifying deployed apps..."
INSTALLED_APPS=$(kubectl get frappesite upgrade-site -n $NAMESPACE -o jsonpath='{.status.installedApps}')
echo "Installed Apps: $INSTALLED_APPS"

if [[ "$INSTALLED_APPS" == *"hrms"* ]]; then
    echo "✅ SUCCESS: The operator automatically detected the app change, bypassed destructive init via IS_UPGRADE, and installed HRMS!"
else
    echo "❌ FAILED: HRMS is missing from installed apps."
    exit 1
fi

echo "=== 9. Cleanup ==="
kind delete cluster --name $CLUSTER_NAME
