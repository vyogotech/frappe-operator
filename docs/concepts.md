# Concepts

Understanding the key concepts and architecture of Frappe Operator.

## Overview

Frappe Operator manages Frappe deployments using two primary resources:
- **FrappeBench**: Shared infrastructure serving multiple sites
- **FrappeSite**: Individual Frappe sites

This architecture enables efficient multi-tenancy while maintaining isolation where needed.

## Core Concepts

### FrappeBench

A **FrappeBench** represents a Frappe bench environment with shared infrastructure components:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms"]'
```

**Components Created:**
- **NGINX**: Reverse proxy and static file serving
- **Redis/DragonFly**: Cache and queue backend
- **Shared Storage**: Common configuration and apps

**Purpose:**
- Serve multiple sites efficiently
- Share common apps and configuration
- Reduce resource overhead
- Simplify app management

**When to Use Multiple Benches:**
- Different Frappe versions
- Different app sets
- Environment separation (dev/staging/prod)
- Tenant isolation requirements

### FrappeSite

A **FrappeSite** represents an individual Frappe site with its own:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: customer1-site
spec:
  benchRef:
    name: production-bench
  siteName: "customer1.example.com"
  dbConfig:
    mode: dedicated
```

**Components Created:**
- **Gunicorn**: Web application servers
- **Socketio**: Real-time communication
- **Scheduler**: Background task scheduler
- **Workers**: Background job processors (default, long, short)
- **Database**: Per-site database
- **Storage**: Site-specific files

**Site Lifecycle:**
1. **Pending**: Site resource created
2. **Provisioning**: Database and storage being set up
3. **Ready**: Site is accessible
4. **Failed**: Error occurred (check events)

## Architecture

### Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         FrappeBench                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌──────────┐         ┌───────────┐        ┌─────────────┐     │
│  │  NGINX   │◄────────┤  Redis/   │        │   Common    │     │
│  │  (Proxy) │         │ DragonFly │        │   Storage   │     │
│  └─────┬────┘         └───────────┘        └─────────────┘     │
│        │                                                         │
└────────┼─────────────────────────────────────────────────────────┘
         │
         │ Route based on Host header
         ├──────────┬──────────┬──────────┐
         │          │          │          │
    ┌────▼────┐ ┌──▼─────┐ ┌──▼─────┐ ┌──▼─────┐
    │ Site 1  │ │ Site 2 │ │ Site 3 │ │ Site N │
    │─────────│ │────────│ │────────│ │────────│
    │Gunicorn │ │Gunicorn│ │Gunicorn│ │Gunicorn│
    │Socketio │ │Socketio│ │Socketio│ │Socketio│
    │Scheduler│ │Scheduler│ │Scheduler│ │Scheduler│
    │Workers  │ │Workers │ │Workers │ │Workers │
    │         │ │        │ │        │ │        │
    │  ┌──┐   │ │  ┌──┐  │ │  ┌──┐  │ │  ┌──┐  │
    │  │DB│   │ │  │DB│  │ │  │DB│  │ │  │DB│  │
    │  └──┘   │ │  └──┘  │ │  └──┘  │ │  └──┘  │
    └─────────┘ └────────┘ └────────┘ └────────┘
```

### Request Flow

1. **External Request** → Ingress Controller
2. **Ingress** → NGINX Service (bench-level)
3. **NGINX** → Routes to correct site based on Host header
4. **Gunicorn** → Processes request
5. **Database** → Site-specific data
6. **Redis** → Shared cache/queue

### Storage Architecture

```
FrappeBench
└── Common Storage (RWX)
    └── /home/frappe/frappe-bench
        ├── apps/          # Frappe apps
        ├── config/        # Common config
        └── sites/
            ├── site1.com/
            │   ├── private/
            │   └── public/
            ├── site2.com/
            │   ├── private/
            │   └── public/
            └── common_site_config.json
