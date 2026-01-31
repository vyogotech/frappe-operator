/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package scripts provides embedded shell script templates for Frappe operations
package scripts

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*.sh templates/*.py
var templateFS embed.FS

// ScriptName represents available script templates
type ScriptName string

const (
	// SiteInit initializes a new Frappe site
	SiteInit ScriptName = "site_init.sh"
	// SiteDelete removes a Frappe site
	SiteDelete ScriptName = "site_delete.sh"
	// SiteBackup creates a backup of a Frappe site
	SiteBackup ScriptName = "site_backup.sh"
	// BenchInit initializes a Frappe bench (sites dir, common_site_config.json, assets)
	BenchInit ScriptName = "bench_init.sh"
	// AppInstall installs an app on a Frappe site
	AppInstall ScriptName = "app_install.sh"
	// UpdateSiteConfig updates site_config.json
	UpdateSiteConfig ScriptName = "update_site_config.py"
)

// GetScript returns the raw script content
func GetScript(name ScriptName) (string, error) {
	content, err := templateFS.ReadFile(fmt.Sprintf("templates/%s", name))
	if err != nil {
		return "", fmt.Errorf("failed to read script %s: %w", name, err)
	}
	return string(content), nil
}

// MustGetScript returns the script content or panics
func MustGetScript(name ScriptName) string {
	content, err := GetScript(name)
	if err != nil {
		panic(err)
	}
	return content
}

// RenderScript renders a script template with the given data
func RenderScript(name ScriptName, data interface{}) (string, error) {
	content, err := GetScript(name)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(string(name)).Parse(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", name, err)
	}

	return buf.String(), nil
}

// SiteInitData provides data for site initialization script
type SiteInitData struct {
	SiteName      string
	Domain        string
	BenchName     string
	DBProvider    string
	AppsToInstall []string
}

// SiteDeleteData provides data for site deletion script
type SiteDeleteData struct {
	SiteName string
}

// BenchInitData provides data for bench initialization script
type BenchInitData struct {
	BenchName string
}

// SiteBackupData provides data for site backup script
type SiteBackupData struct {
	SiteName     string
	IncludeFiles bool
}

// AppInstallData provides data for app installation script
type AppInstallData struct {
	SiteName  string
	AppName   string
	AppSource string // git, fpm, or image
	GitURL    string
	GitBranch string
}

// ListScripts returns all available script names
func ListScripts() []ScriptName {
	return []ScriptName{
		SiteInit,
		SiteDelete,
		SiteBackup,
		BenchInit,
		AppInstall,
		UpdateSiteConfig,
	}
}

// ValidateScripts checks that all embedded scripts are parseable
func ValidateScripts() error {
	for _, name := range ListScripts() {
		content, err := GetScript(name)
		if err != nil {
			return fmt.Errorf("script %s: %w", name, err)
		}
		if len(content) == 0 {
			return fmt.Errorf("script %s is empty", name)
		}
	}
	return nil
}
