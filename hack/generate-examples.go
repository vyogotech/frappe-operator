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

// Generate example manifests from templates
// Usage: go run hack/generate-examples.go [scenario]
//
// Scenarios:
//   - basic: Simple single-site setup
//   - production: Production-ready with HA
//   - development: Local development setup
//   - all: Generate all scenarios (default)

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

type Config struct {
	// Common
	Name      string
	Namespace string

	// Bench
	FrappeVersion    string
	ImageRepository  string
	ImageTag         string
	GunicornReplicas int
	NginxReplicas    int
	SocketioReplicas int
	StorageSize      string
	StorageClass     string
	DBMode           string
	MariaDBRef       string
	IncludeERPNext   bool

	// Site
	SiteName            string
	BenchRef            string
	Domain              string
	TLSEnabled          bool
	TLSSecretName       string
	TLSIssuer           string
	IngressClass        string
	IngressAnnotations  map[string]string
	AdminPasswordSecret string

	// MariaDB
	RootPassword         string
	MariaDBVersion       string
	HighAvailability     bool
	InnoDBBufferPoolSize string
	InnoDBLogFileSize    string
	MaxConnections       int
}

var scenarios = map[string]Config{
	"basic": {
		Name:             "demo",
		Namespace:        "frappe",
		FrappeVersion:    "v15",
		ImageRepository:  "ghcr.io/vyogotech/frappe-bench",
		ImageTag:         "v15",
		GunicornReplicas: 1,
		NginxReplicas:    1,
		SocketioReplicas: 1,
		StorageSize:      "10Gi",
		DBMode:           "shared",
		MariaDBRef:       "frappe-mariadb",
		SiteName:         "demo.local",
		BenchRef:         "demo",
		MariaDBVersion:   "11.4",
		RootPassword:     "changeme-in-production",
		MaxConnections:   100,
	},
	"production": {
		Name:                 "prod",
		Namespace:            "frappe-prod",
		FrappeVersion:        "v15",
		ImageRepository:      "ghcr.io/vyogotech/frappe-bench",
		ImageTag:             "v15",
		GunicornReplicas:     3,
		NginxReplicas:        2,
		SocketioReplicas:     2,
		StorageSize:          "100Gi",
		DBMode:               "shared",
		MariaDBRef:           "prod-mariadb",
		IncludeERPNext:       true,
		SiteName:             "erp.company.com",
		BenchRef:             "prod",
		Domain:               "erp.company.com",
		TLSEnabled:           true,
		TLSIssuer:            "letsencrypt-prod",
		IngressClass:         "nginx",
		MariaDBVersion:       "11.4",
		HighAvailability:     true,
		InnoDBBufferPoolSize: "2G",
		InnoDBLogFileSize:    "512M",
		MaxConnections:       500,
		IngressAnnotations: map[string]string{
			"cert-manager.io/cluster-issuer":              "letsencrypt-prod",
			"nginx.ingress.kubernetes.io/proxy-body-size": "50m",
		},
	},
	"development": {
		Name:             "dev",
		Namespace:        "frappe-dev",
		FrappeVersion:    "develop",
		ImageRepository:  "ghcr.io/vyogotech/frappe-bench",
		ImageTag:         "develop",
		GunicornReplicas: 1,
		NginxReplicas:    1,
		SocketioReplicas: 1,
		StorageSize:      "5Gi",
		DBMode:           "dedicated",
		SiteName:         "dev.localhost",
		BenchRef:         "dev",
		MariaDBVersion:   "11.4",
		RootPassword:     "dev-password",
		MaxConnections:   50,
	},
}

func main() {
	scenario := flag.String("scenario", "all", "Scenario to generate (basic, production, development, all)")
	outputDir := flag.String("output", "examples/generated", "Output directory")
	flag.Parse()

	templates := []string{"bench.yaml.tmpl", "site.yaml.tmpl", "mariadb.yaml.tmpl"}

	if *scenario == "all" {
		for name := range scenarios {
			if err := generateScenario(name, templates, *outputDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating %s: %v\n", name, err)
				os.Exit(1)
			}
		}
	} else {
		if _, ok := scenarios[*scenario]; !ok {
			fmt.Fprintf(os.Stderr, "Unknown scenario: %s\n", *scenario)
			os.Exit(1)
		}
		if err := generateScenario(*scenario, templates, *outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("Examples generated successfully!")
}

func generateScenario(name string, templates []string, outputDir string) error {
	config := scenarios[name]
	scenarioDir := filepath.Join(outputDir, name)

	if err := os.MkdirAll(scenarioDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	funcMap := template.FuncMap{
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" || val == 0 || val == false {
				return def
			}
			return val
		},
		"required": func(msg string, val interface{}) (interface{}, error) {
			if val == nil || val == "" {
				return nil, fmt.Errorf("%s", msg)
			}
			return val, nil
		},
	}

	for _, tmplFile := range templates {
		tmplPath := filepath.Join("examples/templates", tmplFile)
		content, err := os.ReadFile(tmplPath)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", tmplFile, err)
		}

		tmpl, err := template.New(tmplFile).Funcs(funcMap).Parse(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", tmplFile, err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, config); err != nil {
			return fmt.Errorf("failed to execute template %s: %w", tmplFile, err)
		}

		outputFile := filepath.Join(scenarioDir, tmplFile[:len(tmplFile)-5]) // remove .tmpl
		if err := os.WriteFile(outputFile, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", outputFile, err)
		}

		fmt.Printf("Generated: %s\n", outputFile)
	}

	return nil
}
