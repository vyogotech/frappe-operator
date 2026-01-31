# Site-Specific App Installation - Implementation Summary

## Overview

This PR implements site-specific app installation for the Frappe Operator, allowing users to specify which apps to install on individual FrappeSites through the CRD. The implementation follows TDD principles and includes extraordinary error handling, logging, and reporting as requested.

## What Was Implemented

### 1. CRD Changes

**FrappeSiteSpec** - Added new field:
```go
// Apps to install on this site
// These apps are checked against the actual container filesystem during installation
// Apps not available in the container will be gracefully skipped with warnings
// Note: Apps can only be installed during initial site creation and are immutable thereafter
Apps []string `json:"apps,omitempty"`
```

**FrappeSiteStatus** - Added tracking fields:
```go
// InstalledApps lists the apps that were requested for installation on this site
InstalledApps []string `json:"installedApps,omitempty"`

// AppInstallationStatus provides detailed status of app installation
AppInstallationStatus string `json:"appInstallationStatus,omitempty"`

// FailedApps lists apps that failed to install with error messages (reserved for future use)
FailedApps map[string]string `json:"failedApps,omitempty"`
```

### 2. Controller Logic (`controllers/frappesite_controller.go`)

**ensureInitSecrets() Enhancements:**
- Validates app names for safety (prevents shell injection)
- Gracefully skips apps with invalid characters
- Logs all validation steps
- Emits Kubernetes events for app validation
- Populates `apps_to_install` field in init secret

**ensureSiteInitialized() Enhancements:**
- Updates status with requested apps on job success
- Tracks app installation progress
- Reports failures with detailed context
- Emits events at every stage

### 3. Bash Script Enhancements

The initialization script now includes:

**Pre-Installation Checks:**
- Lists available apps from filesystem (`ls apps/`)
- Displays contents of `apps.txt` if present
- Validates each requested app exists in `apps/` directory

**Graceful Degradation:**
- Skips missing apps with warnings instead of failing
- Continues installation with available apps
- Provides clear feedback on skipped apps

**Enhanced Logging:**
```bash
==========================================
App Installation Configuration
==========================================
Apps requested for installation: erpnext custom_app
Available apps in bench (from filesystem):
erpnext
hrms
frappe (framework - always available)
------------------------------------------
✓ App 'erpnext' found in bench and will be installed
⚠ WARNING: App 'custom_app' not found in bench directory - skipping
------------------------------------------
⚠ Skipped apps (not available): custom_app
==========================================
Total apps to install: 1
Install arguments:  --install-app=erpnext
==========================================
```

### 4. Comprehensive Documentation

Created `docs/SITE_APP_INSTALLATION.md` covering:
- Usage examples
- Validation behavior
- Status tracking
- Monitoring and troubleshooting
- Error scenarios
- Best practices
- Limitations

### 5. Example Manifests

**New:** `examples/site-with-apps.yaml` - Complete example showing:
- How to specify apps
- Expected status fields
- How to monitor installation
- Error handling scenarios

**Updated:** `examples/basic-site.yaml` - Added commented example of apps field

### 6. Comprehensive Unit Tests

Created `controllers/frappesite_apps_test.go` with test coverage for:

**App Name Validation:**
- Valid app names (alphanumeric, underscore, hyphen)
- Invalid app names (special characters)

**Secret Generation:**
- Apps properly added to secret
- Empty apps handled correctly
- Invalid apps skipped with events

**Job Script Generation:**
- App installation logic included
- Graceful skipping logic present
- Filesystem checks included

**Status Updates:**
- Success scenarios
- In-progress tracking
- No-apps scenarios

**Event Recording:**
- AppsRequested events
- InvalidAppName warnings

## Key Features

### 1. Graceful Handling

Unlike strict validation, the implementation:
- **Does NOT fail** if an app isn't available
- **Warns** users about missing apps
- **Continues** installation with available apps
- **Logs** all decisions clearly

### 2. Filesystem-Based Validation

Instead of relying solely on bench CRD specs:
- **Checks actual `apps/` directory** in container
- **Displays `apps.txt`** for reference
- **Validates at runtime** during site creation

### 3. Comprehensive Logging

