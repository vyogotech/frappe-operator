#!/bin/bash
set -e

# Test script for Frappe Operator on Minishift/OpenShift
# This script assumes 'oc' is already logged in to a cluster.

NAMESPACE="frappe-test"
OPERATOR_NAMESPACE="frappe-operator-system"

echo "========================================="
echo "Frappe Operator - Minishift Testing"
echo "========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# 1. Check for oc
if ! command -v oc &> /dev/null; then
    print_error "oc CLI is not installed."
    exit 1
fi

# 2. Setup Project
print_status "Step 1: Setting up projects..."
oc new-project $OPERATOR_NAMESPACE 2>/dev/null || oc project $OPERATOR_NAMESPACE
oc new-project $NAMESPACE 2>/dev/null || oc project $NAMESPACE

# 3. Install Dependencies
print_status "Step 2: Installing MariaDB Operator..."
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo update
helm upgrade --install mariadb-operator mariadb-operator/mariadb-operator \
  --namespace $OPERATOR_NAMESPACE \
  --set crds.enabled=true \
  --wait --timeout 5m

print_status "Step 3: Installing KEDA..."
helm repo add keda https://kedacore.github.io/charts
helm repo update
helm upgrade --install keda keda/keda \
  --namespace $OPERATOR_NAMESPACE \
  --set crds.install=true \
  --wait --timeout 5m

# 4. Install Frappe Operator
print_status "Step 4: Installing Frappe Operator..."
# Use the local chart
helm upgrade --install frappe-operator ./helm/frappe-operator \
  --namespace $OPERATOR_NAMESPACE \
  --set mariadb.enabled=true \
  --wait --timeout 5m

# 5. Verify Platform Detection
print_status "Step 5: Verifying OpenShift platform detection..."
# Wait for pod to start
oc wait --for=condition=Ready pod -l control-plane=controller-manager -n $OPERATOR_NAMESPACE --timeout=60s

# Check logs
for i in {1..10}; do
    if oc logs -l control-plane=controller-manager -n $OPERATOR_NAMESPACE | grep -q "OpenShift platform detected"; then
        print_status "✓ Success: OpenShift platform detected correctly."
        break
    else
        if [ $i -eq 10 ]; then
            print_error "Failed to detect OpenShift in operator logs."
            oc logs -l control-plane=controller-manager -n $OPERATOR_NAMESPACE | tail -20
            exit 1
        fi
        print_status "Waiting for platform detection log... ($i/10)"
        sleep 5
    fi
done

# 6. Deploy Test Bench and Site
print_status "Step 6: Deploying test FrappeBench and FrappeSite..."
cat <<EOF | oc apply -f -
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: minishift-bench
  namespace: ${NAMESPACE}
spec:
  frappeVersion: "15"
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: minishift-site
  namespace: ${NAMESPACE}
spec:
  benchRef:
    name: minishift-bench
  siteName: minishift.apps.cluster.local
  routeConfig:
    enabled: true
EOF

# 7. Verify Route Creation
print_status "Step 7: Verifying OpenShift Route creation..."
for i in {1..20}; do
    if oc get route minishift-site -n $NAMESPACE &>/dev/null; then
        print_status "✓ Success: OpenShift Route 'minishift-site' found."
        oc get route minishift-site -n $NAMESPACE
        break
    else
        if [ $i -eq 20 ]; then
            print_error "OpenShift Route was not created within timeout."
            oc describe frappesite minishift-site -n $NAMESPACE
            exit 1
        fi
        print_status "Waiting for Route creation... ($i/20)"
        sleep 5
    fi
done

print_status "========================================="
print_status "Test completed successfully!"
print_status "========================================="
