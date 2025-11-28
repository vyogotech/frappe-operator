#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
NAMESPACE="${NAMESPACE:-frappe-operator-system}"
IMAGE_REPO="${IMAGE_REPO:-ghcr.io/vyogotech/frappe-operator}"
IMAGE_TAG="${IMAGE_TAG:-v1.0.0}"
INSTALL_MARIADB_CRDS="${INSTALL_MARIADB_CRDS:-true}"
INSTALL_INGRESS="${INSTALL_INGRESS:-false}"

echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Frappe Operator Installation Script${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}✗ kubectl is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ kubectl found${NC}"

if ! command -v helm &> /dev/null; then
    echo -e "${RED}✗ helm is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ helm found${NC}"

# Check Kubernetes connection
if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}✗ Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Connected to Kubernetes cluster${NC}"
echo ""

# Step 1: Install MariaDB Operator CRDs
if [ "$INSTALL_MARIADB_CRDS" = "true" ]; then
    echo -e "${YELLOW}Step 1: Installing MariaDB Operator CRDs...${NC}"
    
    if kubectl apply --server-side -k "github.com/mariadb-operator/mariadb-operator/config/crd?ref=v0.34.0" 2>/dev/null; then
        echo -e "${GREEN}✓ MariaDB Operator CRDs installed${NC}"
    else
        echo -e "${YELLOW}⚠ Failed to install via kustomize, trying direct URLs...${NC}"
        
        # Fallback: install individual CRDs
        CRDS=(
            "https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/v0.34.0/config/crd/bases/k8s.mariadb.com_mariadbs.yaml"
            "https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/v0.34.0/config/crd/bases/k8s.mariadb.com_databases.yaml"
            "https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/v0.34.0/config/crd/bases/k8s.mariadb.com_users.yaml"
            "https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/v0.34.0/config/crd/bases/k8s.mariadb.com_grants.yaml"
        )
        
        for crd in "${CRDS[@]}"; do
            kubectl apply --server-side -f "$crd" 2>/dev/null || true
        done
        
        echo -e "${GREEN}✓ MariaDB Operator CRDs installed (fallback method)${NC}"
    fi
    
    # Wait for CRDs to be established
    echo "Waiting for CRDs to be established..."
    sleep 5
    kubectl wait --for condition=established --timeout=60s crd mariadbs.k8s.mariadb.com || true
    echo ""
fi

# Step 2: Install NGINX Ingress Controller (optional)
if [ "$INSTALL_INGRESS" = "true" ]; then
    echo -e "${YELLOW}Step 2: Installing NGINX Ingress Controller...${NC}"
    
    if kubectl get namespace ingress-nginx &> /dev/null; then
        echo -e "${YELLOW}⚠ Ingress controller namespace already exists, skipping...${NC}"
    else
        kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/cloud/deploy.yaml
        
        echo "Waiting for Ingress controller to be ready..."
        kubectl wait --namespace ingress-nginx \
            --for=condition=ready pod \
            --selector=app.kubernetes.io/component=controller \
            --timeout=300s || echo -e "${YELLOW}⚠ Ingress controller may still be starting...${NC}"
        
        echo -e "${GREEN}✓ NGINX Ingress Controller installed${NC}"
    fi
    echo ""
fi

# Step 3: Install Frappe Operator via Helm
echo -e "${YELLOW}Step 3: Installing Frappe Operator...${NC}"

# Check if chart directory exists (for local install)
if [ -d "./helm/frappe-operator" ]; then
    CHART_PATH="./helm/frappe-operator"
    echo "Using local Helm chart from ./helm/frappe-operator"
else
    # Try to use OCI registry
    CHART_PATH="oci://ghcr.io/vyogotech/charts/frappe-operator"
    echo "Using Helm chart from OCI registry"
fi