```

## Database Configurations

### Shared Database Mode

Multiple sites share a MariaDB instance but have separate databases:

```yaml
dbConfig:
  mode: shared
  mariadbRef:
    name: shared-mariadb
    namespace: databases
```

**Pros:**
- Cost-effective for many small sites
- Simplified management
- Quick provisioning

**Cons:**
- Resource contention
- No per-site tuning
- Shared failure domain

**Use Cases:**
- Development environments
- Small customer sites
- Cost-sensitive deployments

### Dedicated Database Mode

Each site gets its own MariaDB instance:

```yaml
dbConfig:
  mode: dedicated
  storageSize: 50Gi
  resources:
    requests:
      cpu: 1000m
      memory: 2Gi
```

**Pros:**
- Isolated performance
- Per-site scaling
- Independent backups
- Better security isolation

**Cons:**
- Higher resource usage
- More management overhead

**Use Cases:**
- Enterprise customers
- High-traffic sites
- Compliance requirements

### External Database Mode

Use an existing database (RDS, Cloud SQL, etc.):

```yaml
dbConfig:
  mode: external
  connectionSecretRef:
    name: site1-db-credentials
```

Secret format:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: site1-db-credentials
stringData:
  host: "mysql.example.com"
  port: "3306"
  database: "site1_db"
  username: "site1_user"
  password: "secure_password"
```

**Pros:**
- Use managed services
- Leverage existing infrastructure
- Advanced features (read replicas, etc.)

**Cons:**
- External dependency
- Network latency
- Cost varies

**Use Cases:**
- Cloud deployments
- Existing database infrastructure
- Managed service preference

## Component Roles

### NGINX (Bench-Level)
- **Role**: Reverse proxy, static file serving
- **Scope**: Shared across all sites on bench
- **Scaling**: Typically 1-2 replicas
- **Resource**: Moderate CPU, low memory

### Gunicorn (Site-Level)
- **Role**: WSGI application server
- **Scope**: Per-site
- **Scaling**: Horizontal (2-10+ replicas)
- **Resource**: High CPU and memory

### Socketio (Site-Level)
- **Role**: Real-time WebSocket connections
- **Scope**: Per-site
- **Scaling**: Horizontal (1-5 replicas)
- **Resource**: Moderate CPU, low memory

### Scheduler (Site-Level)
- **Role**: Cron job scheduler
- **Scope**: Per-site
- **Scaling**: Single instance (no scaling)
- **Resource**: Low CPU and memory

### Workers (Site-Level)

Three worker types with different queue priorities:

#### Worker Default
- **Queue**: `default`, `short`, `long`
- **Use**: General background tasks
- **Timeout**: Medium
- **Scaling**: 1-5+ replicas

#### Worker Long
- **Queue**: `long`
- **Use**: Long-running tasks (reports, imports)
- **Timeout**: Long (30+ minutes)
- **Scaling**: 1-3 replicas

#### Worker Short
- **Queue**: `short`
- **Use**: Quick tasks (emails, notifications)
- **Timeout**: Short (5 minutes)
- **Scaling**: 1-3 replicas

### Redis/DragonFly (Bench-Level)
- **Role**: Cache, queue, session storage
- **Scope**: Shared across sites
- **Scaling**: Typically single instance
- **Resource**: High memory, moderate CPU

**DragonFly vs Redis:**
- DragonFly: Better performance, lower memory
- Redis: More mature, wider compatibility

## Domain Resolution

Frappe uses the HTTP Host header to determine which site to serve. The operator provides multiple ways to configure domains:

### 1. Explicit Domain

```yaml
spec:
  siteName: "mysite"
  domain: "mysite.example.com"
```

### 2. Bench-Level Suffix

```yaml
# In FrappeBench
spec:
  domainConfig:
    suffix: ".platform.com"

# In FrappeSite
spec:
  siteName: "customer1"
  # Results in: customer1.platform.com
```

### 3. Auto-Detection

```yaml
spec:
  domainConfig:
    autoDetect: true
    ingressControllerRef:
      name: ingress-nginx-controller
      namespace: ingress-nginx
```

