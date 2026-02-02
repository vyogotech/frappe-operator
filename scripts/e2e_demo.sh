#!/bin/bash
set -e

# Configuration
CRC_IP=$(crc ip)
DOMAIN_SUFFIX="apps-crc.testing"
SITE_NAME="demo.vyogo.$DOMAIN_SUFFIX"
EXPECTED_URL="http://$SITE_NAME"
LOG_FILE="e2e_demo_$(date +%Y%m%d_%H%M%S).log"

# Setup Logging
exec > >(tee -a "${LOG_FILE}") 2>&1
echo "Starting E2E Demo Test at $(date)"
echo "Logs will be saved to ${LOG_FILE}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${GREEN}[INFO] $1${NC}"; }
error() { echo -e "${RED}[ERROR] $1${NC}"; exit 1; }

check_status() {
    local namespace=$1
    local kind=$2
    local name=$3
    local condition=$4
    local timeout=600

    log "Waiting for $kind/$name in namespace $namespace to be $condition..."
    if ! kubectl wait --for=condition=$condition $kind/$name -n $namespace --timeout=${timeout}s; then
        error "Timed out waiting for $kind/$name to be $condition"
    fi
}

cleanup() {
    log "Starting cleanup..."
    oc delete project frappe --ignore-not-found
    oc delete project mariadb --ignore-not-found
    helm uninstall frappe-operator -n frappe-operator-system --ignore-not-found
    helm uninstall mariadb-operator -n mariadb-operator-system --ignore-not-found
    oc delete project frappe-operator-system --ignore-not-found
    oc delete project mariadb-operator-system --ignore-not-found
    log "Cleanup complete."
}

# Trap error to cleanup on failure
trap 'if [ $? -ne 0 ]; then error "Script failed. Check logs."; fi' EXIT

# 1. Setup Helm
log "Cleaning up old Helm cache..."
helm repo remove frappe-operator || true
rm -rf $(helm env HELM_CACHE_HOME)/repository/frappe-operator-index.yaml || true
rm -rf $(helm env HELM_CACHE_HOME)/repository/frappe-operator-charts.txt || true

log "Setting up Helm repositories..."
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator --force-update
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo --force-update
helm repo update

# Diagnostic: Check what versions Helm sees
log "Helm Repo Search Results:"
helm search repo frappe-operator/frappe-operator --versions | head -n 5

# 2. Install Operators
log "Installing MariaDB Operator..."
helm upgrade --install mariadb-operator mariadb-operator/mariadb-operator \
  --namespace mariadb-operator-system \
  --set crds.enabled=true \
  --create-namespace 

log "Installing Frappe Operator..."
helm upgrade --install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --set keda.enabled=false \
  --create-namespace

# Verify Operators
check_status frappe-operator-system deployment/frappe-operator-controller-manager available
check_status mariadb-operator-system deployment/mariadb-operator-controller-manager available

# 3. Deploy Database
log "Deploying MariaDB..."
oc apply -f - <<EOF
apiVersion: project.openshift.io/v1
kind: Project
metadata:
  name: mariadb
---
apiVersion: v1
kind: Secret
metadata:
  name: mariadb-root-password
  namespace: mariadb
type: Opaque
stringData:
  password: frappe
---
apiVersion: k8s.mariadb.com/v1alpha1
kind: MariaDB
metadata:
  name: frappe-mariadb
  namespace: mariadb
spec:
  rootPasswordSecretKeyRef:
    name: mariadb-root-password
    key: password
  image: mariadb:10.11
  storage:
    size: 2Gi
  replicas: 1
EOF

# Wait for MariaDB
log "Waiting for MariaDB to be ready..."
# MariaDB operator might take a moment to create the StatefulSet
sleep 10
check_status mariadb mariadb/frappe-mariadb Ready

# 4. Deploy Frappe Bench
log "Deploying Frappe Bench..."
oc apply -f - <<EOF
apiVersion: project.openshift.io/v1
kind: Project
metadata:
  name: frappe
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: bench-test
  namespace: frappe
spec:
  frappeVersion: "15.0.0"
  imageConfig:
    repository: ghcr.io/rmallam/frappe_docker
    tag: sha-7bc7484
    pullPolicy: Always
  storageClassName: crc-csi-hostpath-provisioner
  storageSize: 2Gi
  apps:
    - name: frappe
      source: image
    - name: erpnext
      source: image
  componentReplicas:
    gunicorn: 1
    nginx: 1
    socketio: 1
    scheduler: 0
    workers:
      default: 1
      short: 0
      long: 0
EOF

log "Waiting for Bench to be Ready..."
# Bench takes time to initialize jobs
sleep 30
check_status frappe frappebench/bench-test Ready

# 5. Deploy Frappe Site
log "Deploying Frappe Site..."
oc apply -f - <<EOF
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: vyogotechdemo
  namespace: frappe
spec:
  siteName: $SITE_NAME
  benchRef:
    name: bench-test
    namespace: frappe
  dbConfig:
    provider: mariadb
    mode: shared
    mariadbRef:
      name: frappe-mariadb
      namespace: mariadb
  ingress:
    enabled: true
    className: openshift-default
EOF

log "Waiting for Site to be Ready..."
check_status frappe frappesite/vyogotechdemo Ready

# 6. Test Accessibility
log "Testing Site Accessibility ($EXPECTED_URL)..."
if curl --output /dev/null --silent --head --fail "$EXPECTED_URL"; then
    log "Site is accessible!"
else
    # Retry once after 10s
    sleep 10
    if curl --output /dev/null --silent --head --fail "$EXPECTED_URL"; then
        log "Site is accessible (after retry)!"
    else
        error "Site is NOT accessible at $EXPECTED_URL"
    fi
fi

# 7. Cleanup (Only if successful, as per request)
log "Tests Passed. Cleaning up..."
cleanup
exit 0
