# Frappe Operator Helm Repository

This directory contains the Helm chart repository for the Frappe Operator.

## Repository URL

```
https://vyogotech.github.io/frappe-operator/helm-repo
```

## Usage

### Add the Repository

```bash
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo
helm repo update
```

### Install the Chart

```bash
helm install my-frappe-operator frappe-operator/frappe-operator
```

### Upgrade the Chart

```bash
helm upgrade my-frappe-operator frappe-operator/frappe-operator
```

### View Available Versions

```bash
helm search repo frappe-operator --versions
```

## Chart Versions

See `index.yaml` for available chart versions and their metadata.

## Publishing New Versions

New chart versions are automatically published via GitHub Actions when:
- Changes are pushed to the `helm/` directory
- A new release is published on GitHub
- The workflow is manually triggered

To manually publish a chart version:

```bash
./scripts/package-helm-chart.sh [version]
git add docs/helm-repo/
git commit -m "chore: publish Helm chart v[version]"
git push
```

## GitHub Pages Setup

1. Go to repository Settings → Pages
2. Set Source to "Deploy from a branch"
3. Select branch: `main` or `master`
4. Select folder: `/docs`
5. Click Save

The Helm repository will be available at:
`https://vyogotech.github.io/frappe-operator/helm-repo`
