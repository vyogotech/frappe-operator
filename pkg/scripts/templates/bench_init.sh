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
  "redis_cache": "redis://{{.RedisCacheAddress}}",
  "redis_queue": "redis://{{.RedisQueueAddress}}",
  "socketio_port": 9000
}
EOF

# Sync assets from the image cache to the Persistent Volume
if [ -d "/home/frappe/assets_cache" ]; then
    echo "Syncing pre-built assets from image to PVC (preserving dynamic app hashes)..."
    mkdir -p sites/assets
    # Copy asset subdirectories with -rn (no-clobber) so new assets copy without overwriting existing files
    cp -rn /home/frappe/assets_cache/* sites/assets/ 2>/dev/null || true

    # Intelligently MERGE assets.json to update core hashes while preserving dynamic app hashes
    python3 -c "
import json, os

cache_json = '/home/frappe/assets_cache/assets.json'
site_json = 'sites/assets/assets.json'

if os.path.exists(cache_json):
    try:
        with open(cache_json, 'r') as f:
            new_assets = json.load(f)

        current_assets = {}
        if os.path.exists(site_json):
            try:
                with open(site_json, 'r') as f:
                    current_assets = json.load(f)
            except Exception:
                pass

        current_assets.update(new_assets)

        with open(site_json, 'w') as f:
            json.dump(current_assets, f, indent=2)
        print('✓ Intelligently merged assets.json without losing dynamic app hashes')
    except Exception as e:
        print(f'Warning: assets.json merge failed: {e}')
" || echo "Warning: Failed to merge assets.json"
fi

echo "Bench configuration complete"
