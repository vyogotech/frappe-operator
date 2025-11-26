# GitHub Workflows

This directory contains GitHub Actions workflows for the Frappe Operator project.

## Workflows

### CI Workflow (`ci.yml`)

Runs on every push to `main`, `master`, or `develop` branches and on all pull requests.

**Jobs:**

1. **Test**
   - Sets up Go 1.19
   - Downloads and verifies dependencies
   - Runs `go vet` for static analysis
   - Checks code formatting with `go fmt`
   - Runs unit tests with race detection
   - Uploads coverage reports to Codecov

2. **Build**
   - Builds the operator binary
   - Uploads the binary as a workflow artifact (7-day retention)

3. **Docker Build**
   - Builds multi-platform Docker images (linux/amd64, linux/arm64)
   - Pushes images to GitHub Container Registry (ghcr.io)
   - Tags images based on branch/PR
   - Uses GitHub Actions cache for faster builds
   - Only pushes images on branch pushes (not on PRs)

**Image Tags (CI):**
- `ghcr.io/[owner]/frappe-operator:[branch-name]` - Branch builds
- `ghcr.io/[owner]/frappe-operator:pr-[number]` - PR builds (not pushed)
- `ghcr.io/[owner]/frappe-operator:[branch-name]-[sha]` - SHA-based tags

### Release Workflow (`release.yml`)

Triggered when a new tag matching `v*` is pushed (e.g., `v0.1.0`, `v1.2.3`).

**Jobs:**

1. **Build and Release**
   - Runs all tests to ensure quality
   - Builds multi-platform Docker images (linux/amd64, linux/arm64)
   - Pushes images to GitHub Container Registry with version tags
   - Builds operator binaries for multiple platforms:
     - Linux AMD64
     - Linux ARM64
     - macOS AMD64
     - macOS ARM64 (Apple Silicon)
   - Generates Kubernetes manifests
   - Creates release archives (`.tar.gz`)
   - Generates SHA256 checksums
   - Creates a GitHub Release with:
     - Release notes
     - Binary downloads
     - Manifest archives
     - Checksums

**Image Tags (Release):**
- `ghcr.io/[owner]/frappe-operator:v[version]` - Version tag (e.g., v0.1.0)
- `ghcr.io/[owner]/frappe-operator:[major].[minor]` - Minor version tag (e.g., 0.1)
- `ghcr.io/[owner]/frappe-operator:[major]` - Major version tag (e.g., 0)
- `ghcr.io/[owner]/frappe-operator:latest` - Latest release

### Documentation Workflow (`docs.yml`)

Triggered when:
- Changes are pushed to the `docs/` directory or the workflow file itself
- Manual trigger via `workflow_dispatch`

**Jobs:**

1. **Build**
   - Checks out the repository
   - Sets up GitHub Pages configuration
   - Builds the documentation with Jekyll from the `docs/` directory
   - Uploads the built site as a Pages artifact

2. **Deploy**
   - Deploys the built site to GitHub Pages
   - Provides the deployed URL

**Concurrency:** Only one Pages deployment runs at a time to prevent conflicts.

## Usage

### For Developers

**Running CI locally before pushing:**

```bash
# Run tests
make test

# Build the binary
make build

# Build Docker image
make docker-build IMG=frappe-operator:test
```

### For End Users

**Installing from a release:**

1. **Using Docker:**
   ```bash
   docker pull ghcr.io/[owner]/frappe-operator:v0.1.0
   ```

2. **Using kubectl:**
   ```bash
   # Download and extract manifests
   wget https://github.com/[owner]/frappe-operator/releases/download/v0.1.0/manifests-0.1.0.tar.gz
   tar xzf manifests-0.1.0.tar.gz
   
   # Apply manifests
   kubectl apply -k manifests/config/default
   ```

3. **Using the binary:**
   ```bash
   # Download for your platform
   wget https://github.com/[owner]/frappe-operator/releases/download/v0.1.0/frappe-operator-linux-amd64.tar.gz
   
   # Extract and install
   tar xzf frappe-operator-linux-amd64.tar.gz
   chmod +x frappe-operator-linux-amd64
   sudo mv frappe-operator-linux-amd64 /usr/local/bin/frappe-operator
   ```

