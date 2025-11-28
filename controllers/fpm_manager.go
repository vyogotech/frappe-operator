package controllers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// FPMManager handles FPM CLI operations
type FPMManager struct {
	fpmPath string
}

// NewFPMManager creates a new FPM manager instance
func NewFPMManager(fpmPath string) *FPMManager {
	if fpmPath == "" {
		fpmPath = "/usr/local/bin/fpm" // Default path
	}
	return &FPMManager{fpmPath: fpmPath}
}

// ConfigureRepositories configures FPM repositories
func (m *FPMManager) ConfigureRepositories(ctx context.Context, repos []vyogotechv1alpha1.FPMRepository) error {
	logger := log.FromContext(ctx)

	for _, repo := range repos {
		logger.Info("Configuring FPM repository", "name", repo.Name, "url", repo.URL, "priority", repo.Priority)

		args := []string{"repo", "add", repo.Name, repo.URL}
		if repo.Priority != 0 {
			args = append(args, "--priority", fmt.Sprintf("%d", repo.Priority))
		}

		cmd := exec.CommandContext(ctx, m.fpmPath, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Error(err, "Failed to add FPM repository", "name", repo.Name, "output", string(output))
			return fmt.Errorf("failed to add repo %s: %w (output: %s)", repo.Name, err, string(output))
		}

		logger.Info("FPM repository added successfully", "name", repo.Name)
	}

	return nil
}

// SetDefaultRepository sets the default repository for publishing
func (m *FPMManager) SetDefaultRepository(ctx context.Context, repoName string) error {
	logger := log.FromContext(ctx)

	if repoName == "" {
		return nil // No default to set
	}

	logger.Info("Setting default FPM repository", "name", repoName)

	cmd := exec.CommandContext(ctx, m.fpmPath, "repo", "default", repoName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error(err, "Failed to set default FPM repository", "name", repoName, "output", string(output))
		return fmt.Errorf("failed to set default repo %s: %w (output: %s)", repoName, err, string(output))
	}

	logger.Info("Default FPM repository set successfully", "name", repoName)
	return nil
}

// InstallApp installs an app using FPM
func (m *FPMManager) InstallApp(ctx context.Context, org, name, version, benchPath string) error {
	logger := log.FromContext(ctx)

	packageID := fmt.Sprintf("%s/%s==%s", org, name, version)
	logger.Info("Installing FPM package", "package", packageID, "benchPath", benchPath)

	cmd := exec.CommandContext(ctx, m.fpmPath, "install", packageID, "--bench-path", benchPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error(err, "Failed to install FPM package", "package", packageID, "output", string(output))
		return fmt.Errorf("failed to install %s: %w (output: %s)", packageID, err, string(output))
	}

	logger.Info("FPM package installed successfully", "package", packageID)
	return nil
}

// GetApp downloads a package from repository to local store
func (m *FPMManager) GetApp(ctx context.Context, repoName, org, appName, version string) error {
	logger := log.FromContext(ctx)

	identifier := fmt.Sprintf("%s/%s/%s", repoName, org, appName)
	if version != "" && version != "latest" {
		identifier = fmt.Sprintf("%s:%s", identifier, version)
	}

	logger.Info("Downloading FPM package to local store", "identifier", identifier)

	cmd := exec.CommandContext(ctx, m.fpmPath, "get-app", identifier)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error(err, "Failed to download FPM package", "identifier", identifier, "output", string(output))
		return fmt.Errorf("failed to get-app %s: %w (output: %s)", identifier, err, string(output))
	}

	logger.Info("FPM package downloaded successfully", "identifier", identifier)
	return nil
}

// SearchPackage searches for packages in configured repositories
func (m *FPMManager) SearchPackage(ctx context.Context, query string) (string, error) {
	logger := log.FromContext(ctx)

	logger.Info("Searching for FPM packages", "query", query)

	args := []string{"search"}
	if query != "" {
		args = append(args, query)
	}

	cmd := exec.CommandContext(ctx, m.fpmPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error(err, "Failed to search FPM packages", "query", query, "output", string(output))
		return "", fmt.Errorf("failed to search %s: %w (output: %s)", query, err, string(output))
	}

	return string(output), nil
}

