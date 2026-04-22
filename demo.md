# Frappe Operator on OpenShift (CRC) Demo

This guide walks you through setting up a complete Frappe environment on OpenShift Local (CRC). It includes steps for installing operators, deploying a database, and launching the Frappe Bench and Site.

## 1. Setup Helm Repositories

First, add the required Helm repositories.

```bash
helm repo add mariadb-operator https://mariadb-operator.github.io/mariadb-operator
helm repo add frappe-operator https://rmallam.github.io/frappe-operator/helm-repo
helm repo update
```

---

## 2. Install Operators

### 2.1 MariaDB Operator
Install the MariaDB operator with CRDs enabled.

```bash
helm upgrade --install mariadb-operator mariadb-operator/mariadb-operator \
  --namespace mariadb-operator-system \
  --set crds.enabled=true \
  --create-namespace
```

**Verify Installation:**
```bash
oc get pods -n mariadb-operator-system
```

### 2.2 Frappe Operator
Install the Frappe Operator (KEDA is disabled for this simple demo).

```bash
helm upgrade --install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --set keda.enabled=false \
  --create-namespace
```

**Verify Installation:**
```bash
oc get pods -n frappe-operator-system
```

---

## 3. Deploy Database (MariaDB)

Create the `mariadb` namespace, configure the root password, and deploy the MariaDB instance.

```yaml
apiVersion: project.openshift.io/v1
kind: Project
metadata:
  name: mariadb
---
apiVersion: v1
kind: Secret
metadata:
  name: mariadb-root-password
  namespace: mariadb
type: Opaque
stringData:
  password: frappe
---
apiVersion: k8s.mariadb.com/v1alpha1
kind: MariaDB
metadata:
  name: frappe-mariadb
  namespace: mariadb
spec:
  rootPasswordSecretKeyRef:
    name: mariadb-root-password
    key: password
  image: mariadb:10.11
  storage:
    size: 2Gi
  replicas: 1
```

**Verify Database Status:**
```bash
oc get pods -n mariadb
oc get mariadb -n mariadb
```
*Wait until the MariaDB status is `Ready`.*

---

## 4. Deploy Frappe Bench

This creates the `frappe` namespace and the Frappe Bench (the cluster environment).

> **Note:** This configuration uses the standard `crc-csi-hostpath-provisioner` storage class (RWO), which is fine for a single-replica demo. For production or scaling, use a **ReadWriteMany (RWX)** class like `openebs-kernel-nfs`.

```yaml
apiVersion: project.openshift.io/v1
kind: Project
metadata:
  name: frappe
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: bench-test
  namespace: frappe
spec:
  frappeVersion: "15.0.0"
  imageConfig:
    repository: ghcr.io/rmallam/frappe_docker
    tag: sha-7bc7484
    pullPolicy: Always
  storageClassName: crc-csi-hostpath-provisioner
  storageSize: 2Gi

  # Apps to install
  apps:
    - name: frappe
      source: image
    - name: erpnext
      source: image

  # Component replicas (minimal for development)
  componentReplicas:
    gunicorn: 1
    nginx: 1
    socketio: 1
    scheduler: 0
    workers:
      default: 1
      short: 0
      long: 0
```

**Verify Bench Creation:**
```bash
oc get frappebench -n frappe
oc get pods -n frappe
```

---

## 5. Deploy Frappe Site

Deploy the actual Frappe Site and connect it to the MariaDB database.

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: vyogotechdemo
  namespace: frappe
spec:
  siteName: demo.vyogo.apps-crc.testing
  benchRef:
    name: bench-test
    namespace: frappe

  # Database configuration
  dbConfig:
    provider: mariadb
    mode: shared
    mariadbRef:
      name: frappe-mariadb
      namespace: mariadb

  # Ingress configuration for OpenShift
  ingress:
    enabled: true
    className: openshift-default
```

**Verify Site Status:**
```bash
oc get frappesite -n frappe
oc get pods -n frappe
```

Once the site status is `Ready`, you can access it at `http://demo.vyogo.apps-crc.testing`.