The operator detects the Load Balancer IP/hostname and uses it.

### 4. SiteName Default

If no domain is specified, `siteName` is used as the domain.

**Best Practices:**
- Production: Use explicit domains
- SaaS: Use bench-level suffix
- Development: Use auto-detection or .local

## Resource Management

### Resource Tiers

**Small (Development):**
```yaml
componentResources:
  gunicorn:
    requests: {cpu: "200m", memory: "256Mi"}
    limits: {cpu: "500m", memory: "512Mi"}
  workerDefault:
    requests: {cpu: "100m", memory: "128Mi"}
```

**Medium (Small Production):**
```yaml
componentResources:
  gunicorn:
    requests: {cpu: "500m", memory: "512Mi"}
    limits: {cpu: "1", memory: "1Gi"}
  workerDefault:
    requests: {cpu: "200m", memory: "256Mi"}
```

**Large (Production):**
```yaml
componentResources:
  gunicorn:
    requests: {cpu: "1", memory: "2Gi"}
    limits: {cpu: "2", memory: "4Gi"}
  workerDefault:
    requests: {cpu: "500m", memory: "1Gi"}
```

### Autoscaling

Enable Horizontal Pod Autoscaling:

```yaml
componentHPA:
  gunicorn:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilization: 70
```

## Multi-Tenancy Models

### Model 1: Shared Bench, Shared Database
- **Setup**: One bench, one MariaDB, many sites
- **Cost**: Lowest
- **Isolation**: Minimal
- **Use**: Small startups, development

### Model 2: Shared Bench, Dedicated Databases
- **Setup**: One bench, MariaDB per site
- **Cost**: Medium
- **Isolation**: Database-level
- **Use**: SaaS platforms, mid-size customers

### Model 3: Dedicated Bench per Tenant
- **Setup**: Bench per customer, dedicated resources
- **Cost**: Highest
- **Isolation**: Complete
- **Use**: Enterprise, compliance requirements

### Model 4: Bench per Environment
- **Setup**: Separate benches for dev/staging/prod
- **Cost**: Medium
- **Isolation**: Environment-level
- **Use**: Standard enterprise setup

## Secrets Management

The operator manages several types of secrets:

### Admin Password
```yaml
spec:
  adminPasswordSecretRef:
    name: site-admin-pwd
```

### Database Credentials
Auto-generated or external:
```yaml
# Auto-generated (dedicated mode)
status:
  dbConnectionSecret: site1-db-connection

# External mode
spec:
  dbConfig:
    connectionSecretRef:
      name: custom-db-credentials
```

### Image Pull Secrets
For private registries:
```yaml
spec:
  imageConfig:
    pullSecrets:
      - name: docker-registry-secret
```

## Storage Classes

### Site Storage (RWO)
- **Type**: ReadWriteOnce
- **Content**: Site files (private, public)
- **Size**: 5-100Gi per site
- **Backup**: Critical

### Logs Storage (RWO)
- **Type**: ReadWriteOnce
- **Content**: Application logs
- **Size**: 1-10Gi per site
- **Backup**: Optional

### Bench Storage (RWX)
- **Type**: ReadWriteMany (if available)
- **Content**: Frappe apps, common config
- **Size**: 10-50Gi per bench
- **Backup**: Important

## High Availability

### Application HA
- Multiple gunicorn replicas
- Multiple worker replicas
- Load balancing via Services

### Database HA
- Use MariaDB Operator with Galera cluster
- Or external managed databases with HA

### Redis HA
- Redis Sentinel (Redis mode)
- DragonFly replication
- Or external managed Redis

### Ingress HA
- Multiple ingress controller replicas
- Cloud load balancers

## Next Steps

- **[API Reference](api-reference.md)** - Detailed field specifications
- **[Examples](examples.md)** - Real-world deployment patterns
- **[Operations](operations.md)** - Production best practices

