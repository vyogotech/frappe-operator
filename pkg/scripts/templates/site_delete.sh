#!/bin/bash
# Site deletion script for Frappe
# This script is embedded in the operator and executed in deletion jobs

set -e

# Setup user for OpenShift compatibility (fixes getpwuid() error)
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  # Try to add user to /etc/passwd if writable
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench

# Link apps.txt to site path for bench to find it
# The apps.txt is in the sites directory, but bench expects it in the root
if [ -f sites/apps.txt ]; then
    ln -sf sites/apps.txt apps.txt || cp sites/apps.txt apps.txt || echo "Warning: Failed to create apps.txt in root"
else
    echo "Warning: sites/apps.txt not found!"
fi

# Read credentials from mounted secret files
DB_ROOT_USER=$(cat /tmp/secrets/db_root_user)
DB_ROOT_PASSWORD=$(cat /tmp/secrets/db_root_password)
SITE_NAME=$(cat /tmp/secrets/site_name)

echo "Dropping Frappe site: $SITE_NAME"
echo "Using MariaDB root credentials from secret volume for secure deletion"

# Use root credentials to drop the site (site user cannot drop database)
bench drop-site "$SITE_NAME" --force --db-root-username "$DB_ROOT_USER" --db-root-password "$DB_ROOT_PASSWORD" --no-backup

echo "Site $SITE_NAME dropped successfully!"
