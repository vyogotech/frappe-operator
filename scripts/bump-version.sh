#!/bin/bash

# Update version across the operator repository

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <new_version>"
    echo "Example: $0 3.2.0"
    exit 1
fi

NEW_VERSION=$1
# Strip the 'v' prefix if user provided it
CLEAN_VERSION=${NEW_VERSION#v}
V_VERSION="v${CLEAN_VERSION}"

echo "Bumping version to $CLEAN_VERSION ($V_VERSION)..."

# 1. Update Makefile
echo "Updating Makefile..."
sed -i.bak -e "s/^VERSION ?= .*/VERSION ?= $CLEAN_VERSION/" Makefile
rm -f Makefile.bak

# 2. Update Kustomization
echo "Updating config/manager/kustomization.yaml..."
sed -i.bak -e "s/newTag: .*/newTag: $CLEAN_VERSION/" config/manager/kustomization.yaml
rm -f config/manager/kustomization.yaml.bak

# 3. Update Helm Chart values.yaml
echo "Updating helm/frappe-operator/values.yaml..."
sed -i.bak -e "s/tag: \".*\"/tag: \"$V_VERSION\"/" helm/frappe-operator/values.yaml
rm -f helm/frappe-operator/values.yaml.bak

# 4. Update Helm Chart.yaml
echo "Updating helm/frappe-operator/Chart.yaml..."
sed -i.bak -e "s/^version: .*/version: $CLEAN_VERSION/" helm/frappe-operator/Chart.yaml
sed -i.bak -e "s/^appVersion: \".*\"/appVersion: \"$V_VERSION\"/" helm/frappe-operator/Chart.yaml
rm -f helm/frappe-operator/Chart.yaml.bak

# 5. Update install.yaml if it exists
if [ -f "install.yaml" ]; then
    echo "Updating install.yaml..."
    # Looks for 'image: ghcr.io/vyogotech/frappe-operator:...' and updates the tag
    sed -i.bak -e "s|image: ghcr.io/vyogotech/frappe-operator:.*|image: ghcr.io/vyogotech/frappe-operator:$CLEAN_VERSION|g" install.yaml
    rm -f install.yaml.bak
fi

echo "✅ Version bump complete!"
echo "Check your git diff, then run 'make manifests' or 'make bundle' (if using OLM) to ensure everything is generated cleanly."
