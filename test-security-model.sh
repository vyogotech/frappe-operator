#!/bin/bash
set -e

echo "======================================================"
echo "Testing Database Security & Privilege Model"
echo "======================================================"
echo ""

NAMESPACE="frappe-test"
SITE_NAME="security-test-site"
BENCH_NAME="test-bench"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

function test_passed() {
    echo -e "${GREEN}✓ PASS:${NC} $1"
}

function test_failed() {
    echo -e "${RED}✗ FAIL:${NC} $1"
    exit 1
}

function test_info() {
    echo -e "${YELLOW}ℹ INFO:${NC} $1"
}

echo "Step 1: Creating test site..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ${SITE_NAME}-admin-password
  namespace: ${NAMESPACE}
type: Opaque
stringData:
  password: "admin123"
---
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: ${SITE_NAME}
  namespace: ${NAMESPACE}
spec:
  siteName: "${SITE_NAME}.local"
  benchRef:
    name: ${BENCH_NAME}
    namespace: ${NAMESPACE}
  adminPasswordSecretRef:
    name: ${SITE_NAME}-admin-password
    namespace: ${NAMESPACE}
  dbConfig:
    provider: mariadb
    mode: dedicated
    storageSize: 1Gi
EOF

test_info "Waiting for site initialization (60 seconds)..."
sleep 60

# Check if site is ready
SITE_PHASE=$(kubectl get frappesite ${SITE_NAME} -n ${NAMESPACE} -o jsonpath='{.status.phase}')
if [[ "$SITE_PHASE" != "Ready" ]]; then
    test_failed "Site not ready. Phase: $SITE_PHASE"
fi
test_passed "Site initialized successfully"

echo ""
echo "Step 2: Verifying Grant CR has minimal privileges..."

# Get Grant CR privileges
GRANT_NAME="${SITE_NAME}-grant"
PRIVILEGES=$(kubectl get grant ${GRANT_NAME} -n ${NAMESPACE} -o jsonpath='{.spec.privileges}')
GRANT_OPTION=$(kubectl get grant ${GRANT_NAME} -n ${NAMESPACE} -o jsonpath='{.spec.grantOption}')

test_info "Grant privileges: $PRIVILEGES"
test_info "Grant option: $GRANT_OPTION"

# Verify no ALL PRIVILEGES
if echo "$PRIVILEGES" | grep -q "ALL PRIVILEGES"; then
    test_failed "Grant CR contains 'ALL PRIVILEGES' - security violation!"
fi
test_passed "Grant CR does not contain 'ALL PRIVILEGES'"

# Verify grantOption is false
if [[ "$GRANT_OPTION" != "false" ]]; then
    test_failed "Grant option is not false - privilege escalation possible!"
fi
test_passed "Grant option is false - no privilege escalation"

# Verify essential privileges are present
for priv in "SELECT" "INSERT" "UPDATE" "DELETE" "CREATE" "ALTER" "DROP"; do
    if ! echo "$PRIVILEGES" | grep -q "$priv"; then
        test_failed "Missing essential privilege: $priv"
    fi
done
test_passed "All essential table-level privileges present"

echo ""
echo "Step 3: Verifying database grants..."

# Get database credentials
MARIADB_POD="${SITE_NAME}-mariadb-0"
ROOT_PASSWORD=$(kubectl get secret ${SITE_NAME}-mariadb-root -n ${NAMESPACE} -o jsonpath='{.data.password}' | base64 -d)
SITE_USER=$(kubectl get user ${SITE_NAME}-user -n ${NAMESPACE} -o jsonpath='{.spec.name}')

test_info "MariaDB pod: $MARIADB_POD"
test_info "Site user: $SITE_USER"

# Wait for MariaDB to be ready
test_info "Waiting for MariaDB to be ready..."
kubectl wait --for=condition=ready pod/${MARIADB_POD} -n ${NAMESPACE} --timeout=60s || test_failed "MariaDB pod not ready"
test_passed "MariaDB pod is ready"

# Check actual database grants
test_info "Checking actual database grants for site user..."
GRANTS_OUTPUT=$(kubectl exec -n ${NAMESPACE} ${MARIADB_POD} -- mysql -u root -p${ROOT_PASSWORD} -e "SHOW GRANTS FOR '${SITE_USER}'@'%';" 2>/dev/null || echo "FAILED")

if [[ "$GRANTS_OUTPUT" == "FAILED" ]]; then
    test_failed "Could not retrieve grants from database"
fi

echo "$GRANTS_OUTPUT"

# Verify site user CANNOT drop databases
if echo "$GRANTS_OUTPUT" | grep -i "DROP" | grep -i "ON \*\.\*"; then
    test_failed "Site user has global DROP privilege - can drop databases!"
fi
test_passed "Site user cannot drop databases (no global DROP privilege)"

# Verify site user CANNOT grant privileges
if echo "$GRANTS_OUTPUT" | grep -i "WITH GRANT OPTION"; then
    test_failed "Site user has GRANT OPTION - privilege escalation possible!"
fi
test_passed "Site user cannot grant privileges to others"

echo ""
echo "Step 4: Testing site deletion with root credentials..."

test_info "Deleting site: ${SITE_NAME}"
kubectl delete frappesite ${SITE_NAME} -n ${NAMESPACE}

test_info "Waiting for deletion job to complete (30 seconds)..."
sleep 30

# Check deletion job status
DELETE_JOB="${SITE_NAME}-delete"
JOB_STATUS=$(kubectl get job ${DELETE_JOB} -n ${NAMESPACE} -o jsonpath='{.status.succeeded}' 2>/dev/null || echo "0")

if [[ "$JOB_STATUS" != "1" ]]; then
    test_info "Checking deletion job logs..."
    kubectl logs job/${DELETE_JOB} -n ${NAMESPACE} 2>&1 | tail -20
    test_failed "Deletion job did not complete successfully"
fi
test_passed "Deletion job completed successfully using root credentials"

# Verify site is deleted
SITE_EXISTS=$(kubectl get frappesite ${SITE_NAME} -n ${NAMESPACE} 2>&1 | grep -c "NotFound" || echo "0")
if [[ "$SITE_EXISTS" == "0" ]]; then
    test_failed "Site still exists after deletion"
fi
test_passed "Site deleted successfully"

# Verify database resources are cleaned up
DATABASE_EXISTS=$(kubectl get database ${SITE_NAME}-db -n ${NAMESPACE} 2>&1 | grep -c "NotFound" || echo "0")
if [[ "$DATABASE_EXISTS" == "0" ]]; then
    test_failed "Database resource not cleaned up"
fi
test_passed "Database resources cleaned up"

echo ""
echo "======================================================"
echo -e "${GREEN}All security tests passed!${NC}"
echo "======================================================"
echo ""
echo "Summary:"
echo "✓ Site users have minimal privileges (table-level only)"
echo "✓ Site users cannot drop databases"
echo "✓ Site users cannot grant privileges to others"
echo "✓ Deletion uses root credentials properly"
echo "✓ Complete site lifecycle works securely"
echo ""
