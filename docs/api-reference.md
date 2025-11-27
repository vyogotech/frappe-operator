# API Reference

Complete specification of Frappe Operator Custom Resource Definitions (CRDs).

## FrappeBench

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `FrappeBench`

A FrappeBench represents a Frappe bench environment with shared infrastructure.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: <bench-name>
  namespace: <namespace>
spec:
  # Required: Frappe version
  frappeVersion: string
  
  # Optional: Apps to install as JSON array
  appsJSON: string
  
  # Optional: Container image configuration
  imageConfig:
    repository: string
    tag: string
    pullPolicy: string  # Always, Never, IfNotPresent
    pullSecrets:
      - name: string
  
  # Optional: Replica counts for components
  componentReplicas:
    gunicorn: int32
    socketio: int32
    workerDefault: int32
    workerLong: int32
    workerShort: int32
  
  # Optional: Resource requirements for components
  componentResources:
    gunicorn:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    nginx:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    scheduler:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    socketio:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    workerDefault:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    workerLong:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    workerShort:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
  
  # Optional: Domain configuration
  domainConfig:
    suffix: string
    autoDetect: bool
    ingressControllerRef:
      name: string
      namespace: string
  
  # Optional: Redis/DragonFly configuration
  redisConfig:
    type: string  # redis or dragonfly
    image: string
    maxMemory: string
    resources:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    storageSize: string
```

### Status

```yaml
status:
  # Indicates if the bench is ready
  ready: bool
  
  # List of sites using this bench
  sites:
    - string
```

### Field Details

#### `frappeVersion` (required)
- **Type:** `string`
- **Description:** Frappe framework version
- **Example:** `"version-15"`, `"v15.0.0"`

#### `appsJSON` (optional)
- **Type:** `string`
- **Description:** JSON array of apps to install
- **Example:** `'["erpnext", "hrms"]'`
- **Default:** `'["frappe"]'`

#### `imageConfig` (optional)
Container image configuration.

- **`repository`** (string): Image repository (e.g., `frappe/erpnext`)
- **`tag`** (string): Image tag (e.g., `v15.0.0`)
- **`pullPolicy`** (string): Image pull policy - `Always`, `Never`, or `IfNotPresent`
- **`pullSecrets`** (array): Secrets for private registries

#### `componentReplicas` (optional)
Replica counts for each component.

- **`gunicorn`** (int32): Number of gunicorn replicas (default: 1, min: 1)
- **`socketio`** (int32): Number of socketio replicas (default: 1, min: 1)
- **`workerDefault`** (int32): Number of default worker replicas (default: 1, min: 0)
- **`workerLong`** (int32): Number of long worker replicas (default: 1, min: 0)
- **`workerShort`** (int32): Number of short worker replicas (default: 1, min: 0)

#### `componentResources` (optional)
Resource requirements for each component.

Each component can specify:
- **`requests`**: Minimum guaranteed resources
- **`limits`**: Maximum allowed resources

Common values:
```yaml
requests: {cpu: "100m", memory: "128Mi"}
limits: {cpu: "500m", memory: "512Mi"}
```

#### `domainConfig` (optional)
Domain resolution configuration.

- **`suffix`** (string): Domain suffix to append to site names
- **`autoDetect`** (bool): Enable automatic domain detection (default: true)
- **`ingressControllerRef`**: Reference to ingress controller for domain detection

#### `redisConfig` (optional)
Redis or DragonFly configuration.

- **`type`** (string): `redis` or `dragonfly` (default: `redis`)
- **`image`** (string): Custom image
- **`maxMemory`** (string): Maximum memory (e.g., `"4gb"`)
- **`resources`**: Resource requirements
- **`storageSize`**: Persistent storage size

---

## FrappeSite

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `FrappeSite`

A FrappeSite represents an individual Frappe site.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: <site-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeBench
  benchRef:
    name: string
    namespace: string  # optional, defaults to same namespace
  
  # Required: Site name (must match domain)
  siteName: string
  
  # Optional: Admin password secret
  adminPasswordSecretRef:
    name: string
    namespace: string
  
  # Optional: Database configuration
  dbConfig:
    mode: string  # shared, dedicated, or external
    mariadbRef:
      name: string
      namespace: string
    storageSize: string
    resources:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    connectionSecretRef:
      name: string
      namespace: string
  
  # Optional: External domain (defaults to siteName)
  domain: string
  
  # Optional: TLS configuration
  tls:
    enabled: bool
    certManagerIssuer: string
    secretName: string
  
  # Optional: Ingress class name
  ingressClassName: string
  
  # Optional: Ingress configuration
  ingress:
    enabled: bool
    className: string
    annotations:
      key: value
    tls:
      enabled: bool
      certManagerIssuer: string
      secretName: string
```