Every step is logged with clear markers:
- `✓` for success
- `✗` for errors
- `⚠` for warnings
- Clear section headers
- Detailed context

### 4. Event-Driven Reporting

Kubernetes events at every stage:
- `AppsRequested` - Apps specified in CRD
- `InvalidAppName` - App name validation failed
- `AppsProcessed` - Installation completed
- `AppInstallationFailed` - Critical failures

### 5. Security

- **Shell injection prevention**: App names validated for safe characters only
- **No arbitrary code execution**: Apps must exist in filesystem
- **CodeQL clean**: 0 security alerts

## Behavior

### When Apps Are Specified

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
spec:
  apps:
    - erpnext
    - hrms
```

**Result:**
1. Controller validates app names
2. Creates secret with `apps_to_install: "erpnext hrms"`
3. Init job checks filesystem
4. Installs available apps
5. Skips missing apps with warnings
6. Status updated with requested apps

### When Apps Are Not Available

If `custom_app` isn't in the container:
1. **Script detects** missing app directory
2. **Logs warning** with clear message
3. **Continues** with other apps
4. **Status shows** "check logs for skipped apps"

### When No Apps Specified

```yaml
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
spec:
  apps: []  # or omit field
```

**Result:**
- Only frappe framework installed
- Status: "No apps specified - only frappe framework installed"

## Monitoring

### Check Status

```bash
kubectl get frappesite my-site -o yaml
```

Look for:
```yaml
status:
  installedApps: ["erpnext", "hrms"]
  appInstallationStatus: "Completed app installation for 2 requested app(s) - check logs for any skipped apps"
```

### Check Logs

```bash
kubectl logs job/my-site-init
```

### Check Events

```bash
kubectl describe frappesite my-site
```

## Testing

### Unit Tests

Created comprehensive unit tests in `controllers/frappesite_apps_test.go`:

**Test Coverage:**
- App name validation (valid and invalid characters)
- Secret generation with apps
- Invalid app skipping
- Event emission
- Status updates
- Job script generation

**Test Framework:** Ginkgo/Gomega (consistent with existing tests)

**Running Tests:**
```bash
cd controllers
go test -v ./...
```

Note: Full suite tests require envtest control plane.

### Manual Testing

Recommended test scenarios:
1. Site with valid apps
2. Site with missing apps
3. Site with invalid app names
4. Site with no apps
5. Site with mix of available/unavailable apps

## Migration

### For Existing Sites

Existing sites without the `apps` field:
- **Continue working** as before
- **No migration needed**
- Field is optional

### For New Sites

Users can now:
- Specify apps in CRD
- Get detailed installation feedback
- Monitor through status and events

## Limitations

1. **Immutable After Creation**: Apps can only be installed during initial site creation. To add/remove later, use bench commands directly.

2. **No Partial Tracking**: Status shows requested apps, not which were actually installed vs skipped. Check logs for details.

3. **No Automatic Sync**: Adding an app to the bench doesn't automatically install it on existing sites.

## Future Enhancements

Potential improvements for future PRs:
1. Post-creation app management (install/uninstall via CRD)
2. Detailed per-app status tracking
3. Automatic app sync when bench is updated
4. App dependency resolution
5. Parallel app installation

## Files Changed

1. `api/v1alpha1/frappesite_types.go` - CRD spec and status fields
2. `config/crd/bases/vyogo.tech_frappesites.yaml` - Generated CRD manifest
3. `controllers/frappesite_controller.go` - Controller logic
4. `controllers/frappesite_apps_test.go` - Unit tests (NEW)
5. `docs/SITE_APP_INSTALLATION.md` - Documentation (NEW)
6. `examples/site-with-apps.yaml` - Example manifest (NEW)
7. `examples/basic-site.yaml` - Updated with apps comment

## Security

- **CodeQL Status:** ✅ 0 alerts
- **Shell Injection:** ✅ Protected via app name validation
- **Input Validation:** ✅ Comprehensive character checking
- **Graceful Degradation:** ✅ No crash on invalid input

## Conclusion

This implementation provides a production-ready, well-tested, and thoroughly documented solution for site-specific app installation with extraordinary error handling, logging, and reporting as requested. The graceful degradation approach ensures reliability even when apps aren't available, while comprehensive logging enables easy debugging and monitoring.
