#!/bin/sh
set -e

# ─────────────────────────────────────────────────────────────────────────────
# Frappe Operator installer.
#
# This is the one place installation logic lives. Every other entry point --
# the DigitalOcean 1-Click deploy.sh, an Ansible task, a future marketplace
# wrapper -- sets a few environment variables and runs this script.
#
# WHAT gets installed is declared by the Helm chart: the operator plus optional
# subcharts (mariadb-operator, keda, ingress-nginx, cert-manager). This script
# only decides WHICH of those to switch on for the cluster it finds itself on,
# and handles the handful of things a chart cannot express: OpenShift
# SecurityContextConstraints, and StackGres via OLM where Helm is not the path.
#
# POSIX sh on purpose. It is curl-piped into environments whose only proven
# shell is /bin/sh, so no bashisms: no arrays, no [[ ]], no &>, no echo -e.
#
# Environment (all optional):
#   NAMESPACE              target namespace                 frappe-operator-system
#   CHART_VERSION          pin the chart version            unset = latest
#   VALUES_FILE            extra values file, path or URL   unset
#   IMAGE_REPO, IMAGE_TAG  override the operator image      unset = chart default
#   INSTALL_MARIADB_CRDS   MariaDB CRDs outside Helm        true
#   INSTALL_KEDA           KEDA subchart                    true
#   INSTALL_INGRESS        ingress-nginx subchart           false
#   INSTALL_CERT_MANAGER   cert-manager subchart            false
#   INSTALL_STACKGRES      StackGres PostgreSQL operator    false
#   INSTALL_POSTGRES_SCC   Percona SCC (OpenShift only)     false
#   POSTGRES_NAMESPACE     for the Percona SCC bindings     frappe-pg
#   STACKGRES_NAMESPACE                                     stackgres
# ─────────────────────────────────────────────────────────────────────────────

NAMESPACE="${NAMESPACE:-frappe-operator-system}"
CHART_VERSION="${CHART_VERSION:-}"
VALUES_FILE="${VALUES_FILE:-}"
# Unset by default: the chart's own values.yaml carries the version that matches
# this checkout, and scripts/bump-version.sh keeps it current. Hardcoding a
# default here meant the script pinned a stale tag over the correct one on
# every install. Export these only to override.
IMAGE_REPO="${IMAGE_REPO:-}"
IMAGE_TAG="${IMAGE_TAG:-}"
INSTALL_MARIADB_CRDS="${INSTALL_MARIADB_CRDS:-true}"
INSTALL_KEDA="${INSTALL_KEDA:-true}"
INSTALL_INGRESS="${INSTALL_INGRESS:-false}"
INSTALL_CERT_MANAGER="${INSTALL_CERT_MANAGER:-false}"
INSTALL_STACKGRES="${INSTALL_STACKGRES:-false}"
STACKGRES_NAMESPACE="${STACKGRES_NAMESPACE:-stackgres}"
# Percona's PostgreSQL runs as uid/gid 26 and never sets an fsGroup - its
# "openshift: true" mode only sets fsGroupChangePolicy, assuming SCC admission
# will supply one. Where it does not, /pgdata stays root-owned and postgres
# cannot start. Only needed if you provision Postgres-backed sites.
INSTALL_POSTGRES_SCC="${INSTALL_POSTGRES_SCC:-false}"
POSTGRES_NAMESPACE="${POSTGRES_NAMESPACE:-frappe-pg}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
say()  { printf '%b\n' "$1"; }
ok()   { say "${GREEN}✓ $1${NC}"; }
warn() { say "${YELLOW}⚠ $1${NC}"; }
step() { say "${YELLOW}$1${NC}"; }
fail() { say "${RED}✗ $1${NC}"; exit 1; }
is_openshift() {
    kubectl api-resources --api-group=security.openshift.io 2>/dev/null | grep -q securitycontextconstraints
}

say "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
say "${GREEN}  Frappe Operator Installation Script${NC}"
say "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

