#!/bin/bash
# Site backup script for Frappe
# This script is embedded in the operator and executed in backup jobs

set -e

# Setup user for OpenShift compatibility
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench

# Link apps.txt for bench
if [ -f sites/apps.txt ]; then
    ln -sf sites/apps.txt apps.txt || echo "Warning: Failed to create apps.txt link"
fi

SITE_NAME="${SITE_NAME:-$(cat /tmp/backup-config/site_name 2>/dev/null || echo '')}"
INCLUDE_FILES="${INCLUDE_FILES:-true}"

if [[ -z "$SITE_NAME" ]]; then
    echo "ERROR: SITE_NAME not set"
    exit 1
fi

echo "Starting backup for site: $SITE_NAME"
echo "Include files: $INCLUDE_FILES"

# Run backup
if [[ "$INCLUDE_FILES" == "true" ]]; then
    bench --site "$SITE_NAME" backup --with-files
else
    bench --site "$SITE_NAME" backup
fi

# List backup files
BACKUP_DIR="/home/frappe/frappe-bench/sites/$SITE_NAME/private/backups"
echo "Backup completed. Files in $BACKUP_DIR:"
ls -la "$BACKUP_DIR" | tail -10

echo "Backup completed successfully!"
