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

package database

import (
	"context"
	"fmt"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SQLiteProvider implements database provisioning for SQLite (Frappe v16+)
// SQLite uses PVC for database file storage - no external database server needed
type SQLiteProvider struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewSQLiteProvider creates a new SQLite provider
func NewSQLiteProvider(client client.Client, scheme *runtime.Scheme) Provider {
	return &SQLiteProvider{
		client: client,
		scheme: scheme,
	}
}

// EnsureDatabase for SQLite - database file is created by Frappe itself
func (p *SQLiteProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseInfo, error) {
	// SQLite database is file-based, created automatically by bench new-site
	// No external provisioning needed
	return &DatabaseInfo{
		Host:     "",     // Not applicable for SQLite
		Port:     "",     // Not applicable for SQLite
		Name:     "site", // SQLite uses file-based storage
		Provider: "sqlite",
	}, nil
}

// IsReady for SQLite - always ready (no external dependencies)
func (p *SQLiteProvider) IsReady(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (bool, error) {
	// SQLite has no external dependencies, so it's always ready
	return true, nil
}

// GetCredentials for SQLite - no credentials needed
func (p *SQLiteProvider) GetCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseCredentials, error) {
	// SQLite doesn't require credentials
	return &DatabaseCredentials{
		Username:   "", // Not applicable
		Password:   "", // Not applicable
		SecretName: "", // Not applicable
	}, nil
}

// Cleanup for SQLite - database files are in PVC, cleaned up with site
func (p *SQLiteProvider) Cleanup(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	// SQLite database files are stored in the site's PVC
	// They will be cleaned up automatically when the site is deleted
	return nil
}

// Note: SQLite support requires Frappe v16 or later
// This provider is a placeholder for future implementation when Frappe v16 is widely adopted
// Current implementation (v1.0.0) focuses on MariaDB for production use cases
func init() {
	fmt.Println("SQLite provider loaded - requires Frappe v16+ (experimental)")
}

