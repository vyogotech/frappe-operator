# Frappe Operator on OpenShift: Full Deployment Manual

This guide provides the authoritative, step-by-step workflow for deploying the Frappe Operator and ERPNext in an enterprise OpenShift 4.x environment.

---

## 1. Prerequisites

-   Access to an OpenShift 4.x cluster (v4.10+ recommended).
-   `oc` CLI authenticated (`oc login`).
-   `helm` CLI (v3+) installed.
-   Cluster-admin permissions or equivalent RBAC to create Namespaces, CRDs, and cluster-wide RBAC.

---

## 2. Prepare the Infrastructure Project

All operator components and infrastructure (MariaDB, Redis) should reside in a dedicated namespace to maintain clean security boundaries.

```bash
oc new-project frappe-operator-system
```

---

## 3. Install MariaDB Operator (Mandatory)

The Frappe Operator delegates all database lifecycle operations to the [MariaDB Operator](https://mariadb-operator.github.io/mariadb-operator/). This must be installed first.

```bash
# Add the MariaDB Operator repository
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo update

# Install the MariaDB Operator
helm upgrade --install mariadb-operator mariadb-operator/mariadb-operator \
  --namespace frappe-operator-system \
  --set crds.enabled=true \
  --create-namespace \
  --wait
```

---

## 4. Install Frappe Operator

Install the Frappe Operator using the official Helm chart.

```bash
# Add the Frappe Operator repository
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm repo update

# Install the operator (disabling nested dependencies as they were manually installed)
helm upgrade --install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --set mariadb-operator.enabled=false \
  --set keda.enabled=false \
  --wait
```

---

## 5. Security Note: SCC Compatibility

The Frappe Operator is natively compatible with OpenShift's `restricted-v2` Security Context Constraint (SCC).

-   **Dynamic UIDs**: The operator uses `nil` defaults for `runAsUser`, allowing OpenShift to automatically assign a compliant UID.
-   **Filesystem Access**: It uses platform-aware volume permissions for GID 0 compatibility.

> [!TIP]
> For deep technical details on SCC handling, see the [OpenShift Technical Guide](./openshift.md).

---

## 6. Preparing OpenShift-Compatible Container Images

OpenShift requires containers to run as non-root (GID 0). The official images must be patched. This is typically a one-time activity per ERP release.

### 6.1 Base Image – OpenShift Compatibility Patch

Use this container file to create a base image.

```dockerfile
# ── OpenShift-Compatible ERPNext Base Image ──────────────────────────
FROM frappe/erpnext:v15.100.0 AS erpnext

USER root

# 1. Nginx and Permission Fixes for GID 0
RUN rm -fr /etc/nginx/sites-enabled/default && \
    sed -i '/user www-data/d' /etc/nginx/nginx.conf && \
    sed -i 's/listen 80 default_server/listen 8080 default_server/' /etc/nginx/sites-available/default || true && \
    ln -sf /dev/stdout /var/log/nginx/access.log && ln -sf /dev/stderr /var/log/nginx/error.log && \
    touch /run/nginx.pid && \
    chown -R frappe:root /etc/nginx /var/log/nginx /var/lib/nginx /run && \
    chmod -R g=u /etc/nginx /var/log/nginx /var/lib/nginx /run

# 2. Copy Helper Scripts
COPY resources/nginx-template.conf /templates/nginx/frappe.conf.template
COPY resources/nginx-entrypoint.sh /usr/local/bin/nginx-entrypoint.sh
COPY resources/entrypoint.sh /usr/local/bin/entrypoint.sh

# 3. Recursive chown/chmod for the frappe-bench directory
RUN chmod +x /usr/local/bin/nginx-entrypoint.sh /usr/local/bin/entrypoint.sh && \
    if [ -d "/home/frappe/frappe-bench/sites/assets" ]; then \
        mkdir -p /home/frappe/assets_cache; \
        mv /home/frappe/frappe-bench/sites/assets/* /home/frappe/assets_cache/ || true; \
        rm -rf /home/frappe/frappe-bench/sites/assets; \
    fi && \
    mkdir -p /home/frappe/frappe-bench/sites/assets && \
    mkdir -p /home/frappe/assets_cache && \
    chown -R frappe:root /home/frappe/frappe-bench /home/frappe/assets_cache && \
    chmod -R g=u /home/frappe/frappe-bench /home/frappe/assets_cache && \
    chmod g=u /etc/passwd

USER 1000
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/home/frappe/frappe-bench/env/bin/gunicorn", "--chdir=/home/frappe/frappe-bench/sites", "--bind=0.0.0.0:8000", "--threads=4", "--workers=2", "--worker-class=gthread", "frappe.app:application"]
```

#### Build and Push Base Image
```bash
docker build -f Containerfile.base \
  -t your-registry.io/frappe/erpnext-ocp-base:v15 .

docker push your-registry.io/frappe/erpnext-ocp-base:v15
```

---

### 6.2 Custom-Apps Image – Two-Stage Build

To include custom applications, use a two-stage build to keep the runtime image lean.

#### 1. Define apps.json
```json
{
  "my_custom_app": {
    "url": "https://github.com/yourorg/my_custom_app.git",
    "branch": "main"
  },
  "another_app": {
    "url": "https://github.com/yourorg/another_app.git",
    "branch": "version-15"
  }
}
```

#### 2. Create the Custom Containerfile
```dockerfile
# ── Stage 1: Builder ─────────────────────────────────────────────────
FROM ghcr.io/vyogotech/frappe_base:latest AS builder

USER root
RUN apt-get update && apt-get install --no-install-recommends -y \
    git build-essential python3-dev libffi-dev libmariadb-dev jq \
    && npm install -g yarn \
    && rm -rf /var/lib/apt/lists/*

USER 1000
WORKDIR /home/frappe/frappe-bench
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ARG APPS_JSON_BASE64

# Create dummy config for build orchestration
RUN mkdir -p sites && \
    echo '{"socketio_port": 9000}' > sites/common_site_config.json

# Install apps from base64 encoded apps.json
RUN if [ -n "${APPS_JSON_BASE64}" ]; then \
    echo "${APPS_JSON_BASE64}" | base64 -d > apps.json && \
    cat apps.json | jq -r 'to_entries[] | .value.url + (if .value.branch and .value.branch != "" then " --branch " + .value.branch else "" end)' > apps_list.txt && \
    while read app_args; do \
      eval "bench get-app $app_args"; \
    done < apps_list.txt; \
  fi

# Build assets for the new apps
RUN bench build

# ── Stage 2: Final Image (Runtime) ───────────────────────────────────
FROM your-registry.io/frappe/erpnext-ocp-base:v15

USER 1000
WORKDIR /home/frappe/frappe-bench

# Copy apps and pre-built assets from builder
COPY --from=builder --chown=1000:0 /home/frappe/frappe-bench/apps ./apps
COPY --from=builder --chown=1000:0 /home/frappe/frappe-bench/sites/assets /home/frappe/assets_cache

# Register Apps in Python Environment
RUN for app_dir in apps/*; do \
    if [ -d "$app_dir" ]; then \
        if [ -f "$app_dir/setup.py" ] || [ -f "$app_dir/pyproject.toml" ]; then \
            abs_app_path=$(realpath "$app_dir"); \
            ./env/bin/pip install -e "$abs_app_path"; \
        fi \
    fi \
done
```

#### 3. Build and Push Custom Image
```bash
APPS_B64=$(base64 -w 0 apps.json)

docker build --build-arg APPS_JSON_BASE64="${APPS_B64}" -f Containerfile -t your-registry.io/frappe/custom-apps:v15-1.0.0 .

docker push your-registry.io/frappe/custom-apps:v15-1.0.0
```

---

## 7. Deployment Examples

With the infrastructure ready and images built, you can now deploy your Bench and Site.

### Step 7.1: Create MariaDB Shared Instance
Create `mariadb-instance.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: frappe-mariadb-root
  namespace: frappe-operator-system
type: Opaque
stringData:
  password: "StrongRootPassword123"
---
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

### Step 7.2: Create the FrappeBench
A Bench represents your application environment. Create `my-bench.yaml`. **This uses the custom image built in Section 6.2.**

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: prod-bench
  namespace: frappe
spec:
  frappeVersion: "15.0.0"
  imageConfig:
    repository: your-registry.io/frappe/custom-apps
    tag: v15-1.0.0
    pullPolicy: Always
    pullSecrets:
      - name: registry-credentials
  
  componentAutoscaling:
    gunicorn:
      enabled: true
      minReplicas: 2
      maxReplicas: 5
      provider: hpa
      hpa:
        metric: cpu
        targetUtilization: 75
        scaleUpStabilization: 0
        scaleDownStabilization: 60
    nginx:
      enabled: true
      minReplicas: 2
      maxReplicas: 4
      provider: hpa
      hpa:
        metric: cpu
        targetUtilization: 80
        scaleUpStabilization: 0
        scaleDownStabilization: 60
    workerDefault:
      enabled: true
      minReplicas: 1
      maxReplicas: 5
      provider: hpa
      hpa:
        metric: cpu
        targetUtilization: 85
    
    workerShort:
      enabled: false # Static replicas
      staticReplicas: 1
    
    workerLong:
      enabled: false
      staticReplicas: 1
      
    socketio:
      enabled: false
      staticReplicas: 1
  
  storageSize: 5Gi
```

### Step 7.3: Create the FrappeSite
A Site is your actual application instance. Create `my-site.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: prod-site-admin
  namespace: frappe
type: Opaque
stringData:
  password: "AdminPassword123"
---
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: prod-site
  namespace: frappe
spec:
  benchRef: prod-bench # point to the bench created above.
  siteName: prod-site.apps.cluster.example.com  # Must be a valid domain
  adminPasswordSecretRef:
    name: prod-site-admin
    key: password
  dbConfig:
    mode: shared
    mariadbRef:
      name: frappe-mariadb
  routeConfig:
    enabled: true
    termination: edge
```

Apply all manifests:
```bash
oc apply -f mariadb-instance.yaml
oc apply -f my-bench.yaml
oc apply -f my-site.yaml
```

---

## 8. Verification & Troubleshooting

### Check Pods and Jobs
```bash
oc get pods -n frappe-operator-system
oc get jobs -n frappe-operator-system
```

### Access the Site
Once the site phase is `Ready`, retrieve the URL from the OpenShift Route:
```bash
oc get route prod-site -n frappe-operator-system -o jsonpath='{.spec.host}'
```

---

For more details, see:
- [OpenShift Technical Guide](./openshift.md)
- [MariaDB Integration Guide](./MARIADB_INTEGRATION.md)