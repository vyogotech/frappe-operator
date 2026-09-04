#!/bin/bash
# Site initialization script for Frappe (embedded in operator, executed in init jobs)

set -e
umask 0002

# Post-hook to ensure site remains functional (CSS loads) even if upgrade/migration fails
cleanup() {
    # Only clear cache if the site directory exists (prevents failing during initial new-site creation)
    if [ -n "$SITE_NAME" ] && [ -d "/home/frappe/frappe-bench/sites/$SITE_NAME" ] && [ -f "/home/frappe/frappe-bench/sites/$SITE_NAME/site_config.json" ]; then
        echo "Running post-execution hook: clearing cache to ensure static assets load from new image..."
        
        # 1. Try standard bench clear-cache first
        if ! bench --site "$SITE_NAME" clear-cache; then
            echo "⚠ Warning: 'bench clear-cache' failed (likely due to broken app code)."
            echo "Attempting to flush Redis cache directly as a fallback..."
            
            # 2. Fallback to direct Redis flush if bench context is broken
            if [ -n "$REDIS_CACHE_ADDRESS" ]; then
                # REDIS_CACHE_ADDRESS is typically host:port
                REDIS_HOST=$(echo "$REDIS_CACHE_ADDRESS" | cut -d':' -f1)
                REDIS_PORT=$(echo "$REDIS_CACHE_ADDRESS" | cut -d':' -f2)
                
                # Use frappe's venv which guarantees the redis package is installed
                env/bin/python -c "
import redis, sys
try:
    r = redis.Redis(host='$REDIS_HOST', port=int('$REDIS_PORT'))
    keys_deleted = 0
    for key in r.scan_iter('*'):
        # Preserve user sessions so we don't log everyone out during a failed upgrade
        if b'session' not in key:
            r.delete(key)
            keys_deleted += 1
    print(f'✓ Safely flushed {keys_deleted} non-session keys from Redis cache directly.')
except Exception as e:
    print(f'Failed to flush redis safely: {e}')
    sys.exit(1)
" || echo "✗ Warning: Direct Redis flush also failed."
            else
                echo "✗ No REDIS_CACHE_ADDRESS found, skipping direct flush."
            fi
        else
            echo "✓ Cache cleared successfully."
        fi
    fi
}
trap cleanup EXIT

echo "DEBUG: Running site_init.sh version $(date)"

# Setup user for OpenShift compatibility (fixes getpwuid() error)
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  # Try to add user to /etc/passwd if writable (rarely the case on OpenShift, but good practice)
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench

# Read from secret files mounted at /tmp/site-secrets
SITE_NAME=$(cat /tmp/site-secrets/site_name)
DOMAIN=$(cat /tmp/site-secrets/domain)
ADMIN_PASSWORD=$(cat /tmp/site-secrets/admin_password)
BENCH_NAME=$(cat /tmp/site-secrets/bench_name)
DB_PROVIDER=$(cat /tmp/site-secrets/db_provider)
APPS_TO_INSTALL=$(cat /tmp/site-secrets/apps_to_install 2>/dev/null || echo "")
REDIS_CACHE_ADDRESS=$(cat /tmp/site-secrets/redis_cache_address 2>/dev/null || echo "")
REDIS_QUEUE_ADDRESS=$(cat /tmp/site-secrets/redis_queue_address 2>/dev/null || echo "")
SKIP_INIT=$(cat /tmp/site-secrets/skip_init 2>/dev/null || echo "false")
ENCRYPTION_KEY=$(cat /tmp/site-secrets/encryption_key 2>/dev/null || echo "")

# Sync assets from the image cache to the Persistent Volume BEFORE migration/initialization
# This guarantees that even if database migration fails, assets are present for the web service
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

# Create or update common_site_config.json BEFORE migration/initialization
echo "Creating common_site_config.json..."
cat > sites/common_site_config.json <<EOF
{
  "redis_cache": "redis://${REDIS_CACHE_ADDRESS}",
  "redis_queue": "redis://${REDIS_QUEUE_ADDRESS}",
  "socketio_port": 9000
}
EOF

