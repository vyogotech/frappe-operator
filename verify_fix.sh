#!/bin/bash
set -e

# Test script to update operator on existing Kind cluster
CLUSTER_NAME="frappe-repro"
NAMESPACE="default"

echo "========================================="
echo "Frappe Operator - Update & Verify Fix"
echo "========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

# Step 1: Build and load operator image
print_status "Step 1: Building operator image..."
make manifests
make build
podman build -t localhost:5001/frappe-operator:repro -f Dockerfile .
podman save localhost:5001/frappe-operator:repro -o /tmp/frappe-operator.tar
kind load image-archive /tmp/frappe-operator.tar --name "$CLUSTER_NAME"
rm -f /tmp/frappe-operator.tar

# Step 2: Deploy updated RBAC and restart operator
print_status "Step 2: Deploying updated RBAC and restarting operator..."
make deploy IMG=localhost:5001/frappe-operator:repro
kubectl -n frappe-operator-system rollout restart deployment/frappe-operator-controller-manager
kubectl -n frappe-operator-system rollout status deployment/frappe-operator-controller-manager

# Step 3: Delete failed job to trigger reconciliation
print_status "Step 3: Triggering reconciliation..."
kubectl delete job test-keda-bench-init -n ${NAMESPACE} --ignore-not-found=true

# Wait for issue to be resolved
print_status "Waiting for bench-init to start..."
sleep 10
echo "Init Job Status:"
kubectl get job test-keda-bench-init -n ${NAMESPACE} -o yaml
echo "Init Job Pods:"
kubectl get pods -l job-name=test-keda-bench-init -n ${NAMESPACE}
echo "Pod Description:"
kubectl describe pod -l job-name=test-keda-bench-init -n ${NAMESPACE}
