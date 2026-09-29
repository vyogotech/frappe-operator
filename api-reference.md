# <i data-lucide="file-code-2"></i> API Reference

Complete specification of Frappe Operator Custom Resource Definitions (CRDs) by **Vyogo Technologies**.

---

## <i data-lucide="server"></i> FrappeBench

**API Group:** `vyogo.tech/v1`  
**Kind:** `FrappeBench`

A `FrappeBench` represents a shared Frappe bench runtime environment hosting web proxies, worker processes, and common storage.

### Spec

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: <bench-name>
  namespace: <namespace>
spec:
  # Required: Frappe framework version
  frappeVersion: string
  
  # Optional: Apps to install on this bench
  apps:
    - string
  
  # Optional (Deprecated): Apps to install as JSON array
  appsJSON: string
  
  # Optional: Container image configuration
  imageConfig:
    repository: string
    tag: string
    pullPolicy: string  # Always, Never, IfNotPresent
    pullSecrets:
      - name: string
  
  # Optional: Autoscaling and replica configuration for components
  # Map keys: nginx, gunicorn, socketio, scheduler, worker-default, worker-long, worker-short
  componentAutoscaling:
    <component-name>:
      enabled: bool
      staticReplicas: int32
      minReplicas: int32
      maxReplicas: int32
      provider: string    # keda or hpa
      keda:
        trigger: string   # cpu, memory, redis
        targetValue: string
      hpa:
        metric: string    # cpu or memory
        targetUtilization: int32
  
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
  
  # Optional: Sizing for batch jobs (benchInit, siteInit, appInstall, backup, migration)
  jobResources:
    default:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    siteInit:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
  
  # Optional: Shared bench storage size and storage class
  storageSize: string            # default: "10Gi"
  storageClassName: string
  
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
  
  # Optional: Suggests max concurrent site reconciles for sites on this bench
  siteReconcileConcurrency: int32
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

#### `apps` (optional)
- **Type:** `[]string`
- **Description:** List of Frappe applications to install and enable on the bench
- **Example:** `["erpnext", "hrms"]`

#### `appsJSON` (deprecated)
- **Type:** `string`
- **Description:** Deprecated JSON array string. Prefer `apps: []string` instead.
- **Example:** `'["erpnext", "hrms"]'`

#### `imageConfig` (optional)
Container image configuration.

- **`repository`** (string): Image repository (e.g., `frappe/erpnext`)
- **`tag`** (string): Image tag (e.g., `v15.0.0`)
- **`pullPolicy`** (string): Image pull policy - `Always`, `Never`, or `IfNotPresent`
- **`pullSecrets`** (array): Secrets for private registries

#### `componentAutoscaling` (optional)
Autoscaling and replica configuration for each component.

- **`enabled`** (bool): Whether to enable autoscaling for this component. If false, uses `staticReplicas`.
- **`staticReplicas`** (int32): Fixed number of replicas when autoscaling is disabled.
- **`minReplicas`** (int32): Minimum number of replicas when autoscaling is enabled (supports 0 for workers).
- **`maxReplicas`** (int32): Maximum number of replicas when autoscaling is enabled.
- **`provider`** (string): Scaling backend - `hpa` (default) or `keda`.

##### `keda` (optional)
KEDA-specific configuration.
- **`trigger`** (string): `cpu`, `memory`, or `redis` (for workers).
- **`targetValue`** (string): Threshold for the trigger (e.g., `"70"` for CPU, `"5"` for Redis queue).

##### `hpa` (optional)
HPA-specific configuration.
- **`metric`** (string): `cpu` or `memory`.
- **`targetUtilization`** (int32): Target percentage (1-100).

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

#### `siteReconcileConcurrency` (optional)
- **Type:** `int32`
- **Description:** Suggests max concurrent FrappeSite reconciles for sites on this bench. The operator uses **max(operator config `maxConcurrentSiteReconciles`, max across all benches)** at startup. Useful when running 100+ sites. Only applied at operator startup; changing it requires an operator restart.
- **Example:** `20`

---

## <i data-lucide="globe"></i> FrappeSite

**API Group:** `vyogo.tech/v1`  
**Kind:** `FrappeSite`

A `FrappeSite` represents an individual tenant site within a `FrappeBench`.

### Spec

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: <site-name>
  namespace: <namespace>
