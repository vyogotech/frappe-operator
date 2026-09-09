# Getting Started with Frappe Operator

This guide walks you through installing the **Frappe Operator** by **Vyogo Technologies** and deploying your first Frappe/ERPNext site in 5 minutes.

---

## Prerequisites

Before you begin, ensure you have:

- **Kubernetes Cluster** (v1.22+) or **OpenShift** (4.10+)
  - [KIND](https://kind.sigs.k8s.io/) or [Minikube](https://minikube.sigs.k8s.io/) for local development
  - OpenShift Local (CRC), Red Hat OpenShift on AWS (ROSA), or public cloud (EKS, GKE, AKS)
- **kubectl** or **oc** CLI configured to access your cluster
- **Helm 3.x** installed

---

## 1. Database Backend (Choose One)

The Frappe Operator features a polymorphic database architecture supporting multiple database engines:

| Engine | Recommended Provider | Documentation |
|---|---|---|
| **PostgreSQL** (Default) | [StackGres Operator](https://stackgres.io) or [Percona PG Operator](https://docs.percona.com/percona-operator-for-postgresql/2.0/) | [PostgreSQL Integration Guide](POSTGRESQL_INTEGRATION.md) |
| **MariaDB** | [MariaDB Operator](https://github.com/mariadb-operator/mariadb-operator) | [MariaDB Integration Guide](MARIADB_INTEGRATION.md) |
| **External Database** | AWS RDS, Google Cloud SQL, Azure Database, or on-premise | [External Resources](external-resources.md) |

For quick testing with PostgreSQL via StackGres:
```bash
# Add StackGres Helm repo and install operator
helm repo add stackgres https://stackgres.io/downloads/stackgres-k8s/stackgres/helm
helm repo update
helm install stackgres-operator stackgres/stackgres-operator \
  --namespace stackgres --create-namespace
```

---

## 2. Install Frappe Operator

Install the operator using the official Vyogo Technologies Helm repository:

```bash
# Add the official Vyogo Helm repository
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm repo update

# Install the operator into frappe-operator-system namespace
helm install frappe-operator frappe-operator/frappe-operator \
  --namespace frappe-operator-system \
  --create-namespace
```

### Verify Installation

Verify that the controller manager pod is running:

```bash
kubectl get pods -n frappe-operator-system
```

Verify that the CRDs are registered:

```bash
kubectl get crd | grep vyogo.tech
```

The operator registers all 24 `vyogo.tech/v1` Custom Resource Definitions, including:
- `frappebenches.vyogo.tech`
- `frappesites.vyogo.tech`
- `sitedomains.vyogo.tech`
- `siteapps.vyogo.tech`
- `sitebackups.vyogo.tech`
- `sitejobs.vyogo.tech`

---

## 3. Deploy Your First Bench and Site

### Step 3.1: Deploy the FrappeBench (Shared Infrastructure)

The `FrappeBench` represents shared infrastructure (Nginx, Gunicorn, Redis Cache/Queue, SocketIO, Scheduler, and background workers):

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: prod-bench
  namespace: default
spec:
  frappeVersion: "version-15"
  imageConfig:
    repository: ghcr.io/vyogotech/erpnext-for-operator
    tag: version-15
    pullPolicy: IfNotPresent
  apps:
    - name: erpnext
```

Apply the manifest:
```bash
kubectl apply -f prod-bench.yaml
kubectl wait --for=condition=Ready frappebench/prod-bench --timeout=300s
```

---

### Step 3.2: Deploy the FrappeSite (Tenant Site)

Create a dedicated PostgreSQL tenant site using StackGres:

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: my-first-site
  namespace: default
spec:
  benchRef:
    name: prod-bench
  siteName: my-first-site.example.com
  dbConfig:
    provider: postgres
    mode: dedicated
    postgresEngine: stackgres
  deletionPolicy: Delete
```

Apply the manifest:
```bash
kubectl apply -f my-first-site.yaml
kubectl wait --for=condition=Ready frappesite/my-first-site --timeout=300s
```

---

## 4. Accessing Your Site

### OpenShift
On OpenShift, an OpenShift Route (`<site-name>-route`) is automatically provisioned with edge TLS termination and HTTP-to-HTTPS redirect (`tls.insecureEdgeTerminationPolicy: Redirect`):

```bash
oc get routes -l site=my-first-site
```

### Kubernetes (Port-Forward for Local Development)
```bash
# Forward traffic to the bench nginx service
kubectl port-forward svc/prod-bench-nginx 8080:8080

# Add host entry in /etc/hosts:
# 127.0.0.1 my-first-site.example.com

# Access in browser:
# http://my-first-site.example.com:8080
```

---

## 5. Security & OpenShift Compliance

Frappe Operator is engineered for strict enterprise compliance:

- **OpenShift `restricted-v2` SCC**: Default configurations use dynamic non-root UIDs allocated by OpenShift (`runAsUser: nil`), dynamic filesystem groups, and drop all Linux capabilities (`drop: ["ALL"]`). No cluster-admin or root privileges are required.
- **HTTPS Enforcement**: Ingresses and OpenShift Routes strictly enforce HTTPS redirection.
- **Database Credential Isolation**: Site credentials and passwords are automatically generated and mounted via Kubernetes Secrets.

---

## Next Steps

- **[Comprehensive Guide](COMPREHENSIVE_GUIDE.md)**: Explore high-availability production configurations.
- **[PostgreSQL Integration Guide](POSTGRESQL_INTEGRATION.md)**: Sizing, pooling, and StackGres/Percona features.
- **[Operations Guide](operations.md)**: Site backups, autoscaling with HPA/KEDA, and rolling updates.