### Status

```yaml
status:
  # Current phase of the site
  phase: string  # Pending, Provisioning, Ready, Failed
  
  # Indicates if the referenced bench is ready
  benchReady: bool
  
  # Accessible URL for the site
  siteURL: string
  
  # Database connection secret name
  dbConnectionSecret: string
  
  # Resolved domain after configuration
  resolvedDomain: string
  
  # How domain was determined
  domainSource: string  # explicit, bench-suffix, auto-detected, sitename-default
```

### Field Details

#### `benchRef` (required)
Reference to the FrappeBench this site belongs to.

```yaml
benchRef:
  name: "production-bench"
  namespace: "default"  # optional
```

#### `siteName` (required)
- **Type:** `string`
- **Description:** Site name - MUST match the domain that will receive traffic
- **Validation:** Must be a valid DNS name
- **Example:** `"customer1.example.com"`, `"mysite.local"`

**Important:** This is what Frappe uses to route requests based on the HTTP Host header.

#### `adminPasswordSecretRef` (optional)
Reference to a Secret containing the admin password.

```yaml
adminPasswordSecretRef:
  name: "site-admin-password"
```

Secret format:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: site-admin-password
stringData:
  password: "your-secure-password"
```

#### `dbConfig` (optional)
Database configuration for the site.

##### Shared Mode
```yaml
dbConfig:
  mode: shared
  mariadbRef:
    name: shared-mariadb
    namespace: databases
```

##### Dedicated Mode
```yaml
dbConfig:
  mode: dedicated
  storageSize: "50Gi"
  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "2"
      memory: "4Gi"
```

##### External Mode
```yaml
dbConfig:
  mode: external
  connectionSecretRef:
    name: external-db-credentials
```

External secret format:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: external-db-credentials
stringData:
  host: "mysql.example.com"
  port: "3306"
  database: "site_db"
  username: "site_user"
  password: "db_password"
```

#### `domain` (optional)
- **Type:** `string`
- **Description:** External domain for ingress
- **Default:** Uses `siteName` if not specified
- **Example:** `"customer1.example.com"`

#### `tls` (optional)
TLS configuration for the site.

```yaml
tls:
  enabled: true
  certManagerIssuer: "letsencrypt-prod"  # cert-manager ClusterIssuer
  secretName: "site-tls-cert"  # optional, auto-generated if not specified
```

#### `ingressClassName` (optional)
- **Type:** `string`
- **Description:** Ingress class to use
- **Example:** `"nginx"`, `"traefik"`

#### `ingress` (optional)
Complete ingress configuration.

```yaml
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
  tls:
    enabled: true
    certManagerIssuer: "letsencrypt-prod"
```

---

## SiteUser

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `SiteUser`

Manages users on a Frappe site.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteUser
metadata:
  name: <user-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeSite
  siteRef:
    name: string
    namespace: string
  
  # Required: User email
  email: string
  
  # Required: First name
  firstName: string
  
  # Optional: Last name
  lastName: string
  
  # Optional: Roles
  roles:
    - string
  
  # Optional: Password secret
  passwordSecretRef:
    name: string
```

---

## SiteWorkspace

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `SiteWorkspace`

Creates a workspace on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteWorkspace
metadata:
  name: <workspace-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeSite
  siteRef:
    name: string
    namespace: string
  
  # Required: Workspace title
  title: string
  
  # Optional: Workspace configuration
  workspaceConfig:
    # Workspace-specific fields
```

---

