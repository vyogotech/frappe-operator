# Release Notes - v2.5.0 (OpenShift Compatibility & Security)

This release focuses on making the Frappe Operator fully compatible with OpenShift's strict security model and improving general pod security across all Kubernetes environments.

## Highlights

### 🛡️ Out-of-the-box OpenShift Compatibility
The operator now applies secure defaults that align with OpenShift's `restricted-v2` Security Context Constraint (SCC). All managed pods (Gunicorn, NGINX, Redis, Workers) now run with:
- `runAsUser: 1001` (OpenShift standard arbitrary UID)
- `runAsGroup: 0` (root group for OpenShift compatibility)
- `fsGroup: 0` (root group for filesystem permissions)
- `allowPrivilegeEscalation: false`
- `seccompProfile: { type: RuntimeDefault }`
- `capabilities: { drop: ["ALL"] }`

### ⚙️ Multi-Level Security Configuration
The operator provides **three levels** of UID/GID configuration:

1. **Per-Resource Override** - Set `spec.security` in FrappeBench/FrappeSite (highest priority)
2. **Operator-Level Defaults** - Configure via environment variables: `FRAPPE_DEFAULT_UID`, `FRAPPE_DEFAULT_GID`, `FRAPPE_DEFAULT_FSGROUP`
3. **Hardcoded Defaults** - OpenShift-compatible defaults (1001/0/0) when nothing else is set

This flexible approach allows different UIDs for different benches in the same cluster, organization-wide defaults, or OpenShift-compatible out-of-the-box behavior.

## API Changes

### FrappeBench & FrappeSite Spec
Added a `security` field of type `SecurityConfig`:

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  # Option 1: Use OpenShift defaults (no configuration needed)
  # Automatically uses UID 1001, GID 0, FSGroup 0
  
  # Option 2: Override for specific bench
  security:
    podSecurityContext:
      runAsUser: 2000      # Custom UID for this bench
      runAsGroup: 2000     # Custom GID
      fsGroup: 2000        # Custom FSGroup
    securityContext:
      runAsUser: 2000
      runAsGroup: 2000
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
```

### Operator Environment Variables
Configure cluster-wide defaults by setting environment variables in the operator deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frappe-operator-controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: FRAPPE_DEFAULT_UID
          value: "2000"        # All benches default to this UID
        - name: FRAPPE_DEFAULT_GID
          value: "2000"
        - name: FRAPPE_DEFAULT_FSGROUP
          value: "2000"
```

## Configuration Examples

### Example 1: OpenShift (Default)
No configuration needed - works out of the box:
```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
spec:
  # Automatically uses UID 1001, GID 0 for OpenShift compatibility
  apps:
    - name: erpnext
```

### Example 2: Mixed UIDs in Same Cluster
```yaml
# Production bench: Uses OpenShift defaults (1001/0/0)
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: production-bench
spec:
  apps:
    - name: erpnext

---
# Compliance bench: Custom UID required
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: compliance-bench
spec:
  security:
    podSecurityContext:
      runAsUser: 5000  # Override for compliance requirements
      runAsGroup: 5000
      fsGroup: 5000
  apps:
    - name: erpnext
```

## Implementation Details

- **Secure Defaults:** All controllers now use OpenShift-compatible defaults (UID 1001, GID 0, FSGroup 0) that work with OpenShift's arbitrary UID mechanism
- **Configurable via Environment:** Set `FRAPPE_DEFAULT_UID`, `FRAPPE_DEFAULT_GID`, `FRAPPE_DEFAULT_FSGROUP` environment variables in the operator deployment to change cluster-wide defaults
- **User Overrides:** If `spec.security` is provided in a FrappeBench or FrappeSite, it takes precedence over environment variables and hardcoded defaults
- **Priority Chain:** `spec.security` → Environment Variables → Hardcoded Defaults (1001/0/0)
- **Initialization Jobs:** Both bench and site initialization jobs inherit the same security policies
- **Full Security Context:** All three values (runAsUser, runAsGroup, fsGroup) are now properly set together to avoid PSP validation errors

## Why UID 1001 and GID 0?

**OpenShift Security Model:**
- OpenShift assigns arbitrary UIDs in the range 1000000000-2147483647
- Uses GID 0 (root group) for filesystem compatibility without granting root user privileges
- This allows containers built for UID 1001 to run with arbitrary UIDs assigned by OpenShift
- The root group (GID 0) provides supplementary permissions for shared filesystem access

**Frappe Container Compatibility:**
- Standard Frappe containers are built with `USER 1001` and GID 0
- Files are owned by UID 1001, GID 0
- This matches OpenShift's arbitrary UID support pattern perfectly

## Upgrade Path

1. **Update the operator** to v2.5.0:
   ```bash
   kubectl set image deployment/frappe-operator-controller-manager \
     -n frappe-operator-system \
     manager=vyogo.tech/frappe-operator:v2.5.0
   ```

2. **Existing resources** will be updated automatically on the next reconciliation with the new security defaults (UID 1001, GID 0)

3. **OpenShift users:** No additional configuration needed - the operator now uses OpenShift-compatible defaults

4. **Custom UID requirements:**
   - **Cluster-wide:** Set environment variables in the operator deployment
   - **Per-bench:** Use the `spec.security` field in your FrappeBench/FrappeSite resources

5. **Verify the upgrade:**
   ```bash
   # Check operator version
   kubectl get deployment frappe-operator-controller-manager \
     -n frappe-operator-system \
     -o jsonpath='{.spec.template.spec.containers[0].image}'
   
   # Verify security context on running pods
   kubectl get pod -l app=your-bench-gunicorn \
     -o jsonpath='{.items[0].spec.securityContext}'
   ```

## Breaking Changes

**None.** This release is fully backward compatible:
- OpenShift-compatible defaults (1001/0/0) match standard Frappe container builds
- Existing benches with custom `spec.security` continue to work unchanged
- Environment variables provide global configuration without requiring manifest changes
- All changes are applied during normal reconciliation without manual intervention

## Documentation

For complete documentation on security context configuration, see:
- [SECURITY_CONTEXT_FIX.md](../SECURITY_CONTEXT_FIX.md) - Implementation details and examples
- [Operations Guide](operations.md#security) - Production security best practices
- [Examples](../examples/) - Sample configurations for different scenarios

---
[Full Changelog](file:///Users/varkrish/personal/1frappe_ecosystem/frappe-operator/CHANGELOG.md)
