# Site-Specific App Installation

## Overview

The Frappe Operator supports installing specific apps during site creation through the FrappeSite CRD. This feature provides:

- **Site-level App Selection**: Choose which apps to install per site, even when multiple sites share the same bench
- **Graceful Handling**: Apps that aren't available in the container are skipped with warnings, not errors
- **Filesystem Verification**: Apps are checked against actual filesystem (apps directory) and apps.txt
- **Detailed Logging**: Every step of app installation is logged for debugging and auditing
- **Flexible Validation**: Invalid app names cause warnings, missing apps are skipped gracefully
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

1. **Apps are checked in the container**: The operator checks for apps in the actual container filesystem (apps directory) and apps.txt, not just the bench CRD specification.

2. **Graceful handling of missing apps**: If an app specified in the CRD is not available in the container, it will be skipped with a warning in the logs. The site creation will continue successfully with the available apps.

3. **Apps are installed during initial site creation only**: Apps are installed when the site is first created using `bench new-site --install-app=<app>`. The apps field is effectively immutable - if you update it after the site is created, the operator will not attempt to install or remove apps. To add/remove apps on an existing site, use bench commands directly (`bench install-app` or `bench uninstall-app`).

4. **If no apps are specified**: Only the frappe framework will be installed on the site (no additional apps beyond frappe).

## Validation

The operator performs the following validations and checks:

1. **App name validation**: App names are checked for valid characters (alphanumeric, underscore, hyphen only) to prevent shell injection. Invalid names generate warnings and are skipped.

2. **Filesystem verification**: During site creation, the initialization script checks that each app's directory exists in the bench's apps folder.

3. **apps.txt check**: The script also displays contents of apps.txt if available for reference.

4. **Graceful skipping**: If an app isn't available in the filesystem, it's skipped with a warning rather than failing the entire site creation.

### Example: App Not Available

Instead of failing, the script outputs:
```
⚠ WARNING: App 'custom_app' not found in bench directory - skipping
  The app may not be installed in this bench yet
```

And continues with the apps that are available.

## Status Tracking

The FrappeSite status includes several fields to track app installation:

### Status Fields

```yaml
status:
  # List of requested apps (some may have been skipped if not available)
  installedApps:
    - erpnext
    - hrms
  
  # Overall installation status message
  appInstallationStatus: "Completed app installation for 2 requested app(s) - check logs for any skipped apps"
```

Note: The `installedApps` field shows apps that were requested, not necessarily all installed. Check the job logs to see which apps were actually installed vs skipped. The `failedApps` field is reserved for future use.

### Status Messages

During installation:
- `"Installing <N> app(s)..."` - Installation in progress

On completion:
- `"Completed app installation for N requested app(s) - check logs for any skipped apps"` - Installation completed (some may have been skipped)
- `"No apps specified - only frappe framework installed"` - No apps requested

On failure:
- `"Failed to install apps: <error details>"` - Site creation failed

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
Normal  AppsRequested          Requested 2 app(s): [erpnext hrms] - will check availability in container
Normal  AppsProcessed          Processed app installation for: [erpnext hrms] - check job logs for any skipped apps
Warning InvalidAppName         App 'my-app@123' contains invalid characters and will be skipped
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
Available apps in bench (from filesystem):
erpnext
hrms

Apps listed in apps.txt:
frappe
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

### App Skipping (Graceful Handling)

```
==========================================
App Installation Configuration
==========================================
Apps requested for installation: erpnext custom_app hrms
Available apps in bench (from filesystem):
erpnext
hrms
frappe (framework - always available)
------------------------------------------
✓ App 'erpnext' found in bench and will be installed
⚠ WARNING: App 'custom_app' not found in bench directory - skipping
  The app may not be installed in this bench yet
✓ App 'hrms' found in bench and will be installed
------------------------------------------
⚠ Skipped apps (not available): custom_app
  These apps will not be installed on this site
  To install them later, ensure they're available in the bench
  and use: bench --site mysite.example.com install-app custom_app
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

### App Not Found - Graceful Skipping

**Log Message:**
```
⚠ WARNING: App 'custom_app' not found in bench directory - skipping
  The app may not be installed in this bench yet
⚠ Skipped apps (not available): custom_app
  These apps will not be installed on this site
```

**Behavior:** The site creation continues successfully with the apps that are available. No error is raised.

**Kubernetes Event:**
```
Normal  AppsProcessed  Processed app installation for: [erpnext custom_app hrms] - check job logs for any skipped apps
```

### Invalid App Name

**Log Message:**
```
Skipping app with invalid characters: my-app@123
```

**Kubernetes Event:**
```
Warning  InvalidAppName  App 'my-app@123' contains invalid characters and will be skipped
```

**Behavior:** App is skipped, site creation continues.

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

### Problem: App listed in CRD but not installing

**Check:**
1. View the initialization job logs to see if the app was skipped:
   ```bash
   kubectl logs job/my-site-init
   ```
   Look for warning messages about skipped apps.

2. Verify the app exists in the bench pod:
   ```bash
   kubectl exec -it deployment/my-bench-gunicorn -- ls -la apps/
   kubectl exec -it deployment/my-bench-gunicorn -- cat sites/apps.txt
   ```

3. If the app is missing, it needs to be installed in the bench first before the site can use it.

### Problem: Want to install app after site creation

**Solution:**
Since apps are only installed during initial site creation through the CRD, you need to use bench commands:

```bash
# Get a shell in the bench pod
kubectl exec -it deployment/my-bench-gunicorn -- bash

# Install the app
cd /home/frappe/frappe-bench
bench --site mysite.example.com install-app custom_app
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

1. **Apps are immutable after initial site creation**: Once a site is created, updating the apps field in the CRD spec will have no effect. The apps can only be installed during the initial `bench new-site` command. If you need to add or remove apps after site creation, use `bench install-app` or `bench uninstall-app` commands directly on the bench.

2. **Apps must exist in container filesystem**: Apps are checked in the actual container (apps directory), not just the bench CRD spec.

3. **No partial installation tracking**: Currently, the status shows all requested apps, not which were actually installed vs skipped. Check job logs for details on skipped apps.

4. **No automatic app sync**: If you add an app to the bench after site creation, existing sites won't automatically get it installed. You must manually install it on each site that needs it.

## Future Enhancements

Potential future improvements:

- Support for installing apps after site creation
- Ability to uninstall apps through the CRD
- App dependency resolution
- Parallel app installation for faster setup
- App version pinning per site
