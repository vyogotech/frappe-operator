# Operations Guide

Day-2 operations, maintenance, and best practices for running Frappe Operator in production.

## Table of Contents

- [Production Deployment](#production-deployment)
- [Monitoring and Observability](#monitoring-and-observability)
- [Backup and Restore](#backup-and-restore)
- [Scaling](#scaling)
- [Updates and Upgrades](#updates-and-upgrades)
- [Security](#security)
- [Database Management](#database-management)
- [Disaster Recovery](#disaster-recovery)

---

## Production Deployment

### Pre-Production Checklist

Before deploying to production, ensure you have:

- [ ] Kubernetes cluster with adequate resources
- [ ] Persistent storage configured (StorageClass)
- [ ] Ingress controller installed and configured
- [ ] cert-manager for TLS certificates (optional)
- [ ] MariaDB/MySQL database available
- [ ] Monitoring stack (Prometheus + Grafana)
- [ ] Backup solution in place
- [ ] DNS configured for your domains
- [ ] Resource limits defined
- [ ] Secrets management strategy

### Resource Planning

#### Bench Resources

Estimate resources based on expected load:

**Small (< 50 users):**
- Gunicorn: 2 replicas, 500m CPU, 1Gi RAM each
- Workers: 1-2 replicas, 250m CPU, 512Mi RAM each
- Redis: 1Gi RAM

**Medium (50-200 users):**
- Gunicorn: 3-5 replicas, 1 CPU, 2Gi RAM each
- Workers: 2-3 replicas, 500m CPU, 1Gi RAM each
- Redis: 4Gi RAM

**Large (200+ users):**
- Gunicorn: 5-10+ replicas, 2 CPU, 4Gi RAM each
- Workers: 5+ replicas, 1 CPU, 2Gi RAM each
- Redis: 8Gi+ RAM

#### Database Resources

**Shared Database:**
- 2-4 CPU cores
- 4-8Gi RAM
- 100Gi+ storage

**Dedicated Database (per site):**
- 1-2 CPU cores
- 2-4Gi RAM
- 50-200Gi storage

### MariaDB Operator Setup

For production, use MariaDB Operator for managed databases:

```bash
# Install MariaDB Operator
kubectl apply -f https://github.com/mariadb-operator/mariadb-operator/releases/latest/download/crds.yaml
kubectl apply -f https://github.com/mariadb-operator/mariadb-operator/releases/latest/download/mariadb-operator.yaml
```

Create a shared MariaDB instance:

```yaml
apiVersion: k8s.mariadb.com/v1alpha1
kind: MariaDB
metadata:
  name: shared-mariadb
  namespace: databases
spec:
  rootPasswordSecretKeyRef:
    name: mariadb-root
    key: password
  
  database: frappe
  username: frappe
  passwordSecretKeyRef:
    name: mariadb-frappe
    key: password
  
  storage:
    size: 500Gi
    storageClassName: fast-ssd
  
  replicas: 3
  galera:
    enabled: true
  
  resources:
    requests:
      cpu: 2
      memory: 8Gi
    limits:
      cpu: 4
      memory: 16Gi
```

### Namespace Strategy

Organize resources by environment:

```bash
# Create namespaces
kubectl create namespace frappe-operator-system
kubectl create namespace production
kubectl create namespace staging
kubectl create namespace development
kubectl create namespace databases

# Apply resource quotas
kubectl create quota production-quota \
  --hard=requests.cpu=50,requests.memory=100Gi,pods=100 \
  -n production
```

---

## Monitoring and Observability

### Prometheus Integration

The operator exposes metrics for Prometheus scraping.

**ServiceMonitor for Operator:**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: frappe-operator
  namespace: frappe-operator-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
  - port: https
    scheme: https
    tlsConfig:
      insecureSkipVerify: true
```

**PodMonitor for Frappe Components:**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: frappe-benches
  namespace: production
spec:
  selector:
    matchLabels:
      app: frappe
  podMetricsEndpoints:
  - port: metrics
    path: /metrics
```

### Key Metrics to Monitor

**Application Metrics:**
- Request rate and latency
- Error rate (4xx, 5xx)
- Queue length (Redis)
- Worker job processing time
- Database connections

**Resource Metrics:**
- CPU utilization
- Memory usage
- Disk I/O
- Network throughput

**Business Metrics:**
- Active users
- Concurrent sessions
- Background job completion rate

### Logging

Configure centralized logging:

```yaml
# Fluent Bit DaemonSet
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
  namespace: logging
data:
  fluent-bit.conf: |
    [INPUT]
        Name              tail
        Path              /var/log/containers/frappe-*.log
        Parser            docker
        Tag               frappe.*
    
    [OUTPUT]
        Name              es
        Match             frappe.*
        Host              elasticsearch
        Port              9200
        Index             frappe-logs
```

### Alerting Rules

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: frappe-alerts
  namespace: production
spec:
  groups:
  - name: frappe
    interval: 30s
    rules:
    - alert: FrappeSiteDown
      expr: up{job="frappe-site"} == 0
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "Frappe site {{ $labels.site }} is down"
    
    - alert: HighErrorRate
      expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "High error rate on {{ $labels.site }}"
    
    - alert: HighMemoryUsage
      expr: container_memory_usage_bytes{pod=~".*-gunicorn.*"} / container_spec_memory_limit_bytes > 0.9
      for: 10m
      labels:
        severity: warning
      annotations:
        summary: "High memory usage on {{ $labels.pod }}"
```

---

## Backup and Restore

### Automated Backups with SiteBackup

Create a SiteBackup resource:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteBackup
metadata:
  name: daily-backup
  namespace: production
spec:
  siteRef:
    name: prod-site
  
  # Daily backup at 2 AM
  schedule: "0 2 * * *"
  
  retention:
    days: 30
    count: 90
  
  destination:
    type: s3
    config:
      bucket: frappe-backups
      region: us-east-1
      credentialsSecret: aws-s3-credentials
```

### Manual Backup

```bash
# Create manual backup
kubectl create -f - <<EOF
apiVersion: vyogo.tech/v1alpha1
kind: SiteJob
metadata:
  name: manual-backup-$(date +%Y%m%d-%H%M%S)
  namespace: production
spec:
  siteRef:
    name: prod-site
  jobType: backup
  jobConfig:
    withFiles: true
    compress: true
EOF

# Check backup status
kubectl get sitejob -n production
```

### Restore from Backup

```bash
# Restore database
kubectl exec -it <site-pod> -- bench --site <site-name> \
  restore --mariadb-root-password <password> /path/to/backup.sql.gz

# Restore files
kubectl exec -it <site-pod> -- bench --site <site-name> \
  restore --with-private-files /path/to/files-backup.tar.gz
```

---

## Scaling

### Manual Scaling

Scale components manually:

```bash
# Scale gunicorn replicas
kubectl patch frappebench prod-bench --type=merge -p '{
  "spec": {
    "componentReplicas": {
      "gunicorn": 5,
      "workerDefault": 3
    }
  }
}'

# Verify scaling
kubectl get pods -l bench=prod-bench
```

### Horizontal Pod Autoscaling

Enable HPA for automatic scaling:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: prod-bench-gunicorn-hpa
  namespace: production
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: prod-bench-gunicorn
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
```

### Vertical Scaling

Update resource limits:

```bash
kubectl patch frappebench prod-bench --type=merge -p '{
  "spec": {
    "componentResources": {
      "gunicorn": {
        "requests": {"cpu": "2", "memory": "4Gi"},
        "limits": {"cpu": "4", "memory": "8Gi"}
      }
    }
  }
}'
```

---

## Updates and Upgrades

### Updating Frappe Version

```bash
# Update bench version
kubectl patch frappebench prod-bench --type=merge -p '{
  "spec": {
    "frappeVersion": "version-15",
    "imageConfig": {
      "tag": "v15.1.0"
    }
  }
}'

# Monitor rollout
kubectl rollout status deployment/prod-bench-gunicorn -n production
```

### App Updates

```bash
# Update apps
kubectl patch frappebench prod-bench --type=merge -p '{
  "spec": {
    "appsJSON": "[\"erpnext\", \"hrms\", \"custom_app@v2.0.0\"]"
  }
}'
```

### Site Migration

Run migrations after updates:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteJob
metadata:
  name: migrate-prod-site
  namespace: production
spec:
  siteRef:
    name: prod-site
  jobType: migrate
```

### Operator Upgrade

```bash
# Upgrade operator
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/v1.1.0/config/install.yaml

# Verify upgrade
kubectl get deployment -n frappe-operator-system
```

---

## Security

### Network Policies

Restrict network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: frappe-network-policy
  namespace: production
spec:
  podSelector:
    matchLabels:
      app: frappe
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
    ports:
    - protocol: TCP
      port: 8000
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: databases
    ports:
    - protocol: TCP
      port: 3306
```

### Pod Security Standards

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

### Secrets Management

Use external secrets operator:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: vault-backend
  namespace: production
spec:
  provider:
    vault:
      server: "https://vault.example.com"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "frappe-operator"

---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: prod-site-admin-password
  namespace: production
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: prod-site-admin-password
  data:
  - secretKey: password
    remoteRef:
      key: prod-site
      property: admin_password
```

---

## Database Management

### Connection Pooling

Configure ProxySQL for connection pooling:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: proxysql
  namespace: databases
spec:
  replicas: 2
  selector:
    matchLabels:
      app: proxysql
  template:
    metadata:
      labels:
        app: proxysql
    spec:
      containers:
      - name: proxysql
        image: proxysql/proxysql:2.5
        ports:
        - containerPort: 6033
        - containerPort: 6032
```

### Database Maintenance

```bash
# Optimize tables
kubectl exec -it mariadb-0 -n databases -- \
  mysql -u root -p -e "OPTIMIZE TABLE <database>.<table>;"

# Check database size
kubectl exec -it mariadb-0 -n databases -- \
  mysql -u root -p -e "SELECT table_schema AS 'Database', 
  ROUND(SUM(data_length + index_length) / 1024 / 1024, 2) AS 'Size (MB)' 
  FROM information_schema.tables GROUP BY table_schema;"
```

---

## Disaster Recovery

### Backup Strategy

1. **Database Backups**: Daily full backups, hourly incrementals
2. **File Backups**: Daily backups of site files
3. **Configuration Backups**: Version control for manifests
4. **Off-site Replication**: Store backups in different region

### Recovery Procedures

#### Complete Cluster Failure

```bash
# 1. Restore database from backup
kubectl apply -f mariadb-restore.yaml

# 2. Recreate operator
kubectl apply -f https://raw.githubusercontent.com/vyogotech/frappe-operator/main/config/install.yaml

# 3. Recreate bench
kubectl apply -f bench.yaml

# 4. Recreate sites
kubectl apply -f sites/

# 5. Restore site data
kubectl exec -it <pod> -- bench --site <site> restore <backup>
```

#### Site Recovery

```bash
# Create new site from backup
kubectl apply -f - <<EOF
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: recovered-site
spec:
  benchRef:
    name: prod-bench
  siteName: "site.example.com"
  restoreFrom:
    backup: "s3://bucket/backup.sql.gz"
    files: "s3://bucket/files.tar.gz"
EOF
```

---

## Best Practices

### 1. Use GitOps

Store all manifests in Git and use tools like ArgoCD or Flux:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: frappe-production
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/yourorg/frappe-k8s
    targetRevision: main
    path: production
  destination:
    server: https://kubernetes.default.svc
    namespace: production
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

### 2. Resource Quotas

Set limits per namespace:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: production-quota
  namespace: production
spec:
  hard:
    requests.cpu: "50"
    requests.memory: 100Gi
    persistentvolumeclaims: "20"
    pods: "100"
```

### 3. Pod Disruption Budgets

Ensure availability during maintenance:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: prod-bench-gunicorn-pdb
  namespace: production
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: prod-bench-gunicorn
```

### 4. Health Checks

Configure liveness and readiness probes (handled by operator).

### 5. Regular Testing

- Test disaster recovery procedures quarterly
- Validate backups monthly
- Performance test before major updates
- Security audits semi-annually

---

## Maintenance Windows

### Planning Maintenance

1. **Schedule**: Off-peak hours
2. **Communication**: Notify users in advance
3. **Backups**: Take fresh backups before maintenance
4. **Rollback Plan**: Prepare rollback procedures
5. **Monitoring**: Extra vigilance during and after

### Performing Maintenance

```bash
# 1. Drain traffic (if using multiple sites)
kubectl patch frappesite <site> --type=merge -p '{
  "spec": {"ingress": {"enabled": false}}
}'

# 2. Perform maintenance
kubectl apply -f updated-manifests.yaml

# 3. Verify
kubectl get pods -n production
kubectl logs -l app=<component>

# 4. Re-enable traffic
kubectl patch frappesite <site> --type=merge -p '{
  "spec": {"ingress": {"enabled": true}}
}'
```

---

## Next Steps

- **[Troubleshooting](troubleshooting.md)** - Debugging and problem resolution
- **[API Reference](api-reference.md)** - Complete specification
- **[Examples](examples.md)** - Configuration examples

