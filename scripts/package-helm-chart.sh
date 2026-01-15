#!/bin/bash
set -e

# Script to package Helm chart and update repository index
# Usage: ./scripts/package-helm-chart.sh [version]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HELM_DIR="$PROJECT_ROOT/helm"
CHART_NAME="frappe-operator"
CHART_DIR="$HELM_DIR/$CHART_NAME"
REPO_DIR="$PROJECT_ROOT/docs/helm-repo"
VERSION="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo '1.0.0')}"

echo "📦 Packaging Helm chart: $CHART_NAME version $VERSION"

# Create repo directory if it doesn't exist
mkdir -p "$REPO_DIR"

# Package the chart
echo "🔨 Packaging chart..."
helm package "$CHART_DIR" \
  --destination "$REPO_DIR" \
  --version "$VERSION" \
  --app-version "$VERSION"

# Generate or update the index
echo "📇 Generating repository index..."
helm repo index "$REPO_DIR" \
  --url "https://vyogotech.github.io/frappe-operator/helm-repo" \
  --merge "$REPO_DIR/index.yaml" 2>/dev/null || \
helm repo index "$REPO_DIR" \
  --url "https://vyogotech.github.io/frappe-operator/helm-repo"

echo "✅ Chart packaged successfully!"
echo "📦 Chart location: $REPO_DIR/$CHART_NAME-$VERSION.tgz"
echo ""
echo "To publish:"
echo "  1. Commit the changes in docs/helm-repo/"
echo "  2. Push to GitHub"
echo "  3. Enable GitHub Pages for the docs/ directory"
echo ""
echo "Users can then add the repo with:"
echo "  helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo"
echo "  helm repo update"
echo "  helm install my-frappe-operator frappe-operator/frappe-operator"
