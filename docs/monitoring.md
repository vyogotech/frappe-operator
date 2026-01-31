# Monitoring Guide

This guide covers monitoring the Frappe Operator and Frappe workloads.

## Table of Contents

- [Prometheus Metrics](#prometheus-metrics)
- [Grafana Dashboards](#grafana-dashboards)
- [Alert Rules](#alert-rules)
- [Logging](#logging)
- [Health Checks](#health-checks)

## Prometheus Metrics

The Frappe Operator exposes Prometheus metrics on port 8080 at `/metrics`.

### Available Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `frappe_operator_reconciliation_duration_seconds` | Histogram | `controller`, `result` | Time spent reconciling resources |
| `frappe_operator_reconciliation_errors_total` | Counter | `controller`, `error_type` | Total number of reconciliation errors |
| `frappe_operator_job_status` | Gauge | `job_name`, `namespace`, `status` | Current status of operator jobs |
| `frappe_operator_resource_total` | Gauge | `resource_type`, `namespace` | Total count of managed resources |

### Enabling Metrics

Metrics are enabled by default. The ServiceMonitor is created if Prometheus Operator is detected.

```yaml
# Custom ServiceMonitor
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: frappe-operator-metrics
  namespace: frappe-operator-system
  labels:
    control-plane: controller-manager
spec:
  endpoints:
  - port: https
    scheme: https
    bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    tlsConfig:
      insecureSkipVerify: true
    interval: 30s
  selector:
    matchLabels:
      control-plane: controller-manager
```

### Scraping Without Prometheus Operator

```yaml
# Prometheus scrape config
scrape_configs:
  - job_name: 'frappe-operator'
    kubernetes_sd_configs:
      - role: endpoints
        namespaces:
          names:
            - frappe-operator-system
    relabel_configs:
      - source_labels: [__meta_kubernetes_service_label_control_plane]
        action: keep
        regex: controller-manager
      - source_labels: [__meta_kubernetes_endpoint_port_name]
        action: keep
        regex: https
```

## Grafana Dashboards

### Operator Dashboard

A standalone dashboard JSON is available at [docs/grafana-dashboard.json](grafana-dashboard.json) for one-click import. Alternatively, use the embedded JSON below.

```json
{
  "annotations": {
    "list": []
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 0,
  "id": null,
  "links": [],
  "panels": [
    {
      "datasource": {
        "type": "prometheus",
        "uid": "prometheus"
      },
      "fieldConfig": {
        "defaults": {
          "color": {
            "mode": "palette-classic"
          },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 80 }
            ]
          }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 0 },
      "id": 1,
      "options": {
        "legend": { "calcs": [], "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "single" }
      },
      "targets": [
        {
          "expr": "rate(frappe_operator_reconciliation_duration_seconds_sum[5m]) / rate(frappe_operator_reconciliation_duration_seconds_count[5m])",
          "legendFormat": "{{controller}}",
          "refId": "A"
        }
      ],
      "title": "Reconciliation Duration (avg)",
      "type": "timeseries"
    },
    {
      "datasource": {
        "type": "prometheus",
        "uid": "prometheus"
      },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "yellow", "value": 1 },
              { "color": "red", "value": 5 }
            ]
          }
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 0 },
      "id": 2,
      "options": {
        "legend": { "calcs": [], "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "single" }
      },
      "targets": [
        {
          "expr": "rate(frappe_operator_reconciliation_errors_total[5m])",
          "legendFormat": "{{controller}} - {{error_type}}",
          "refId": "A"
        }
      ],
      "title": "Reconciliation Errors",
      "type": "timeseries"
    },
    {
      "datasource": {
        "type": "prometheus",
        "uid": "prometheus"
      },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null }
            ]
          }
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 8 },
      "id": 3,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": {
          "calcs": ["lastNotNull"],
          "fields": "",
          "values": false
        },
        "textMode": "auto"
      },
      "targets": [
        {
          "expr": "sum(frappe_operator_resource_total{resource_type=\"frappebench\"})",
          "refId": "A"
        }
      ],
      "title": "Total Benches",
      "type": "stat"
    },
    {
      "datasource": {
        "type": "prometheus",
        "uid": "prometheus"
      },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "thresholds" },
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null }
            ]
          }
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 8 },
      "id": 4,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": {
          "calcs": ["lastNotNull"],
          "fields": "",
          "values": false
        },
        "textMode": "auto"
      },
      "targets": [
        {
          "expr": "sum(frappe_operator_resource_total{resource_type=\"frappesite\"})",
          "refId": "A"
        }
      ],
      "title": "Total Sites",
      "type": "stat"
    }
  ],
  "schemaVersion": 38,
  "tags": ["frappe", "operator", "kubernetes"],
  "templating": { "list": [] },
  "time": { "from": "now-1h", "to": "now" },
  "timepicker": {},
  "timezone": "",
  "title": "Frappe Operator",
  "uid": "frappe-operator",
  "version": 1,
  "weekStart": ""
}
```

### Frappe Workload Dashboard

Monitor the Frappe application workloads:

```json
{
  "title": "Frappe Workloads",
  "uid": "frappe-workloads",
  "panels": [
    {
      "title": "Gunicorn Response Time",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, rate(nginx_http_request_duration_seconds_bucket{upstream=~\".*gunicorn.*\"}[5m]))",
          "legendFormat": "p95"
        }
      ],
      "type": "timeseries"
    },
    {
      "title": "Worker Queue Depth",
      "targets": [
        {
          "expr": "frappe_worker_queue_length",
          "legendFormat": "{{queue}}"
        }
      ],
      "type": "timeseries"
    },
    {
      "title": "Database Connections",
      "targets": [
        {
          "expr": "mysql_global_status_threads_connected",
          "legendFormat": "connections"
        }
      ],
      "type": "timeseries"
    }
  ]
}
```

## Alert Rules

A standalone PrometheusRule is available at [docs/alert-rules.yaml](alert-rules.yaml). Apply with `kubectl apply -f docs/alert-rules.yaml`. Full content below:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: frappe-operator-alerts
  namespace: frappe-operator-system
  labels:
    prometheus: k8s
    role: alert-rules
spec:
  groups:
  - name: frappe-operator
    rules:
    # Operator is not running
    - alert: FrappeOperatorDown
      expr: absent(up{job="frappe-operator"}) == 1
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "Frappe Operator is down"
        description: "Frappe Operator has been down for more than 5 minutes"

    # High reconciliation error rate
    - alert: FrappeOperatorHighErrorRate
      expr: |
        rate(frappe_operator_reconciliation_errors_total[5m]) > 0.1
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "High reconciliation error rate"
        description: "Controller {{ $labels.controller }} has error rate of {{ $value }} errors/sec"

    # Slow reconciliation
    - alert: FrappeOperatorSlowReconciliation
      expr: |
        histogram_quantile(0.95, rate(frappe_operator_reconciliation_duration_seconds_bucket[5m])) > 60
      for: 10m
      labels:
        severity: warning
      annotations:
        summary: "Slow reconciliation detected"
        description: "Controller {{ $labels.controller }} p95 reconciliation time is {{ $value }}s"

    # Site not ready
    - alert: FrappeSiteNotReady
      expr: |
        kube_customresource_frappesite_status_phase{phase!="Ready"} == 1
      for: 15m
      labels:
        severity: warning
      annotations:
        summary: "FrappeSite not ready"
        description: "Site {{ $labels.name }} in namespace {{ $labels.namespace }} is not ready"

    # Bench not ready
    - alert: FrappeBenchNotReady
      expr: |
        kube_customresource_frappebench_status_phase{phase!="Ready"} == 1
      for: 15m
      labels:
        severity: warning
      annotations:
        summary: "FrappeBench not ready"
        description: "Bench {{ $labels.name }} in namespace {{ $labels.namespace }} is not ready"

  - name: frappe-workloads
    rules:
    # Gunicorn pod restarts
    - alert: FrappeGunicornRestarts
      expr: |
        increase(kube_pod_container_status_restarts_total{container="gunicorn"}[1h]) > 3
      labels:
        severity: warning
      annotations:
        summary: "Gunicorn container restarting"
        description: "Gunicorn container in pod {{ $labels.pod }} has restarted {{ $value }} times in the last hour"

    # Worker queue buildup
    - alert: FrappeWorkerQueueHigh
      expr: |
        frappe_worker_queue_length > 100
      for: 10m
      labels:
        severity: warning
      annotations:
        summary: "Worker queue is backing up"
        description: "Queue {{ $labels.queue }} has {{ $value }} pending jobs"

    # Database connections high
    - alert: FrappeDatabaseConnectionsHigh
      expr: |
        mysql_global_status_threads_connected / mysql_global_variables_max_connections > 0.8
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "Database connections near limit"
        description: "{{ $value | humanizePercentage }} of max connections in use"
```

## Logging

### Operator Logs

```bash
# Stream operator logs
kubectl logs -n frappe-operator-system -l control-plane=controller-manager -f

# Filter by log level
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager | grep -E 'level=(error|warn)'

# JSON structured logs (if enabled)
kubectl logs -n frappe-operator-system deployment/frappe-operator-controller-manager -o json
```

### Configuring Log Verbosity

```yaml
# In Helm values
controller:
  manager:
    args:
      - "--zap-log-level=debug"
      - "--zap-encoder=json"
```

### Log Aggregation

Forward logs to your logging stack:

```yaml
# Fluent Bit ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
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

## Health Checks

### Operator Health

The operator exposes health endpoints on port 8081:

- `/healthz` - Liveness probe
- `/readyz` - Readiness probe

```bash
# Port-forward to check health
kubectl port-forward -n frappe-operator-system svc/frappe-operator-controller-manager-metrics-service 8081:8081

# Check liveness
curl http://localhost:8081/healthz

# Check readiness
curl http://localhost:8081/readyz
```

### Frappe Site Health

Check site health using the Frappe API:

```bash
# Get site URL
SITE_URL=$(kubectl get frappesite mysite -o jsonpath='{.status.siteURL}')

# Check API
curl -s "$SITE_URL/api/method/ping"
```

### Kubernetes Health Probes

The operator configures health probes for Frappe pods:

```yaml
# Gunicorn container probes
livenessProbe:
  httpGet:
    path: /api/method/ping
    port: 8000
  initialDelaySeconds: 30
  periodSeconds: 10
  
readinessProbe:
  httpGet:
    path: /api/method/ping
    port: 8000
  initialDelaySeconds: 10
  periodSeconds: 5
```