# Centralized function to intelligently merge site_config.json without destroying existing keys
update_site_config_json() {
    echo "Intelligently updating site_config.json..."
    python3 << 'PYTHON_SCRIPT'
import json, os

# Read from secret files mounted at /tmp/site-secrets
with open('/tmp/site-secrets/site_name', 'r') as f:
    site_name = f.read().strip()
with open('/tmp/site-secrets/domain', 'r') as f:
    domain = f.read().strip()
try:
    with open('/tmp/site-secrets/redis_cache_address', 'r') as f:
        redis_cache_address = f.read().strip()
except FileNotFoundError:
    redis_cache_address = ""
try:
    with open('/tmp/site-secrets/redis_queue_address', 'r') as f:
        redis_queue_address = f.read().strip()
except FileNotFoundError:
    redis_queue_address = ""
with open('/tmp/site-secrets/db_host', 'r') as f:
    db_host = f.read().strip()
with open('/tmp/site-secrets/db_port', 'r') as f:
    db_port = f.read().strip()
with open('/tmp/site-secrets/db_name', 'r') as f:
    db_name = f.read().strip()
with open('/tmp/site-secrets/db_user', 'r') as f:
    db_user = f.read().strip()
with open('/tmp/site-secrets/db_password', 'r') as f:
    db_password = f.read().strip()
with open('/tmp/site-secrets/db_provider', 'r') as f:
    db_provider = f.read().strip()

try:
    with open('/tmp/site-secrets/encryption_key', 'r') as f:
        encryption_key = f.read().strip()
except FileNotFoundError:
    encryption_key = ""

site_path = f"/home/frappe/frappe-bench/sites/{site_name}"
config_file = os.path.join(site_path, "site_config.json")

# Ensure directories exist
os.makedirs(site_path, exist_ok=True)
os.makedirs(os.path.join(site_path, "logs"), exist_ok=True)

# Read or initialize config
try:
    with open(config_file, 'r') as f:
        config = json.load(f)
except FileNotFoundError:
    config = {}

# Update with resolved domain and redis configuration
if domain:
    config['host_name'] = domain
if redis_cache_address:
    config['redis_cache'] = f"redis://{redis_cache_address}"
if redis_queue_address:
    config['redis_queue'] = f"redis://{redis_queue_address}"

# Explicitly add database credentials for self-healing
config['db_name'] = db_name
config['db_user'] = db_user
config['db_password'] = db_password
config['db_host'] = db_host
config['db_port'] = int(db_port) if db_port.isdigit() else db_port
config['db_type'] = db_provider

# If an external encryption_key was securely provided, inject it
if encryption_key:
    config['encryption_key'] = encryption_key

# Write back reliably
with open(config_file, 'w') as f:
    json.dump(config, f, indent=2)

print(f"✓ Updated site_config.json successfully for {site_name}")
PYTHON_SCRIPT
}

# --- Initialization Flow --- 

IS_UPGRADE=$(cat /tmp/site-secrets/is_upgrade 2>/dev/null || echo "false")
SKIP_INIT=$(cat /tmp/site-secrets/skip_init 2>/dev/null || echo "false")

echo "Creating Frappe site: $SITE_NAME"
echo "Domain: $DOMAIN"

