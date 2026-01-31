#!/bin/bash
# Bench initialization script for Frappe (embedded in operator, executed in bench init jobs)

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

echo "Checking directory permissions..."
id

echo "Configuring Frappe bench..."

# The PVC is mounted directly at /home/frappe/frappe-bench/sites
# Frappe expects this directory structure for proper operation
mkdir -p sites

# Test write access to the mounted volume
if ! touch sites/.permission_test 2>/dev/null; then
    echo "ERROR: sites directory is NOT writable by $(whoami) (UID $(id -u), GID $(id -g))."
    ls -ld sites
    exit 1
fi
rm sites/.permission_test

# Create apps.txt from existing apps
if [ -d "apps" ]; then
    echo "Creating apps.txt..."
    # Write to sites/apps.txt since that is the shared volume
    ls -1 apps > sites/apps.txt || { echo "ERROR: Failed to write to sites/apps.txt"; exit 1; }
fi

# Create or update common_site_config.json
echo "Creating common_site_config.json..."
cat > sites/common_site_config.json <<EOF
{
  "redis_cache": "redis://{{.BenchName}}-redis-cache:6379",
  "redis_queue": "redis://{{.BenchName}}-redis-queue:6379",
  "socketio_port": 9000
}
EOF

# Sync assets from the image cache to the Persistent Volume
if [ -d "/home/frappe/assets_cache" ]; then
    echo "Syncing pre-built assets from image to PVC..."
    mkdir -p sites/assets
    # Use -n to not overwrite existing files, preserving permissions where possible
    cp -rn /home/frappe/assets_cache/* sites/assets/ || true
fi

echo "Bench configuration complete"
