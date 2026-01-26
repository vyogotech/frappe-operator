# Frappe Operator User Guide

This section describes how to deploy a Frappe Bench and a Frappe Site using the Frappe Operator.

## 1. Deploying a Frappe Bench

The `FrappeBench` custom resource represents a cluster of Frappe components (Workers, Scheduler, Redis, SocketIO, etc.).

### Manifest Example (`deploy/bench.yaml`)

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
  namespace: default
  annotations:
    # Use RWO if your storage class doesn't support RWX
    frappe.tech/storage-access-mode: ReadWriteOnce
spec:
  frappeVersion: "v15.0.0"
  
  # Image Configuration
  imageConfig:
    repository: ghcr.io/rmallam/frappe_docker
    tag: latest
    pullPolicy: Always

  # Apps to install
  apps:
    - name: frappe
      source: image
    - name: erpnext
      source: image
  
  # Component Replicas
  componentReplicas:
    gunicorn: 1
    nginx: 1
    socketio: 1
    scheduler: 1
    workers:
      default: 1
  
  # Storage
  storageSize: "10Gi"
  
  # Security Context (Critical for OpenShift)
  security:
    podSecurityContext:
      fsGroup: 0  # Ensures shared volumes are writable by the group
```

### Apply the Bench

```bash
kubectl apply -f deploy/bench.yaml
```

### Verification

Check that all bench pods are running:

```bash
kubectl get pods -l bench=my-bench
```

## 2. Deploying a Frappe Site

The `FrappeSite` custom resource represents a tenant or a specific site within the Bench.

### Manifest Example (`deploy/site.yaml`)

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: my-site
  namespace: default
spec:
  benchRef:
    name: my-bench
    namespace: default
  
  # The domain name for the site
  siteName: my-site.localhost
  
  # Database Configuration
  dbConfig:
    provider: mariadb
    mode: shared
    mariadbRef:
      name: frappe-mariadb
      namespace: frappe-operator-system
  
  # Ingress Configuration
  ingress:
    enabled: true
    # Use 'openshift-default' for OpenShift, or 'nginx' for generic K8s
    className: openshift-default
```

### Apply the Site

```bash
kubectl apply -f deploy/site.yaml
```

### Verification

Check the site status:

```bash
kubectl get frappesite my-site
```

The status should eventually change to `Ready`.

## Troubleshooting

### Site Initialization Stuck
If the `site-init` job is stuck in `ContainerCreating` or fails with "Secret not found":
1. Check if the operator created the secret: `kubectl get secrets`
2. If `my-site-init-secrets` is missing, check the operator logs.

### "No such file or directory" for Secrets
If you see errors like `cat: /run/secrets/site_name: No such file or directory`:
- This indicates the Operator container image expects secrets at `/run/secrets` but the platform (OpenShift) might be mounting them elsewhere or they are hidden.
- **Workaround (for current image version)**: Manually create the initialization job with corrected mount paths (see troubleshooting docs or ask support).
