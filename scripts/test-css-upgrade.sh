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

CLUSTER_NAME="test-upgrade-cluster"

echo "=== 1. Starting Kind cluster ==="
kind create cluster --name $CLUSTER_NAME

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

echo "=== 3b. Preloading Frappe app image into Kind ==="
# The scenarios use the Frappe app image with pullPolicy: IfNotPresent. Preload it
# into the Kind node once (a single retryable pull we control) so the site-init job
# and the gunicorn/nginx pods never re-pull ~2GB from ghcr on every reconcile. That
# registry round-trip on each upgrade is what made this job flaky on CI runners:
# a slow/rate-limited pull left the rolled gunicorn stuck ContainerCreating (site 500)
# and the upgrade init job Pending, so the waits below timed out non-deterministically.
FRAPPE_IMG="ghcr.io/vyogotech/frappe_base:latest"
if [ "$CONTAINER_TOOL" == "podman" ]; then
    podman pull "$FRAPPE_IMG"
    podman save -o frappe_base.tar "$FRAPPE_IMG"
    kind load image-archive frappe_base.tar --name $CLUSTER_NAME
else
    docker pull "$FRAPPE_IMG"
    kind load docker-image "$FRAPPE_IMG" --name $CLUSTER_NAME
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
kubectl create namespace e2e-upgrade-test --dry-run=client -o yaml | kubectl apply -f -
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
kubectl apply -f test/scenarios/apps-upgrade.yaml -n e2e-upgrade-test

echo "Waiting for initial site to be Ready..."
kubectl wait --for=condition=Ready pod/frappe-mariadb-0 -n mariadb --timeout=5m || echo "mariadb wait failed"
kubectl wait --for=condition=Complete job/upgrade-bench-init -n e2e-upgrade-test --timeout=5m || echo "bench init wait failed"
# Give the operator a few seconds to create the site-init job
sleep 15
kubectl wait --for=condition=Complete job/upgrade-site-init -n e2e-upgrade-test --timeout=5m || echo "site init wait failed"
kubectl wait --for=condition=Ready frappesite/upgrade-site -n e2e-upgrade-test --timeout=5m || echo "site wait failed"

echo "=== 6. Port forwarding to verify CSS ==="
kubectl port-forward svc/upgrade-bench-nginx 8080:8080 -n e2e-upgrade-test &
PF_PID=$!
sleep 10

echo "Fetching page..."
# Adding host header because site matches on Host or IP if it's default. We'll use curl -H "Host: upgrade.test.local"
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
ADMIN_PASSWORD=$(kubectl get secret upgrade-site-admin -n e2e-upgrade-test -o jsonpath='{.data.password}' | base64 -d)
if ! ADMIN_PASSWORD=$ADMIN_PASSWORD npm --prefix tests/e2e run test; then
    echo "❌ Playwright UI Admin login failed!"
    kill $PF_PID
    exit 1
fi
echo "✅ Playwright UI Admin login successful!"

kill $PF_PID

echo "=== 7. Upgrading the site ==="
kubectl apply -f test/scenarios/apps-upgrade-v2.yaml -n e2e-upgrade-test
echo "Waiting for upgrade..."
sleep 15
# Adding HRMS runs get-app (git clone) + install-app + `bench migrate`, which is
# legitimately slow and variable on CI runners, so allow generous time. The
# frappesite only reports Ready once the migration completes.
kubectl wait --for=condition=Complete job/upgrade-site-init -n e2e-upgrade-test --timeout=12m || echo "site init upgrade wait failed"
kubectl wait --for=condition=Ready frappesite/upgrade-site -n e2e-upgrade-test --timeout=12m || echo "site upgrade wait failed"

echo "=== 8. Port forwarding to verify CSS after upgrade ==="
kubectl port-forward svc/upgrade-bench-nginx 8080:8080 -n e2e-upgrade-test &
PF_PID=$!
sleep 10

# Gate the CSS/login checks on the site actually serving: while `bench migrate`
# runs, Frappe returns 500, so poll until it recovers rather than asserting
# against a mid-migration site (up to ~5 min).
echo "Waiting for the upgraded site to serve requests (post-migration)..."
for i in $(seq 1 60); do
    STATUS=$(curl -H "Host: upgrade.test.local" -o /dev/null -s -w "%{http_code}" http://localhost:8080/login || echo 000)
    if [ "$STATUS" == "200" ]; then
        echo "Site is serving (HTTP 200) after $((i*5))s."
        break
    fi
    echo "  site not ready yet (HTTP $STATUS), waiting… ($((i*5))s)"
    sleep 5
done

# If the site still isn't serving, dump diagnostics so a real failure is debuggable
# from the CI logs (image pull vs. migrate hang vs. crashloop) rather than opaque.
if [ "$STATUS" != "200" ]; then
    echo "::group::DIAGNOSTICS: site not serving after upgrade (HTTP $STATUS)"
    kubectl get pods -n e2e-upgrade-test -o wide || true
    kubectl get events -n e2e-upgrade-test --sort-by=.lastTimestamp | tail -30 || true
    echo "--- upgrade-site-init job + pod ---"
    kubectl describe job/upgrade-site-init -n e2e-upgrade-test | tail -40 || true
    kubectl describe pod -l job-name=upgrade-site-init -n e2e-upgrade-test | sed -n '/Events:/,$p' || true
    kubectl logs -l job-name=upgrade-site-init -n e2e-upgrade-test --tail=60 || true
    echo "--- gunicorn/nginx pods (fallback serving path) ---"
    kubectl describe pod -l 'app in (upgrade-bench-gunicorn,upgrade-bench-nginx)' -n e2e-upgrade-test | sed -n '/Events:/,$p' || true
    echo "::endgroup::"
fi

HTML=$(curl -s -H "Host: upgrade.test.local" http://localhost:8080)
CSS_PATH=$(echo "$HTML" | grep -oE '/assets/[^"]+\.css' | head -1)

if [ -z "$CSS_PATH" ]; then
    echo "❌ No CSS found in HTML after upgrade!"
else
    echo "Found CSS at: $CSS_PATH after upgrade"
    HTTP_STATUS=$(curl -H "Host: upgrade.test.local" -o /dev/null -s -w "%{http_code}\n" http://localhost:8080$CSS_PATH)
    if [ "$HTTP_STATUS" == "200" ]; then
        echo "✅ CSS loaded successfully after upgrade!"
    else
        echo "❌ CSS failed to load after upgrade! Status: $HTTP_STATUS"
        echo "Attempting flushall on redis..."
        kubectl exec upgrade-bench-redis-cache-0 -n e2e-upgrade-test -- redis-cli flushall
        echo "Re-checking CSS..."
        HTTP_STATUS=$(curl -H "Host: upgrade.test.local" -o /dev/null -s -w "%{http_code}\n" http://localhost:8080$CSS_PATH)
        if [ "$HTTP_STATUS" == "200" ]; then
             echo "✅ CSS loaded successfully after flushall!"
        fi
    fi
fi

echo "Testing Admin Login UI with Playwright after upgrade..."
ADMIN_PASSWORD=$(kubectl get secret upgrade-site-admin -n e2e-upgrade-test -o jsonpath='{.data.password}' | base64 -d)
if ! ADMIN_PASSWORD=$ADMIN_PASSWORD npm --prefix tests/e2e run test; then
    echo "❌ Playwright UI Admin login failed after upgrade!"
    kill $PF_PID
    exit 1
fi
echo "✅ Playwright UI Admin login successful after upgrade!"

kill $PF_PID
echo "=== 9. Cleanup ==="
kind delete cluster --name $CLUSTER_NAME
