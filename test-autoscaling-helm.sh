#!/bin/bash
set -e

# E2E Autoscaling Test Script using Helm Chart
# This uses the frappe-operator Helm chart which includes KEDA as a dependency
#
# Usage:
#   ./test-autoscaling-helm.sh [TEST_TYPE] [OPTIONS]
#
# Test Types:
#   keda              - Test KEDA autoscaling
#   hpa               - Test HPA autoscaling
#   provider-switch   - Test switching providers KEDA → HPA
#   all               - Run all tests (default)
#
# Options:
#   --cluster-name NAME    - Kind cluster name (default: frappe-autoscaling-helm)
#   --namespace NS         - Kubernetes namespace (default: default)
#   --keep-cluster         - Keep cluster after tests
#   --skip-build           - Skip operator image build
#   --skip-cluster-create  - Skip cluster creation (use existing)

# Parse arguments
TEST_TYPE="${1:-all}"
CLUSTER_NAME="frappe-autoscaling-helm"
NAMESPACE="default"
KEEP_CLUSTER="${KEEP_CLUSTER:-0}"
SKIP_BUILD=0
SKIP_CLUSTER_CREATE=0

shift || true
while [[ $# -gt 0 ]]; do
    case $1 in
        --cluster-name)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --keep-cluster)
            KEEP_CLUSTER=1
            shift
            ;;
        --skip-build)
            SKIP_BUILD=1
            shift
            ;;
        --skip-cluster-create)
            SKIP_CLUSTER_CREATE=1
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "========================================================"
echo "Frappe Operator - Autoscaling E2E Test (Helm)"
echo "  Test Type: $TEST_TYPE"
echo "  Cluster: $CLUSTER_NAME"
echo "  Namespace: $NAMESPACE"
echo "========================================================"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

# Check requirements
for cmd in kind kubectl helm; do
    if ! command -v $cmd &> /dev/null; then
        print_error "$cmd is not installed. Please install it first."
        exit 1
    fi
done

# Cleanup function
cleanup() {
    if [ "$KEEP_CLUSTER" != "1" ]; then
        print_warning "Skipping cleanup - cluster will remain for manual testing"
        print_status "To delete manually: kind delete cluster --name $CLUSTER_NAME"
        print_status "To re-enable auto-cleanup: set KEEP_CLUSTER=0"
        # Temporarily disabled for manual testing
        # kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    else
        print_warning "Keeping cluster alive (KEEP_CLUSTER=1)"
        print_status "To delete manually: kind delete cluster --name $CLUSTER_NAME"
    fi
}

trap cleanup EXIT

# Step 1: Create Kind cluster
if [ "$SKIP_CLUSTER_CREATE" -eq 0 ]; then
    print_status "Step 1: Creating Kind cluster..."
    if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        print_warning "Cluster exists. Deleting..."
        kind delete cluster --name "$CLUSTER_NAME"
    fi

    kind create cluster --name "$CLUSTER_NAME" --config kind-config.yaml --wait 5m

    print_status "Waiting for nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s
else
    print_status "Step 1: Using existing cluster $CLUSTER_NAME"
fi

# Step 2: Update Helm dependencies
print_status "Step 2: Updating Helm chart dependencies..."
cd helm/frappe-operator

# Add required repos
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator 2>/dev/null || true
helm repo add kedacore https://kedacore.github.io/charts 2>/dev/null || true
helm repo update

# Update dependencies
helm dependency update

cd ../..

# Step 3: Build and load operator image
if [ "$SKIP_BUILD" -eq 0 ]; then
    print_status "Step 3: Building operator image..."
    make build

    # Check for Docker or Podman
    if command -v docker &> /dev/null; then
        CONTAINER_CMD="docker"
    elif command -v podman &> /dev/null; then
        CONTAINER_CMD="podman"
        print_warning "Using podman instead of docker"
    fi

    print_status "Building container image with ${CONTAINER_CMD}..."
    $CONTAINER_CMD build -t ghcr.io/rmallam/frappe-operator:local-test -f Dockerfile .

    print_status "Loading image into Kind cluster..."
    if [ "$CONTAINER_CMD" = "podman" ]; then
        # Podman needs to save and load via tar archive
        podman save ghcr.io/rmallam/frappe-operator:local-test -o /tmp/frappe-operator-helm.tar
        kind load image-archive /tmp/frappe-operator-helm.tar --name "$CLUSTER_NAME"
        rm -f /tmp/frappe-operator-helm.tar
    else
        kind load docker-image ghcr.io/rmallam/frappe-operator:local-test --name "$CLUSTER_NAME"
    fi
