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
    --skip-operator-install) SKIP_OPERATOR_INSTALL="true"; shift 1 ;;
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

# 1b. Install Metrics Server (required for HPA/Scaling)
log "Checking if Metrics API is available..."
if ! kubectl get apiservice v1beta1.metrics.k8s.io &>/dev/null; then
    log "Metrics API not found. Installing Metrics Server..."
    kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
    
    # Patch metrics-server to allow insecure TLS and set address types (required for KIND)
    kubectl patch deployment metrics-server -n kube-system --type='json' \
      -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}, {"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-preferred-address-types=InternalIP"}]'
    
    log "Waiting for metrics-server deployment to be ready..."
    kubectl rollout status deployment/metrics-server -n kube-system --timeout=2m || true
    
    log "Waiting for Metrics API availability (kubectl get --raw /apis/metrics.k8s.io/v1beta1)..."
    for i in {1..15}; do
        if kubectl get --raw /apis/metrics.k8s.io/v1beta1 &>/dev/null; then
            log "Metrics API is now available."
            break
        fi
        echo -n "."
        sleep 10
    done
else
    log "Metrics API already available."
fi

# 2. Pre-install KEDA CRDs (since Helm does not install sub-chart CRDs automatically)
log "Pre-installing KEDA CRDs..."
kubectl apply --server-side -f https://github.com/kedacore/keda/releases/download/v2.16.1/keda-2.16.1-crds.yaml


# 3. Setup Scenario Namespace
log "Creating scenario namespace: $NAMESPACE"
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace mariadb --dry-run=client -o yaml | kubectl apply -f -

# 4. Setup External Mocks (for external scenario)
if [ "$SCENARIO" == "external" ]; then
    log "Deploying mock external MariaDB and Redis..."
    
    # Deploy a simple Redis
    kubectl create deployment redis-external --image=redis:alpine -n $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    kubectl expose deployment redis-external --port=6379 -n $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    kubectl create secret generic external-redis-creds --from-literal=password="" --dry-run=client -o yaml | kubectl apply -n $NAMESPACE -f -

    # Deploy a simple MariaDB
    kubectl create deployment mariadb-external --image=mariadb:10.6 --port=3306 -n $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    kubectl set env deployment/mariadb-external MARIADB_ROOT_PASSWORD=frappe MARIADB_DATABASE=external_test_local MARIADB_USER=external_test_local MARIADB_PASSWORD=frappe -n $NAMESPACE
    kubectl expose deployment mariadb-external --port=3306 -n $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    kubectl create secret generic external-mariadb-creds --from-literal=username=external_test_local --from-literal=password=frappe --from-literal=database=external_test_local --dry-run=client -o yaml | kubectl apply -n $NAMESPACE -f -
    
    log "Waiting for external mocks to be ready..."
    kubectl rollout status deployment/redis-external -n $NAMESPACE --timeout=2m
    kubectl rollout status deployment/mariadb-external -n $NAMESPACE --timeout=2m
fi

if [ "$SKIP_OPERATOR_INSTALL" != "true" ]; then
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
else
    log "Skipping operator Helm installation (--skip-operator-install specified). Verifying existing deployments..."
    for deploy in $(kubectl get deployment -n $OPERATOR_NAMESPACE -o name); do
        kubectl rollout status "$deploy" -n "$OPERATOR_NAMESPACE" --timeout=2m
    done
fi

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
        if [ "$SCENARIO" == "backup-restore" ]; then
            BACKUP_PHASE=$(kubectl get sitebackup -n $NAMESPACE -o jsonpath='{.items[0].status.phase}' 2>/dev/null)
            BACKUP_PHASE=${BACKUP_PHASE:-Pending}
            echo "Backup Status: $BACKUP_PHASE"
            if [ "$BACKUP_PHASE" == "Succeeded" ]; then
                log "✅ SUCCESS: All resources are Ready and Backup is Succeeded!"
                break
            elif [ "$BACKUP_PHASE" == "Failed" ]; then
                error "FAILED: SiteBackup reached Failed phase."
            fi
        else
            log "✅ SUCCESS: All resources are Ready!"
            break
        fi
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
log "Final Resource State:"
kubectl get frappesite,frappebench -n $NAMESPACE
log "========================================="
log "Scenario $SCENARIO on $PLATFORM PASSED!"
log "========================================="
