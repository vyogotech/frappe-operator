# Installing Frappe Operator on OpenShift (Production Guide)

This guide provides the authoritative, enterprise-grade procedure for deploying the **Frappe Operator** by **Vyogo Technologies** on Red Hat OpenShift 4.x.

---

## 1. Prerequisites

- **OpenShift Cluster**: Version 4.10+ (compatible with OpenShift 4.12, 4.14, 4.16+).
- **CLI Tools**: `oc` CLI authenticated (`oc login`) with cluster-admin or project-admin permissions, and `helm` v3+.
- **Storage**: A dynamic StorageClass supporting `ReadWriteMany` (RWX) (e.g., OpenShift Data Foundation / CephFS, AWS EFS, or Azure Files). For single-node test clusters (such as OpenShift Local / CRC), `ReadWriteOnce` (RWO) with pod-affinity fallback is supported.

---

## 2. Prepare the Operator Project

Create a dedicated namespace for the operator:

```bash
oc new-project frappe-operator-system
```

---

## 3. Database Provider Setup (Choose One)

Frappe Operator provides polymorphic database management. Choose your preferred database backend:

### Option A: PostgreSQL via StackGres (Recommended on OpenShift)

StackGres is certified on Red Hat OpenShift and runs natively under `restricted-v2` SCCs:

```bash
# Add StackGres Helm repository
helm repo add stackgres https://stackgres.io/downloads/stackgres-k8s/stackgres/helm
helm repo update

# Install StackGres Operator
helm install stackgres-operator stackgres/stackgres-operator \
  --namespace stackgres --create-namespace
```
*Alternatively, install the **StackGres Community Operator** directly from OpenShift OperatorHub via the OpenShift Web Console.*

---

### Option B: MariaDB Operator

If your Frappe/ERPNext workload requires MariaDB:

```bash
# Add MariaDB Operator Helm repository
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo update

# Install MariaDB Operator
helm upgrade --install mariadb-operator mariadb-operator/mariadb-operator \
  --namespace frappe-operator-system \
  --set crds.enabled=true \
  --create-namespace \
  --wait
```

---

### Option C: External Database (Managed Cloud SQL / RDS)

No operator is required. Simply provision a Kubernetes Secret with your database credentials as documented in [External Resources](external-resources.md).

---

## 4. Install Frappe Operator

Deploy the operator using the official Vyogo Technologies Helm repository:

```bash
# Add the official Vyogo Helm repository
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm repo update

# Install Frappe Operator v5.2.0
helm upgrade --install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --set operator.image.repository=ghcr.io/vyogotech/frappe-operator \
  --set operator.image.tag=v5.2.0 \
  --set operatorConfig.enforceHTTPS=true \
  --wait
```

### Verification
Verify that the controller manager is running and has identified OpenShift:

```bash
oc logs -l control-plane=controller-manager -n frappe-operator-system -c manager | grep "OpenShift platform detected"
```

---

## 5. Security Context Constraints (`restricted-v2`) Architecture

The Frappe Operator is engineered for strict adherence to OpenShift's default `restricted-v2` Security Context Constraint:

- **Dynamic Non-Root UIDs**: The operator sets `runAsUser: nil` in its pod definitions. OpenShift's admission controller automatically assigns a compliant non-root UID from the project's allocated UID range (e.g., `1000880000/10000`).
- **SELinux & MCS Label Isolation**: Containers run with unique SELinux Multi-Category Security (MCS) labels managed dynamically by OpenShift.
- **Filesystem Permissions**: Workloads rely on platform-managed group allocations rather than hardcoding `fsGroup: 0` (which is prohibited by `restricted-v2`).
- **Dropped Capabilities**: All containers drop all Linux capabilities (`capabilities: drop: ["ALL"]`) and set `allowPrivilegeEscalation: false`.

You can verify that all workloads run under `restricted-v2`:
```bash
oc get pods -n <your-project> -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.openshift\.io/scc}{"\n"}{end}'
```

---

## 6. Deploy a Production Bench & Site

### Step 6.1: Create a Production Project
```bash
oc new-project erp-prod
```

### Step 6.2: Deploy the FrappeBench
Create `prod-bench.yaml`:

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: erp-bench
  namespace: erp-prod
spec:
  frappeVersion: "version-15"
  imageConfig:
    repository: ghcr.io/vyogotech/erpnext-for-operator
    tag: version-15
    pullPolicy: IfNotPresent
  apps:
    - name: erpnext
```

Deploy the bench:
```bash
oc apply -f prod-bench.yaml
oc wait --for=condition=Ready frappebench/erp-bench -n erp-prod --timeout=300s
```

---

### Step 6.3: Deploy a Dedicated Tenant Site

Create `prod-site.yaml`:

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: my-company
  namespace: erp-prod
spec:
  benchRef:
    name: erp-bench
  siteName: my-company.apps.cluster.example.com
  dbConfig:
    provider: postgres
    mode: dedicated
    postgresEngine: stackgres
  deletionPolicy: Delete
```

Deploy the site:
```bash
oc apply -f prod-site.yaml
oc wait --for=condition=Ready frappesite/my-company -n erp-prod --timeout=300s
```

---

## 7. OpenShift Route & Automatic HTTPS Redirection

When running on OpenShift, the Frappe Operator automatically provisions an OpenShift Route:

- **Edge TLS Termination**: Route specifies `tls.termination: edge`.
- **Enforced Redirection**: Route specifies `tls.insecureEdgeTerminationPolicy: Redirect`, ensuring all plain HTTP traffic is redirected to HTTPS automatically.
- **Site URL**: The site's status URL is populated as `https://my-company.apps.cluster.example.com`.

Check the route:
```bash
oc get route -n erp-prod
```

Test access over HTTPS:
```bash
curl -I https://my-company.apps.cluster.example.com/login
```

---

## 8. Day-2 Diagnostics & Troubleshooting

```bash
# View operator controller manager logs
oc logs -l control-plane=controller-manager -n frappe-operator-system -f

# Inspect site initialization job logs
oc logs -l job-name=my-company-init -n erp-prod -f

# Check bench runtime pods
oc get pods -n erp-prod -l bench=erp-bench
```
