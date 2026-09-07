#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

NAMESPACE="${NAMESPACE:-frappe-operator-system}"
# KEDA and the MariaDB CRDs are shared cluster infrastructure that something
# else may depend on, so removing them is opt-in.
REMOVE_KEDA="${REMOVE_KEDA:-false}"
REMOVE_MARIADB_CRDS="${REMOVE_MARIADB_CRDS:-false}"
REMOVE_CRDS="${REMOVE_CRDS:-true}"
DELETE_SITES="${DELETE_SITES:-false}"
TIMEOUT="${TIMEOUT:-300s}"

echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Frappe Operator Uninstall${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

for tool in kubectl helm; do
    command -v "$tool" &> /dev/null || { echo -e "${RED}✗ $tool is not installed${NC}"; exit 1; }
done
kubectl cluster-info &> /dev/null || { echo -e "${RED}✗ Cannot connect to Kubernetes cluster${NC}"; exit 1; }
echo -e "${GREEN}✓ Connected to Kubernetes cluster${NC}"
echo ""

# Removing the CRDs destroys every Frappe site on the cluster, databases
# included. Make that impossible to do by accident.
SITE_COUNT=0
if kubectl get crd frappesites.vyogo.tech &> /dev/null; then
    SITE_COUNT=$(kubectl get frappesites.vyogo.tech -A --no-headers 2>/dev/null | wc -l | tr -d ' ')
fi

if [ "$SITE_COUNT" != "0" ]; then
    echo -e "${YELLOW}Found $SITE_COUNT FrappeSite(s) on this cluster:${NC}"
    kubectl get frappesites.vyogo.tech -A --no-headers 2>/dev/null | awk '{print "  - " $1 "/" $2}'
    echo ""
    if [ "$DELETE_SITES" != "true" ]; then
        echo -e "${RED}✗ Refusing to continue: this would delete those sites and their databases.${NC}"
        echo "  Re-run with DELETE_SITES=true to remove them, or delete them yourself first."
        exit 1
    fi
    echo -e "${YELLOW}DELETE_SITES=true — the sites above will be destroyed.${NC}"
    echo ""
fi

# Step 1: Frappe custom resources
# Sites go first: the bench finalizer blocks deletion while a site still
# references it, so deleting benches first simply hangs.
echo -e "${YELLOW}Step 1: Removing Frappe resources...${NC}"
for kind in sitebackups siterestores sitemigrations siteapps sitedomains sitecrons \
            siteconfigs siteusers frappesites frappebenches; do
    if kubectl get crd "${kind}.vyogo.tech" &> /dev/null; then
        if [ -n "$(kubectl get "${kind}.vyogo.tech" -A --no-headers 2>/dev/null)" ]; then
            echo "  deleting all ${kind}..."
            kubectl delete "${kind}.vyogo.tech" --all -A --timeout="$TIMEOUT" 2>/dev/null \
                || echo -e "${YELLOW}  ⚠ some ${kind} did not delete cleanly${NC}"
        fi
    fi
done
echo -e "${GREEN}✓ Frappe resources removed${NC}"
echo ""

# Step 2: the Helm release
echo -e "${YELLOW}Step 2: Uninstalling the Helm release...${NC}"
if helm status frappe-operator -n "$NAMESPACE" &> /dev/null; then
    helm uninstall frappe-operator --namespace "$NAMESPACE" --timeout "$TIMEOUT"
    echo -e "${GREEN}✓ Helm release uninstalled${NC}"
else
    echo -e "${YELLOW}⚠ No frappe-operator release in $NAMESPACE, skipping${NC}"
fi
echo ""

# Step 3: MariaDB instances and their CRDs
if [ "$REMOVE_MARIADB_CRDS" = "true" ]; then
    echo -e "${YELLOW}Step 3: Removing MariaDB CRDs...${NC}"
    if kubectl get crd mariadbs.k8s.mariadb.com &> /dev/null; then
        kubectl delete mariadbs.k8s.mariadb.com --all -A --timeout="$TIMEOUT" 2>/dev/null || true
    fi
    # The mariadb-operator is gone by now, so nothing is left to process the
    # finalizers on any straggler CR - and a CRD will not finalize while one
    # of its objects still holds one. Clear them explicitly.
    for crd in $(kubectl get crd -o name 2>/dev/null | grep "k8s.mariadb.com" || true); do
        kind="${crd#customresourcedefinition.apiextensions.k8s.io/}"
        kind="${kind%%.*}"
        for obj in $(kubectl get "$kind.k8s.mariadb.com" -A -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}' 2>/dev/null || true); do
            ns="${obj%%/*}"; name="${obj##*/}"
            echo "  clearing finalizers on $kind/$name in $ns"
            kubectl patch "$kind.k8s.mariadb.com" "$name" -n "$ns" \
                --type=merge -p '{"metadata":{"finalizers":[]}}' &> /dev/null || true
        done
    done
    kubectl get crd -o name 2>/dev/null | grep "k8s.mariadb.com" | xargs -r kubectl delete --timeout="$TIMEOUT" 2>/dev/null || true
    echo -e "${GREEN}✓ MariaDB CRDs removed${NC}"
else
    echo -e "${YELLOW}Step 3: Leaving MariaDB CRDs in place (REMOVE_MARIADB_CRDS=true to remove)${NC}"
fi
echo ""

# Step 4: KEDA
if [ "$REMOVE_KEDA" = "true" ]; then
    echo -e "${YELLOW}Step 4: Removing KEDA...${NC}"
    # Delete this first. It points at a Service inside the keda namespace, and
    # once that Service is gone the APIService fails discovery - which blocks
    # the namespace from finalizing and breaks `kubectl api-resources`
    # cluster-wide until it is removed.
    kubectl delete apiservice v1beta1.external.metrics.k8s.io --ignore-not-found 2>/dev/null || true
    kubectl delete validatingwebhookconfiguration keda-admission --ignore-not-found 2>/dev/null || true
    kubectl delete namespace keda --ignore-not-found --timeout="$TIMEOUT" 2>/dev/null || true
    kubectl get crd -o name 2>/dev/null | grep "keda.sh" | xargs -r kubectl delete --timeout="$TIMEOUT" 2>/dev/null || true
    kubectl delete clusterrole keda-external-metrics-reader keda-operator --ignore-not-found 2>/dev/null || true
    kubectl delete clusterrolebinding keda-hpa-controller-external-metrics keda-operator \
        keda-system-auth-delegator --ignore-not-found 2>/dev/null || true
    echo -e "${GREEN}✓ KEDA removed${NC}"
else
    echo -e "${YELLOW}Step 4: Leaving KEDA in place (REMOVE_KEDA=true to remove)${NC}"
fi
echo ""

# Step 5: Frappe CRDs
# Helm installs a chart's crds/ directory outside release bookkeeping and never
# removes them, so `helm uninstall` always leaves these behind.
if [ "$REMOVE_CRDS" = "true" ]; then
    echo -e "${YELLOW}Step 5: Removing Frappe CRDs...${NC}"
    kubectl get crd -o name 2>/dev/null | grep "vyogo.tech" | xargs -r kubectl delete --timeout="$TIMEOUT" 2>/dev/null || true
    echo -e "${GREEN}✓ Frappe CRDs removed${NC}"
else
    echo -e "${YELLOW}Step 5: Leaving Frappe CRDs in place (REMOVE_CRDS=true to remove)${NC}"
fi
echo ""

# Step 6: namespace and any cluster RBAC Helm did not own
echo -e "${YELLOW}Step 6: Removing the operator namespace...${NC}"
kubectl delete namespace "$NAMESPACE" --ignore-not-found --timeout="$TIMEOUT" 2>/dev/null || true
for r in $(kubectl get clusterrole,clusterrolebinding -o name 2>/dev/null | grep "frappe-operator" || true); do
    echo "  removing leftover $r"
    kubectl delete "$r" --ignore-not-found 2>/dev/null || true
done
echo -e "${GREEN}✓ Namespace removed${NC}"
echo ""

# Verify
echo -e "${YELLOW}Verifying...${NC}"
REMAINING=0
kubectl get namespace "$NAMESPACE" &> /dev/null \
    && { echo -e "${YELLOW}⚠ namespace $NAMESPACE still present (may be Terminating)${NC}"; REMAINING=1; } \
    || echo -e "${GREEN}✓ namespace gone${NC}"

CRD_LEFT=$(kubectl get crd --no-headers 2>/dev/null | grep -c "vyogo.tech" || true)
if [ "$CRD_LEFT" = "0" ]; then
    echo -e "${GREEN}✓ no Frappe CRDs remain${NC}"
else
    echo -e "${YELLOW}⚠ $CRD_LEFT Frappe CRD(s) remain${NC}"; REMAINING=1
fi

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [ "$REMAINING" = "0" ]; then
    echo -e "${GREEN}  Uninstall Complete${NC}"
else
    echo -e "${YELLOW}  Uninstall finished with leftovers noted above${NC}"
fi
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Options (environment variables):"
echo "  DELETE_SITES=true         also delete FrappeSites - destroys site databases"
echo "  REMOVE_KEDA=true          also remove KEDA"
echo "  REMOVE_MARIADB_CRDS=true  also remove the MariaDB operator CRDs"
echo "  REMOVE_CRDS=false         keep the Frappe CRDs (and your site definitions)"
echo "  NAMESPACE=<ns>            operator namespace (default: frappe-operator-system)"
