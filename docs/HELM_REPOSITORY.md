# Helm Repository Setup Guide

This guide explains how to set up and use the Helm repository for Frappe Operator hosted on GitHub Pages.

## Overview

The Helm repository is hosted on GitHub Pages using the `docs/helm-repo/` directory. This allows users to install and update the Frappe Operator using standard Helm commands.

## Repository URL

```
https://vyogotech.github.io/frappe-operator/helm-repo
```

## Quick Start

### For Users

1. **Add the repository:**
   ```bash
   helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
   helm repo update
   ```

2. **Install the operator:**
   ```bash
   helm install my-frappe-operator frappe-operator/frappe-operator
   ```

3. **Upgrade to latest version:**
   ```bash
   helm repo update
   helm upgrade my-frappe-operator frappe-operator/frappe-operator
   ```

### For Maintainers

#### Manual Publishing

1. **Package the chart:**
   ```bash
   ./scripts/package-helm-chart.sh [version]
   ```
   
   Example:
   ```bash
   ./scripts/package-helm-chart.sh 1.0.0
   ```

2. **Commit and push:**
   ```bash
   git add docs/helm-repo/
   git commit -m "chore: publish Helm chart v1.0.0"
   git push
   ```

3. **Verify:**
   ```bash
   helm repo add test-repo https://vyogotech.github.io/frappe-operator/helm-repo
   helm repo update
   helm search repo frappe-operator
   ```

#### Automatic Publishing

The repository is automatically updated via GitHub Actions when:
- Changes are pushed to `helm/` directory
- A new GitHub release is published
- The workflow is manually triggered

## GitHub Pages Configuration

### Initial Setup

1. Go to your GitHub repository
2. Navigate to **Settings** → **Pages**
3. Configure:
   - **Source**: Deploy from a branch
   - **Branch**: `main` (or `master`)
   - **Folder**: `/docs`
4. Click **Save**

### Verification

After setup, verify the repository is accessible:

```bash
curl https://vyogotech.github.io/frappe-operator/helm-repo/index.yaml
```

You should see the `index.yaml` file with chart metadata.

## Repository Structure

```
docs/helm-repo/
├── README.md           # Repository documentation
├── index.yaml          # Helm repository index (auto-generated)
├── .gitignore          # Ignore .tgz files, keep index.yaml
└── frappe-operator-*.tgz  # Packaged chart archives
```

## Chart Versioning

Chart versions should follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

Example versions: `1.0.0`, `1.1.0`, `2.0.0`

## Troubleshooting

### Repository Not Found

- Verify GitHub Pages is enabled and configured correctly
- Check that `docs/helm-repo/index.yaml` exists
- Ensure the repository URL is correct

### Chart Not Found

- Run `helm repo update` to refresh the local cache
- Verify the chart version exists in `index.yaml`
- Check that the `.tgz` file is accessible via the URL

### Authentication Issues

If your repository is private, you'll need to:
1. Use GitHub Personal Access Token
2. Configure Helm with credentials:
   ```bash
   helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo \
     --username <github-username> \
     --password <github-token>
   ```

## Advanced Usage

### Install Specific Version

```bash
helm install my-frappe-operator frappe-operator/frappe-operator --version 1.0.0
```

### View Chart Values

```bash
helm show values frappe-operator/frappe-operator
```

### Dry Run Installation

```bash
helm install my-frappe-operator frappe-operator/frappe-operator --dry-run --debug
```

### Uninstall

```bash
helm uninstall my-frappe-operator
```

## CI/CD Integration

The repository includes a GitHub Actions workflow (`.github/workflows/publish-helm-chart.yml`) that:
- Automatically packages charts when Helm files change
- Publishes charts on new releases
- Updates the repository index
- Deploys to GitHub Pages

## Security Considerations

- Charts are served over HTTPS via GitHub Pages
- Chart checksums are included in `index.yaml`
- Users can verify chart integrity using Helm's built-in verification

## References

- [Helm Documentation](https://helm.sh/docs/)
- [Helm Chart Repository Guide](https://helm.sh/docs/topics/chart_repository/)
- [GitHub Pages Documentation](https://docs.github.com/en/pages)
