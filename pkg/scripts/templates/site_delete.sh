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

# Use root credentials to drop the site (site user cannot drop database)
bench drop-site "$SITE_NAME" --force --db-root-username "$DB_ROOT_USER" --db-root-password "$DB_ROOT_PASSWORD" --no-backup

# If site_init aliased the resolved domain onto this site, that symlink is now
# dangling and would shadow any future site created under that host - remove it.
# Only ever remove a symlink, never a directory, so a misconfigured domain
# cannot delete another site.
if [ -n "$DOMAIN" ] && [ "$DOMAIN" != "$SITE_NAME" ] && [ -L "sites/$DOMAIN" ]; then
    rm -f "sites/$DOMAIN" && echo "Removed alias sites/$DOMAIN"
fi

# drop-site is expected to move the site directory to archived_sites/, but on a
# shared ReadWriteMany volume it has been observed to drop the database and
# leave sites/<name>/ behind. Those orphans are not free: the bench scheduler
# enumerates sites/ and holds roughly 5MiB per entry, so a bench that had
# provisioned and deleted 100+ sites was holding 866MiB while serving one site,
# against a 128MiB request. Left alone, a long-lived multi-tenant bench degrades
# for its whole life.
#
# Guarded so this can only ever remove the site it was asked to drop: the name
# must contain no path separator, the path must resolve inside sites/, and it
# must be a real directory rather than a symlink.
case "$SITE_NAME" in
    */*|.|..|"")
        echo "Refusing to clean up a site name containing a path separator: '$SITE_NAME'"
        ;;
    *)
        SITE_DIR="sites/$SITE_NAME"
        if [ -d "$SITE_DIR" ] && [ ! -L "$SITE_DIR" ]; then
            echo "drop-site left $SITE_DIR behind; removing it so the scheduler stops tracking it"
            rm -rf "$SITE_DIR" && echo "Removed $SITE_DIR"
        fi
        # drop-site may instead have archived it; that copy is equally stale.
        if [ -d "sites/archived_sites/$SITE_NAME" ]; then
            rm -rf "sites/archived_sites/$SITE_NAME" && echo "Removed archived copy of $SITE_NAME"
        fi
        ;;
esac

echo "Site $SITE_NAME dropped successfully!"
