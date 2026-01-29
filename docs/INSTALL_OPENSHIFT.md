# Installing Frappe Operator on OpenShift

This guide provides step-by-step instructions for deploying the Frappe Operator in an OpenShift environment.

## 1. Prerequisites

-   Access to an OpenShift 4.x cluster.
-   `oc` CLI authenticated (`oc login`).
-   `helm` CLI installed.
-   Sufficient permissions to create Namespaces, CRDs, and cluster-wide RBAC.

## 2. Prepare the Project

Create a dedicated namespace for the operator and its components:

```bash
oc new-project frappe-operator-system
```

## 3. Install MariaDB Operator (Mandatory)

The Frappe Operator relies on the [MariaDB Operator](https://mariadb-operator.github.io/mariadb-operator/) for database provisioning. This must be installed before deploying any Frappe sites.

```bash
# Add the MariaDB Operator repository
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo update

# Install the MariaDB Operator
helm upgrade --install mariadb-operator mariadb-operator/mariadb-operator \
  --namespace frappe-operator-system \
  --set crds.enabled=true 
```

## 4. Install Frappe Operator

Install the Frappe Operator using the official Helm chart and the stable `v2.6.3` image.

```bash
# Add the Frappe Operator repository
helm repo add frappe-operator https://rmallam.github.io/frappe-operator/helm-repo
helm repo update

# Install the Frappe Operator
helm upgrade --install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --set operator.image.repository=ghcr.io/rmallam/frappe-operator \
  --set operator.image.tag=v2.6.3 \
  --wait
```

### Verification
Ensure the operator is running and has correctly detected the OpenShift platform:

```bash
oc logs -l control-plane=controller-manager -n frappe-operator-system -c manager | grep "OpenShift platform detected"
```

## 5. Security Note: SCC Compatibility

The Frappe Operator `v2.6.3` is designed to work with OpenShift's standard `restricted-v2` Security Context Constraint (SCC) out of the box. 

-   **Dynamic UIDs**: The operator uses `nil` defaults for `runAsUser`, allowing OpenShift to automatically assign a compliant UID from the namespace's range.
-   **Filesystem Access**: It uses a platform-aware approach to volume permissions, ensuring compatibility with OpenShift's filesystem group management.

For more technical details on how we handle SCCs, see the [OpenShift Technical Guide](./openshift.md).

## 6. Deployment Examples

Once the operators are running, you can deploy your first site using the following sequence of manifests.

### Step 6.1: Create MariaDB Shared Instance
Create a file named `mariadb-instance.yaml`. This provides the database infrastructure for multiple sites.

```yaml
# 1. Root password for MariaDB admin access
apiVersion: v1
kind: Secret
metadata:
  name: frappe-mariadb-root
  namespace: frappe-operator-system
type: Opaque
stringData:
  password: "StrongRootPassword123"  # CHANGE THIS
---
# 2. Main MariaDB deployment managed by mariadb-operator
apiVersion: k8s.mariadb.com/v1alpha1
kind: MariaDB
metadata:
  name: frappe-mariadb
  namespace: frappe-operator-system
spec:
  rootPasswordSecretKeyRef:
    name: frappe-mariadb-root
    key: password
  image: mariadb:10.11
  storage:
    size: 20Gi
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
  replicas: 1
```

### Step 6.2: Create a FrappeBench
A Bench represents your application environment (source code and common infrastructure). Create `my-bench.yaml`:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: prod-bench
  namespace: frappe-operator-system
spec:
  # The Frappe framework version to use
  frappeVersion: "version-15"
  # Common apps to include in this environment
  apps:
    - name: frappe
    - name: erpnext
```

### Step 6.3: Create a FrappeSite
A Site is your actual application instance (SaaS tenant). Create `my-site.yaml`:

```yaml
# 1. Admin user password for the Frappe web interface
apiVersion: v1
kind: Secret
metadata:
  name: prod-site-admin
  namespace: frappe-operator-system
type: Opaque
stringData:
  password: "AdminPassword123"  # CHANGE THIS
---
# 2. The Site resource
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: prod-site
  namespace: frappe-operator-system
spec:
  benchRef: prod-bench
  siteName: prod-site.apps.cluster.example.com  # Must be a valid domain
  adminPasswordSecretRef:
    name: prod-site-admin
    key: password
  dbConfig:
    mode: shared
    mariadbRef:
      name: frappe-mariadb
  routeConfig:
    enabled: true        # Automatically create OpenShift Route
    termination: edge    # Provide SSL termination (OOB certificates)
```

Apply all manifests in the following order:
```bash
oc apply -f mariadb-instance.yaml
oc apply -f my-bench.yaml
oc apply -f my-site.yaml
```

## 7. Verification & Troubleshooting

### Check Pods and Jobs
OpenShift monitors resource compatibility. You can verify the status with:
```bash
oc get pods -n frappe-operator-system
oc get jobs -n frappe-operator-system
```

### Access the Site
Once the site phase is `Ready`, retrieve the URL from the OpenShift Route:
```bash
oc get route prod-site -n frappe-operator-system -o jsonpath='{.spec.host}'
```

For more architectural details on Security Contexts (SCC) or database isolation, see:
- [OpenShift Technical Guide](./openshift.md).
- [MariaDB Integration Guide](./MARIADB_INTEGRATION.md).
