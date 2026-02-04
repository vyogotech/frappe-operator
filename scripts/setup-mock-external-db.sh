#!/bin/bash
# Script to mock an external MariaDB setup using the internal frappe-mariadb instance

set -e

NAMESPACE="mariadb"
TARGET_NAMESPACE="frappe"
DB_NAME="mock_external_db"
DB_USER="mock_external_db"
DB_PASS="mock_password"
MARIADB_ROOT_PASS="frappe"

echo "1. Creating Database and User in frappe-mariadb..."

# Get the MariaDB pod name
MARIADB_POD=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=mariadb -o jsonpath='{.items[0].metadata.name}')

# Execute SQL commands
kubectl exec -it $MARIADB_POD -n $NAMESPACE -- mysql -u root -p$MARIADB_ROOT_PASS -e "
DROP USER IF EXISTS '$DB_USER'@'%';
CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER '$DB_USER'@'%' IDENTIFIED BY '$DB_PASS';
GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'%';
FLUSH PRIVILEGES;
"

echo "✓ Database and User created."

echo "2. Creating Kubernetes Secret for the external credentials..."

kubectl create secret generic external-mariadb-creds \
  -n $TARGET_NAMESPACE \
  --from-literal=username=$DB_USER \
  --from-literal=password=$DB_PASS \
  --from-literal=database=$DB_NAME \
  --dry-run=client -o yaml | kubectl apply -f -

echo "✓ Secret created in namespace $TARGET_NAMESPACE."

echo "3. Generating FrappeSite manifest..."

cat > external-db-mock-site.yaml <<EOF
apiVersion: vyogo.tech/v1alpha1
kind: FrappeSite
metadata:
  name: external-mock-site
  namespace: $TARGET_NAMESPACE
spec:
  benchRef:
    name: e2e-test-bench
  siteName: external-mock.local
  dbConfig:
    provider: external
    host: frappe-mariadb.$NAMESPACE.svc.cluster.local
    port: "3306"
    connectionSecretRef:
      name: external-mariadb-creds
EOF

echo "✓ Manifest generated: external-db-mock-site.yaml"
echo ""
echo "Next step: kubectl apply -f external-db-mock-site.yaml"
