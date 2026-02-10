#!/bin/bash
set -e

# Generate test manifests from configuration
# Usage: ./generate-test-manifests.sh [--config FILE]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load configuration file
CONFIG_FILE="${SCRIPT_DIR}/test-config.conf"
if [ ! -f "$CONFIG_FILE" ]; then
    echo "ERROR: Configuration file not found: $CONFIG_FILE"
    exit 1
fi

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --config)
            CONFIG_FILE="$2"
            if [ ! -f "$CONFIG_FILE" ]; then
                echo "ERROR: Configuration file not found: $CONFIG_FILE"
                exit 1
            fi
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

source "$CONFIG_FILE"

echo "Generating test manifests..."

# Generate KEDA test manifest
cat > "${SCRIPT_DIR}/examples/test-keda-bench.yaml" << EOF
# Simple FrappeBench with KEDA autoscaling for E2E testing
# Generated from configuration: $CONFIG_FILE
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: ${KEDA_BENCH_NAME}
  namespace: ${DEFAULT_NAMESPACE}
spec:
  frappeVersion: "${DEFAULT_FRAPPE_VERSION}"
  
  # Use custom Frappe image
  imageConfig:
    repository: ${FRAPPE_IMAGE_REPOSITORY}
    tag: ${FRAPPE_IMAGE_TAG}
  
  apps:
    - name: erpnext
      source: image
  
  componentAutoscaling:
    ${NGINX_COMPONENT}:
      enabled: true
      provider: keda
      minReplicas: ${DEFAULT_MIN_REPLICAS}
      maxReplicas: ${DEFAULT_MAX_REPLICAS}
      pollingInterval: ${DEFAULT_KEDA_POLLING_INTERVAL}
      cooldownPeriod: ${DEFAULT_KEDA_COOLDOWN_PERIOD}
      keda:
        trigger: cpu
        targetValue: "${DEFAULT_KEDA_TARGET_VALUE}"
EOF

# Generate HPA test manifest
cat > "${SCRIPT_DIR}/examples/test-hpa-bench.yaml" << EOF
# Simple FrappeBench with HPA autoscaling for E2E testing
# Generated from configuration: $CONFIG_FILE
apiVersion: vyogo.tech/v1alpha1
kind: FrappeBench
metadata:
  name: ${HPA_BENCH_NAME}
  namespace: ${DEFAULT_NAMESPACE}
spec:
  frappeVersion: "${DEFAULT_FRAPPE_VERSION}"
  
  # Use custom Frappe image
  imageConfig:
    repository: ${FRAPPE_IMAGE_REPOSITORY}
    tag: ${FRAPPE_IMAGE_TAG}
  
  apps:
    - name: erpnext
      source: image
  
  componentAutoscaling:
    ${NGINX_COMPONENT}:
      enabled: true
      provider: hpa
      minReplicas: ${DEFAULT_MIN_REPLICAS}
      maxReplicas: ${DEFAULT_MAX_REPLICAS}
      hpa:
        metric: cpu
        targetUtilization: ${DEFAULT_HPA_TARGET_UTILIZATION}
EOF

echo "Generated manifests:"
echo "  - ${SCRIPT_DIR}/examples/test-keda-bench.yaml"
echo "  - ${SCRIPT_DIR}/examples/test-hpa-bench.yaml"
echo ""
echo "Configuration used:"
echo "  KEDA Bench: ${KEDA_BENCH_NAME}"
echo "  HPA Bench: ${HPA_BENCH_NAME}"
echo "  Namespace: ${DEFAULT_NAMESPACE}"
echo "  Frappe Version: ${DEFAULT_FRAPPE_VERSION}"
echo "  Image: ${FRAPPE_IMAGE_REPOSITORY}:${FRAPPE_IMAGE_TAG}"