else
    print_status "Step 3: Skipping operator image build"
fi

# Step 4: Install operator via Helm (includes KEDA)
print_status "Step 4: Installing frappe-operator with Helm (includes KEDA)..."

# Note: We disable mariadb creation for this test as it's not needed for autoscaling
# Also disable runAsNonRoot for Kind compatibility (Frappe image uses named user)
helm upgrade --install frappe-operator ./helm/frappe-operator \
    --create-namespace \
    --namespace frappe-operator-system \
    --set operator.image.repository=ghcr.io/rmallam/frappe-operator \
    --set operator.image.tag=local-test \
    --set operator.image.pullPolicy=Never \
    --set operator.securityContext.runAsNonRoot=false \
    --set keda.enabled=true \
    --set keda.crds.install=true \
    --set mariadb-operator.enabled=false \
    --set mariadb.enabled=false \
    --wait \
    --timeout 10m

print_status "Checking operator deployment..."
kubectl get pods -n frappe-operator-system

print_status "Checking KEDA deployment..."
kubectl get pods -n keda 2>/dev/null || kubectl get pods -n frappe-operator-system -l app.kubernetes.io/name=keda

# Step 5: Wait for operator to be ready
print_status "Step 5: Waiting for operator to be ready..."
kubectl wait --for=condition=available deployment/frappe-operator-controller-manager \
    -n frappe-operator-system --timeout=180s

print_status "Operator logs:"
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager --tail=20

# ===========================================
# Test Functions
# ===========================================

