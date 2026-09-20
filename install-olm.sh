#!/bin/bash
set -euo pipefail

# Install the Frappe Operator through the Operator Lifecycle Manager (OLM), so
# it appears under "Installed Operators" in the OpenShift console and is managed
# as a ClusterServiceVersion rather than a plain Helm release.
#
# Quick start (OpenShift, no external registry needed):
#   ./install-olm.sh
#
# Quick start (any Kubernetes with OLM, pushing to your own registry):
#   REGISTRY_MODE=external IMAGE_TAG_BASE=ghcr.io/vyogotech/frappe-operator ./install-olm.sh
#
# Remove it again:
#   ./install-olm.sh --uninstall

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

NAMESPACE="${NAMESPACE:-frappe-operator-system}"
VERSION="${VERSION:-$(grep -E '^VERSION \?=' Makefile | awk '{print $3}')}"
PACKAGE_NAME="${PACKAGE_NAME:-frappe-operator}"
CHANNEL="${CHANNEL:-alpha}"

# internal = build in-cluster with an OpenShift BuildConfig and push to the
#            integrated registry. No registry credentials required.
# external = build locally with podman/docker and push to IMAGE_TAG_BASE.
REGISTRY_MODE="${REGISTRY_MODE:-auto}"

# catalog = build a real catalog (index) image with opm and install via a
#           CatalogSource + Subscription. Repeatable, what you want for a real
#           install and for anything resembling production.
# dev     = use `operator-sdk run bundle`, which serves the bundle from a
#           throwaway in-cluster pod. Fastest, needs no opm, but the resulting
#           install is not backed by a durable catalog image.
INSTALL_MODE="${INSTALL_MODE:-auto}"

IMAGE_TAG_BASE="${IMAGE_TAG_BASE:-}"
CONTAINER_TOOL="${CONTAINER_TOOL:-docker}"

# Refuse to run while a Helm-managed operator is live, because both would watch
# every namespace and fight over the same custom resources. Set to "true" only
# if you have already stopped the Helm one yourself.
ALLOW_HELM_COEXIST="${ALLOW_HELM_COEXIST:-false}"

UNINSTALL=false
[ "${1:-}" = "--uninstall" ] && UNINSTALL=true

KUBECTL="kubectl"
command -v oc >/dev/null 2>&1 && KUBECTL="oc"

step()  { echo -e "\n${BLUE}▶ $*${NC}"; }
ok()    { echo -e "${GREEN}✓ $*${NC}"; }
warn()  { echo -e "${YELLOW}⚠ $*${NC}"; }
die()   { echo -e "${RED}✗ $*${NC}" >&2; exit 1; }

echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Frappe Operator — OLM installation${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# ---------------------------------------------------------------------------
# Uninstall
# ---------------------------------------------------------------------------

if [ "$UNINSTALL" = "true" ]; then
    step "Removing the OLM installation"
    $KUBECTL delete subscription "$PACKAGE_NAME" -n "$NAMESPACE" --ignore-not-found
    CSV=$($KUBECTL get csv -n "$NAMESPACE" -o name 2>/dev/null | grep "$PACKAGE_NAME" || true)
    [ -n "$CSV" ] && $KUBECTL delete "$CSV" -n "$NAMESPACE" --ignore-not-found
    $KUBECTL delete catalogsource "${PACKAGE_NAME}-catalog" -n "$NAMESPACE" --ignore-not-found
    ok "OLM resources removed"
    echo
    warn "CRDs and your FrappeSite/FrappeBench resources were left in place on purpose."
    warn "If you stopped a Helm-managed operator to make room for this one, start it again:"
    echo "    $KUBECTL scale deployment/frappe-operator-controller-manager -n $NAMESPACE --replicas=1"
    exit 0
fi

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

step "Checking prerequisites"

command -v "$KUBECTL" >/dev/null 2>&1 || die "$KUBECTL not found"
$KUBECTL auth can-i create namespace >/dev/null 2>&1 || die "not authenticated, or insufficient permissions"
[ -f bundle.Dockerfile ] || die "run this from the repository root (bundle.Dockerfile not found)"
[ -d bundle/manifests ] || die "bundle/ not found — run 'make bundle IMG=<your-operator-image>' first"

if ! $KUBECTL get crd clusterserviceversions.operators.coreos.com >/dev/null 2>&1; then
    die "OLM is not installed on this cluster. On OpenShift it is built in; elsewhere run 'operator-sdk olm install'."
fi
ok "OLM is available"

IS_OPENSHIFT=false
$KUBECTL get route --all-namespaces >/dev/null 2>&1 && IS_OPENSHIFT=true

if [ "$REGISTRY_MODE" = "auto" ]; then
    if [ "$IS_OPENSHIFT" = "true" ] && [ -z "$IMAGE_TAG_BASE" ]; then
        REGISTRY_MODE="internal"
    else
        REGISTRY_MODE="external"
    fi
fi

if [ "$INSTALL_MODE" = "auto" ]; then
    # The integrated registry usually has no route reachable from a laptop, so
    # opm cannot push a catalog image to it. Use the in-cluster bundle server.
    if [ "$REGISTRY_MODE" = "internal" ]; then INSTALL_MODE="dev"; else INSTALL_MODE="catalog"; fi
fi

echo "  version        : $VERSION"
echo "  namespace      : $NAMESPACE"
echo "  registry mode  : $REGISTRY_MODE"
echo "  install mode   : $INSTALL_MODE"

# The operator image the CSV points at must be pullable by the cluster. This is
# the single most common reason an OLM install lands in ImagePullBackOff.
CSV_IMAGE=$(grep -oE 'image: [^ ]*frappe-operator[^ ]*' bundle/manifests/*.clusterserviceversion.yaml | head -1 | awk '{print $2}' || true)
if [ -z "$CSV_IMAGE" ] || echo "$CSV_IMAGE" | grep -q "controller:latest"; then
    die "the bundle CSV has no real operator image (found '${CSV_IMAGE:-none}').
   Regenerate it first:  make bundle IMG=ghcr.io/vyogotech/frappe-operator:v${VERSION}"
fi
ok "CSV operator image: $CSV_IMAGE"

# ---------------------------------------------------------------------------
# Refuse to fight a running Helm install
# ---------------------------------------------------------------------------

step "Checking for a conflicting Helm installation"

HELM_REPLICAS=$($KUBECTL get deployment frappe-operator-controller-manager -n "$NAMESPACE" \
    -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "")
HELM_MANAGED=$($KUBECTL get deployment frappe-operator-controller-manager -n "$NAMESPACE" \
    -o jsonpath='{.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null || echo "")

if [ "$HELM_MANAGED" = "Helm" ] && [ "${HELM_REPLICAS:-0}" != "0" ]; then
    if [ "$ALLOW_HELM_COEXIST" != "true" ]; then
        echo
        die "a Helm-managed Frappe Operator is running in $NAMESPACE with $HELM_REPLICAS replica(s).

   Running it alongside an OLM install is not supported: both watch every
   namespace and would reconcile the same FrappeSites against each other.

   Stop the Helm one first, which leaves its CRDs, your sites, and the bundled
   MariaDB operator untouched:

       $KUBECTL scale deployment/frappe-operator-controller-manager -n $NAMESPACE --replicas=0

   Then run this script again. To go back afterwards, scale it to 1.

   Do NOT 'helm uninstall' — that also removes the MariaDB operator this chart
   installs as a subchart, which the OLM bundle does not include.

   If you have already handled this yourself, re-run with ALLOW_HELM_COEXIST=true."
    fi
    warn "Helm-managed operator is still running; continuing because ALLOW_HELM_COEXIST=true"
else
    ok "no conflicting Helm-managed operator is running"
fi

# ---------------------------------------------------------------------------
# Build and publish the bundle image
# ---------------------------------------------------------------------------

step "Building the bundle image"

if [ "$REGISTRY_MODE" = "internal" ]; then
    BC_NAME="${PACKAGE_NAME}-bundle"
    BUNDLE_IMG="image-registry.openshift-image-registry.svc:5000/${NAMESPACE}/${BC_NAME}:v${VERSION}"

    $KUBECTL create namespace "$NAMESPACE" --dry-run=client -o yaml | $KUBECTL apply -f - >/dev/null

    if ! $KUBECTL get bc "$BC_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
        oc new-build --name "$BC_NAME" --binary --strategy docker \
            --to "${BC_NAME}:v${VERSION}" -n "$NAMESPACE" >/dev/null
        ok "created BuildConfig $BC_NAME"
    fi
    # bundle.Dockerfile only copies manifests onto scratch, so this is quick.
    oc start-build "$BC_NAME" --from-dir=. --follow -n "$NAMESPACE" \
        --build-arg=DOCKERFILE=bundle.Dockerfile >/dev/null 2>&1 || \
        oc start-build "$BC_NAME" --from-dir=. --follow -n "$NAMESPACE"
    ok "bundle image built in-cluster: $BUNDLE_IMG"
else
    [ -n "$IMAGE_TAG_BASE" ] || die "REGISTRY_MODE=external requires IMAGE_TAG_BASE, e.g.
   IMAGE_TAG_BASE=ghcr.io/vyogotech/frappe-operator"
    command -v "$CONTAINER_TOOL" >/dev/null 2>&1 || die "$CONTAINER_TOOL not found"

    BUNDLE_IMG="${IMAGE_TAG_BASE}-bundle:v${VERSION}"
    "$CONTAINER_TOOL" build -f bundle.Dockerfile -t "$BUNDLE_IMG" . >/dev/null
    "$CONTAINER_TOOL" push "$BUNDLE_IMG"
    ok "bundle image pushed: $BUNDLE_IMG"
fi

# ---------------------------------------------------------------------------
# Namespace and OperatorGroup
# ---------------------------------------------------------------------------

step "Preparing the namespace"

$KUBECTL create namespace "$NAMESPACE" --dry-run=client -o yaml | $KUBECTL apply -f - >/dev/null

# The bundle only supports AllNamespaces, so the OperatorGroup must not name any
# target namespaces.
if ! $KUBECTL get operatorgroup -n "$NAMESPACE" 2>/dev/null | grep -q .; then
    cat <<EOF | $KUBECTL apply -f - >/dev/null
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: ${PACKAGE_NAME}-og
  namespace: ${NAMESPACE}
spec: {}
EOF
    ok "created OperatorGroup (AllNamespaces)"
else
    ok "OperatorGroup already present"
fi

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------

if [ "$INSTALL_MODE" = "dev" ]; then
    step "Installing with operator-sdk run bundle"
    command -v operator-sdk >/dev/null 2>&1 || die "operator-sdk not found.
   Install it from https://sdk.operatorframework.io/docs/installation/ or use INSTALL_MODE=catalog."
    operator-sdk run bundle "$BUNDLE_IMG" -n "$NAMESPACE" --timeout 10m
else
    step "Building the catalog image"
    OPM="./bin/opm"
    if [ ! -x "$OPM" ]; then
        command -v opm >/dev/null 2>&1 && OPM="$(command -v opm)" || make opm >/dev/null 2>&1 || true
    fi
    [ -x "$OPM" ] || command -v opm >/dev/null 2>&1 || die "opm not found. Run 'make opm' or install it manually."

    CATALOG_IMG="${IMAGE_TAG_BASE}-catalog:v${VERSION}"
    "$OPM" index add --container-tool "$CONTAINER_TOOL" --mode semver \
        --tag "$CATALOG_IMG" --bundles "$BUNDLE_IMG"
    "$CONTAINER_TOOL" push "$CATALOG_IMG"
    ok "catalog image pushed: $CATALOG_IMG"

    step "Creating the CatalogSource and Subscription"
    cat <<EOF | $KUBECTL apply -f - >/dev/null
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ${PACKAGE_NAME}-catalog
  namespace: ${NAMESPACE}
spec:
  sourceType: grpc
  image: ${CATALOG_IMG}
  displayName: Frappe Operator Catalog
  publisher: Vyogo Technologies
  updateStrategy:
    registryPoll:
      interval: 10m
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: ${PACKAGE_NAME}
  namespace: ${NAMESPACE}
spec:
  channel: ${CHANNEL}
  name: ${PACKAGE_NAME}
  source: ${PACKAGE_NAME}-catalog
  sourceNamespace: ${NAMESPACE}
  installPlanApproval: Automatic
EOF
    ok "CatalogSource and Subscription created"
fi

# ---------------------------------------------------------------------------
# Wait for the ClusterServiceVersion
# ---------------------------------------------------------------------------

step "Waiting for the operator to become ready"

for i in $(seq 1 60); do
    PHASE=$($KUBECTL get csv -n "$NAMESPACE" -o jsonpath="{.items[?(@.spec.displayName=='Frappe Operator')].status.phase}" 2>/dev/null || true)
    [ -z "$PHASE" ] && PHASE=$($KUBECTL get csv -n "$NAMESPACE" --no-headers 2>/dev/null | grep "$PACKAGE_NAME" | awk '{print $NF}' || true)
    case "$PHASE" in
        Succeeded) ok "ClusterServiceVersion reports Succeeded"; break ;;
        Failed)    die "ClusterServiceVersion failed. Inspect it with:
   $KUBECTL get csv -n $NAMESPACE
   $KUBECTL describe csv -n $NAMESPACE" ;;
        *)         echo "  [$i/60] phase=${PHASE:-pending}"; sleep 10 ;;
    esac
done

echo
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Installed${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo
echo "The operator now appears in the OpenShift console under"
echo "Operators → Installed Operators, in project ${NAMESPACE}."
echo
echo "Check it from the command line with:"
echo "    $KUBECTL get csv -n $NAMESPACE"
echo "    $KUBECTL get pods -n $NAMESPACE"
echo
echo "Remove it again with:"
echo "    ./install-olm.sh --uninstall"
echo
warn "The OLM bundle does not install the MariaDB operator, which the Helm chart"
warn "bundles as a subchart. If your sites use MariaDB, install it separately:"
echo "    $KUBECTL create -f https://operatorhub.io/install/mariadb-operator.yaml"