**Viewing Documentation:**

Documentation is automatically published to GitHub Pages and available at:
```
https://[owner].github.io/frappe-operator/
```

## Creating a Release

To create a new release:

1. **Update version** (if needed):
   ```bash
   # Update VERSION in Makefile or other version files
   ```

2. **Create and push a tag:**
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```

3. **Wait for the workflow:**
   - The release workflow will automatically:
     - Run tests
     - Build and push Docker images
     - Build binaries for all platforms
     - Create a GitHub Release with all artifacts

4. **Edit release notes** (optional):
   - Go to GitHub Releases
   - Edit the auto-generated release
   - Add more details about features, bug fixes, etc.

## Updating Documentation

To update the documentation:

1. **Edit files in the `docs/` directory:**
   ```bash
   # Edit markdown files
   vim docs/index.md
   ```

2. **Commit and push:**
   ```bash
   git add docs/
   git commit -m "Update documentation"
   git push
   ```

3. **Wait for deployment:**
   - The docs workflow will automatically build and deploy to GitHub Pages
   - Changes will be live within a few minutes

4. **Manual deployment:**
   ```bash
   # Trigger manually from GitHub Actions UI
   # Go to Actions → Deploy Documentation → Run workflow
   ```

## Setting up GitHub Pages

To enable GitHub Pages for this repository:

1. Go to **Settings** → **Pages**
2. Under **Source**, select **GitHub Actions**
3. The docs workflow will automatically deploy to Pages

## Permissions

The workflows require the following permissions:

- **CI Workflow:**
  - `contents: read` - Read repository code
  - `packages: write` - Push Docker images to GHCR

- **Release Workflow:**
  - `contents: write` - Create releases and upload assets
  - `packages: write` - Push Docker images to GHCR

- **Docs Workflow:**
  - `contents: read` - Read repository code
  - `pages: write` - Deploy to GitHub Pages
  - `id-token: write` - Required for Pages deployment

These permissions are automatically granted by GitHub Actions via the `GITHUB_TOKEN` secret.

## Container Registry

Images are pushed to GitHub Container Registry (ghcr.io). To pull images, you may need to authenticate:

```bash
# Create a personal access token with read:packages scope
# Then login:
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Pull the image
docker pull ghcr.io/[owner]/frappe-operator:latest
```

## Troubleshooting

### Workflow fails with "permission denied"
- Check that the repository has Actions enabled
- Ensure the `GITHUB_TOKEN` has sufficient permissions
- For organization repos, check organization settings for package permissions

### Docker build is slow
- The workflows use GitHub Actions cache to speed up builds
- First build will be slower, subsequent builds will be faster

### Release workflow not triggered
- Ensure you're pushing a tag that matches `v*` pattern
- Check that you have push permissions to the repository
- Verify the tag format: `v0.1.0` (not `0.1.0` or `ver-0.1.0`)

### GitHub Pages not deploying
- Ensure GitHub Pages is enabled in repository settings
- Check that the source is set to "GitHub Actions"
- Verify the docs workflow has the correct permissions
- Check the Actions tab for any deployment errors

### Documentation not updating
- Ensure changes are in the `docs/` directory
- Check that the docs workflow was triggered
- Clear browser cache if changes don't appear

## Customization

### Changing Go version
Edit the `go-version` field in both workflows:

```yaml
- name: Set up Go
  uses: actions/setup-go@v5
  with:
    go-version: '1.19'  # Change this
```

### Adding more platforms
Edit the `platforms` field in the Docker build steps:

```yaml
platforms: linux/amd64,linux/arm64,linux/s390x  # Add more here
```

### Changing image registry
Replace `ghcr.io` with your preferred registry (Docker Hub, Quay.io, etc.) and update the login action accordingly.

### Customizing documentation theme
Edit `docs/_config.yml` to customize Jekyll settings:

```yaml
theme: jekyll-theme-cayman  # Change to your preferred theme
title: Frappe Operator
description: Kubernetes operator for Frappe/ERPNext
```

