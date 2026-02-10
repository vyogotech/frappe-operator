# Configuration System for Frappe Operator Autoscaling Tests

This document describes the configuration system that replaces hardcoded values in the autoscaling test infrastructure.

## Overview

The configuration system allows flexible customization of test parameters without modifying scripts. It follows TDD principles by ensuring all existing tests pass while adding new functionality.

## Files

### Configuration Files
- **`test-config.conf`** - Default configuration with all customizable parameters
- Custom configuration files can be created for different test scenarios

### Scripts
- **`test-autoscaling-helm.sh`** - Main test script now uses configuration
- **`generate-test-manifests.sh`** - Generates test manifests from configuration
- **`test-configuration.sh`** - Tests configuration functionality

### Generated Files
- **`examples/test-keda-bench.yaml`** - Generated KEDA test manifest
- **`examples/test-hpa-bench.yaml`** - Generated HPA test manifest

## Usage

### Basic Usage (uses default configuration)
```bash
./test-autoscaling-helm.sh keda
./test-autoscaling-helm.sh hpa
./test-autoscaling-helm.sh provider-switch
./test-autoscaling-helm.sh all
```

### Custom Configuration
```bash
./test-autoscaling-helm.sh --config my-config.conf keda
```

### Override Specific Parameters
```bash
./test-autoscaling-helm.sh --cluster-name my-cluster --namespace my-ns keda
```

### Generate Test Manifests
```bash
./generate-test-manifests.sh
./generate-test-manifests.sh --config my-config.conf
```

## Configuration Parameters

### Cluster Configuration
- `DEFAULT_CLUSTER_NAME` - Kind cluster name (default: "frappe-autoscaling-helm")
- `DEFAULT_NAMESPACE` - Kubernetes namespace (default: "default")

### Container Images
- `OPERATOR_IMAGE_REPOSITORY` - Operator image repository
- `OPERATOR_IMAGE_TAG` - Operator image tag
- `FRAPPE_IMAGE_REPOSITORY` - Frappe image repository
- `FRAPPE_IMAGE_TAG` - Frappe image tag

### Timeouts and Intervals
- `HELM_TIMEOUT` - Helm installation timeout
- `OPERATOR_TIMEOUT` - Operator readiness timeout
- `BENCH_READY_TIMEOUT` - Bench readiness timeout
- `BENCH_READY_SLEEP` - Polling interval for bench readiness
- `PROVIDER_SWITCH_TIMEOUT` - Provider switch timeout
- `PROVIDER_SWITCH_SLEEP` - Polling interval for provider switch

### Test Configuration
- `KEDA_BENCH_NAME` - KEDA test bench name
- `HPA_BENCH_NAME` - HPA test bench name
- `DEFAULT_MIN_REPLICAS` - Default minimum replicas
- `DEFAULT_MAX_REPLICAS` - Default maximum replicas
- `DEFAULT_HPA_TARGET_UTILIZATION` - HPA target utilization
- `DEFAULT_KEDA_TARGET_VALUE` - KEDA target value
- `DEFAULT_KEDA_POLLING_INTERVAL` - KEDA polling interval
- `DEFAULT_KEDA_COOLDOWN_PERIOD` - KEDA cooldown period

### Application Configuration
- `DEFAULT_FRAPPE_VERSION` - Frappe version
- `NGINX_COMPONENT` - Nginx component name

## Creating Custom Configuration

Create a copy of `test-config.conf` and modify the values:

```bash
cp test-config.conf my-custom-config.conf
# Edit my-custom-config.conf with your desired values
./test-autoscaling-helm.sh --config my-custom-config.conf all
```

## Testing

Run configuration functionality tests:

```bash
./test-configuration.sh
```

This validates:
- Configuration file loading
- Manifest generation
- Custom configuration support
- Script argument parsing
- Configuration override functionality

## Benefits

1. **Flexibility**: Easy customization without code changes
2. **Maintainability**: Centralized configuration management
3. **Testability**: Configuration changes can be tested independently
4. **Reusability**: Same scripts work across different environments
5. **TDD Compliance**: All existing tests pass, new tests added

## Migration from Hardcoded Values

The following hardcoded values have been parameterized:

- Cluster names and namespaces
- Image repositories and tags
- Timeout values and polling intervals
- Test bench names
- Autoscaling parameters
- Component names

All existing functionality remains unchanged - the system now loads these values from configuration files instead of having them hardcoded in scripts.