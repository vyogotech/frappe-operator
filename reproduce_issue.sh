#!/bin/bash
set -e

# Test script for Frappe Operator on Kind cluster (Reproduction)
CLUSTER_NAME="frappe-repro"
NAMESPACE="default"

echo "========================================="
echo "Frappe Operator - Kind Cluster Reproduction"
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

cleanup() {
    print_status "Cleaning up..."
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
}

# Step 1: Create Kind cluster
print_status "Step 1: Creating Kind cluster '$CLUSTER_NAME'..."
# Check if cluster exists
if ! kind get clusters | grep -q "$CLUSTER_NAME"; then
    kind create cluster --name "$CLUSTER_NAME" --wait 5m
else
    print_status "Cluster '$CLUSTER_NAME' already exists."
fi

# Step 2: Build and load operator image
print_status "Step 2: Building operator image..."
make build
podman build -t localhost:5001/frappe-operator:repro -f Dockerfile .
podman save localhost:5001/frappe-operator:repro -o /tmp/frappe-operator.tar
kind load image-archive /tmp/frappe-operator.tar --name "$CLUSTER_NAME"
rm -f /tmp/frappe-operator.tar

# Step 3: Install CRDs
print_status "Step 3: Installing CRDs..."
make install
kubectl wait --for condition=established --timeout=60s crd frappebenches.vyogo.tech || true
kubectl wait --for condition=established --timeout=60s crd frappesites.vyogo.tech || true

# Step 4: Deploy operator
print_status "Step 4: Deploying operator..."
cd config/manager && kustomize edit set image controller=localhost:5001/frappe-operator:repro && cd ../..
make deploy

# Wait for operator to be ready
print_status "Waiting for operator to be ready..."
kubectl wait --for=condition=available deployment/frappe-operator-controller-manager -n frappe-operator-system --timeout=120s

# Step 5: Install KEDA (required for reproduction)
print_status "Step 5: Installing KEDA..."
helm repo add keda https://kedacore.github.io/charts
helm repo update
helm upgrade --install keda keda/keda --namespace frappe-operator-system --create-namespace --set crds.install=true --wait
kubectl get crd scaledobjects.keda.sh

# Step 6: Create failing FrappeBench
print_status "Step 6: Creating failing FrappeBench..."
cat <<EOF | kubectl apply -f -
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: test-keda-bench
  namespace: ${NAMESPACE}
spec:
  frappeVersion: "v15"
  imageConfig:
    repository: ghcr.io/rmallam/frappe_docker
    tag: sha-ae82c47@sha256:fc7d472b57a1f5a75bfffb9985d9d2298a2a910d0b17ec23e36fb11b053da565
  security:
    podSecurityContext:
      runAsUser: 1001
      runAsGroup: 1001
      fsGroup: 1001
  apps:
    - name: erpnext
      source: image
  
  redisConfig:
    type: redis
  
  componentAutoscaling:
    worker-short:
      enabled: true
      provider: keda
      minReplicas: 0
      maxReplicas: 3
      cooldownPeriod: 30
      pollingInterval: 15
      keda:
        trigger: redis
        targetValue: "2"
    
    worker-long:
      enabled: true
      provider: keda
      minReplicas: 0
      maxReplicas: 2
      cooldownPeriod: 60
      pollingInterval: 30
      keda:
        trigger: redis
        targetValue: "1"
    
    worker-default:
      enabled: false
      staticReplicas: 1
    
    nginx:
      enabled: false
      staticReplicas: 1
    
    gunicorn:
      enabled: false
      staticReplicas: 1
    
    scheduler:
      enabled: false
      staticReplicas: 1
EOF

# Wait for issue to reproduce
print_status "Waiting for issue to reproduce..."
sleep 30
echo "Bench Status:"
kubectl get frappebench test-keda-bench -n ${NAMESPACE} -o yaml
echo "Init Job Status:"
kubectl get job test-keda-bench-init -n ${NAMESPACE} -o yaml
echo "Init Job Pods:"
kubectl get pods -l job-name=test-keda-bench-init -n ${NAMESPACE}
echo "Pod Description:"
kubectl describe pod -l job-name=test-keda-bench-init -n ${NAMESPACE}