# Check directly against the database whether Frappe is already installed.
# Emits exactly one of:
#   "true"    - connected, the frappe app is installed (site exists)
#   "false"   - connected, and the install table is genuinely absent (empty DB)
#   "unknown" - could NOT determine (connection/query error, or non-SQL provider)
# The "unknown" state is critical: a transient DB error (e.g. "Packet sequence number
# wrong") must never be conflated with "empty", or we would destroy and recreate a
# live, working site during a failed/racy upgrade.
DB_HAS_FRAPPE=$(python3 << 'PYTHON_CHECK'
import sys

try:
    with open('/tmp/site-secrets/db_host', 'r') as f: db_host = f.read().strip()
    with open('/tmp/site-secrets/db_port', 'r') as f: db_port = f.read().strip()
    with open('/tmp/site-secrets/db_name', 'r') as f: db_name = f.read().strip()
    with open('/tmp/site-secrets/db_user', 'r') as f: db_user = f.read().strip()
    with open('/tmp/site-secrets/db_password', 'r') as f: db_password = f.read().strip()
    with open('/tmp/site-secrets/db_provider', 'r') as f: db_provider = f.read().strip()

    if db_provider not in ('mariadb', 'external'):
        print("unknown"); sys.exit(0)

    import MySQLdb
    db = MySQLdb.connect(host=db_host, port=int(db_port), user=db_user, passwd=db_password, db=db_name)
    cur = db.cursor()
    cur.execute("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = %s AND table_name = 'tabInstalled Application'", (db_name,))
    if cur.fetchone()[0] == 0:
        print("false"); sys.exit(0)          # connected, table absent -> genuinely empty
    cur.execute("SELECT COUNT(*) FROM `tabInstalled Application` WHERE app_name = 'frappe'")
    print("true" if cur.fetchone()[0] > 0 else "false")
except Exception as e:
    sys.stderr.write("DB init-check could not determine site state: %s\n" % e)
    print("unknown")                          # do NOT assume empty on error
PYTHON_CHECK
)

# Durable local evidence that this site was already initialized. These live on the
# sites PVC, so they survive bench image changes (unlike the racy is_upgrade flag,
# which is derived from site.Status.Phase==Ready and can flip to false mid-rollout).
SITE_DIR="/home/frappe/frappe-bench/sites/$SITE_NAME"
SITE_ALREADY_INITIALIZED="false"
if [[ -f "$SITE_DIR/.init_complete" ]] || [[ -f "$SITE_DIR/site_config.json" ]]; then
    SITE_ALREADY_INITIALIZED="true"
fi

# Decision — skip the destructive new-site whenever ANYTHING indicates the site already
# exists: DB confirms frappe, an explicit upgrade/skip flag, durable local evidence, OR
# the DB state is merely unknown while a site directory is present. We create fresh ONLY
# when we are confident the site is new: the DB is definitively empty AND there is no
# local evidence of a prior init. This prevents a transient DB error or a racy is_upgrade
# from wiping a working site during a failed upgrade (the fallback-trap guarantee).
if [[ "$DB_HAS_FRAPPE" == "true" ]] || [[ "$IS_UPGRADE" == "true" ]] || [[ "$SKIP_INIT" == "true" ]] \
   || [[ "$SITE_ALREADY_INITIALIZED" == "true" ]] \
   || { [[ "$DB_HAS_FRAPPE" == "unknown" ]] && [[ -d "$SITE_DIR" ]]; }; then
    echo "✓ Site $SITE_NAME already initialized (db=$DB_HAS_FRAPPE upgrade=$IS_UPGRADE skip=$SKIP_INIT local=$SITE_ALREADY_INITIALIZED); skipping new-site and updating config."
    goto_update_config=1
    mkdir -p "$SITE_DIR"
    touch "$SITE_DIR/.init_complete"
else
    if [[ -d "$SITE_DIR" ]]; then
        # Reached only when the DB is CONFIRMED empty AND there is no durable local
        # evidence of a prior init — i.e. a genuinely stale/half-created directory.
        echo "Warning: Site directory exists but DB is confirmed uninitialized and no init marker present; cleaning up for a fresh new-site..."
        rm -rf "$SITE_DIR"
    fi
    goto_update_config=0
fi

# Force update apps.txt to ensure it reflects current image state
# The apps directory in the container is the source of truth
if [ -d "apps" ]; then
    echo "Updating apps.txt from container image..."
    ls -1 apps > sites/apps.txt || echo "Warning: Failed to update apps.txt"
    # Create symlink if needed (bench expects apps.txt in root)
    ln -sf sites/apps.txt apps.txt || cp sites/apps.txt apps.txt || echo "Warning: Failed to create apps.txt in root"
else
    echo "Warning: apps directory not found in container!"
fi
ls -l apps.txt || true