spec:
  # Required: Reference to FrappeBench
  benchRef:
    name: string
    namespace: string  # optional, defaults to same namespace
  
  # Required: Site name (must match domain that will receive traffic)
  siteName: string
  
  # Optional: Apps to install on this site during creation
  apps:
    - string
  
  # Optional: Admin password secret reference
  adminPasswordSecretRef:
    name: string
    namespace: string
  
  # Optional: External encryption key secret reference
  encryptionKeySecretRef:
    name: string
    key: string        # must be "encryption_key"
  
  # Optional: Polymorphic database configuration
  dbConfig:
    provider: string         # postgres, mariadb, sqlite, external (default: mariadb)
    postgresEngine: string   # stackgres, percona (when provider: postgres)
    mode: string             # shared or dedicated (default: shared)
    mariadbRef:
      name: string
      namespace: string
    postgresRef:
      name: string
      namespace: string
    storageSize: string      # e.g., "20Gi"
    resources:
      requests: {cpu: string, memory: string}
      limits: {cpu: string, memory: string}
    host: string             # for external or remote databases
    port: string             # e.g., "5432" or "3306"
    connectionSecretRef:
      name: string
      namespace: string
    maxStatementTimeSeconds: int64
  
  # Optional: Deletion policy for database resources upon CR deletion
  # Retain (default): keeps database and credentials safe from accidental deletion
  # Delete: purges the database and user
  deletionPolicy: string     # Retain or Delete (default: Retain)
  
  # Optional: Skip bench new-site initialization if database already contains valid schema
  skipInit: bool
  
  # Optional: External domain (defaults to siteName)
  domain: string
  
  # Optional: Kubernetes Ingress configuration
  ingress:
    enabled: bool
    className: string
    annotations:
      key: value
    tls:
      enabled: bool
      certManagerIssuer: string
      secretName: string
  
  # Optional: Red Hat OpenShift Route configuration
  routeConfig:
    enabled: bool
    tls:
      termination: string                   # edge, reencrypt, passthrough (default: edge)
      insecureEdgeTerminationPolicy: string # Redirect, Allow, None (default: Redirect)
  
  # Optional: Advanced pod scheduling for site jobs
  podConfig:
    labels:
      key: value
    annotations:
      key: value
    geoTag: string
```

### Status

```yaml
status:
  # Current lifecycle phase: Pending, Provisioning, Ready, Failed
  phase: string
  
  # Indicates if the referenced bench is ready
  benchReady: bool
  
  # Indicates if the database is provisioned and ready
  databaseReady: bool
  
  # Name of the actual database created
  databaseName: string
  
  # Secret containing site-specific DB credentials
  databaseCredentialsSecret: string
  
  # Accessible URL for the site
  siteURL: string
  
  # Final resolved domain
  resolvedDomain: string
  
  # How domain was determined: explicit, bench-suffix, auto-detected, sitename-default
  domainSource: string
  
  # Apps requested and installed
  installedApps:
    - string
  
  # Status of app installation
  appInstallationStatus: string
  
  # Detailed error map for failed apps
  failedApps:
    appName: "error message"
```

### Field Details

#### `benchRef` (required)
Reference to the `FrappeBench` this site belongs to.

```yaml
benchRef:
  name: "production-bench"
  namespace: "frappe-system"  # optional, defaults to site's namespace
```

#### `siteName` (required)
- **Type:** `string`
- **Description:** Site name - MUST match the hostname/domain that will receive traffic
- **Validation:** Must be a valid RFC 1123 DNS name
- **Example:** `"customer1.example.com"`

#### `apps` (optional)
- **Type:** `[]string`
- **Description:** List of apps to install on this site during initial creation
- **Behavior:** Apps are validated against the container filesystem. Missing apps are gracefully skipped with warnings. Immutable after initial site creation.
- **Example:**
```yaml
apps:
  - erpnext
  - hrms
```

#### `dbConfig` (optional)
Polymorphic database configuration supporting PostgreSQL, MariaDB, and external providers.

##### 1. PostgreSQL with StackGres (`stackgres`)
```yaml
dbConfig:
  provider: postgres
  postgresEngine: stackgres
  mode: dedicated
  storageSize: "20Gi"
  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "2"
      memory: "4Gi"
```

##### 2. PostgreSQL with Percona (`percona`)
```yaml
dbConfig:
  provider: postgres
  postgresEngine: percona
  mode: dedicated
  storageSize: "50Gi"
