#!/bin/bash
set -e

# verify-all.sh
# Comprehensive verification script for Frappe Operator on Kind

export KIND_EXPERIMENTAL_PROVIDER=podman
CLUSTER_NAME="frappe-verify"
NAMESPACE="default"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 1. Cleanup and Create Cluster
print_status "Step 1: Creating Kind cluster '$CLUSTER_NAME'..."
kind delete cluster --name "$CLUSTER_NAME" || true
kind create cluster --name "$CLUSTER_NAME" --wait 5m

# 2. Build and Load Images
print_status "Step 2: Building and loading images..."

# Operator Image
print_status "Building operator image..."
make build
podman build -t localhost/frappe-operator:test -f Dockerfile .
print_status "Loading operator image into Kind..."
podman save localhost/frappe-operator:test -o /tmp/frappe-operator.tar
kind load image-archive /tmp/frappe-operator.tar --name "$CLUSTER_NAME"

# Production Image (already built manually)
print_status "Loading production image into Kind..."
podman save localhost/frappe-production:test -o /tmp/frappe-production.tar
kind load image-archive /tmp/frappe-production.tar --name "$CLUSTER_NAME"

# 3. Install Cert-Manager
print_status "Step 3: Installing cert-manager..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.2/cert-manager.yaml
print_status "Waiting for cert-manager to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment -n cert-manager --all

# 4. Install MariaDB Operator
print_status "Step 4: Installing MariaDB operator..."
# Manually apply CRDs first as helm install often fails on them in Kind
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_mariadbs.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_databases.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_users.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_grants.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_maxscales.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_connections.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_backups.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_sqljobs.yaml
kubectl apply -f https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/main/config/crd/bases/k8s.mariadb.com_restores.yaml

helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo update
helm install mariadb-operator mariadb-operator/mariadb-operator -n mariadb-operator-system --create-namespace --wait

# 5. Install Frappe Operator
print_status "Step 5: Installing Frappe operator..."
make install
# Deploy with correct image
make deploy IMG=localhost/frappe-operator:test
# Patch for local image pull policy
kubectl patch deployment frappe-operator-controller-manager -n frappe-operator-system --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/1/imagePullPolicy", "value":"IfNotPresent"}]'

print_status "Waiting for Frappe operator to be ready..."
kubectl wait --for=condition=available --timeout=120s deployment/frappe-operator-controller-manager -n frappe-operator-system

# 6. Create MariaDB instance
print_status "Step 6: Creating MariaDB instance..."
cat <<EOF | kubectl apply -f -
apiVersion: k8s.mariadb.com/v1alpha1
kind: MariaDB
metadata:
  name: test-mariadb
spec:
  rootPasswordSecretKeyRef:
    name: mariadb-root-password
    key: password
  storage:
    size: 1Gi
---
apiVersion: v1
kind: Secret
metadata:
  name: mariadb-root-password
type: Opaque
stringData:
  password: root
EOF

# 7. Create FrappeBench
print_status "Step 7: Creating test FrappeBench..."
cat <<EOF | kubectl apply -f -
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: test-bench
  annotations:
    frappe.tech/skip-bench-build: "1"
spec:
  frappeVersion: "15"
  imageConfig:
    repository: localhost/frappe-production
    tag: test
    pullPolicy: IfNotPresent
  storageSize: "1Gi"
  security:
    podSecurityContext:
      fsGroup: 0
  apps:
    - name: erpnext
      source: image
EOF

# 8. Create FrappeSite
print_status "Step 8: Creating test FrappeSite..."
cat <<EOF | kubectl apply -f -
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: test-site
spec:
  benchRef:
    name: test-bench
  siteName: test-site.localhost
  dbConfig:
    mariadbRef:
      name: test-mariadb
EOF

# 9. Verification Loop
print_status "Step 9: Verifying deployment (this may take 5-10 minutes)..."

TIMEOUT=600
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    PHASE=$(kubectl get frappesite test-site -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
    print_status "Current Site Phase: $PHASE"
    
    if [ "$PHASE" = "Ready" ]; then
        print_status "✓ FrappeSite is Ready!"
        break
    fi
    
    if [ "$PHASE" = "Failed" ]; then
        print_error "FrappeSite failed!"
        kubectl describe frappesite test-site
        kubectl logs -l job-name=test-site-init --all-containers || true
        exit 1
    fi
    
    # Check for jobs
    kubectl get jobs
    kubectl get pods
    
    sleep 20
    ELAPSED=$((ELAPSED + 20))
done

if [ "$PHASE" != "Ready" ]; then
    print_error "Verification timed out"
    kubectl describe frappesite test-site
    kubectl logs -l job-name=test-site-init --all-containers || true
    exit 1
fi

print_status "========================================="
print_status "SUCCESS: Setup verified on Kind cluster"
print_status "========================================="
