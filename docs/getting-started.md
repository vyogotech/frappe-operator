# Getting Started

This guide will help you install the Frappe Operator and deploy your first Frappe site.

## Prerequisites

Before you begin, ensure you have:

- **Kubernetes cluster** (1.19+)
  - [KIND](https://kind.sigs.k8s.io/) for local development
  - [Minikube](https://minikube.sigs.k8s.io/) as an alternative
  - Or any cloud Kubernetes service (EKS, GKE, AKS)
- **kubectl** configured to access your cluster
- **At least 4GB RAM** available in your cluster

### Required Dependencies (v1.0.0+)

- **MariaDB Operator** - For secure, declarative database provisioning
  - Install from: https://github.com/mariadb-operator/mariadb-operator

### Optional Dependencies

- **Ingress Controller** - For external access (nginx, traefik)
- **cert-manager** - For automatic TLS certificate management

## Installation

### Step 1: Install Frappe Operator

Install the operator and its CRDs:

```bash
# Install CRDs and operator
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/config/install.yaml

# Verify installation
kubectl get deployment -n frappe-operator-system
kubectl get crd | grep vyogo.tech
```

You should see the following CRDs:
- `frappebenchs.vyogo.tech`
- `frappesites.vyogo.tech`
- `siteusers.vyogo.tech`
- `siteworkspaces.vyogo.tech`
- `sitedashboards.vyogo.tech`
- `sitedashboardcharts.vyogo.tech`
- `sitebackups.vyogo.tech`
- `sitejobs.vyogo.tech`

### Step 2: Install MariaDB (For Development)

For a quick development setup, install a simple MariaDB instance:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mariadb
  namespace: default
spec:
  serviceName: mariadb
  replicas: 1
  selector:
    matchLabels:
      app: mariadb
  template:
    metadata:
      labels:
        app: mariadb
    spec:
      containers:
      - name: mariadb
        image: mariadb:10.6
        env:
        - name: MYSQL_ROOT_PASSWORD
          value: "admin"
        ports:
        - containerPort: 3306
          name: mysql
        volumeMounts:
        - name: mariadb-data
          mountPath: /var/lib/mysql
  volumeClaimTemplates:
  - metadata:
      name: mariadb-data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: mariadb
  namespace: default
spec:
  type: ClusterIP
  selector:
    app: mariadb
  ports:
  - port: 3306
    targetPort: 3306
EOF
```

**For production**, use [MariaDB Operator](operations.md#mariadb-operator-setup) instead.

### Step 3: Install Ingress Controller (Optional)

If you want external access to your sites:

```bash
# Install NGINX Ingress Controller
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/cloud/deploy.yaml

# Wait for it to be ready
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s
```

## Deploying Your First Site

### Method 1: Minimal Setup (Recommended for First-Time Users)

Create a file named `my-first-site.yaml`:

```yaml
---
# FrappeBench: Shared infrastructure
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: dev-bench
  namespace: default
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext"]'

---
# FrappeSite: Your site
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: mysite
  namespace: default
spec:
  benchRef:
    name: dev-bench
  siteName: "mysite.local"
  dbConfig:
    mode: shared
```

Deploy it:

```bash
# Apply the manifest
kubectl apply -f my-first-site.yaml

# Watch the resources being created
kubectl get frappebench,frappesite -w
```

### Method 2: Using Example Manifests

```bash
# Download and apply the minimal example
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/examples/minimal-bench-and-site.yaml

# Check status
kubectl get frappebench,frappesite
```

### Understanding What Gets Created

When you deploy a bench and site, the operator creates:

**For FrappeBench:**
- Deployments: nginx, redis/dragonfly
- Services: for internal communication
- ConfigMaps: Frappe configuration
- Jobs: bench initialization

**For FrappeSite:**
- Deployments: gunicorn, socketio, scheduler, workers
- Services: gunicorn, socketio
- Ingress: (if enabled) external access
- Jobs: site creation, migration
- PVCs: site files and logs storage
- Secrets: database credentials, admin password

## Checking Deployment Status

### Check Custom Resources

```bash
# Check bench status
kubectl get frappebench dev-bench -o yaml

# Check site status  
kubectl get frappesite mysite -o yaml

# View all Frappe resources
kubectl get frappebench,frappesite
```

### Check Kubernetes Resources

```bash
# View all pods
kubectl get pods

# Check deployments
kubectl get deployments

# Check services
kubectl get services

# Check jobs
kubectl get jobs
```

### Watch Logs

```bash
# Bench initialization
kubectl logs -l job-name=dev-bench-init -f

# Site initialization
kubectl logs -l job-name=mysite-init -f

# Application logs
kubectl logs -l app=mysite-gunicorn -f
```

## Accessing Your Site

### Method 1: Port Forwarding (Easiest for Testing)

```bash
# Forward nginx service to localhost
kubectl port-forward service/dev-bench-nginx 8080:8080

# Add to /etc/hosts (Linux/Mac)
echo "127.0.0.1 mysite.local" | sudo tee -a /etc/hosts

# Or on Windows (as Administrator)
# echo 127.0.0.1 mysite.local >> C:\Windows\System32\drivers\etc\hosts

# Access in browser
# http://mysite.local:8080
```

### Method 2: Using Ingress (Production)

If you have an ingress controller and proper DNS:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: mysite
spec:
  benchRef:
    name: dev-bench
  siteName: "mysite.example.com"
  domain: "mysite.example.com"
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"
  dbConfig:
    mode: shared
```

Access at: `https://mysite.example.com`

### Method 3: NodePort (Development)

For local development without DNS:

```bash
# Expose nginx service as NodePort
kubectl patch service dev-bench-nginx -p '{"spec":{"type":"NodePort"}}'

# Get the NodePort
kubectl get service dev-bench-nginx

# Access via NodePort
# http://<node-ip>:<node-port>
```

## Default Credentials

After site initialization, use these default credentials:

- **Username**: `Administrator`
- **Password**: `admin` (or check the secret if custom password was set)

```bash
# Get admin password from secret (if configured)
kubectl get secret mysite-admin-password -o jsonpath='{.data.password}' | base64 -d
```

## Verifying the Installation

### 1. Check Bench Status

```bash
kubectl get frappebench dev-bench -o jsonpath='{.status.ready}'
# Should return: true
```

### 2. Check Site Status

```bash
kubectl get frappesite mysite -o jsonpath='{.status.phase}'
# Should return: Ready
```

### 3. Test Site Access

```bash
# Get site URL
kubectl get frappesite mysite -o jsonpath='{.status.siteURL}'

# Test with curl
curl -H "Host: mysite.local" http://localhost:8080/api/method/ping
# Should return: {"message":"pong"}
```

### 4. Check Component Health

```bash
# All pods should be running
kubectl get pods -l bench=dev-bench
kubectl get pods -l site=mysite

# Check endpoints
kubectl get endpoints
```

## Common Initial Setup Tasks

### Change Admin Password

```bash
# Create a password secret
kubectl create secret generic mysite-admin-pwd \
  --from-literal=password='YourSecurePassword123!'

# Reference it in FrappeSite
kubectl patch frappesite mysite --type=merge -p '{
  "spec": {
    "adminPasswordSecretRef": {
      "name": "mysite-admin-pwd"
    }
  }
}'
```

### Install Additional Apps

```bash
# Update bench with more apps
kubectl patch frappebench dev-bench --type=merge -p '{
  "spec": {
    "appsJSON": "[\"erpnext\", \"hrms\", \"custom_app\"]"
  }
}'

# The operator will handle the update
```

### Scale Components

```bash
# Scale gunicorn replicas manually
kubectl patch frappebench dev-bench --type=merge -p '{
  "spec": {
    "componentReplicas": {
      "gunicorn": 3,
      "workerDefault": 2
    }
  }
}'
```

### Enable Worker Autoscaling (NEW)

For production workloads, enable KEDA-based autoscaling to automatically scale workers based on queue length:

```bash
kubectl patch frappebench dev-bench --type=merge -p '{
  "spec": {
    "workerAutoscaling": {
      "short": {
        "enabled": true,
        "minReplicas": 0,
        "maxReplicas": 10,
        "queueLength": 2
      },
      "long": {
        "enabled": true,
        "minReplicas": 1,
        "maxReplicas": 5,
        "queueLength": 5
      },
      "default": {
        "enabled": false,
        "staticReplicas": 2
      }
    }
  }
}'

# Check autoscaling status
kubectl get scaledobjects
kubectl get frappebench dev-bench -o jsonpath='{.status.workerScaling}' | jq
```

**Benefits:**
- Workers scale to zero when idle (save costs)
- Auto-scale based on actual job queue length
- Handle traffic spikes automatically
- Fine-tune scaling per queue type

For more details, see [Worker Autoscaling](operations.md#worker-autoscaling-with-keda-recommended).

## Next Steps

Now that you have a working installation:

1. **[Learn about concepts](concepts.md)** - Understanding benches, sites, and architecture
2. **[Explore examples](examples.md)** - Common deployment patterns
3. **[Configure for production](operations.md#production-deployment)** - Best practices
4. **[Set up monitoring](operations.md#monitoring)** - Observability
5. **[Configure backups](operations.md#backups)** - Data protection

## Troubleshooting

If something goes wrong, check:

1. **Operator logs**:
   ```bash
   kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager
   ```

2. **Resource events**:
   ```bash
   kubectl describe frappebench dev-bench
   kubectl describe frappesite mysite
   ```

3. **Pod logs**:
   ```bash
   kubectl logs <pod-name>
   ```

For more detailed troubleshooting, see the [Troubleshooting Guide](troubleshooting.md).

## Clean Up

To remove everything:

```bash
# Delete site
kubectl delete frappesite mysite

# Delete bench
kubectl delete frappebench dev-bench

# Delete MariaDB (if you created it)
kubectl delete statefulset mariadb
kubectl delete service mariadb

# Uninstall operator (optional)
kubectl delete -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/config/install.yaml
```

