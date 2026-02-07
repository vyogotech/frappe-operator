#!/bin/bash
set -e

# E2E Autoscaling Test Script for Local Testing
# Replicates the CI autoscaling tests locally using Kind

CLUSTER_NAME="frappe-autoscaling-test"
NAMESPACE="default"
KEDA_VERSION="2.13.0"

echo "========================================================"
echo "Frappe Operator - Autoscaling E2E Test (Local)"
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

# Check for Docker or Podman
if command -v docker &> /dev/null; then
    CONTAINER_CMD="docker"
elif command -v podman &> /dev/null; then
    CONTAINER_CMD="podman"
    print_warning "Using podman instead of docker"
else
    print_error "Neither docker nor podman is installed. Please install one."
    exit 1
fi

# Cleanup function
cleanup() {
    if [ "$KEEP_CLUSTER" != "1" ]; then
        print_status "Cleaning up cluster..."
        kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    else
        print_warning "Keeping cluster alive (KEEP_CLUSTER=1)"
        print_status "To delete manually: kind delete cluster --name $CLUSTER_NAME"
    fi
}

trap cleanup EXIT

# Step 1: Create Kind cluster
print_status "Step 1: Creating Kind cluster with config..."
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    print_warning "Cluster exists. Deleting..."
    kind delete cluster --name "$CLUSTER_NAME"
fi

kind create cluster --name "$CLUSTER_NAME" --config kind-config.yaml --wait 5m

print_status "Waiting for nodes to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=120s

# Step 2: Install KEDA
print_status "Step 2: Installing KEDA ${KEDA_VERSION} via Helm..."
kubectl create namespace keda 2>/dev/null || true

# Add KEDA Helm repo if not already added
helm repo add kedacore https://kedacore.github.io/charts 2>/dev/null || true
helm repo update

# Install KEDA
print_status "Installing KEDA chart..."
helm upgrade --install keda kedacore/keda \
    --namespace keda \
    --version 2.13.1 \
    --wait \
    --timeout 5m

print_status "Waiting for KEDA operator to be ready..."
kubectl wait --for=condition=available deployment/keda-operator -n keda --timeout=120s 2>/dev/null || true
kubectl wait --for=condition=available deployment/keda-admission-webhooks -n keda --timeout=120s 2>/dev/null || true
kubectl wait --for=condition=available deployment/keda-operator-metrics-apiserver -n keda --timeout=120s 2>/dev/null || true

print_status "KEDA pods:"
kubectl get pods -n keda

# Step 3: Build and load operator image
print_status "Step 3: Building operator..."
make build

print_status "Building container image with ${CONTAINER_CMD}..."
$CONTAINER_CMD build -t frappe-operator:local-test -f Dockerfile .

print_status "Loading image into Kind..."
if [ "$CONTAINER_CMD" = "podman" ]; then
    # Podman adds localhost/ prefix, need to handle this for kind
    podman save frappe-operator:local-test -o /tmp/frappe-operator-local.tar
    kind load image-archive /tmp/frappe-operator-local.tar --name "$CLUSTER_NAME"
    rm -f /tmp/frappe-operator-local.tar
else
    kind load docker-image frappe-operator:local-test --name "$CLUSTER_NAME"
fi

# Step 4: Install operator
print_status "Step 4: Installing operator CRDs..."
make install

print_status "Waiting for CRDs..."
kubectl wait --for condition=established --timeout=60s crd frappebenches.vyogo.tech
kubectl wait --for condition=established --timeout=60s crd frappesites.vyogo.tech

print_status "Deploying operator..."
cd config/manager && kustomize edit set image controller=frappe-operator:local-test && cd ../..
make deploy IMG=frappe-operator:local-test

print_status "Waiting for operator to be ready..."
kubectl wait --for=condition=available deployment/frappe-operator-controller-manager \
    -n frappe-operator-system --timeout=180s

print_status "Operator logs (initial):"
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager --tail=20

# ===========================================
# TEST 1: KEDA Autoscaling
# ===========================================
print_test "=========================================="
print_test "TEST 1: KEDA Autoscaling"
print_test "=========================================="

print_status "Creating FrappeBench with KEDA autoscaling..."
cat <<EOF | kubectl apply -f -
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: test-bench-keda
  namespace: ${NAMESPACE}
spec:
  frappeVersion: "15"
  apps:
    - name: erpnext
      source: image
  autoscaling:
    nginx:
      enabled: true
      provider: keda
      minReplicas: 1
      maxReplicas: 3
      keda:
        triggers:
          - type: cpu
            metadata:
              type: Utilization
              value: "50"
EOF

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
SCALED_OBJECT=$(kubectl get scaledobject -n ${NAMESPACE} -l app.kubernetes.io/component=nginx 2>/dev/null | grep -v NAME | wc -l)
if [ "$SCALED_OBJECT" -ge 1 ]; then
    print_status "✓ KEDA ScaledObject created successfully"
    kubectl get scaledobject -n ${NAMESPACE}