# Dynamically build the --install-app argument with validation and logging
INSTALL_APP_ARG=""
APPS_TO_INSTALL_COUNT=0
if [[ -n "$APPS_TO_INSTALL" ]]; then
	echo "=========================================="
	echo "App Installation Configuration"
	echo "=========================================="
	echo "Apps requested for installation: $APPS_TO_INSTALL"
	
	# Validate apps directory exists and list available apps
	if [[ -d "apps" ]]; then
		# Get list of available apps from apps directory
		AVAILABLE_APPS=$(ls -1 apps/ 2>/dev/null | grep -v "^frappe$" || true)
		echo "Available apps in bench (from filesystem):"
		if [[ -n "$AVAILABLE_APPS" ]]; then
			echo "$AVAILABLE_APPS"
		fi
		echo "frappe (framework - always available)"
		
		# Also check apps.txt if it exists
		if [[ -f "sites/apps.txt" ]]; then
			echo ""
			echo "Apps listed in apps.txt:"
			cat sites/apps.txt || true
		fi
	else
		echo "WARNING: apps directory not found in bench - this is unexpected"
	fi
	echo "------------------------------------------"
	
	# Build install arguments and validate each app
	# New approach: Skip apps that aren't available instead of failing
	SKIPPED_APPS=""
	for app in $APPS_TO_INSTALL; do
		# Check if app directory exists
		if [[ -d "apps/$app" ]]; then
			INSTALL_APP_ARG+=" --install-app=$app"
			APPS_TO_INSTALL_COUNT=$((APPS_TO_INSTALL_COUNT + 1))
			echo "✓ App '$app' found in bench and will be installed"
		else
			# Gracefully skip missing apps
			echo "⚠ WARNING: App '$app' not found in bench directory - skipping"
			echo "  The app may not be installed in this bench yet"
			if [[ -n "$SKIPPED_APPS" ]]; then
				SKIPPED_APPS="$SKIPPED_APPS, $app"
			else
				SKIPPED_APPS="$app"
			fi
		fi
	done
	
	if [[ -n "$SKIPPED_APPS" ]]; then
		echo "------------------------------------------"
		echo "⚠ Skipped apps (not available): $SKIPPED_APPS"
		echo "  These apps will not be installed on this site"
		echo "  To install them later, ensure they're available in the bench"
		echo "  and use: bench --site $SITE_NAME install-app <app_name>"
	fi
	
	echo "=========================================="
	echo "Total apps to install: $APPS_TO_INSTALL_COUNT"
	if [[ $APPS_TO_INSTALL_COUNT -gt 0 ]]; then
		echo "Install arguments: $INSTALL_APP_ARG"
	else
		echo "No apps will be installed (none available or none specified)"
	fi
	echo "=========================================="
else
	echo "=========================================="
	echo "No apps specified for installation"
	echo "Only frappe framework will be installed"
	echo "=========================================="
fi

