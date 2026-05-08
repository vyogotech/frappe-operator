#!/bin/bash
set -e
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
if command -v docker &> /dev/null; then
    export CONTAINER_TOOL="docker"
elif command -v podman &> /dev/null; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
    export CONTAINER_TOOL="podman"
else
    echo "Neither docker nor podman found"
    exit 1
fi

CLUSTER_NAME="test-failed-upgrade-cluster"

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
kubectl create namespace e2e-upgrade-test-fail --dry-run=client -o yaml | kubectl apply -f -
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
kubectl apply -f test/scenarios/apps-upgrade.yaml -n e2e-upgrade-test-fail

echo "Waiting for initial site to be Ready..."
kubectl wait --for=condition=Ready pod/frappe-mariadb-0 -n mariadb --timeout=5m || echo "mariadb wait failed"
kubectl wait --for=condition=Complete job/upgrade-bench-init -n e2e-upgrade-test-fail --timeout=5m || echo "bench init wait failed"
kubectl wait --for=condition=Complete job/upgrade-site-init -n e2e-upgrade-test-fail --timeout=5m || echo "site init wait failed"
kubectl wait --for=condition=Ready frappesite/upgrade-site -n e2e-upgrade-test-fail --timeout=5m || echo "site wait failed"

echo "=== 6. Port forwarding to verify CSS ==="
kubectl port-forward svc/upgrade-bench-nginx 8080:8080 -n e2e-upgrade-test-fail &
PF_PID=$!
sleep 10

echo "Fetching page..."
HTML=$(curl -s -H "Host: upgrade.test.local" http://localhost:8080)
CSS_PATH=$(echo "$HTML" | grep -oE '/assets/[^"]+\.css' | head -1)

if [ -z "$CSS_PATH" ]; then
    echo "❌ No CSS found in HTML! HTML snippet:"
    echo "$HTML" | head -20
else
    echo "Found CSS at: $CSS_PATH"
    HTTP_STATUS=$(curl -H "Host: upgrade.test.local" -o /dev/null -s -w "%{http_code}\n" http://localhost:8080$CSS_PATH)
    if [ "$HTTP_STATUS" == "200" ]; then
        echo "✅ CSS loaded successfully with status 200."
    else
        echo "❌ CSS failed to load! Status: $HTTP_STATUS"
    fi
fi

echo "Testing Admin Login UI with Playwright..."
ADMIN_PASSWORD=$(kubectl get secret upgrade-site-admin -n e2e-upgrade-test-fail -o jsonpath='{.data.password}' | base64 -d)
if ! ADMIN_PASSWORD=$ADMIN_PASSWORD npm --prefix tests/e2e run test; then
    echo "❌ Playwright UI Admin login failed!"
    kill $PF_PID
    exit 1
fi
echo "✅ Playwright UI Admin login successful!"

kill $PF_PID

echo "=== 7. Upgrading the site with FAILED config ==="
kubectl apply -f test/scenarios/image-upgrade-fail.yaml -n e2e-upgrade-test-fail
echo "Waiting for upgrade..."
sleep 15
kubectl wait --for=condition=Failed job/upgrade-site-init -n e2e-upgrade-test-fail --timeout=2m || echo "site init upgrade wait failed or completed unexpectedly"

echo "=== 8. Port forwarding to verify CSS AFTER FAILED upgrade ==="
kubectl port-forward svc/upgrade-bench-nginx 8080:8080 -n e2e-upgrade-test-fail &
PF_PID=$!
sleep 10

HTML=$(curl -s -H "Host: upgrade.test.local" http://localhost:8080)
CSS_PATH=$(echo "$HTML" | grep -oE '/assets/[^"]+\.css' | head -1)

if [ -z "$CSS_PATH" ]; then
    echo "❌ No CSS found in HTML after upgrade!"
else
    echo "Found CSS at: $CSS_PATH after upgrade"
    HTTP_STATUS=$(curl -H "Host: upgrade.test.local" -o /dev/null -s -w "%{http_code}\n" http://localhost:8080$CSS_PATH)
    if [ "$HTTP_STATUS" == "200" ]; then
        echo "✅ CSS loaded successfully after FAILED upgrade! (Fallback trap worked!)"
    else
        echo "❌ CSS failed to load after FAILED upgrade! Status: $HTTP_STATUS (Fallback trap failed)"
    fi
fi

echo "Testing Admin Login UI with Playwright after FAILED upgrade..."
ADMIN_PASSWORD=$(kubectl get secret upgrade-site-admin -n e2e-upgrade-test-fail -o jsonpath='{.data.password}' | base64 -d)
if ! ADMIN_PASSWORD=$ADMIN_PASSWORD npm --prefix tests/e2e run test; then
    echo "❌ Playwright UI Admin login failed after FAILED upgrade!"
    kill $PF_PID
    exit 1
fi
echo "✅ Playwright UI Admin login successful after FAILED upgrade!"

kill $PF_PID
echo "=== 9. Cleanup ==="
kind delete cluster --name $CLUSTER_NAME
