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

package scripts

import (
	"strings"
	"testing"
)

func TestGetScript(t *testing.T) {
	tests := []struct {
		name     ScriptName
		contains string
	}{
		{SiteInit, "bench new-site"},
		{SiteDelete, "bench drop-site"},
		{SiteBackup, "bench --site"},
		{AppInstall, "install-app"},
		{UpdateSiteConfig, "site_config.json"},
	}

	for _, tc := range tests {
		t.Run(string(tc.name), func(t *testing.T) {
			content, err := GetScript(tc.name)
			if err != nil {
				t.Fatalf("GetScript(%s) error: %v", tc.name, err)
			}
			if content == "" {
				t.Errorf("GetScript(%s) returned empty content", tc.name)
			}
			if !strings.Contains(content, tc.contains) {
				t.Errorf("GetScript(%s) should contain '%s'", tc.name, tc.contains)
			}
		})
	}
}

func TestMustGetScript(t *testing.T) {
	// Should not panic for valid scripts
	for _, name := range ListScripts() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("MustGetScript(%s) panicked: %v", name, r)
				}
			}()
			content := MustGetScript(name)
			if content == "" {
				t.Errorf("MustGetScript(%s) returned empty content", name)
			}
		}()
	}
}

func TestGetScriptNotFound(t *testing.T) {
	_, err := GetScript("nonexistent.sh")
	if err == nil {
		t.Error("expected error for nonexistent script")
	}
}

func TestValidateScripts(t *testing.T) {
	if err := ValidateScripts(); err != nil {
		t.Errorf("ValidateScripts() error: %v", err)
	}
}

func TestListScripts(t *testing.T) {
	scripts := ListScripts()
	if len(scripts) == 0 {
		t.Error("ListScripts() returned empty list")
	}

	expected := []ScriptName{SiteInit, SiteDelete, SiteBackup, BenchInit, AppInstall, UpdateSiteConfig}
	if len(scripts) != len(expected) {
		t.Errorf("expected %d scripts, got %d", len(expected), len(scripts))
	}
}

func TestScriptShebang(t *testing.T) {
	// Shell scripts should have proper shebang
	shellScripts := []ScriptName{SiteInit, SiteDelete, SiteBackup, BenchInit, AppInstall}
	for _, name := range shellScripts {
		content, err := GetScript(name)
		if err != nil {
			t.Fatalf("GetScript(%s) error: %v", name, err)
		}
		if !strings.HasPrefix(content, "#!/bin/bash") {
			t.Errorf("script %s should start with #!/bin/bash", name)
		}
	}
}

func TestScriptSetE(t *testing.T) {
	// Shell scripts should use set -e for error handling
	shellScripts := []ScriptName{SiteInit, SiteDelete, SiteBackup, BenchInit, AppInstall}
	for _, name := range shellScripts {
		content, err := GetScript(name)
		if err != nil {
			t.Fatalf("GetScript(%s) error: %v", name, err)
		}
		if !strings.Contains(content, "set -e") {
			t.Errorf("script %s should contain 'set -e'", name)
		}
	}
}

func TestPythonScriptImports(t *testing.T) {
	content, err := GetScript(UpdateSiteConfig)
	if err != nil {
		t.Fatalf("GetScript(%s) error: %v", UpdateSiteConfig, err)
	}
	if !strings.Contains(content, "import json") {
		t.Error("Python script should import json")
	}
	if !strings.Contains(content, "import os") {
		t.Error("Python script should import os")
	}
}

func TestRenderScript(t *testing.T) {
	data := SiteInitData{
		SiteName:      "test-site",
		Domain:        "test.example.com",
		BenchName:     "my-bench",
		DBProvider:    "mariadb",
		AppsToInstall: []string{"frappe", "erpnext"},
	}
	content, err := RenderScript(SiteInit, data)
	if err != nil {
		t.Fatalf("RenderScript(SiteInit, data) error: %v", err)
	}
	if content == "" {
		t.Error("RenderScript returned empty content")
	}
	if !strings.Contains(content, "set -e") {
		t.Error("rendered script should contain set -e")
	}
	// SiteDeleteData
	delData := SiteDeleteData{SiteName: "test-site"}
	delContent, err := RenderScript(SiteDelete, delData)
	if err != nil {
		t.Fatalf("RenderScript(SiteDelete, delData) error: %v", err)
	}
	if !strings.Contains(delContent, "bench drop-site") {
		t.Error("rendered delete script should contain bench drop-site")
	}
	// BenchInitData
	benchData := BenchInitData{BenchName: "e2e-bench"}
	benchContent, err := RenderScript(BenchInit, benchData)
	if err != nil {
		t.Fatalf("RenderScript(BenchInit, benchData) error: %v", err)
	}
	if !strings.Contains(benchContent, "redis://e2e-bench-redis-cache:6379") {
		t.Error("rendered bench init script should contain bench name in redis_cache URL")
	}
	if !strings.Contains(benchContent, "redis://e2e-bench-redis-queue:6379") {
		t.Error("rendered bench init script should contain bench name in redis_queue URL")
	}
}
