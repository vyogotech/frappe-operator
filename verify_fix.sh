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
# Set environment variables for security context defaults
kubectl -n frappe-operator-system set env deployment/frappe-operator-controller-manager \
    FRAPPE_DEFAULT_UID=1001 FRAPPE_DEFAULT_GID=0 FRAPPE_DEFAULT_FSGROUP=1001
kubectl -n frappe-operator-system rollout restart deployment/frappe-operator-controller-manager
kubectl -n frappe-operator-system rollout status deployment/frappe-operator-controller-manager

# Step 3: Trigger reconciliation
print_status "Step 3: Triggering reconciliation..."
kubectl delete job test-keda-bench-init -n ${NAMESPACE} --ignore-not-found=true
kubectl delete pvc test-keda-bench-sites -n ${NAMESPACE} --ignore-not-found=true

# Patch the FrappeBench to use the custom image and correct field
kubectl patch frappebench test-keda-bench -n ${NAMESPACE} --type=merge -p '{
  "spec": {
    "imageConfig": {
      "repository": "ghcr.io/rmallam/frappe_docker",
      "tag": "sha-ae82c47",
      "pullPolicy": "Never"
    }
  }
}'

# Wait for issue to be resolved
print_status "Waiting for bench-init to start..."
for i in {1..20}; do
    if kubectl get job test-keda-bench-init -n ${NAMESPACE} &>/dev/null; then
        print_status "Job created!"
        break
    fi
    echo "Waiting for job to be created (iteration $i/20)..."
    sleep 3
done

echo "Init Job Status:"
kubectl get job test-keda-bench-init -n ${NAMESPACE} -o yaml || true
echo "Init Job Pods:"
kubectl get pods -l job-name=test-keda-bench-init -n ${NAMESPACE} || true
echo "Pod Description:"
kubectl describe pod -l job-name=test-keda-bench-init -n ${NAMESPACE} || true
echo "Pod Logs:"
kubectl logs -l job-name=test-keda-bench-init -n ${NAMESPACE} --tail=100 || true
