/*
Copyright 2024 Vyogo Technologies.

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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExternalProvider implements database provisioning for externally managed databases
type ExternalProvider struct {
	client client.Client
}

// NewExternalProvider creates a new external database provider
func NewExternalProvider(client client.Client) Provider {
	return &ExternalProvider{
		client: client,
	}
}

// EnsureDatabase retrieves connection info from the site spec or secret
func (p *ExternalProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseInfo, error) {
	host := site.Spec.DBConfig.Host
	port := site.Spec.DBConfig.Port
	dbName := site.Spec.SiteName // Default to site name

	dbType := "mariadb" // Default
	if site.Spec.DBConfig.ConnectionSecretRef != nil {
		secret := &corev1.Secret{}
		err := p.client.Get(ctx, types.NamespacedName{
			Name:      site.Spec.DBConfig.ConnectionSecretRef.Name,
			Namespace: site.Namespace,
		}, secret)
		if err == nil {
			if h, ok := secret.Data["host"]; ok {
				host = string(h)
			}
			if pt, ok := secret.Data["port"]; ok {
				port = string(pt)
			}
			if dn, ok := secret.Data["database"]; ok {
				dbName = string(dn)
			}
			if configType, ok := secret.Data["type"]; ok {
				dbType = string(configType)
			}
		}
	}

	if host == "" {
		return nil, fmt.Errorf("database host is required (either in spec or secret)")
	}

	if port == "" {
		port = "3306" // Default for MariaDB/MySQL
	}

	if dbType == "mariadb" && site.Spec.DBConfig.Provider != "external" && site.Spec.DBConfig.Provider != "" {
		dbType = site.Spec.DBConfig.Provider
	}

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: dbType,
	}, nil
}

// IsReady checks if the connection secret exists
func (p *ExternalProvider) IsReady(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (bool, error) {
	if site.Spec.DBConfig.ConnectionSecretRef == nil {
		// If no secret ref, we assume it's "ready" if host is provided in spec,
		// but typically we need credentials.
		if site.Spec.DBConfig.Host != "" {
			return true, nil
		}
		return false, fmt.Errorf("connectionSecretRef or host is required for external database provider")
	}

	secret := &corev1.Secret{}
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      site.Spec.DBConfig.ConnectionSecretRef.Name,
		Namespace: site.Namespace,
	}, secret)
	if err != nil {
		return false, fmt.Errorf("failed to get database secret: %w", err)
	}

	return true, nil
}

// GetCredentials retrieves credentials from the secret
func (p *ExternalProvider) GetCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseCredentials, error) {
	if site.Spec.DBConfig.ConnectionSecretRef == nil {
		return nil, fmt.Errorf("connectionSecretRef is required for external database provider to retrieve credentials")
	}

	secret := &corev1.Secret{}
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      site.Spec.DBConfig.ConnectionSecretRef.Name,
		Namespace: site.Namespace,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get database secret: %w", err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return nil, fmt.Errorf("username not found in database secret '%s'", secret.Name)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("password not found in database secret '%s'", secret.Name)
	}

	return &DatabaseCredentials{
		Username:   string(username),
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

// Cleanup does nothing for external databases
func (p *ExternalProvider) Cleanup(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	return nil
}
