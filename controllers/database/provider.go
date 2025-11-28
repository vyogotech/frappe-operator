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

// Provider defines the interface for database provisioning
type Provider interface {
	// EnsureDatabase ensures database and user exist for the site
	EnsureDatabase(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseInfo, error)

	// IsReady checks if database is ready
	IsReady(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (bool, error)

	// GetCredentials retrieves database credentials
	GetCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseCredentials, error)

	// Cleanup removes database resources (on site deletion)
	Cleanup(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error
}

// DatabaseInfo contains database connection information
type DatabaseInfo struct {
	Host     string
	Port     string
	Name     string
	Provider string
}

// DatabaseCredentials contains database authentication information
type DatabaseCredentials struct {
	Username   string
	Password   string
	SecretName string
}

// NewProvider returns the appropriate provider based on config
func NewProvider(providerType string, client client.Client, scheme *runtime.Scheme) (Provider, error) {
	if providerType == "" {
		providerType = "mariadb" // Default provider
	}

	switch providerType {
	case "mariadb":
		return NewMariaDBProvider(client, scheme), nil
	case "postgres":
		return nil, fmt.Errorf("PostgreSQL provider not yet implemented - planned for v1.1.0")
	case "sqlite":
		return NewSQLiteProvider(client, scheme), nil
	default:
		return nil, fmt.Errorf("unsupported database provider: %s (supported: mariadb, postgres, sqlite)", providerType)
	}
}
