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
#   --config FILE          - Configuration file (default: test-config.conf)

# Get script directory for absolute paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse arguments
TEST_TYPE="all"
CONFIG_FILE="${SCRIPT_DIR}/test-config.conf"
CLUSTER_NAME=""
NAMESPACE=""
KEEP_CLUSTER="${KEEP_CLUSTER:-0}"
SKIP_BUILD=0
SKIP_CLUSTER_CREATE=0

# Check if first argument is a test type (not an option)
if [[ $# -gt 0 && ! "$1" =~ ^-- ]]; then
    TEST_TYPE="$1"
    shift
fi

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
        --config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Load configuration file
if [ ! -f "$CONFIG_FILE" ]; then
    echo "ERROR: Configuration file not found: $CONFIG_FILE"
    exit 1
fi
source "$CONFIG_FILE"

# Set defaults if not overridden by arguments
[ -z "$CLUSTER_NAME" ] && CLUSTER_NAME="$DEFAULT_CLUSTER_NAME"
[ -z "$NAMESPACE" ] && NAMESPACE="$DEFAULT_NAMESPACE"

echo "========================================================"
echo "Frappe Operator - Autoscaling E2E Test (Helm)"
echo "  Test Type: $TEST_TYPE"
echo "  Cluster: $CLUSTER_NAME"
echo "  Namespace: $NAMESPACE"
echo "  Config: $CONFIG_FILE"
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
    if [ "$KEEP_CLUSTER" = "1" ]; then
        print_warning "Keeping cluster alive (KEEP_CLUSTER=1)"
        print_status "To delete manually: kind delete cluster --name $CLUSTER_NAME"
    else
        print_status "Cleaning up Kind cluster..."
        kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
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

    kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG_FILE" --wait "$CLUSTER_CREATE_TIMEOUT"

print_status "Waiting for nodes to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout="$NODE_READY_TIMEOUT"
else
    print_status "Step 1: Using existing cluster $CLUSTER_NAME"
fi

# Step 2: Update Helm dependencies
print_status "Step 2: Updating Helm chart dependencies..."

# Add required repos
print_status "Adding Helm repositories..."
helm repo add mariadb-operator "$MARIADB_OPERATOR_REPO" 2>/dev/null || true
helm repo add kedacore "$KEDA_CORE_REPO" 2>/dev/null || true
helm repo update

# Update dependencies
print_status "Updating Helm chart dependencies..."
cd "${SCRIPT_DIR}/helm/frappe-operator" || {
    print_error "Failed to change to helm/frappe-operator directory"
    exit 1
}

helm dependency update || {
    print_error "Helm dependency update failed"
    exit 1
}

helm dependency build || {
    print_error "Helm dependency build failed"
    exit 1
}

cd "$SCRIPT_DIR"

# Generate test manifests from configuration
print_status "Generating test manifests from configuration..."
"${SCRIPT_DIR}/generate-test-manifests.sh" --config "$CONFIG_FILE"

# Step 3: Build and load operator image
if [ "$SKIP_BUILD" -eq 0 ]; then
    print_status "Step 3: Building operator image..."
    cd "$SCRIPT_DIR"
    make build

    # Check for Docker or Podman
    if command -v docker &> /dev/null; then
        CONTAINER_CMD="docker"
    elif command -v podman &> /dev/null; then
        CONTAINER_CMD="podman"
        print_warning "Using podman instead of docker"
    fi

    print_status "Building container image with ${CONTAINER_CMD}..."
    $CONTAINER_CMD build -t "${OPERATOR_IMAGE_REPOSITORY}:${OPERATOR_IMAGE_TAG}" -f Dockerfile .

    print_status "Loading image into Kind cluster..."
    if [ "$CONTAINER_CMD" = "podman" ]; then
        # Podman needs to save and load via tar archive
        podman save "${OPERATOR_IMAGE_REPOSITORY}:${OPERATOR_IMAGE_TAG}" -o "$TEMP_IMAGE_ARCHIVE"
        kind load image-archive "$TEMP_IMAGE_ARCHIVE" --name "$CLUSTER_NAME"
        rm -f "$TEMP_IMAGE_ARCHIVE"
    else
        kind load docker-image "${OPERATOR_IMAGE_REPOSITORY}:${OPERATOR_IMAGE_TAG}" --name "$CLUSTER_NAME"
    fi
else
    print_status "Step 3: Skipping operator image build (using pre-built image)"
    # Image is already in Docker (e.g., loaded from CI artifact), load into Kind
    print_status "Loading pre-built image into Kind cluster..."
    kind load docker-image "${OPERATOR_IMAGE_REPOSITORY}:${OPERATOR_IMAGE_TAG}" --name "$CLUSTER_NAME"
fi

# Step 4: Install operator via Helm (includes KEDA)
print_status "Step 4: Installing frappe-operator with Helm (includes KEDA)..."

# Note: We disable mariadb creation for this test as it's not needed for autoscaling
# Also disable runAsNonRoot for Kind compatibility (Frappe image uses named user)
helm upgrade --install frappe-operator "${SCRIPT_DIR}/helm/frappe-operator" \
    --create-namespace \
    --namespace frappe-operator-system \
    --set operator.image.repository="$OPERATOR_IMAGE_REPOSITORY" \
    --set operator.image.tag="$OPERATOR_IMAGE_TAG" \
    --set operator.image.pullPolicy=Never \
    --set operator.securityContext.runAsNonRoot=false \
    --set keda.enabled=true \
    --set keda.crds.install=true \
    --set mariadb-operator.enabled=false \
    --set mariadb.enabled=false \
    --wait \
    --timeout "$HELM_TIMEOUT"

print_status "Checking operator deployment..."
kubectl get pods -n frappe-operator-system

print_status "Checking KEDA deployment..."
kubectl get pods -n keda 2>/dev/null || kubectl get pods -n frappe-operator-system -l app.kubernetes.io/name=keda

# Step 5: Wait for operator to be ready
print_status "Step 5: Waiting for operator to be ready..."
kubectl wait --for=condition=available deployment/frappe-operator-controller-manager \
    -n frappe-operator-system --timeout="$OPERATOR_TIMEOUT"

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
    kubectl apply -f "${SCRIPT_DIR}/examples/test-keda-bench.yaml"

    print_status "Waiting for bench to initialize..."
    for i in $(seq 1 "$BENCH_READY_TIMEOUT"); do
        PHASE=$(kubectl get frappebench "$KEDA_BENCH_NAME" -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        echo "  [$i/$BENCH_READY_TIMEOUT] Phase: $PHASE"
        
        if [ "$PHASE" = "Ready" ]; then
            print_status "✓ Bench is Ready!"
            break
        fi
        
        if [ $i -eq "$BENCH_READY_TIMEOUT" ]; then
            print_error "Timeout waiting for Ready phase"
            kubectl describe frappebench "$KEDA_BENCH_NAME" -n ${NAMESPACE}
            kubectl get pods -A
            exit 1
        fi
        sleep "$BENCH_READY_SLEEP"
    done

    print_status "Verifying KEDA ScaledObject was created..."
    sleep 5
    SCALED_OBJECT=$(kubectl get scaledobject -A 2>/dev/null | grep -c "$NGINX_COMPONENT" || echo "0")
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
    kubectl apply -f "${SCRIPT_DIR}/examples/test-hpa-bench.yaml"

    print_status "Waiting for HPA bench to initialize..."
    for i in $(seq 1 "$BENCH_READY_TIMEOUT"); do
        PHASE=$(kubectl get frappebench "$HPA_BENCH_NAME" -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        echo "  [$i/$BENCH_READY_TIMEOUT] Phase: $PHASE"
        
        if [ "$PHASE" = "Ready" ]; then
            print_status "✓ Bench is Ready!"
            break
        fi
        
        if [ $i -eq "$BENCH_READY_TIMEOUT" ]; then
            print_error "Timeout waiting for Ready phase"
            exit 1
        fi
        sleep "$BENCH_READY_SLEEP"
    done

    print_status "Verifying HPA was created..."
    sleep 5
    HPA_COUNT=$(kubectl get hpa -A 2>/dev/null | grep -c "$NGINX_COMPONENT" || echo "0")
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
    if ! kubectl get frappebench "$KEDA_BENCH_NAME" -n ${NAMESPACE} &>/dev/null; then
        print_status "Creating initial KEDA bench for provider switch test..."
        test_keda_autoscaling
    fi

    print_status "Checking initial KEDA ScaledObject..."
    INITIAL_SCALED=$(kubectl get scaledobject -A 2>/dev/null | grep -c "$KEDA_BENCH_NAME" || echo "0")
    print_status "Initial ScaledObjects: $INITIAL_SCALED"

    print_status "Switching $KEDA_BENCH_NAME from KEDA to HPA..."
    kubectl patch frappebench "$KEDA_BENCH_NAME" -n ${NAMESPACE} --type=merge -p "
spec:
  componentAutoscaling:
    $NGINX_COMPONENT:
      provider: hpa
      hpa:
        metric: cpu
        targetUtilization: $DEFAULT_HPA_TARGET_UTILIZATION
"

    print_status "Waiting for provider switch to complete..."
    sleep "$PROVIDER_SWITCH_WAIT"

    print_status "Verifying old KEDA ScaledObject was cleaned up..."
    for i in $(seq 1 "$PROVIDER_SWITCH_TIMEOUT"); do
        SCALED_AFTER=$(kubectl get scaledobject -A 2>/dev/null | grep -c "$KEDA_BENCH_NAME" || echo "0")
        echo "  [$i/$PROVIDER_SWITCH_TIMEOUT] ScaledObjects remaining: $SCALED_AFTER"
        
        if [ "$SCALED_AFTER" -eq 0 ]; then
            print_status "✓ KEDA ScaledObject cleaned up successfully"
            break
        fi
        
        if [ $i -eq "$PROVIDER_SWITCH_TIMEOUT" ]; then
            print_error "KEDA ScaledObject was not cleaned up"
            kubectl get scaledobject -A
            exit 1
        fi
        sleep "$PROVIDER_SWITCH_SLEEP"
    done

    print_status "Verifying new HPA was created..."
    HPA_AFTER=$(kubectl get hpa -A 2>/dev/null | grep -c "$KEDA_BENCH_NAME" || echo "0")
    if [ "$HPA_AFTER" -ge 1 ]; then
        print_status "✓ HPA created after switch"
        kubectl get hpa -A | grep "$KEDA_BENCH_NAME"
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