```

##### 3. Shared MariaDB Mode
```yaml
dbConfig:
  provider: mariadb
  mode: shared
  mariadbRef:
    name: shared-mariadb
    namespace: databases
```

##### 4. External Database Mode
```yaml
dbConfig:
  provider: external
  mode: external
  connectionSecretRef:
    name: external-db-credentials
```

Secret format:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: external-db-credentials
stringData:
  host: "postgres.example.com"
  port: "5432"
  database: "site_db"
  username: "site_user"
  password: "db_password"
```

#### `deletionPolicy` (optional)
- **Type:** `string` (`Retain` | `Delete`)
- **Default:** `Retain`
- **Description:** Controls whether database resources and credentials are retained or hard-deleted when the `FrappeSite` is deleted. `Retain` prevents accidental data loss from GitOps / ArgoCD pruning.

#### `skipInit` (optional)
- **Type:** `bool`
- **Default:** `false`
- **Description:** Bypasses `bench new-site` initialization if the database already contains a valid Frappe schema, running only migrations and configuration updates.

#### `routeConfig` (optional)
OpenShift Route configuration with automatic edge TLS redirection:
```yaml
routeConfig:
  enabled: true
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect
```

#### `ingress` (optional)
Standard Kubernetes Ingress configuration with automatic HTTPS termination:
```yaml
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
  tls:
    enabled: true
    certManagerIssuer: "letsencrypt-prod"
```

---

## SiteUser

**API Group:** `vyogo.tech/v1`  
**Kind:** `SiteUser`

Manages users on a Frappe site.

### Spec

```yaml
apiVersion: vyogo.tech/v1
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

**API Group:** `vyogo.tech/v1`  
**Kind:** `SiteWorkspace`

Creates a workspace on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1
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

**API Group:** `vyogo.tech/v1`  
**Kind:** `SiteDashboard`

Creates a dashboard on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1
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

**API Group:** `vyogo.tech/v1`  
**Kind:** `SiteDashboardChart`

Creates a dashboard chart on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1
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

**API Group:** `vyogo.tech/v1`  
**Kind:** `SiteBackup`

Creates automated backups of Frappe sites using the `bench backup` command.

### Spec

```yaml
apiVersion: vyogo.tech/v1
kind: SiteBackup
metadata:
  name: <backup-name>
  namespace: <namespace>
spec:
  # Required: Site to backup (must match existing FrappeSite)
  site: string

  # Optional: Cron schedule for recurring backups (e.g., "0 2 * * *")
  # If empty, creates one-time backup
  schedule: string

  # Optional: Include private and public files in backup
  withFiles: bool  # default: false

  # Optional: Compress backup files
  compress: bool  # default: false

  # Optional: Custom backup path for all files
  backupPath: string

  # Optional: Separate backup paths for specific components
  backupPathDB: string
  backupPathConf: string
  backupPathFiles: string
  backupPathPrivateFiles: string

  # Optional: DocType filtering (comma-separated lists)
  exclude:  # DocTypes to exclude from backup
    - string
  include:  # DocTypes to include in backup
    - string

  # Optional: Backup configuration
  ignoreBackupConf: bool  # default: false

  # Optional: Enable verbose backup output
  verbose: bool  # default: false
```

### Status

```yaml
status:
  # Phase indicates the current phase of the backup (e.g., "Running", "Succeeded", "Failed", "Scheduled").
  phase: string

  # The timestamp of the last successful backup.
  lastBackup: metav1.Time

  # The name of the last backup job or cronjob.
  lastBackupJob: string

  # Additional information about the backup status.
  message: string
