# Ingress Testing Plan

## Current Status
- demo-site: ingress enabled, status: Failed (site init job failing)
- test-site: no ingress config, status: Unknown
- test-site-v2: ingress enabled, status: Failed (site init job failing)

## Root Cause
Sites are failing initialization with:
```
ERROR 1045 (28000): Access denied for user '_<user>'@'10.244.0.75' (using password: YES)
```

This indicates the database credentials are not being passed correctly to the init job.

## Ingress Testing Strategy

### Phase 1: Fix Site Initialization (Prerequisite)
Before testing ingress, we need working sites. The issue is that the init script
is not receiving proper database credentials from the DatabaseCredentials object.

1. Debug: Check what credentials are being retrieved from the database provider
2. Verify: DB_USER matches the actual MariaDB User CR username
3. Fix: Ensure credentials are passed to init job correctly

### Phase 2: Ingress Creation Testing
Once sites initialize successfully:

1. Verify ingress resources are created:
   ```bash
   kubectl get ingress -n frappe-test
   ```

2. Check ingress rules:
   ```bash
   kubectl describe ingress <site-name> -n frappe-test
   ```

3. Verify ingress points to correct service:
   ```bash
   kubectl get svc -n frappe-test | grep <site-name>
   ```

### Phase 3: End-to-End Ingress Testing
1. Get ingress hostname/IP
2. Add to /etc/hosts for DNS resolution (for .local domains)
3. Test HTTP request through ingress
4. Verify request reaches correct site pods
5. Test TLS (if enabled)

### Phase 4: Multi-Site Ingress Testing
1. Create multiple sites with different domains
2. Verify each has correct ingress rules
3. Test traffic routing to correct site
4. Verify no cross-site bleeding

## Next Steps

1. First, fix the site initialization issue
2. Then test ingress creation and routing
3. Document findings in TEST_REPORT_INGRESS.md