## SiteDashboard

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `SiteDashboard`

Creates a dashboard on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteDashboard
metadata:
  name: <dashboard-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeSite
  siteRef:
    name: string
    namespace: string
  
  # Required: Dashboard name
  dashboardName: string
  
  # Optional: Dashboard charts
  charts:
    - string
```

---

## SiteDashboardChart

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `SiteDashboardChart`

Creates a dashboard chart on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteDashboardChart
metadata:
  name: <chart-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeSite
  siteRef:
    name: string
    namespace: string
  
  # Required: Chart configuration
  chartConfig:
    # Chart-specific fields
```

---

## SiteBackup

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `SiteBackup`

Manages site backups.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteBackup
metadata:
  name: <backup-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeSite
  siteRef:
    name: string
    namespace: string
  
  # Optional: Backup schedule (cron format)
  schedule: string
  
  # Optional: Backup retention
  retention:
    days: int32
    count: int32
  
  # Optional: Backup destination
  destination:
    type: string  # s3, gcs, azure, pvc
    config:
      # Destination-specific configuration
```

---

## SiteJob

**API Group:** `vyogo.tech/v1alpha1`  
**Kind:** `SiteJob`

Executes custom jobs on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: SiteJob
metadata:
  name: <job-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeSite
  siteRef:
    name: string
    namespace: string
  
  # Required: Job type
  jobType: string  # migrate, backup, custom, console
  
  # Optional: Custom command
  command:
    - string
  
  # Optional: Job configuration
  jobConfig:
    # Job-specific fields
```

---

## Common Types

### NamespacedName

Reference to a resource in a specific namespace.

```yaml
name: string      # Required: Resource name
namespace: string # Optional: Resource namespace (defaults to same namespace)
```

### ResourceRequirements

CPU and memory resource specifications.

```yaml
requests:
  cpu: string     # e.g., "100m", "1", "2.5"
  memory: string  # e.g., "128Mi", "1Gi", "4Gi"
limits:
  cpu: string
  memory: string
```

### TLSConfig

TLS certificate configuration.

```yaml
enabled: bool              # Enable TLS
certManagerIssuer: string  # cert-manager ClusterIssuer name
secretName: string         # TLS secret name (optional)
```

---

## Examples

### Minimal Bench and Site

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: dev-bench
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext"]'
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: mysite
spec:
  benchRef:
    name: dev-bench
  siteName: "mysite.local"
  dbConfig:
    mode: shared
```

### Production Setup

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: prod-bench
spec:
  frappeVersion: "version-15"
  appsJSON: '["erpnext", "hrms"]'
  componentReplicas:
    gunicorn: 3
    workerDefault: 2
  componentResources:
    gunicorn:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: prod-site
spec:
  benchRef:
    name: prod-bench
  siteName: "erp.example.com"
  domain: "erp.example.com"
  dbConfig:
    mode: dedicated
    storageSize: "100Gi"
  ingress:
    enabled: true
    className: "nginx"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"
```

---

## Validation

### FrappeBench Validations

- `frappeVersion` must be specified
- Replica counts must be >= minimum values
- Resource values must be valid Kubernetes quantities

### FrappeSite Validations

- `benchRef.name` must be specified
- `siteName` must be a valid DNS name (RFC 1123)
- `dbConfig.mode` must be one of: `shared`, `dedicated`, `external`
- If `dbConfig.mode` is `external`, `connectionSecretRef` is required

---

## Status Conditions

Resources report their status through the `status` field. Common patterns:

### FrappeBench Status

```yaml
status:
  ready: true
  sites:
    - "site1"
    - "site2"
```

### FrappeSite Status

```yaml
status:
  phase: "Ready"  # Pending, Provisioning, Ready, Failed
  benchReady: true
  siteURL: "https://mysite.example.com"
  dbConnectionSecret: "mysite-db-connection"
  resolvedDomain: "mysite.example.com"
  domainSource: "explicit"
```

---

## Next Steps

- **[Examples](examples.md)** - Real-world configuration examples
- **[Operations](operations.md)** - Managing resources in production
- **[Troubleshooting](troubleshooting.md)** - Debugging issues

