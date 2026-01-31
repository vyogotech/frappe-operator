#!/bin/bash
# Site initialization script for Frappe (embedded in operator, executed in init jobs)

set -e
umask 0002

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

echo "Creating Frappe site: $SITE_NAME"
echo "Domain: $DOMAIN"

# If the site directory already exists, skip creation but update config
if [[ -d "/home/frappe/frappe-bench/sites/$SITE_NAME" ]]; then
    echo "Site $SITE_NAME already exists; skipping new-site and updating config."
    goto_update_config=1
else
    goto_update_config=0
fi

# Link apps.txt to site path for bench to find it
# The apps.txt is in the sites directory, but bench expects it in the root
echo "Debug: Current directory is $(pwd)"
echo "Debug: Contents of $(pwd):"
ls -la
if [ -f sites/apps.txt ]; then
    echo "Debug: sites/apps.txt found, creating link..."
    ln -sf sites/apps.txt apps.txt || cp sites/apps.txt apps.txt || echo "Warning: Failed to create apps.txt in root"
else
    echo "Warning: sites/apps.txt not found!"
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

# Run bench new-site with provider-specific database configuration
if [[ "$DB_PROVIDER" == "mariadb" ]] || [[ "$DB_PROVIDER" == "postgres" ]]; then
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

    if [[ "$goto_update_config" -eq 0 ]]; then
        echo "=========================================="
        echo "Creating Frappe Site"
        echo "=========================================="
        echo "Site Name: $SITE_NAME"
        echo "Database Provider: $DB_PROVIDER"
        echo "Database Name: $DB_NAME"
        echo "Database Host: $DB_HOST:$DB_PORT"
        echo "Apps to install: ${APPS_TO_INSTALL:-none}"
        echo "=========================================="
        
        # Check if bench version supports --db-user flag
        DB_USER_FLAG=""
        if bench new-site --help | grep -q " --db-user"; then
            echo "✓ Detected support for --db-user flag"
            DB_USER_FLAG="--db-user=$DB_USER"
        elif [[ "$DB_USER" != "$DB_NAME" ]]; then
            echo "⚠ WARNING: Your bench version does not support --db-user. Using DB_NAME as username."
        else
            echo "✓ Bench version does not support --db-user, but DB_USER matches DB_NAME. Proceeding."
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
            else
                echo "CRITICAL ERROR: Site creation failed. Exiting."
                exit $SITE_CREATION_EXIT_CODE
            fi
        fi
        echo "=========================================="
    else
        echo "=========================================="
        echo "Site already exists - skipping site creation"
        echo "Will update site_config.json only"
        echo "Note: Apps cannot be installed after site creation"
        echo "=========================================="
    fi
else
    echo "ERROR: Unsupported DB provider: $DB_PROVIDER"
    exit 1
fi

# Create or update common_site_config.json
echo "Creating common_site_config.json..."
cat > sites/common_site_config.json <<EOF
{
  "redis_cache": "redis://${BENCH_NAME}-redis-cache:6379",
  "redis_queue": "redis://${BENCH_NAME}-redis-queue:6379",
  "socketio_port": 9000
}
EOF

# Sync assets from the image cache to the Persistent Volume
if [ -d "/home/frappe/assets_cache" ]; then
    echo "Syncing pre-built assets from image to PVC..."
    mkdir -p sites/assets
    cp -rn /home/frappe/assets_cache/* sites/assets/ || true
fi

echo "Site $SITE_NAME created successfully!"

# Update site_config.json with domain and Redis configuration using Python
echo "Updating site_config.json with domain and Redis"
python3 << 'PYTHON_SCRIPT'
import json, os

# Read from secret files mounted at /tmp/site-secrets
with open('/tmp/site-secrets/site_name', 'r') as f:
    site_name = f.read().strip()
with open('/tmp/site-secrets/domain', 'r') as f:
    domain = f.read().strip()
with open('/tmp/site-secrets/bench_name', 'r') as f:
    bench_name = f.read().strip()
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

site_path = f"/home/frappe/frappe-bench/sites/{site_name}"
config_file = os.path.join(site_path, "site_config.json")

# Read or initialize config
try:
    with open(config_file, 'r') as f:
        config = json.load(f)
except FileNotFoundError:
    config = {}

# Update with resolved domain
config['host_name'] = domain

# Add Redis configuration for this site
config['redis_cache'] = f"redis://{bench_name}-redis-cache:6379"
config['redis_queue'] = f"redis://{bench_name}-redis-queue:6379"

# Explicitly add database credentials for self-healing
config['db_name'] = db_name
config['db_user'] = db_user
config['db_password'] = db_password
config['db_host'] = db_host
config['db_type'] = db_provider

# Ensure directory exists
os.makedirs(site_path, exist_ok=True)

# Ensure logs directory exists
os.makedirs(os.path.join(site_path, "logs"), exist_ok=True)

# Write back
with open(config_file, 'w') as f:
    json.dump(config, f, indent=2)

print(f"Updated site_config.json for domain: {domain}")
print(f"Redis cache: {bench_name}-redis-cache:6379")
print(f"Redis queue: {bench_name}-redis-queue:6379")
PYTHON_SCRIPT

echo "Site initialization complete!"

# Exit success regardless of whether new-site ran
exit 0