```

### Field Details

#### `site` (required)
- **Type:** `string`
- **Description:** Name of the Frappe site to backup
- **Validation:** Must match an existing FrappeSite resource

#### `schedule` (optional)
- **Type:** `string`
- **Description:** Cron expression for scheduled backups
- **Examples:**
  - `"0 2 * * *"` - Daily at 2 AM
  - `"0 */4 * * *"` - Every 4 hours
  - Empty string - One-time backup only

#### Backup Options

##### `withFiles` (optional)
- **Type:** `bool`
- **Default:** `false`
- **Description:** Include private and public files in backup
- **Maps to:** `bench backup --with-files`

##### `compress` (optional)
- **Type:** `bool`
- **Default:** `false`
- **Description:** Compress backup files
- **Maps to:** `bench backup --compress`

##### Custom Paths (optional)
- **`backupPath`**: Main backup path (all files)
- **`backupPathDB`**: Database file path
- **`backupPathConf`**: Configuration file path
- **`backupPathFiles`**: Public files path
- **`backupPathPrivateFiles`**: Private files path

##### DocType Filtering

###### `exclude` (optional)
- **Type:** `[]string`
- **Description:** DocTypes to exclude from backup
- **Example:** `["User", "Role", "Communication"]`
- **Maps to:** `bench backup --exclude "User,Role,Communication"`

###### `include` (optional)
- **Type:** `[]string`
- **Description:** DocTypes to include in backup (all others excluded)
- **Example:** `["DocType", "Module Def", "Custom Field"]`
- **Maps to:** `bench backup --include "DocType,Module Def,Custom Field"`

##### Configuration Flags

###### `ignoreBackupConf` (optional)
- **Type:** `bool`
- **Default:** `false`
- **Description:** Ignore backup configuration excludes/includes
- **Maps to:** `bench backup --ignore-backup-conf`

###### `verbose` (optional)
- **Type:** `bool`
- **Default:** `false`
- **Description:** Enable verbose backup output
- **Maps to:** `bench backup --verbose`

---

## SiteJob

**API Group:** `vyogo.tech/v1`  
**Kind:** `SiteJob`

Executes custom jobs on a site.

### Spec

```yaml
apiVersion: vyogo.tech/v1
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

## <i data-lucide="sparkles"></i> Examples

### Minimal Bench and Site (PostgreSQL StackGres)

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: dev-bench
  namespace: frappe-system
spec:
  frappeVersion: "version-15"
  apps:
    - "erpnext"
---
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: mysite
  namespace: frappe-system
spec:
  benchRef:
    name: dev-bench
  siteName: "mysite.local"
  dbConfig:
    provider: postgres
    postgresEngine: stackgres
    mode: dedicated
    storageSize: "20Gi"
```

### Production Setup (Enterprise HA with Ingress TLS)

```yaml
apiVersion: vyogo.tech/v1
kind: FrappeBench
metadata:
  name: prod-bench
  namespace: frappe-system
spec:
  frappeVersion: "version-15"
  apps:
    - "erpnext"
    - "hrms"
  componentAutoscaling:
    gunicorn:
      enabled: true
      provider: hpa
      minReplicas: 3
      maxReplicas: 10
    worker-short:
      enabled: true
      provider: keda
      minReplicas: 0
      maxReplicas: 5
  componentResources:
    gunicorn:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
---
apiVersion: vyogo.tech/v1
kind: FrappeSite
metadata:
  name: prod-site
  namespace: frappe-system
spec:
  benchRef:
    name: prod-bench
  siteName: "erp.example.com"
  apps:
    - "erpnext"
    - "hrms"
  dbConfig:
    provider: postgres
    postgresEngine: stackgres
    mode: dedicated
    storageSize: "50Gi"
    resources:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "2", memory: "4Gi"}
  ingress:
    enabled: true
    className: "nginx"
    annotations:
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
    tls:
      enabled: true
      certManagerIssuer: "letsencrypt-prod"
```

---

## <i data-lucide="shield-alert"></i> Validation Rules

### FrappeBench Validations

- `frappeVersion` must be specified
- `storageSize` must be a valid Kubernetes resource quantity (e.g. `10Gi`)
- Replica counts must be >= minimum values
- Resource values must be valid Kubernetes quantities

### FrappeSite Validations

- `benchRef.name` must be specified
- `siteName` must be a valid DNS-1123 subdomain
- `dbConfig.provider` must be one of: `postgres`, `mariadb`, `sqlite`, `external`
- `dbConfig.mode` must be one of: `shared`, `dedicated`, `external`
- If `dbConfig.mode` is `external`, `connectionSecretRef` is required

---

## <i data-lucide="check-square"></i> Status Conditions

Resources report their status through the `status` field.

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
  databaseReady: true
  databaseName: "site_prod_db"
  siteURL: "https://erp.example.com"
  databaseCredentialsSecret: "prod-site-db-credentials"
  resolvedDomain: "erp.example.com"
  domainSource: "explicit"
```

---

## <i data-lucide="arrow-right-circle"></i> Next Steps

- **[Examples Directory](../examples/README.md)** - Real-world YAML deployment manifests
- **[Operations Guide](operations.md)** - Managing resources in production
- **[Troubleshooting Guide](troubleshooting.md)** - Resolving common issues and diagnostics

