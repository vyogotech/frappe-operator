#!/bin/bash
# App installation script for Frappe
# This script is embedded in the operator and executed in app install jobs

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

SITE_NAME="${SITE_NAME:-$(cat /tmp/app-config/site_name 2>/dev/null || echo '')}"
APP_NAME="${APP_NAME:-$(cat /tmp/app-config/app_name 2>/dev/null || echo '')}"
APP_SOURCE="${APP_SOURCE:-$(cat /tmp/app-config/app_source 2>/dev/null || echo 'image')}"

if [[ -z "$SITE_NAME" ]]; then
    echo "ERROR: SITE_NAME not set"
    exit 1
fi

if [[ -z "$APP_NAME" ]]; then
    echo "ERROR: APP_NAME not set"
    exit 1
fi

echo "Installing app: $APP_NAME on site: $SITE_NAME"
echo "Source: $APP_SOURCE"

case "$APP_SOURCE" in
    "git")
        GIT_URL="${GIT_URL:-$(cat /tmp/app-config/git_url 2>/dev/null || echo '')}"
        GIT_BRANCH="${GIT_BRANCH:-$(cat /tmp/app-config/git_branch 2>/dev/null || echo '')}"
        
        if [[ -z "$GIT_URL" ]]; then
            echo "ERROR: GIT_URL not set for git source"
            exit 1
        fi
        
        echo "Getting app from git: $GIT_URL (branch: ${GIT_BRANCH:-default})"
        if [[ -n "$GIT_BRANCH" ]]; then
            bench get-app --branch "$GIT_BRANCH" "$GIT_URL"
        else
            bench get-app "$GIT_URL"
        fi
        ;;
    "fpm")
        echo "Installing app from FPM: $APP_NAME"
        bench fpm install "$APP_NAME"
        ;;
    "image")
        echo "App $APP_NAME is pre-installed in image"
        ;;
    *)
        echo "ERROR: Unknown source type: $APP_SOURCE"
        exit 1
        ;;
esac

# Install app on site
echo "Installing $APP_NAME on site $SITE_NAME"
bench --site "$SITE_NAME" install-app "$APP_NAME"

echo "App $APP_NAME installed successfully on $SITE_NAME!"