step "Checking prerequisites..."
command -v kubectl >/dev/null 2>&1 || fail "kubectl is not installed"
ok "kubectl found"
command -v helm >/dev/null 2>&1 || fail "helm is not installed"
ok "helm found"
kubectl cluster-info >/dev/null 2>&1 || fail "Cannot connect to Kubernetes cluster"
ok "Connected to Kubernetes cluster"
echo ""

# ── Step 1: MariaDB Operator CRDs, outside Helm ─────────────────────────────
# Installed with kubectl rather than by the mariadb-operator subchart, so they
# carry no Helm ownership and survive `helm uninstall`. The subchart's own CRD
# install is switched off below; Helm refuses to adopt resources it did not
# create, which failed every clean install with "invalid ownership metadata".
if [ "$INSTALL_MARIADB_CRDS" = "true" ]; then
    step "Step 1: Installing MariaDB Operator CRDs..."
    if kubectl apply --server-side -k "github.com/mariadb-operator/mariadb-operator/config/crd?ref=v0.34.0" >/dev/null 2>&1; then
        ok "MariaDB Operator CRDs installed"
    else
        warn "kustomize fetch failed, applying CRDs individually..."
        for crd in mariadbs databases users grants; do
            kubectl apply --server-side \
                -f "https://raw.githubusercontent.com/mariadb-operator/mariadb-operator/v0.34.0/config/crd/bases/k8s.mariadb.com_${crd}.yaml" \
                >/dev/null 2>&1 || true
        done
        ok "MariaDB Operator CRDs installed (fallback method)"
    fi
    kubectl wait --for condition=established --timeout=60s crd mariadbs.k8s.mariadb.com >/dev/null 2>&1 || true
    echo ""
fi

# ── Step 2: SecurityContextConstraints for Percona PostgreSQL (OpenShift) ───
if [ "$INSTALL_POSTGRES_SCC" = "true" ]; then
    step "Step 2: Installing the PostgreSQL SecurityContextConstraints..."
    if ! is_openshift; then
        warn "Not an OpenShift cluster (no SCC API), skipping"
    else
        kubectl create namespace "$POSTGRES_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

        # Percona's postgres runs as uid/gid 26 and sets no fsGroup, so the data
        # volume stays root-owned and its startup container fails with
        # "cannot change permissions of /pgdata/pg16". Force the fsGroup here.
        # priority 20 so this outranks the default SCCs for these accounts.
        cat <<'SCC' | kubectl apply -f - >/dev/null
apiVersion: security.openshift.io/v1
kind: SecurityContextConstraints
metadata:
  name: percona-pg-fsgroup
priority: 20
allowPrivilegeEscalation: false
allowPrivilegedContainer: false
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: MustRunAs
fsGroup:
  type: MustRunAs
  ranges:
    - min: 26
      max: 26
supplementalGroups:
  type: RunAsAny
volumes: ["configMap","downwardAPI","emptyDir","persistentVolumeClaim","projected","secret"]
SCC

        for sa in frappe-postgres-instance frappe-postgres-pgbouncer \
                  frappe-postgres-repo-host frappe-postgres-upgrade default; do
            kubectl create clusterrolebinding "percona-pg-fsgroup-${sa}-${POSTGRES_NAMESPACE}" \
                --clusterrole=system:openshift:scc:percona-pg-fsgroup \
                --serviceaccount="${POSTGRES_NAMESPACE}:${sa}" \
                --dry-run=client -o yaml 2>/dev/null | kubectl apply -f - >/dev/null || true
        done

        ok "PostgreSQL SCC installed and bound in $POSTGRES_NAMESPACE"
        # Pods in OpenShift's own system namespaces (default, kube-*, openshift-*)
        # are not annotated by SCC admission, so a Percona cluster placed there
        # never picks this up. Keep it in a namespace of your own.
        if [ "$POSTGRES_NAMESPACE" = "default" ]; then
            warn "'default' is a system namespace - SCCs are not applied to pods there."
            warn "  Put the PostgreSQL cluster in its own namespace instead."
        fi
    fi
    echo ""
fi

