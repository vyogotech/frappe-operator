# Release Notes - v2.5.0 (OpenShift Compatibility & Security)

This release focuses on making the Frappe Operator fully compatible with OpenShift's strict security model and improving general pod security across all Kubernetes environments.

## Highlights

### 🛡️ Out-of-the-box OpenShift Compatibility
The operator now applies secure defaults that align with OpenShift's `restricted-v2` Security Context Constraint (SCC). All managed pods (Gunicorn, NGINX, Redis, Workers) now run with:
- `runAsNonRoot: true`
- `allowPrivilegeEscalation: false`
- `seccompProfile: { type: RuntimeDefault }`
- `capabilities: { drop: ["ALL"] }`

### ⚙️ Customizable Security Contexts
A new `security` field has been added to the `FrappeBench` specification, allowing users to override pod-level and container-level security settings for complex environments.

## API Changes

### FrappeBench Spec
Added a `security` field of type `SecurityConfig`:

```yaml
spec:
  security:
    podSecurityContext:
      fsGroup: 1000
    securityContext:
      runAsUser: 1000
```

## Implementation Details

- **Secure Defaults:** All controllers now inject hardcoded secure defaults into `PodSpec` and `Container` templates.
- **User Overrides:** If `spec.security` is provided, user-defined values take precedence over operator defaults (except where absolute security is required).
- **Initialization Jobs:** Both bench and site initialization jobs now inherit the same security policies.

## Upgrade Path

1. Update the operator image to `v2.5.0`.
2. Existing `FrappeBench` resources will be updated automatically on the next reconciliation, applying the new secure defaults to underlying deployments.
3. If running on OpenShift, ensure your `FrappeBench` service account has necessary permissions or use the new `security` field to tune UIDs.

---
[Full Changelog](file:///Users/varkrish/personal/1frappe_ecosystem/frappe-operator/CHANGELOG.md)
