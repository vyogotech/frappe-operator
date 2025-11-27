# Contributing to Frappe Operator

Thank you for your interest in contributing to Frappe Operator! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Submitting Changes](#submitting-changes)
- [Code Style](#code-style)
- [Testing](#testing)
- [Documentation](#documentation)

## Code of Conduct

This project and everyone participating in it is governed by our Code of Conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the maintainers.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally
3. Create a branch for your changes
4. Make your changes
5. Push to your fork
6. Submit a Pull Request

## Development Setup

### Prerequisites

- Go 1.21 or higher
- Docker
- kubectl
- kind or minikube (for local testing)
- make

### Setup Instructions

```bash
# Clone your fork
git clone https://github.com/YOUR-USERNAME/frappe-operator.git
cd frappe-operator

# Add upstream remote
git remote add upstream https://github.com/vyogotech/frappe-operator.git

# Install dependencies
go mod download

# Install CRDs
make install

# Run tests
make test
```

### Running Locally

```bash
# Run the operator locally (outside cluster)
make run

# Or build and run in cluster
make docker-build docker-push IMG=<your-registry>/frappe-operator:tag
make deploy IMG=<your-registry>/frappe-operator:tag
```

## Making Changes

### Branch Naming

Use descriptive branch names:
- `feature/add-new-feature` - For new features
- `fix/bug-description` - For bug fixes
- `docs/improve-readme` - For documentation updates
- `refactor/component-name` - For refactoring

### Commit Messages

Follow conventional commits:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Example:**
```
feat(frappebench): add support for custom apps

Add ability to specify custom apps with git sources in FrappeBench spec.
This allows users to install apps from private repositories.

Closes #123
```

### Code Changes

1. **Add tests** for new functionality
2. **Update documentation** if behavior changes
3. **Follow Go conventions** and existing code style
4. **Keep changes focused** - one feature/fix per PR
5. **Add comments** for complex logic

## Submitting Changes

### Before Submitting

- [ ] Run tests: `make test`
- [ ] Run linters: `make lint`
- [ ] Update documentation if needed
- [ ] Add/update tests for your changes
- [ ] Regenerate manifests: `make manifests`
- [ ] Update API docs if CRDs changed
- [ ] Ensure all CI checks pass

### Pull Request Process

1. **Create a PR** with a clear title and description
2. **Link related issues** using "Closes #123" or "Fixes #123"
3. **Describe your changes** in detail
4. **Add screenshots** if UI/UX changes
5. **Request review** from maintainers
6. **Address feedback** promptly
7. **Keep PR updated** with main branch

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Related Issues
Closes #123

## How Has This Been Tested?
Describe the tests you ran

## Checklist
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] Code follows style guidelines
- [ ] All tests passing
- [ ] No breaking changes (or documented)
```

## Code Style

### Go Code

Follow the [Effective Go](https://golang.org/doc/effective_go.html) guidelines:

```go
// Good
func (r *FrappeBenchReconciler) createNginxDeployment(ctx context.Context, bench *v1alpha1.FrappeBench) error {
    // Implementation
}

// Bad
func (r *FrappeBenchReconciler) CreateNginx(ctx context.Context, bench *v1alpha1.FrappeBench) error {
    // Implementation
}
```

### Kubernetes Resources

Follow Kubernetes conventions:
- Use lowercase for resource names
- Use hyphens for multi-word names
- Add appropriate labels
- Set owner references

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prod-bench-nginx
  labels:
    app: frappe
    component: nginx
    bench: prod-bench
```

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific test
go test ./controllers -run TestFrappeBenchController
```

### Integration Tests

```bash
# Setup test environment
make test-setup

# Run integration tests
make test-integration

# Cleanup
make test-cleanup
```

### Writing Tests

```go
func TestFrappeBenchReconciler_CreateBench(t *testing.T) {
    // Arrange
    ctx := context.Background()
    bench := &v1alpha1.FrappeBench{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-bench",
            Namespace: "default",
        },
        Spec: v1alpha1.FrappeBenchSpec{
            FrappeVersion: "version-15",
        },
    }
    
    // Act
    result, err := reconciler.Reconcile(ctx, req)
    
    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

## Documentation

### Updating Docs

Documentation is in the `docs/` directory using Markdown:

```bash
# Edit documentation
vim docs/getting-started.md

# Preview locally (requires Jekyll)
cd docs
bundle install
bundle exec jekyll serve

# Open http://localhost:4000
```

### Documentation Guidelines

- Use clear, concise language
- Include code examples
- Add diagrams for complex concepts
- Keep examples up-to-date
- Test all commands/examples

### API Documentation

When updating CRDs, regenerate documentation:

```bash
# Update type definitions
vim api/v1alpha1/frappesite_types.go

# Regenerate manifests
make manifests

# Update API reference docs
vim docs/api-reference.md
```

## Release Process

(For maintainers)

1. Update version in `VERSION` file
2. Update CHANGELOG.md
3. Create and push tag
4. GitHub Actions will build and publish release

```bash
# Tag release
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

## Getting Help

- **Questions**: Open a [GitHub Discussion](https://github.com/vyogotech/frappe-operator/discussions)
- **Bugs**: Open a [GitHub Issue](https://github.com/vyogotech/frappe-operator/issues)
- **Chat**: Join our community chat (link TBD)

## Recognition

Contributors will be:
- Added to CONTRIBUTORS.md
- Mentioned in release notes
- Featured on project website (coming soon)

Thank you for contributing to Frappe Operator! 🎉

