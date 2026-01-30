# Site-Specific App Installation

## Overview

The Frappe Operator now supports installing specific apps during site creation through the FrappeSite CRD. This feature provides:

- **Site-level App Selection**: Choose which apps to install per site, even when multiple sites share the same bench
- **Comprehensive Validation**: Apps are validated against the bench's available apps before installation
- **Detailed Logging**: Every step of app installation is logged for debugging and auditing
- **Error Handling**: Clear error messages and events when app installation fails
- **Status Tracking**: Installation status and results are tracked in the site's status fields

## Usage

### Basic Example

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: my-site
  namespace: default
spec:
  benchRef:
    name: my-bench
  siteName: mysite.example.com
  
  # Specify apps to install
  apps:
    - erpnext
    - hrms
  
  dbConfig:
    provider: mariadb
    mode: shared
```

### Important Notes

1. **Apps must be available in the bench**: The apps specified in the site must be available in the referenced FrappeBench. The operator validates this before attempting installation.

2. **Apps are installed during site creation**: Apps are installed when the site is first created using `bench new-site --install-app=<app>`. They cannot be added or removed after site creation through this field.

3. **If no apps are specified**: Only the frappe framework will be installed on the site.

## Validation

The operator performs the following validations:

1. **Pre-creation validation**: Before creating the initialization job, the controller checks that all specified apps are available in the bench's installed apps list.

2. **Runtime validation**: The initialization script validates that each app's directory exists in the bench before attempting installation.

3. **Error reporting**: If validation fails at any stage, clear error messages are logged and emitted as Kubernetes events.

### Example Validation Error

```
ERROR: App 'custom_app' not available in bench my-bench. Available apps: frappe, erpnext, hrms
```

## Status Tracking

The FrappeSite status includes several fields to track app installation:

### Status Fields

```yaml
status:
  # List of successfully installed apps
  installedApps:
    - erpnext
    - hrms
  
  # Overall installation status message
  appInstallationStatus: "Successfully installed 2 app(s)"
  
  # Map of failed apps with error messages (if any)
  failedApps: {}
```

### Status Messages

During installation:
- `"Installing 2 app(s)..."` - Installation in progress

On success:
- `"Successfully installed 2 app(s)"` - All apps installed successfully
- `"No apps specified - only frappe framework installed"` - No apps requested

On failure:
- `"Failed to install apps: <error details>"` - Installation failed with details

## Monitoring Installation

### 1. Check Site Status

```bash
kubectl get frappesite my-site -o yaml
```

### 2. View Installed Apps

```bash
kubectl get frappesite my-site -o jsonpath='{.status.installedApps}'
```

### 3. Check Installation Status

```bash
kubectl get frappesite my-site -o jsonpath='{.status.appInstallationStatus}'
```

### 4. View Events

```bash
kubectl describe frappesite my-site
```

Example events:
```
Normal  AppsSpecified          Apps to install: [erpnext hrms]
Normal  CreatingInitJob        Creating initialization job to install 2 app(s): [erpnext hrms]
Normal  AppsInstalled          Successfully installed apps: [erpnext hrms]
```

### 5. View Initialization Job Logs

```bash
# Get the job name
kubectl get jobs -l site=my-site

# View logs
kubectl logs job/my-site-init
```

## Logging

The initialization script provides comprehensive logging:

### Pre-Installation Logging

```
==========================================
App Installation Configuration
==========================================
Apps requested for installation: erpnext hrms
Available apps in bench:
erpnext
hrms
frappe (framework - always available)
------------------------------------------
✓ App 'erpnext' found in bench and will be installed
✓ App 'hrms' found in bench and will be installed
==========================================
Total apps to install: 2
Install arguments:  --install-app=erpnext --install-app=hrms
==========================================
```

### Site Creation Logging

```
==========================================
Creating Frappe Site
==========================================
Site Name: mysite.example.com
Database Provider: mariadb
Database Name: mysite-db
Database Host: mariadb:3306
Apps to install: erpnext hrms
==========================================
✓ Detected support for --db-user flag

