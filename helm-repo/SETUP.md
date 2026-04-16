# Helm Repository Setup Instructions

## Quick Setup Checklist

### 1. Enable GitHub Pages

1. Go to your GitHub repository: `https://github.com/vyogotech/frappe-operator`
2. Navigate to **Settings** → **Pages**
3. Configure:
   - **Source**: Deploy from a branch
   - **Branch**: `main` (or `master`)
   - **Folder**: `/docs`
4. Click **Save**

### 2. Initial Chart Publication

Run the packaging script to create the first chart:

```bash
./scripts/package-helm-chart.sh
```

This will:
- Package the Helm chart
- Generate `index.yaml`
- Place files in `docs/helm-repo/`

### 3. Commit and Push

```bash
git add docs/helm-repo/
git commit -m "chore: initial Helm repository setup"
git push
```

### 4. Verify

After GitHub Pages is deployed (usually takes 1-2 minutes), verify:

```bash
# Add the repository
helm repo add frappe-operator https://vyogotech.github.io/frappe-operator/helm-repo

# Update repository cache
helm repo update

# Search for the chart
helm search repo frappe-operator
```

You should see the chart listed!

## Automatic Publishing

The repository is automatically updated via GitHub Actions when:
- Changes are pushed to `helm/` directory
- A new GitHub release is published
- The workflow is manually triggered

## Manual Publishing

To manually publish a new chart version:

```bash
# Package with specific version
./scripts/package-helm-chart.sh 1.0.0

# Commit and push
git add docs/helm-repo/
git commit -m "chore: publish Helm chart v1.0.0"
git push
```

## Repository Structure

```
docs/helm-repo/
├── README.md              # This file
├── SETUP.md               # Setup instructions
├── index.yaml             # Helm repository index (auto-generated)
├── .gitignore             # Excludes .tgz files
└── frappe-operator-*.tgz  # Packaged charts (gitignored)
```

## Troubleshooting

### Pages Not Deploying

- Check GitHub Actions for errors
- Verify `docs/helm-repo/index.yaml` exists
- Ensure GitHub Pages is enabled in repository settings

### Chart Not Found

- Run `helm repo update` to refresh cache
- Verify the URL is correct: `https://vyogotech.github.io/frappe-operator/helm-repo`
- Check that `index.yaml` contains your chart version

### 404 Errors

- Wait a few minutes for GitHub Pages to deploy
- Clear browser cache
- Verify the repository is public (or use authentication for private repos)
