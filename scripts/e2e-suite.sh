#!/bin/bash
set -eo pipefail

# Unified E2E Test Suite for Frappe Operator
# Usage: ./scripts/e2e-suite.sh --platform [kind|openshift-sim] --scenario [basic|external|scaling|apps|advanced-config]
#
# openshift-sim is a Kind cluster with OpenShift's API surface layered on top -
# it exercises the OpenShift code paths but enforces nothing. Only a real
# cluster covers SCC admission and fsGroup-honouring storage.

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
#
# This is a Kind cluster wearing enough of OpenShift's API surface to exercise
# the operator's OpenShift code paths - it is NOT OpenShift. There is no SCC
# admission and no CSI driver honouring fsGroup here, so anything that depends
# on enforcement rather than on what the operator emits still needs a real
# cluster. What we can reproduce faithfully are the objects the operator reads:
# the Route CRD, the cluster-scoped ingress config carrying the router's
# wildcard domain, and the SCC allocation annotations OpenShift stamps on every
# namespace. Omitting the latter two is why domain detection and the missing
# fsGroup both shipped green.
if [ "$PLATFORM" == "openshift-sim" ]; then
    log "Simulating OpenShift API surface (Route CRD + cluster ingress config + SCC annotations)..."
    kubectl apply -f https://raw.githubusercontent.com/openshift/router/main/deploy/route_crd.yaml

    # config.openshift.io/v1 Ingress - the operator reads .spec.domain from the
    # object named "cluster" to derive site hostnames.
    kubectl apply -f - <<'EOF'
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: ingresses.config.openshift.io
spec:
  group: config.openshift.io
  scope: Cluster
  names:
    plural: ingresses
    singular: ingress
    kind: Ingress
    listKind: IngressList
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                domain:
                  type: string
            status:
              type: object
              x-kubernetes-preserve-unknown-fields: true
      subresources:
        status: {}
EOF
    kubectl wait --for condition=established --timeout=60s crd/ingresses.config.openshift.io

    kubectl apply -f - <<'EOF'
apiVersion: config.openshift.io/v1
kind: Ingress
metadata:
  name: cluster
spec:
  domain: apps.e2e.example.com
EOF
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

# 2. (Removed independent MariaDB/KEDA installation - now handled by Frappe Operator chart)

# 3. Setup Scenario Namespace
log "Creating scenario namespace: $NAMESPACE"
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace mariadb --dry-run=client -o yaml | kubectl apply -f -

# OpenShift derives a pod's fsGroup from these annotations. Without them the
# operator has no range to draw from and emits no fsGroup at all.
if [ "$PLATFORM" == "openshift-sim" ]; then
    log "Annotating $NAMESPACE with OpenShift SCC allocation ranges"
    kubectl annotate namespace $NAMESPACE --overwrite \
        openshift.io/sa.scc.supplemental-groups=1000670000/10000 \
        openshift.io/sa.scc.uid-range=1000670000/10000 \
        openshift.io/sa.scc.mcs=s0:c26,c15
fi

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
if [ "$PLATFORM" == "openshift-sim" ]; then
    log "Verifying OpenShift Route creation..."
    if ! kubectl get route -n $NAMESPACE &>/dev/null; then
        error "FAILED: Route not created on OpenShift platform."
    fi
    log "✅ SUCCESS: Route created."

    # A Route on a host that resolves nowhere is indistinguishable from a
    # working one unless the host is actually checked. Only sites whose domain
    # the operator resolved itself are checked: a site that sets spec.domain is
    # reported as "explicit" and is meant to keep that host verbatim, so
    # requiring the cluster domain there would be asserting a bug.
    log "Verifying Route host matches the domain the operator resolved..."
    ROUTE_HOSTS=$(kubectl get route -n $NAMESPACE -o jsonpath='{.items[*].spec.host}')
    log "Route hosts: ${ROUTE_HOSTS:-<none>}"
    if [ -z "$ROUTE_HOSTS" ]; then
        error "FAILED: Route has no host set."
    fi

    for site in $(kubectl get frappesite -n $NAMESPACE -o jsonpath='{.items[*].metadata.name}'); do
        SOURCE=$(kubectl get frappesite "$site" -n $NAMESPACE -o jsonpath='{.status.domainSource}')
        RESOLVED=$(kubectl get frappesite "$site" -n $NAMESPACE -o jsonpath='{.status.resolvedDomain}')
        ROUTE_HOST=$(kubectl get route -n $NAMESPACE \
            -o jsonpath="{.items[?(@.metadata.name=='${site}-route')].spec.host}")
        [ -z "$ROUTE_HOST" ] && continue

        # Whatever the source, the Route must serve exactly the name the site was
        # created under - Frappe matches the Host header to a directory in sites/.
        if [ "$ROUTE_HOST" != "$RESOLVED" ]; then
            error "FAILED: site '$site' resolved to '$RESOLVED' but its Route serves '$ROUTE_HOST'; Frappe will answer 404."
        fi

        case "$SOURCE" in
            auto-detected|auto-corrected)
                case "$RESOLVED" in
                    *.apps.e2e.example.com) ;;
                    *) error "FAILED: site '$site' resolved to '$RESOLVED' via '$SOURCE' but did not use the cluster domain from config.openshift.io/v1 Ingress (apps.e2e.example.com)." ;;
                esac
                ;;
            explicit)
                log "site '$site': domainSource=explicit, host kept verbatim ($RESOLVED)"
                ;;
            sitename-default)
                # A cluster ingress object is seeded above, so detection had
                # something to find. Falling back to the raw siteName means it
                # could not read it - which is what a missing
                # config.openshift.io RBAC rule looks like from the outside.
                error "FAILED: site '$site' fell back to sitename-default ('$RESOLVED') even though a config.openshift.io/v1 Ingress is present; cluster domain detection is not working."
                ;;
            *)
                error "FAILED: site '$site' has unexpected domainSource '$SOURCE'."
                ;;
        esac
    done
    log "✅ SUCCESS: Route hosts match the resolved domains."
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
