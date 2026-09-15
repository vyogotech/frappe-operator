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
DOMAIN=$(cat /tmp/secrets/domain 2>/dev/null || true)

echo "Dropping Frappe site: $SITE_NAME"
echo "Using MariaDB root credentials from secret volume for secure deletion"

# Use root credentials to drop the site (site user cannot drop database).
# On an NFS-style shared volume (Longhorn RWX, NFS) drop-site can drop the
# database and then fail removing the site directory: a bench process (the
# scheduler enumerates every site) may hold the site's log open, which the
# volume keeps as a `.nfs*` file until the handle closes. The site is gone at
# that point; a directory holding only such temp files must not fail the
# deletion. So: tolerate the failure, best-effort remove what is left, and
# succeed when site_config.json is gone (the site no longer exists to Frappe).
DROP_RC=0
bench drop-site "$SITE_NAME" --force --db-root-username "$DB_ROOT_USER" --db-root-password "$DB_ROOT_PASSWORD" --no-backup || DROP_RC=$?
if [ -d "sites/$SITE_NAME" ]; then
    rm -rf "sites/$SITE_NAME" 2>/dev/null || true
fi
if [ -f "sites/$SITE_NAME/site_config.json" ]; then
    echo "drop-site failed (exit $DROP_RC) and sites/$SITE_NAME still holds site_config.json"
    exit "${DROP_RC:-1}"
fi
if [ -d "sites/$SITE_NAME" ]; then
    echo "Note: sites/$SITE_NAME still exists (open-file temp entries on a shared volume); it clears when the holder closes the file:"
    find "sites/$SITE_NAME" -type f 2>/dev/null | head -5
fi

# drop-site removes the real site directory only. If site_init aliased the
# resolved domain onto it, that symlink is now dangling and would shadow any
# future site created under that host - remove it. Only ever remove a symlink,
# never a directory, so a misconfigured domain cannot delete another site.
if [ -n "$DOMAIN" ] && [ "$DOMAIN" != "$SITE_NAME" ] && [ -L "sites/$DOMAIN" ]; then
    rm -f "sites/$DOMAIN" && echo "Removed alias sites/$DOMAIN"
fi

echo "Site $SITE_NAME dropped successfully!"