else
    print_error "KEDA ScaledObject was not created"
    kubectl get scaledobject -A
    exit 1
fi

# ===========================================
# TEST 2: HPA Autoscaling
# ===========================================
print_test "=========================================="
print_test "TEST 2: HPA Autoscaling"
print_test "=========================================="

print_status "Creating FrappeBench with HPA autoscaling..."
cat <<EOF | kubectl apply -f -
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: test-bench-hpa
  namespace: ${NAMESPACE}
spec:
  frappeVersion: "15"
  apps:
    - name: erpnext
      source: image
  autoscaling:
    nginx:
      enabled: true
      provider: hpa
      minReplicas: 1
      maxReplicas: 3
      hpa:
        metrics:
          - type: Resource
            resource:
              name: cpu
              target:
                type: Utilization
                averageUtilization: 50
EOF

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
HPA_COUNT=$(kubectl get hpa -n ${NAMESPACE} -l app.kubernetes.io/component=nginx 2>/dev/null | grep -v NAME | wc -l)
if [ "$HPA_COUNT" -ge 1 ]; then
    print_status "✓ HPA created successfully"
    kubectl get hpa -n ${NAMESPACE}
else
    print_error "HPA was not created"
    kubectl get hpa -A
    exit 1
fi

# ===========================================
# TEST 3: Provider Switching (KEDA → HPA)
# ===========================================
print_test "=========================================="
print_test "TEST 3: Provider Switching (KEDA → HPA)"
print_test "=========================================="

print_status "Checking initial KEDA ScaledObject..."
INITIAL_SCALED=$(kubectl get scaledobject -n ${NAMESPACE} -l app.kubernetes.io/instance=test-bench-keda 2>/dev/null | grep -v NAME | wc -l)
print_status "Initial ScaledObjects: $INITIAL_SCALED"

print_status "Switching test-bench-keda from KEDA to HPA..."
kubectl patch frappebench test-bench-keda -n ${NAMESPACE} --type=merge -p '
spec:
  autoscaling:
    nginx:
      provider: hpa
      hpa:
        metrics:
          - type: Resource
            resource:
              name: cpu
              target:
                type: Utilization
                averageUtilization: 50
'

print_status "Waiting for provider switch to complete..."
sleep 10

print_status "Verifying old KEDA ScaledObject was cleaned up..."
for i in {1..30}; do
    SCALED_AFTER=$(kubectl get scaledobject -n ${NAMESPACE} -l app.kubernetes.io/instance=test-bench-keda 2>/dev/null | grep -v NAME | wc -l)
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
HPA_AFTER=$(kubectl get hpa -n ${NAMESPACE} -l app.kubernetes.io/instance=test-bench-keda 2>/dev/null | grep -v NAME | wc -l)
if [ "$HPA_AFTER" -ge 1 ]; then
    print_status "✓ HPA created after switch"
    kubectl get hpa -n ${NAMESPACE} -l app.kubernetes.io/instance=test-bench-keda
else
    print_error "HPA was not created after switch"
    exit 1
fi

# ===========================================
# TEST 4: Image Version Tag Resolution
# ===========================================
print_test "=========================================="
print_test "TEST 4: Image Version Tag Resolution"
print_test "=========================================="

print_status "Checking image tags in pods..."
NGINX_IMAGE=$(kubectl get deployment test-bench-keda-nginx -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo "not-found")
print_status "Nginx image: $NGINX_IMAGE"

if [[ "$NGINX_IMAGE" == *":v15"* ]]; then
    print_status "✓ Image tag correctly has 'v' prefix"
elif [[ "$NGINX_IMAGE" == *":15"* ]]; then
    print_error "Image tag missing 'v' prefix: $NGINX_IMAGE"
    exit 1
else
    print_status "Image uses custom/default tag: $NGINX_IMAGE"
fi

# ===========================================
# Summary
# ===========================================
print_test "=========================================="
print_test "TEST SUMMARY"
print_test "=========================================="
print_status "✓ KEDA autoscaling works"
print_status "✓ HPA autoscaling works"
print_status "✓ Provider switching works (KEDA → HPA)"
print_status "✓ Old provider resources cleaned up"
print_status "✓ Image version tags correct"

print_status ""
print_status "Final resource state:"
echo ""
echo "=== FrappeBenches ==="
kubectl get frappebench -n ${NAMESPACE}
echo ""
echo "=== ScaledObjects (KEDA) ==="
kubectl get scaledobject -A
echo ""
echo "=== HPAs ==="
kubectl get hpa -A
echo ""
echo "=== Deployments ==="
kubectl get deployment -n ${NAMESPACE}

print_status ""
print_status "=========================================="
print_status "ALL TESTS PASSED! 🎉"
print_status "=========================================="
print_status ""
print_status "To explore the cluster: export KUBECONFIG=\$(kind get kubeconfig --name $CLUSTER_NAME)"
print_status "To keep cluster alive: KEEP_CLUSTER=1 $0"
print_status "To delete cluster: kind delete cluster --name $CLUSTER_NAME"
