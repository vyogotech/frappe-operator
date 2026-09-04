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

echo "=== 3b. Preloading Frappe app images into Kind ==="
# Preload both the base image (initial site) and the custom-apps image (failed upgrade
# target) so pods resolve them locally under pullPolicy: IfNotPresent instead of
# re-pulling ~2GB from ghcr on every reconcile. Without this, a slow/rate-limited CI
# pull leaves the upgrade init job Pending (never reaching Failed) and the fallback
# pods stuck ContainerCreating, so the waits below timed out non-deterministically.
for FRAPPE_IMG in ghcr.io/vyogotech/frappe_base:latest ghcr.io/vyogotech/frappe_custom_apps:latest; do
    if [ "$CONTAINER_TOOL" == "podman" ]; then
        podman pull "$FRAPPE_IMG"
        TAR="$(echo "$FRAPPE_IMG" | tr '/:' '__').tar"
        podman save -o "$TAR" "$FRAPPE_IMG"
        kind load image-archive "$TAR" --name $CLUSTER_NAME
    else
        docker pull "$FRAPPE_IMG"
        kind load docker-image "$FRAPPE_IMG" --name $CLUSTER_NAME
    fi
done

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
# Give the operator a few seconds to create the site-init job
sleep 15
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
# The upgrade re-runs the site-init job. With the fallback guard in place the job reaches
# a terminal state — Complete when the bogus app is safely skipped (the expected path), or
# Failed if the operator genuinely can't converge — rather than hanging. Poll for either
# instead of blindly waiting the full timeout for a Failed condition that no longer occurs
# (that wasted ~8 min every run once the site-init fallback fix landed).
echo "Waiting for the upgrade job to reach a terminal state..."
for i in $(seq 1 144); do   # up to 12 min
    ctypes=$(kubectl get job/upgrade-site-init -n e2e-upgrade-test-fail -o jsonpath='{.status.conditions[*].type}' 2>/dev/null)
    if echo "$ctypes" | grep -qE "Complete|Failed"; then
        echo "  upgrade job reached terminal state after $((i*5))s: [$ctypes]"
        break
    fi
    sleep 5
done

echo "=== 8. Port forwarding to verify CSS AFTER FAILED upgrade ==="
kubectl port-forward svc/upgrade-bench-nginx 8080:8080 -n e2e-upgrade-test-fail &
PF_PID=$!
sleep 10

# The fallback trap keeps the site on the last working version when an upgrade
# fails, but the roll-out/roll-back transition briefly returns 500. Poll until
# the site recovers before asserting — a WORKING fallback restores service within
# a few minutes; a genuinely BROKEN one never does and the test still fails below.
echo "Waiting for the fallback to restore the site (post-failed-upgrade)..."
for i in $(seq 1 60); do
    STATUS=$(curl -H "Host: upgrade.test.local" -o /dev/null -s -w "%{http_code}" http://localhost:8080/login || echo 000)
    if [ "$STATUS" == "200" ]; then
        echo "Site recovered (HTTP 200) after $((i*5))s — fallback trap held."
        break
    fi
    echo "  site not serving yet (HTTP $STATUS), waiting… ($((i*5))s)"
    sleep 5
done

# If the fallback never restored service, dump diagnostics so a genuine fallback
# regression is debuggable from CI logs rather than an opaque login failure.
if [ "$STATUS" != "200" ]; then
    echo "::group::DIAGNOSTICS: fallback did not restore the site (HTTP $STATUS)"
    kubectl get pods -n e2e-upgrade-test-fail -o wide || true
    kubectl get events -n e2e-upgrade-test-fail --sort-by=.lastTimestamp | tail -30 || true
    echo "--- upgrade-site-init job + pod (should reach Failed on the bogus app) ---"
    kubectl describe job/upgrade-site-init -n e2e-upgrade-test-fail | tail -40 || true
    kubectl describe pod -l job-name=upgrade-site-init -n e2e-upgrade-test-fail | sed -n '/Events:/,$p' || true
    kubectl logs -l job-name=upgrade-site-init -n e2e-upgrade-test-fail --tail=60 || true
    echo "--- gunicorn/nginx pods (fallback serving path) ---"
    kubectl describe pod -l 'app in (upgrade-bench-gunicorn,upgrade-bench-nginx)' -n e2e-upgrade-test-fail | sed -n '/Events:/,$p' || true
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