# ── Step 3: StackGres PostgreSQL Operator ───────────────────────────────────
# Not a chart dependency: on OpenShift it ships through OLM, which Helm cannot
# express, and it is an alternative to Percona that most installs never enable.
if [ "$INSTALL_STACKGRES" = "true" ]; then
    step "Step 3: Installing StackGres PostgreSQL Operator..."
    if is_openshift; then
        # OpenShift: OLM Subscription against the community catalog.
        kubectl create namespace "$STACKGRES_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
        cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: stackgres-og
  namespace: $STACKGRES_NAMESPACE
spec:
  targetNamespaces:
  - $STACKGRES_NAMESPACE
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: stackgres-community
  namespace: $STACKGRES_NAMESPACE
spec:
  channel: stable
  name: stackgres-community
  source: community-operators
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
EOF
    else
        # Non-OpenShift: upstream Helm chart
        helm repo add stackgres https://stackgres.io/downloads/stackgres-k8s/stackgres/helm >/dev/null
        helm repo update >/dev/null
        helm upgrade --install stackgres-operator stackgres/stackgres-operator \
            -n "$STACKGRES_NAMESPACE" --create-namespace
    fi
    echo "Waiting for StackGres CRDs to be established..."
    kubectl wait --for condition=established --timeout=120s crd sgclusters.stackgres.io || true
    ok "StackGres PostgreSQL Operator installed"
    echo ""
fi

# ── Step 4: Frappe Operator via Helm ────────────────────────────────────────
# Everything else -- KEDA, ingress-nginx, cert-manager, the MariaDB operator --
# is a subchart, switched on or off here. One Helm release owns all of it, so
# `helm uninstall frappe-operator` removes it all.
step "Step 4: Installing Frappe Operator..."

