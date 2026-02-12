#!/bin/bash
set -eo pipefail

# Unified E2E Test Suite for Frappe Operator
# Usage: ./scripts/e2e-suite.sh --platform [kind|openshift] --scenario [basic|external|scaling|apps|advanced-config]

PLATFORM="kind"
SCENARIO="basic"
NAMESPACE="e2e-test"
OPERATOR_NAMESPACE="frappe-operator-system"
TIMEOUT=600 # 10 minutes

while [[ $# -gt 0 ]]; do
  case $1 in
    --platform) PLATFORM="$2"; shift 2 ;;
    --scenario) SCENARIO="$2"; shift 2 ;;
    *) echo "Unknown option $1"; exit 1 ;;
  esac
done

echo "========================================="
echo "Frappe Operator E2E: $PLATFORM / $SCENARIO"
echo "========================================="

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${GREEN}[INFO]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 1. Setup Platform Mocks
if [ "$PLATFORM" == "openshift" ]; then
    log "Mocking OpenShift environment (installing Route CRD)..."
    kubectl apply -f https://raw.githubusercontent.com/openshift/router/main/deploy/route_crd.yaml
    sleep 5
fi

# 2. (Removed independent MariaDB/KEDA installation - now handled by Frappe Operator chart)

# 3. Setup Scenario Namespace
log "Creating scenario namespace: $NAMESPACE"
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace mariadb --dry-run=client -o yaml | kubectl apply -f -

# 4. Setup External Mocks (for external scenario)
if [ "$SCENARIO" == "external" ]; then
    log "Deploying mock external MariaDB and Redis..."
    
    # Deploy a simple Redis
    kubectl create deployment redis-external --image=redis:alpine -n $NAMESPACE
    kubectl expose deployment redis-external --port=6379 -n $NAMESPACE
    kubectl create secret generic external-redis-creds --from-literal=redis-password="" --dry-run=client -o yaml | kubectl apply -n $NAMESPACE -f -

    # Deploy a simple MariaDB
    kubectl create deployment mariadb-external --image=mariadb:10.6 --port=3306 -n $NAMESPACE
    kubectl set env deployment/mariadb-external MARIADB_ROOT_PASSWORD=frappe -n $NAMESPACE
    kubectl expose deployment mariadb-external --port=3306 -n $NAMESPACE
    kubectl create secret generic external-mariadb-creds --from-literal=root-password=frappe --dry-run=client -o yaml | kubectl apply -n $NAMESPACE -f -
    
    log "Waiting for external mocks to be ready..."
    kubectl rollout status deployment/redis-external -n $NAMESPACE --timeout=2m
    kubectl rollout status deployment/mariadb-external -n $NAMESPACE --timeout=2m
fi

# 5. Install Frappe Operator (Unified with MariaDB & KEDA)
log "Building Helm dependencies..."
helm dependency update ./helm/frappe-operator

log "Installing Frappe Operator (with MariaDB and KEDA dependencies)..."
# If OPERATOR_IMAGE is provided (e.g. from CI), split it into repo and tag for Helm
HELM_OPTS=("--set" "mariadb-operator.enabled=true" "--set" "keda.enabled=true" "--set" "operator.image.pullPolicy=IfNotPresent")
if [ -n "$OPERATOR_IMAGE" ]; then
    IFS=':' read -ra ADDR <<< "$OPERATOR_IMAGE"
    HELM_OPTS+=("--set" "operator.image.repository=${ADDR[0]}")
    if [ -n "${ADDR[1]}" ]; then
        HELM_OPTS+=("--set" "operator.image.tag=${ADDR[1]}")
    fi
fi

helm upgrade --install frappe-operator ./helm/frappe-operator \
  --namespace $OPERATOR_NAMESPACE \
  --create-namespace \
  "${HELM_OPTS[@]}"

log "Waiting for all operator components to be ready..."
for deploy in $(kubectl get deployment -n $OPERATOR_NAMESPACE -o name); do
    kubectl rollout status "$deploy" -n "$OPERATOR_NAMESPACE" --timeout=2m
done

log "Giving webhooks a moment to start listening..."
sleep 15

# 6. Apply scenario-independent MariaDB Instance
log "Applying MariaDB instance from deploy/mariadb.yaml..."
kubectl apply -f deploy/mariadb.yaml

# 7. Apply Scenario Manifest
log "Applying scenario: $SCENARIO..."
MANIFEST="test/scenarios/${SCENARIO}.yaml"
if [ ! -f "$MANIFEST" ]; then error "Scenario manifest $MANIFEST not found"; fi

# If BENCH_IMAGE is provided, inject it into the manifest placeholder
if [ -n "$BENCH_IMAGE" ]; then
    IFS=':' read -ra ADDR <<< "$BENCH_IMAGE"
    REPO="${ADDR[0]}"
    TAG="${ADDR[1]:-latest}"
    log "Injecting bench image: $REPO:$TAG"
    sed -e "s|repository: ghcr.io/vyogotech/frappe_base|repository: $REPO|g" \
        -e "s|tag: latest|tag: $TAG|g" \
        "$MANIFEST" | kubectl apply -n $NAMESPACE -f -
else
    kubectl apply -n $NAMESPACE -f "$MANIFEST"
fi

# 7. Verification Loop
log "Waiting for resources to reach Ready phase..."
ELAPSED=0
# Update verification logic to be scenario-agnostic but robust
while [ $ELAPSED -lt $TIMEOUT ]; do
    BENCH_PHASE=$(kubectl get frappebench -n $NAMESPACE -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Unknown")
    SITE_PHASE=$(kubectl get frappesite -n $NAMESPACE -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Unknown")
    
    echo "Current Status: Bench=$BENCH_PHASE, Site=$SITE_PHASE ($ELAPSED/$TIMEOUT)"
    
    if [ "$BENCH_PHASE" == "Ready" ] && [ "$SITE_PHASE" == "Ready" ]; then
        log "✅ SUCCESS: All resources are Ready!"
        break
    fi
    
    if [ "$BENCH_PHASE" == "Failed" ] || [ "$SITE_PHASE" == "Failed" ]; then
        error "FAILED: Resource reached Failed phase."
    fi
    
    sleep 20
    ELAPSED=$((ELAPSED + 20))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
    error "TIMEOUT: Resources did not reach Ready phase."
fi

# 8. Platform Specific Assertions
if [ "$PLATFORM" == "openshift" ]; then
    log "Verifying OpenShift Route creation..."
    if ! kubectl get route -n $NAMESPACE &>/dev/null; then
        error "FAILED: Route not created on OpenShift platform."
    fi
    log "✅ SUCCESS: Route created."
else
    log "Verifying Ingress creation..."
    # Many scenarios don't enable Ingress explicitly in the manifest, 
    # but the operator might create it by default.
    if kubectl get ingress -n $NAMESPACE &>/dev/null; then
         log "✅ SUCCESS: Ingress created."
    else
         log "⚠️  Note: No Ingress found (Standard behavior for some scenarios)."
    fi
fi

log "========================================="
log "Scenario $SCENARIO on $PLATFORM PASSED!"
log "========================================="
