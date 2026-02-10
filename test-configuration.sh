#!/bin/bash
set -e

# Test configuration functionality
# Usage: ./test-configuration.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=========================================="
echo "Configuration Functionality Tests"
echo "=========================================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
}

print_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

# Test 1: Configuration file loading
print_test "Test 1: Configuration file loading"
CONFIG_FILE="${SCRIPT_DIR}/test-config.conf"
if [ -f "$CONFIG_FILE" ]; then
    source "$CONFIG_FILE"
    print_pass "Configuration file loaded successfully"
    
    # Test key variables are set
    if [ -n "$KEDA_BENCH_NAME" ] && [ -n "$HPA_BENCH_NAME" ] && [ -n "$OPERATOR_IMAGE_REPOSITORY" ]; then
        print_pass "Key configuration variables are set"
        print_info "  KEDA_BENCH_NAME: $KEDA_BENCH_NAME"
        print_info "  HPA_BENCH_NAME: $HPA_BENCH_NAME"
        print_info "  OPERATOR_IMAGE_REPOSITORY: $OPERATOR_IMAGE_REPOSITORY"
    else
        print_fail "Some configuration variables are missing"
        exit 1
    fi
else
    print_fail "Configuration file not found: $CONFIG_FILE"
    exit 1
fi

# Test 2: Manifest generation
print_test "Test 2: Manifest generation"
"${SCRIPT_DIR}/generate-test-manifests.sh" --config "$CONFIG_FILE"

if [ -f "${SCRIPT_DIR}/examples/test-keda-bench.yaml" ] && [ -f "${SCRIPT_DIR}/examples/test-hpa-bench.yaml" ]; then
    print_pass "Test manifests generated successfully"
    
    # Check KEDA manifest contains expected values
    if grep -q "name: $KEDA_BENCH_NAME" "${SCRIPT_DIR}/examples/test-keda-bench.yaml" && \
       grep -q "repository: $FRAPPE_IMAGE_REPOSITORY" "${SCRIPT_DIR}/examples/test-keda-bench.yaml"; then
        print_pass "KEDA manifest contains expected configuration values"
    else
        print_fail "KEDA manifest missing expected configuration values"
        exit 1
    fi
    
    # Check HPA manifest contains expected values
    if grep -q "name: $HPA_BENCH_NAME" "${SCRIPT_DIR}/examples/test-hpa-bench.yaml" && \
       grep -q "targetUtilization: $DEFAULT_HPA_TARGET_UTILIZATION" "${SCRIPT_DIR}/examples/test-hpa-bench.yaml"; then
        print_pass "HPA manifest contains expected configuration values"
    else
        print_fail "HPA manifest missing expected configuration values"
        exit 1
    fi
else
    print_fail "Test manifests not generated"
    exit 1
fi

# Test 3: Custom configuration file
print_test "Test 3: Custom configuration file"
CUSTOM_CONFIG="${SCRIPT_DIR}/test-custom-config.conf"

# Create custom configuration
cat > "$CUSTOM_CONFIG" << EOF
# Custom test configuration
DEFAULT_CLUSTER_NAME="custom-cluster"
DEFAULT_NAMESPACE="custom-namespace"
KEDA_BENCH_NAME="custom-keda-bench"
HPA_BENCH_NAME="custom-hpa-bench"
OPERATOR_IMAGE_REPOSITORY="custom/operator"
OPERATOR_IMAGE_TAG="custom-tag"
DEFAULT_MIN_REPLICAS=2
DEFAULT_MAX_REPLICAS=5
DEFAULT_HPA_TARGET_UTILIZATION=75
DEFAULT_FRAPPE_VERSION="16"
EOF

# Generate manifests with custom config
"${SCRIPT_DIR}/generate-test-manifests.sh" --config "$CUSTOM_CONFIG"

# Verify custom values are used
if grep -q "name: custom-keda-bench" "${SCRIPT_DIR}/examples/test-keda-bench.yaml" && \
   grep -q "namespace: custom-namespace" "${SCRIPT_DIR}/examples/test-keda-bench.yaml" && \
   grep -q "minReplicas: 2" "${SCRIPT_DIR}/examples/test-keda-bench.yaml"; then
    print_pass "Custom configuration applied successfully"
else
    print_fail "Custom configuration not applied correctly"
    exit 1
fi

# Cleanup
rm -f "$CUSTOM_CONFIG"

# Test 4: Script argument parsing
print_test "Test 4: Script argument parsing"
TEST_OUTPUT=$(timeout 5s ./test-autoscaling-helm.sh --config "$CONFIG_FILE" --cluster-name test-cluster --namespace test-ns --skip-cluster-create 2>&1 | head -10 || true)

if echo "$TEST_OUTPUT" | grep -q "Cluster: test-cluster" && \
   echo "$TEST_OUTPUT" | grep -q "Namespace: test-ns" && \
   echo "$TEST_OUTPUT" | grep -q "Config: $CONFIG_FILE"; then
    print_pass "Script argument parsing works correctly"
else
    print_fail "Script argument parsing failed"
    echo "Output: $TEST_OUTPUT"
    exit 1
fi

# Test 5: Missing configuration file
print_test "Test 5: Missing configuration file handling"
if "${SCRIPT_DIR}/generate-test-manifests.sh" --config "/nonexistent/config.conf" 2>/dev/null; then
    print_fail "Should have failed with missing configuration file"
    exit 1
else
    print_pass "Missing configuration file handling works correctly"
fi

# Test 6: Configuration override functionality
print_test "Test 6: Configuration override functionality"
OVERRIDE_OUTPUT=$(timeout 3s ./test-autoscaling-helm.sh keda --config "$CONFIG_FILE" --cluster-name override-cluster --namespace override-ns --skip-cluster-create 2>&1 | head -10 || true)

if echo "$OVERRIDE_OUTPUT" | grep -q "Cluster: override-cluster" && \
   echo "$OVERRIDE_OUTPUT" | grep -q "Namespace: override-ns" && \
   echo "$OVERRIDE_OUTPUT" | grep -q "Test Type: keda"; then
    print_pass "Configuration override functionality works correctly"
else
    print_fail "Configuration override functionality failed"
    echo "Output: $OVERRIDE_OUTPUT"
    exit 1
fi

echo ""
print_info "All configuration functionality tests passed! ✅"
print_info "The system now supports:"
print_info "  - Flexible configuration through config files"
print_info "  - Custom test parameters"
print_info "  - Manifest generation from configuration"
print_info "  - Script argument parsing"
print_info "  - Configuration validation"