if [ -d "./helm/frappe-operator" ]; then
    CHART="./helm/frappe-operator"
    echo "Using local Helm chart from $CHART"
    # charts/*.tgz is covered by the *.tgz rule in .gitignore, so a fresh clone
    # has Chart.yaml and Chart.lock but none of the subchart archives, and Helm
    # refuses to install: "found in Chart.yaml, but missing in charts/".
    # `build` (not `update`) fetches exactly the versions Chart.lock pins.
    if ! ls "$CHART"/charts/*.tgz >/dev/null 2>&1; then
        echo "Fetching chart dependencies (charts/ is not checked in)..."
        helm dependency build "$CHART" || fail "Failed to fetch chart dependencies"
    fi
else
    echo "Adding Helm repository..."
    helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo >/dev/null
    helm repo update >/dev/null
    CHART="frappe-operator/frappe-operator"
    echo "Using Helm chart from repository"
fi

# Build the Helm argument list in the positional parameters -- POSIX sh has no
# arrays. This script takes no arguments of its own, so nothing is lost.
set -- --namespace "$NAMESPACE" --create-namespace --timeout 10m \
       --set mariadb-operator.enabled=true \
       --set mariadb.enabled=false

# Step 1 owns the MariaDB CRDs; see the note there.
[ "$INSTALL_MARIADB_CRDS" = "true" ] && set -- "$@" --set mariadb-operator.crds.enabled=false

if [ "$INSTALL_KEDA" = "true" ]; then
    set -- "$@" --set keda.enabled=true
else
    set -- "$@" --set keda.enabled=false
fi
[ "$INSTALL_INGRESS" = "true" ]      && set -- "$@" --set ingress-nginx.enabled=true
[ "$INSTALL_CERT_MANAGER" = "true" ] && set -- "$@" --set cert-manager.enabled=true

[ -n "$CHART_VERSION" ] && set -- "$@" --version "$CHART_VERSION"
[ -n "$VALUES_FILE" ]   && set -- "$@" --values "$VALUES_FILE"
# Only override the chart's image when asked; see IMAGE_REPO/IMAGE_TAG above.
[ -n "$IMAGE_REPO" ]    && set -- "$@" --set operator.image.repository="$IMAGE_REPO"
[ -n "$IMAGE_TAG" ]     && set -- "$@" --set operator.image.tag="$IMAGE_TAG"

echo "Installing Helm chart..."
# Keep stderr: hiding it turned every failure into "may have warnings" and left
# the real reason - usually one line from Helm - entirely undiscoverable.
helm upgrade --install frappe-operator "$CHART" "$@" \
    || fail "Helm installation failed (see the error above)"
ok "Frappe Operator chart installed/upgraded"
echo ""

# ── Step 5: Wait and verify ─────────────────────────────────────────────────
step "Step 5: Waiting for operator to be ready..."
if kubectl wait --namespace "$NAMESPACE" --for=condition=ready pod \
        --selector=control-plane=controller-manager --timeout=180s >/dev/null 2>&1; then
    ok "Operator pod is ready"
else
    warn "Operator pod may still be starting..."
fi
echo ""

step "Step 6: Verifying installation..."
if kubectl get crd frappebenches.vyogo.tech >/dev/null 2>&1; then
    ok "Frappe CRDs installed"
else
    say "${RED}✗ Frappe CRDs not found${NC}"
fi
if kubectl get pod -n "$NAMESPACE" -l control-plane=controller-manager 2>/dev/null | grep -q Running; then
    ok "Operator pod is running"
else
    say "${RED}✗ Operator pod not running${NC}"
fi
if kubectl get crd mariadbs.k8s.mariadb.com >/dev/null 2>&1; then
    ok "MariaDB Operator CRDs installed"
    if kubectl get pod -n "$NAMESPACE" -l app.kubernetes.io/name=mariadb-operator 2>/dev/null | grep -q Running; then
        ok "MariaDB Operator is running"
    else
        warn "MariaDB Operator pods may still be starting..."
    fi
else
    warn "MariaDB Operator CRDs not found"
fi
if [ "$INSTALL_KEDA" = "true" ]; then
    if kubectl get crd scaledobjects.keda.sh >/dev/null 2>&1; then
        ok "KEDA CRDs installed"
        if kubectl get pod -n "$NAMESPACE" -l app=keda-operator 2>/dev/null | grep -q Running; then
            ok "KEDA Operator is running"
        else
            warn "KEDA Operator pods may still be starting..."
        fi
    else
        warn "KEDA CRDs not found"
    fi
fi
if [ "$INSTALL_INGRESS" = "true" ]; then
    if kubectl get pod -n "$NAMESPACE" -l app.kubernetes.io/component=controller 2>/dev/null | grep -q Running; then
        ok "NGINX Ingress Controller is running"
    else
        warn "NGINX Ingress Controller pods may still be starting..."
    fi
fi

echo ""
say "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
say "${GREEN}  Installation Complete!${NC}"
say "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Next steps:"
echo ""
echo "1. Create a FrappeBench with worker autoscaling:"
echo "   kubectl apply -f - <<EOF"
echo "   apiVersion: vyogo.tech/v1"
echo "   kind: FrappeBench"
echo "   metadata:"
echo "     name: my-bench"
echo "     namespace: default"
echo "   spec:"
echo "     frappeVersion: \"version-15\""
echo "     apps:"
echo "       - name: erpnext"
echo "         source: image"
echo "     redisConfig:"
echo "       type: redis"
echo "     # Worker autoscaling (requires KEDA)"
echo "     workerAutoscaling:"
echo "       short:"
echo "         enabled: true"
echo "         minReplicas: 0"
echo "         maxReplicas: 10"
echo "       long:"
echo "         enabled: true"
echo "         minReplicas: 0"
echo "         maxReplicas: 5"
echo "       default:"
echo "         enabled: false"
echo "         staticReplicas: 1"
echo "   EOF"
echo ""
echo "2. Create a FrappeSite:"
echo "   kubectl apply -f - <<EOF"
echo "   apiVersion: vyogo.tech/v1"
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
echo "   kubectl logs -n $NAMESPACE -l control-plane=controller-manager -f"
echo ""
echo "4. Check worker scaling status:"
echo "   kubectl get frappebench my-bench -o jsonpath='{.status.workerScaling}' | jq"
echo ""
if [ "$INSTALL_KEDA" = "true" ]; then
    echo "5. Check KEDA ScaledObjects:"
    echo "   kubectl get scaledobjects -n default"
    echo ""
fi
echo "For more information, see:"
echo "  - GitHub: https://github.com/vyogotech/frappe-operator"
echo "  - Worker Autoscaling: examples/worker-autoscaling.yaml"
