# Image Configuration System

## Overview

The Frappe Operator now supports configurable image defaults at the operator level, which can be overridden per-bench. This provides flexibility for different deployment environments (air-gapped, enterprise registries, etc.).

## Configuration Priority

Image selection follows this priority order:

1. **Bench-level override** (`bench.spec.imageConfig`)
2. **Operator ConfigMap defaults** (`frappe-operator-config`)
3. **Hardcoded constants** (`pkg/constants/images.go`)

## Operator-Level Configuration

### ConfigMap Configuration

The operator reads default images from the `frappe-operator-config` ConfigMap in the `frappe-operator-system` namespace.

**ConfigMap Fields:**
- `defaultFrappeImage`: Default Frappe/ERPNext image (e.g., `docker.io/frappe/erpnext:latest`)
- `defaultMariaDBImage`: Default MariaDB image
- `defaultPostgresImage`: Default PostgreSQL image
- `defaultRedisImage`: Default Redis image
- `defaultNginxImage`: Default Nginx image

### Helm Values Configuration

When deploying via Helm, configure defaults in `values.yaml`:

```yaml
operatorConfig:
  defaultFrappeImage: "myregistry.com/frappe/erpnext:latest"
  defaultMariaDBImage: "myregistry.com/library/mariadb:10.6"
  defaultPostgresImage: "myregistry.com/library/postgres:15-alpine"
  defaultRedisImage: "myregistry.com/library/redis:7-alpine"
  defaultNginxImage: "myregistry.com/library/nginx:1.25-alpine"
```

### Direct ConfigMap Edit

You can also edit the ConfigMap directly:

```bash
kubectl edit configmap frappe-operator-config -n frappe-operator-system
```

## Bench-Level Override

Individual benches can override operator defaults:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  frappeVersion: "15"
  imageConfig:
    repository: "myregistry.com/custom/frappe"
    tag: "v15-custom"
    pullPolicy: Always
    pullSecrets:
      - name: registry-credentials
```

## Version Handling

When `frappeVersion` is specified in the bench spec:
- If `imageConfig.repository` is set but `tag` is not, the version is used as the tag
- If using ConfigMap defaults, the version replaces the tag in the default image
- If no defaults are set, `docker.io/frappe/erpnext:{version}` is used

## Examples

### Example 1: Air-Gapped Environment

```yaml
# Operator ConfigMap
defaultFrappeImage: "internal-registry.company.com/frappe/erpnext:latest"
defaultMariaDBImage: "internal-registry.company.com/library/mariadb:10.6"
```

### Example 2: OpenShift with Red Hat Images

```yaml
# Operator ConfigMap
defaultFrappeImage: "image-registry.openshift-image-registry.svc:5000/frappe/erpnext:latest"
defaultMariaDBImage: "registry.redhat.io/rhel8/mariadb-103:latest"
```

### Example 3: Per-Bench Custom Image

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
spec:
  frappeVersion: "15"
  imageConfig:
    repository: "production-registry.com/frappe/erpnext"
    tag: "v15.41.2"
    pullPolicy: Always
```

## Implementation Details

### Code Changes

1. **`controllers/frappebench_controller.go`**:
   - Updated `getBenchImage()` to check ConfigMap defaults
   - Added `context.Context` parameter for ConfigMap access

2. **`controllers/frappesite_controller.go`**:
   - Updated `getBenchImage()` with same priority logic
   - Added `getOperatorConfig()` helper method

3. **`controllers/frappebench_resources.go`**:
   - Updated all `getBenchImage()` calls to pass context

4. **`config/manager/operator-config.yaml`**:
   - Added default image fields

5. **`helm/frappe-operator/templates/configmap.yaml`**:
   - Added image default fields to ConfigMap template

6. **`helm/frappe-operator/values.yaml`**:
   - Added image default values

## Benefits

1. **Centralized Configuration**: Set defaults once at operator level
2. **Environment Flexibility**: Different defaults for dev/staging/prod
3. **Air-Gapped Support**: Easy configuration for internal registries
4. **Per-Bench Override**: Still allows bench-specific customization
5. **Backward Compatible**: Falls back to constants if ConfigMap not configured

## Migration

Existing deployments continue to work:
- If ConfigMap doesn't have image defaults, constants are used
- If bench has `imageConfig`, it takes precedence
- No breaking changes to existing CRDs