Executing: bench new-site with apps:  --install-app=erpnext --install-app=hrms
------------------------------------------
[bench new-site output...]
------------------------------------------
✓ Site created successfully!
✓ All 2 app(s) installed successfully
Installed apps:
  ✓ erpnext
  ✓ hrms
==========================================
```

## Error Handling

### App Not Found in Bench

**Error Message:**
```
✗ ERROR: App 'custom_app' not found in bench directory
ERROR: Cannot install app 'custom_app' - not available in bench
Available apps: frappe, erpnext, hrms
```

**Kubernetes Event:**
```
Warning  InvalidApps  Apps not available in bench my-bench: [custom_app]. Available apps: [frappe erpnext hrms]
```

**Status:**
```yaml
status:
  appInstallationStatus: "Failed to install apps: <error details>"
```

### Installation Failure

**Error Message:**
```
✗ ERROR: Site creation failed with exit code 1
Error output:
[Detailed traceback and error information]
CRITICAL ERROR: Site creation failed. Exiting.
```

**Kubernetes Event:**
```
Warning  SiteInitializationFailed  Site initialization failed: exit code 1
Warning  AppInstallationFailed     Failed to install apps. Check pod my-site-init-xxxxx logs for details
```

**Pod Status:**
The initialization job pod will show as Failed, and you can retrieve detailed logs using:
```bash
kubectl logs <pod-name>
```

## Troubleshooting

### Problem: App not installing

**Check:**
1. Verify the app is listed in the bench's status:
   ```bash
   kubectl get frappebench my-bench -o jsonpath='{.status.installedApps}'
   ```

2. Check if the app directory exists in the bench:
   ```bash
   kubectl exec -it deployment/my-bench-gunicorn -- ls -la apps/
   ```

3. Review initialization job logs for errors:
   ```bash
   kubectl logs job/my-site-init
   ```

### Problem: Installation job keeps failing

**Steps:**
1. Check the job's pod logs for detailed error messages:
   ```bash
   kubectl get pods -l job-name=my-site-init
   kubectl logs <pod-name>
   ```

2. Verify database connectivity and credentials

3. Check bench status and ensure it's ready:
   ```bash
   kubectl get frappebench my-bench
   ```

4. Review events for the site:
   ```bash
   kubectl describe frappesite my-site
   ```

## Integration with FrappeBench

The FrappeSite's app installation integrates with the FrappeBench's app configuration:

```yaml
# FrappeBench
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: my-bench
spec:
  frappeVersion: "version-15"
  
  # Apps available in this bench
  apps:
    - name: frappe
      source: image
    - name: erpnext
      source: fpm
      org: frappe
      version: "15.0.0"
    - name: hrms
      source: fpm
      org: frappe
      version: "15.0.0"

# FrappeSite (can only install apps available in bench)
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: my-site
spec:
  benchRef:
    name: my-bench
  
  # Select which apps to install on this site
  apps:
    - erpnext
    - hrms
```

## Best Practices

1. **Validate bench apps first**: Before creating a site, check the bench status to see which apps are available.

2. **Use descriptive site names**: This makes it easier to identify sites and their purposes in logs and status.

3. **Monitor events**: Use `kubectl describe` to monitor progress and catch errors early.

4. **Keep logs**: Job logs are automatically retained and can be reviewed for debugging.

5. **Test in development first**: Test app combinations in a development environment before deploying to production.

6. **Version compatibility**: Ensure the apps you're installing are compatible with the Frappe version in the bench.

## Limitations

1. **Apps are immutable after creation**: Once a site is created, you cannot add or remove apps through the CRD. Use `bench install-app` or `bench uninstall-app` commands directly if needed.

2. **Apps must pre-exist in bench**: You cannot specify apps that aren't already available in the bench.

3. **No partial installation**: If one app fails to install, the entire site creation fails. This ensures consistency.

## Future Enhancements

Potential future improvements:

- Support for installing apps after site creation
- Ability to uninstall apps through the CRD
- App dependency resolution
- Parallel app installation for faster setup
- App version pinning per site