# run bench new-site with provider-specific database configuration
if [[ "$DB_PROVIDER" == "mariadb" ]] || [[ "$DB_PROVIDER" == "postgres" ]] || [[ "$DB_PROVIDER" == "external" ]]; then
    # Map 'external' to 'mariadb' (engine) for script logic if it leaked through
    if [[ "$DB_PROVIDER" == "external" ]]; then
        DB_PROVIDER="mariadb"
    fi

	# For MariaDB and PostgreSQL: use pre-provisioned database with dedicated credentials
	# These are mounted from secret volumes, not environment variables
	DB_HOST=$(cat /tmp/site-secrets/db_host)
	DB_PORT=$(cat /tmp/site-secrets/db_port)
	DB_NAME=$(cat /tmp/site-secrets/db_name)
	DB_USER=$(cat /tmp/site-secrets/db_user)
	DB_PASSWORD=$(cat /tmp/site-secrets/db_password)
    
	if [[ -z "$DB_HOST" || -z "$DB_PORT" || -z "$DB_NAME" || -z "$DB_USER" || -z "$DB_PASSWORD" ]]; then
		echo "ERROR: Database connection secret files not found for $DB_PROVIDER"
		exit 1
	fi

    # Strategy: If DB_USER != DB_NAME and bench doesn't support --db-user,
    # or if we explicitly want to skip standard init (e.g. for external managed DBs),
    # we use the manual config + migrate path which is much more reliable.
    
    USE_MANUAL_CONFIG=0
    if [[ "$SKIP_INIT" == "true" ]]; then
        USE_MANUAL_CONFIG=1
    fi

    # Check bench capabilities
    SUPPORTS_DB_USER=0
    if bench new-site --help | grep -qE "db-user|mariadb-user"; then
        SUPPORTS_DB_USER=1
    fi

    if [[ "$DB_USER" != "$DB_NAME" && "$SUPPORTS_DB_USER" -eq 0 ]]; then
        echo "⚠ User '$DB_USER' differs from DB '$DB_NAME' but bench lacks --db-user support."
        echo "  Switching to manual configuration path for reliable connection."
        USE_MANUAL_CONFIG=1
    fi

    if [[ "$USE_MANUAL_CONFIG" -eq 1 ]]; then
        echo "=========================================="
        echo "Manual Site Configuration (Migration Path)"
        echo "=========================================="
        echo "Site Name: $SITE_NAME"
        echo "Database Provider: $DB_PROVIDER"
        echo "Database Name: $DB_NAME"
        echo "Database User: $DB_USER"
        echo "Database Host: $DB_HOST:$DB_PORT"
        echo "=========================================="

        mkdir -p "sites/$SITE_NAME/logs"
        
        # Merge credentials into site_config.json safely before migrating
        update_site_config_json
        
        echo "Running bench migrate..."
        bench --site "$SITE_NAME" migrate
        echo "✓ Bench migrate completed"

        # Install requested apps if skipInit is true
        if [[ $APPS_TO_INSTALL_COUNT -gt 0 ]]; then
            echo "Installing requested apps..."
            for app in $APPS_TO_INSTALL; do
                if [[ -d "apps/$app" ]]; then
                    echo "Installing app: $app"
                    # We use || true because sometimes apps are already installed but not in tabInstalled App
                    # and install-app handles it, but we don't want to crash the init job if one fails
                    bench --site "$SITE_NAME" install-app "$app" || echo "⚠ WARNING: Failed to install $app, it might already be installed."
                fi
            done
        fi
        
        SITE_CREATION_EXIT_CODE=0
    elif [[ "$goto_update_config" -eq 0 ]]; then
        echo "=========================================="
        echo "Creating Frappe Site"
        echo "=========================================="
        echo "Site Name: $SITE_NAME"
        echo "Database Provider: $DB_PROVIDER"
        echo "Database Name: $DB_NAME"
        echo "Database Host: $DB_HOST:$DB_PORT"
        echo "Apps to install: ${APPS_TO_INSTALL:-none}"
        echo "=========================================="
        
        # Check if bench version supports --db-user flag AND if it's needed
        DB_USER_FLAG=""
        if [[ "$DB_USER" != "$DB_NAME" ]]; then
            if [[ "$SUPPORTS_DB_USER" -eq 1 ]]; then
                echo "✓ Detected support for --db-user flag (User differs from DB Name)"
                DB_USER_FLAG="--db-user=$DB_USER"
            else
                echo "⚠ User '$DB_USER' differs from DB '$DB_NAME' but bench lacks --db-user support."
                echo "  Process may fail if database user is not the same as database name."
            fi
        fi

        echo ""
        echo "Executing: bench new-site with apps: $INSTALL_APP_ARG"
        echo "------------------------------------------"
        
        # Capture both stdout and stderr, and exit code
        # Temporarily disable exit-on-error to capture the output
        SITE_CREATION_OUTPUT=""
        SITE_CREATION_EXIT_CODE=0
        set +e  # Don't exit on error yet, we want to capture it
        SITE_CREATION_OUTPUT=$(bench new-site \
          --db-type="$DB_PROVIDER" \
          --db-name="$DB_NAME" \
          --db-host="$DB_HOST" \
          --db-port="$DB_PORT" \
          $DB_USER_FLAG \
          --db-password="$DB_PASSWORD" \
          --no-setup-db \
          --admin-password="$ADMIN_PASSWORD" \
          $INSTALL_APP_ARG \
          --force \
          --verbose \
          "$SITE_NAME" 2>&1)
        SITE_CREATION_EXIT_CODE=$?
        set -e  # Re-enable exit on error
        
        # Always print the output
        echo "$SITE_CREATION_OUTPUT"
        echo "------------------------------------------"
        
        if [[ $SITE_CREATION_EXIT_CODE -eq 0 ]]; then
            echo "✓ Site created successfully!"
            if [[ $APPS_TO_INSTALL_COUNT -gt 0 ]]; then
                echo "✓ Requested installation of $APPS_TO_INSTALL_COUNT app(s). See logs above for per-app status."
                
                # Log each app that was requested for installation
                echo "Apps requested for installation:"
                for app in $APPS_TO_INSTALL; do
                    echo "  - $app"
                done
            fi
        else
            echo "✗ ERROR: Site creation failed with exit code $SITE_CREATION_EXIT_CODE"
            
            # Try to extract error information
            if echo "$SITE_CREATION_OUTPUT" | grep -Eqi "error|traceback|exception|failed"; then
                echo "Error details found in output above"
            fi
            
            # Check for specific app installation failures with more patterns
            if echo "$SITE_CREATION_OUTPUT" | grep -Eqi "app.*not (found|installed)|no module named|cannot import|failed to install"; then
                echo "ERROR: App installation failed - one or more apps could not be found or imported"
            fi
            
            # If site exists, it's not a critical error, continue to config update
            if echo "$SITE_CREATION_OUTPUT" | grep -Eqi "site.*(already exists|exists already)"; then
                echo "⚠ Site already exists, will proceed to update configuration"
                # Don't exit - continue to update config
                SITE_CREATION_EXIT_CODE=0
            else
                echo "CRITICAL ERROR: Site creation failed. Exiting."
                exit $SITE_CREATION_EXIT_CODE
            fi
        fi
        echo "=========================================="
    else
        echo "=========================================="
        echo "Site already exists - Running Maintenance"
        echo "=========================================="
        
        # Ensure site_config.json has the correct DB credentials before migrating
        # using bench set-config or our python helper to ensure proper formatting and handling
        if [[ "$DB_PROVIDER" == "mariadb" ]] || [[ "$DB_PROVIDER" == "postgres" ]]; then
             update_site_config_json
        fi

        echo "1. Running bench migrate..."
        bench --site "$SITE_NAME" migrate
        echo "✓ Migration completed."
        
        echo "2. Installing new apps..."
        if [[ $APPS_TO_INSTALL_COUNT -gt 0 ]]; then
            for app in $APPS_TO_INSTALL; do
                if [[ -d "apps/$app" ]]; then
                    echo "Ensuring app is installed: $app"
                    # install-app usually fails if already installed, so allow failure but log it
                    bench --site "$SITE_NAME" install-app "$app" || echo "Note: $app installation skipped (might be already installed)"
                fi
            done
        fi
        
        echo "3. Syncing Admin Password..."
        if [[ -n "$ADMIN_PASSWORD" ]]; then
            bench --site "$SITE_NAME" set-admin-password "$ADMIN_PASSWORD"
            echo "✓ Admin password synchronized with Kubernetes Secret."
        fi

        echo "4. Cache will be cleared automatically by the post-execution hook."
    fi
else
    echo "ERROR: Unsupported DB provider: $DB_PROVIDER"
    exit 1
fi


echo "Site $SITE_NAME created successfully!"

# Final pass: Ensure site_config.json is up-to-date with domain and Redis configuration
echo "Finalizing site configuration for domain: $DOMAIN and Redis Cache: $REDIS_CACHE_ADDRESS / Queue: $REDIS_QUEUE_ADDRESS"
update_site_config_json


touch "/home/frappe/frappe-bench/sites/$SITE_NAME/.init_complete"
echo "Site initialization complete!"

# Exit success regardless of whether new-site ran
exit 0