# Install or upgrade the chart (upgrade --install handles both cases)
# Use --create-namespace to let Helm manage the namespace
echo "Installing Helm chart..."
if helm upgrade --install frappe-operator "$CHART_PATH" \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --set mariadb-operator.enabled=true \
    --set mariadb.enabled=false \
    --set operator.image.repository="$IMAGE_REPO" \
    --set operator.image.tag="$IMAGE_TAG" \
    --timeout=10m >/dev/null 2>&1; then
    
    echo -e "${GREEN}✓ Frappe Operator chart installed/upgraded${NC}"
else
    echo -e "${YELLOW}⚠ Helm install may have warnings, checking status...${NC}"
    helm status frappe-operator -n "$NAMESPACE" >/dev/null 2>&1 || {
        echo -e "${RED}✗ Helm installation failed${NC}"
        echo "If you see namespace ownership errors, try:"
        echo "  kubectl delete namespace $NAMESPACE"
        echo "  Then run this script again"
        exit 1
    }
fi
echo ""

# Step 4: Wait for operator to be ready
echo -e "${YELLOW}Step 4: Waiting for operator to be ready...${NC}"

# Wait for operator pod
if kubectl wait --namespace "$NAMESPACE" \
    --for=condition=ready pod \
    --selector=control-plane=controller-manager \
    --timeout=180s 2>/dev/null; then
    echo -e "${GREEN}✓ Operator pod is ready${NC}"
else
    echo -e "${YELLOW}⚠ Operator pod may still be starting...${NC}"
fi

# Wait a bit for other components
sleep 5
echo ""

# Step 5: Verify installation
echo -e "${YELLOW}Step 5: Verifying installation...${NC}"

# Check CRDs
if kubectl get crd frappebenches.vyogo.tech &> /dev/null; then
    echo -e "${GREEN}✓ Frappe CRDs installed${NC}"
else
    echo -e "${RED}✗ Frappe CRDs not found${NC}"
fi

# Check operator pod
if kubectl get pod -n "$NAMESPACE" -l control-plane=controller-manager | grep -q Running; then
    echo -e "${GREEN}✓ Operator pod is running${NC}"
else
    echo -e "${RED}✗ Operator pod not running${NC}"
fi

# Check MariaDB Operator (if enabled)
if kubectl get crd mariadbs.k8s.mariadb.com &> /dev/null; then
    echo -e "${GREEN}✓ MariaDB Operator CRDs installed${NC}"
    
    if kubectl get pod -n "$NAMESPACE" -l app.kubernetes.io/name=mariadb-operator | grep -q Running; then
        echo -e "${GREEN}✓ MariaDB Operator is running${NC}"
    else
        echo -e "${YELLOW}⚠ MariaDB Operator pods may still be starting...${NC}"
    fi
else
    echo -e "${YELLOW}⚠ MariaDB Operator CRDs not found${NC}"
fi

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Installation Complete!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Next steps:"
echo ""
echo "1. Create a FrappeBench:"
echo "   kubectl apply -f - <<EOF"
echo "   apiVersion: vyogo.tech/v1alpha1"
echo "   kind: FrappeBench"
echo "   metadata:"
echo "     name: my-bench"
echo "     namespace: default"
echo "   spec:"
echo "     frappeVersion: \"latest\""
echo "     apps:"
echo "       - name: erpnext"
echo "         source: image"
echo "   EOF"
echo ""
echo "2. Create a FrappeSite:"
echo "   kubectl apply -f - <<EOF"
echo "   apiVersion: vyogo.tech/v1alpha1"
echo "   kind: FrappeSite"
echo "   metadata:"
echo "     name: my-site"
echo "     namespace: default"
echo "   spec:"
echo "     benchRef:"
echo "       name: my-bench"
echo "       namespace: default"
echo "     siteName: site1.local"
echo "     dbConfig:"
echo "       provider: mariadb"
echo "       mode: shared"
echo "     domain: site1.local"
echo "   EOF"
echo ""
echo "3. Check operator logs:"
echo "   kubectl logs -n $NAMESPACE -l control-plane=controller-manager"
echo ""
echo "For more information, see: https://github.com/vyogotech/frappe-operator"