// GenerateFPMConfigScript generates a bash script to configure FPM
func (m *FPMManager) GenerateFPMConfigScript(repos []vyogotechv1alpha1.FPMRepository, defaultRepo string) string {
	var script strings.Builder

	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n\n")
	script.WriteString("echo 'Configuring FPM repositories...'\n\n")

	for _, repo := range repos {
		priority := repo.Priority
		if priority == 0 {
			priority = 50 // Default priority
		}

		script.WriteString(fmt.Sprintf("# Add repository: %s\n", repo.Name))
		script.WriteString(fmt.Sprintf("fpm repo add %s %s --priority %d || echo 'Repository %s may already exist'\n\n",
			repo.Name, repo.URL, priority, repo.Name))
	}

	if defaultRepo != "" {
		script.WriteString(fmt.Sprintf("# Set default repository for publishing\n"))
		script.WriteString(fmt.Sprintf("fpm repo default %s || echo 'Could not set default repository'\n\n", defaultRepo))
	}

	script.WriteString("echo 'FPM configuration complete'\n")
	script.WriteString("fpm repo list || echo 'Could not list repositories'\n")

	return script.String()
}

// GenerateAppInstallScript generates a bash script to install apps
func (m *FPMManager) GenerateAppInstallScript(apps []vyogotechv1alpha1.AppSource, gitEnabled bool, benchPath string) string {
	var script strings.Builder

	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n\n")
	script.WriteString(fmt.Sprintf("BENCH_PATH=%s\n", benchPath))
	script.WriteString("cd $BENCH_PATH\n\n")
	script.WriteString("echo 'Installing apps...'\n\n")

	for _, app := range apps {
		script.WriteString(fmt.Sprintf("# Install app: %s (source: %s)\n", app.Name, app.Source))

		switch app.Source {
		case "fpm":
			if app.Org == "" || app.Version == "" {
				script.WriteString(fmt.Sprintf("echo 'Warning: FPM app %s missing org or version, skipping'\n\n", app.Name))
				continue
			}
			packageID := fmt.Sprintf("%s/%s==%s", app.Org, app.Name, app.Version)
			script.WriteString(fmt.Sprintf("echo 'Installing FPM package: %s'\n", packageID))
			script.WriteString(fmt.Sprintf("fpm install %s --bench-path $BENCH_PATH || {\n", packageID))
			script.WriteString(fmt.Sprintf("  echo 'Error: Failed to install FPM package %s'\n", packageID))
			script.WriteString("  exit 1\n")
			script.WriteString("}\n\n")

		case "git":
			if !gitEnabled {
				script.WriteString(fmt.Sprintf("echo 'Skipping Git app %s: Git is disabled in this environment'\n\n", app.Name))
				continue
			}
			if app.GitURL == "" {
				script.WriteString(fmt.Sprintf("echo 'Warning: Git app %s missing gitUrl, skipping'\n\n", app.Name))
				continue
			}
			script.WriteString(fmt.Sprintf("echo 'Installing Git app: %s from %s'\n", app.Name, app.GitURL))
			script.WriteString(fmt.Sprintf("bench get-app %s", app.GitURL))
			if app.GitBranch != "" {
				script.WriteString(fmt.Sprintf(" --branch %s", app.GitBranch))
			}
			script.WriteString(" || {\n")
			script.WriteString(fmt.Sprintf("  echo 'Error: Failed to install Git app %s'\n", app.Name))
			script.WriteString("  exit 1\n")
			script.WriteString("}\n\n")

		case "image":
			script.WriteString(fmt.Sprintf("echo 'App %s expected to be pre-installed in container image'\n", app.Name))
			script.WriteString(fmt.Sprintf("if [ ! -d \"apps/%s\" ]; then\n", app.Name))
			script.WriteString(fmt.Sprintf("  echo 'Warning: App %s not found in image'\n", app.Name))
			script.WriteString("fi\n\n")

		default:
			script.WriteString(fmt.Sprintf("echo 'Warning: Unknown source type \"%s\" for app %s, skipping'\n\n", app.Source, app.Name))
		}
	}

	script.WriteString("echo 'Generating apps.txt...'\n")
	script.WriteString("ls -1 apps/ | grep -v '__pycache__' > sites/apps.txt || echo 'frappe' > sites/apps.txt\n\n")

	script.WriteString("echo 'Building production assets...'\n")
	script.WriteString("bench build --production || {\n")
	script.WriteString("  echo 'Warning: Asset build failed, continuing anyway'\n")
	script.WriteString("}\n\n")

	script.WriteString("echo 'App installation complete'\n")

	return script.String()
}