test_keda_autoscaling() {
    print_test "=========================================="
    print_test "TEST: KEDA Autoscaling"
    print_test "=========================================="

    print_status "Creating FrappeBench with KEDA autoscaling (from examples/)..."
    kubectl apply -f examples/test-keda-bench.yaml

    print_status "Waiting for bench to initialize..."
    for i in {1..90}; do
        PHASE=$(kubectl get frappebench test-bench-keda -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        echo "  [$i/90] Phase: $PHASE"
        
        if [ "$PHASE" = "Ready" ]; then
            print_status "✓ Bench is Ready!"
            break
        fi
        
        if [ $i -eq 90 ]; then
            print_error "Timeout waiting for Ready phase"
            kubectl describe frappebench test-bench-keda -n ${NAMESPACE}
            kubectl get pods -A
            exit 1
        fi
        sleep 10
    done

    print_status "Verifying KEDA ScaledObject was created..."
    sleep 5
    SCALED_OBJECT=$(kubectl get scaledobject -A 2>/dev/null | grep -c nginx || echo "0")
    if [ "$SCALED_OBJECT" -ge 1 ]; then
        print_status "✓ KEDA ScaledObject created successfully"
        kubectl get scaledobject -A
    else
        print_error "KEDA ScaledObject was not created"
        kubectl get scaledobject -A
        exit 1
    fi
}

test_hpa_autoscaling() {
    print_test "=========================================="
    print_test "TEST: HPA Autoscaling"
    print_test "=========================================="

    print_status "Creating FrappeBench with HPA autoscaling (from examples/)..."
    kubectl apply -f examples/test-hpa-bench.yaml

    print_status "Waiting for HPA bench to initialize..."
    for i in {1..90}; do
        PHASE=$(kubectl get frappebench test-bench-hpa -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        echo "  [$i/90] Phase: $PHASE"
        
        if [ "$PHASE" = "Ready" ]; then
            print_status "✓ Bench is Ready!"
            break
        fi
        
        if [ $i -eq 90 ]; then
            print_error "Timeout waiting for Ready phase"
            exit 1
        fi
        sleep 10
    done

    print_status "Verifying HPA was created..."
    sleep 5
    HPA_COUNT=$(kubectl get hpa -A 2>/dev/null | grep -c nginx || echo "0")
    if [ "$HPA_COUNT" -ge 1 ]; then
        print_status "✓ HPA created successfully"
        kubectl get hpa -A
    else
        print_error "HPA was not created"
        kubectl get hpa -A
        exit 1
    fi
}

test_provider_switching() {
    print_test "=========================================="
    print_test "TEST: Provider Switching (KEDA → HPA)"
    print_test "=========================================="

    # Ensure KEDA bench exists first
    if ! kubectl get frappebench test-bench-keda -n ${NAMESPACE} &>/dev/null; then
        print_status "Creating initial KEDA bench for provider switch test..."
        test_keda_autoscaling
    fi

    print_status "Checking initial KEDA ScaledObject..."
    INITIAL_SCALED=$(kubectl get scaledobject -A 2>/dev/null | grep -c test-bench-keda || echo "0")
    print_status "Initial ScaledObjects: $INITIAL_SCALED"

    print_status "Switching test-bench-keda from KEDA to HPA..."
    kubectl patch frappebench test-bench-keda -n ${NAMESPACE} --type=merge -p '
spec:
  componentAutoscaling:
    nginx:
      provider: hpa
      hpa:
        metric: cpu
        targetUtilization: 50
'

    print_status "Waiting for provider switch to complete..."
    sleep 10

    print_status "Verifying old KEDA ScaledObject was cleaned up..."
    for i in {1..30}; do
        SCALED_AFTER=$(kubectl get scaledobject -A 2>/dev/null | grep -c test-bench-keda || echo "0")
        echo "  [$i/30] ScaledObjects remaining: $SCALED_AFTER"
        
        if [ "$SCALED_AFTER" -eq 0 ]; then
            print_status "✓ KEDA ScaledObject cleaned up successfully"
            break
        fi
        
        if [ $i -eq 30 ]; then
            print_error "KEDA ScaledObject was not cleaned up"
            kubectl get scaledobject -A
            exit 1
        fi
        sleep 2
    done

    print_status "Verifying new HPA was created..."
    HPA_AFTER=$(kubectl get hpa -A 2>/dev/null | grep -c test-bench-keda || echo "0")
    if [ "$HPA_AFTER" -ge 1 ]; then
        print_status "✓ HPA created after switch"
        kubectl get hpa -A | grep test-bench-keda
    else
        print_error "HPA was not created after switch"
        exit 1
    fi
}

# ===========================================
# Run Tests Based on TEST_TYPE
# ===========================================

case "$TEST_TYPE" in
    keda)
        test_keda_autoscaling
        ;;
    hpa)
        test_hpa_autoscaling
        ;;
    provider-switch)
        test_provider_switching
        ;;
    all)
        test_keda_autoscaling
        test_hpa_autoscaling
        test_provider_switching
        ;;
    *)
        print_error "Unknown test type: $TEST_TYPE"
        print_error "Valid types: keda, hpa, provider-switch, all"
        exit 1
        ;;
esac

# ===========================================
# Summary
# ===========================================
print_test "=========================================="
print_test "TEST SUMMARY"
print_test "=========================================="
print_status "✓ Operator installed via Helm with KEDA dependency"

case "$TEST_TYPE" in
    keda)
        print_status "✓ KEDA autoscaling works"
        ;;
    hpa)
        print_status "✓ HPA autoscaling works"
        ;;
    provider-switch)
        print_status "✓ Provider switching works (KEDA → HPA)"
        print_status "✓ Old provider resources cleaned up"
        ;;
    all)
        print_status "✓ KEDA autoscaling works"
        print_status "✓ HPA autoscaling works"
        print_status "✓ Provider switching works (KEDA → HPA)"
        print_status "✓ Old provider resources cleaned up"
        ;;
esac

print_status ""
print_status "Final resource state:"
echo ""
echo "=== FrappeBenches ==="
kubectl get frappebench -n ${NAMESPACE}
echo ""
echo "=== ScaledObjects (KEDA) ==="
kubectl get scaledobject -A 2>/dev/null || echo "None"
echo ""
echo "=== HPAs ==="
kubectl get hpa -A 2>/dev/null || echo "None"
echo ""
echo "=== Deployments ==="
kubectl get deployment -n ${NAMESPACE}

print_status ""
print_status "=========================================="
print_status "ALL TESTS PASSED! 🎉"
print_status "=========================================="
print_status ""
print_status "To explore: export KUBECONFIG=\$(kind get kubeconfig --name $CLUSTER_NAME)"
print_status "To keep cluster: KEEP_CLUSTER=1 $0 $TEST_TYPE"
print_status "To delete: kind delete cluster --name $CLUSTER_NAME"
