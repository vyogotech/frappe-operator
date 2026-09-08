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

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PostgresProvider is a delegating database.Provider that resolves the effective
// mode (shared vs dedicated) and engine (Percona vs StackGres) and delegates to the
// corresponding polymorphic engine provider.
type PostgresProvider struct {
	client    client.Client
	scheme    *runtime.Scheme
	shared    Provider
	percona   Provider
	stackgres Provider
}

// NewPostgresProvider constructs a polymorphic PostgresProvider
func NewPostgresProvider(client client.Client, scheme *runtime.Scheme) Provider {
	return &PostgresProvider{
		client:    client,
		scheme:    scheme,
		shared:    NewSharedPostgresProvider(client, scheme),
		percona:   NewPerconaPostgresProvider(client, scheme),
		stackgres: NewStackGresPostgresProvider(client, scheme),
	}
}

// resolvePostgresEngine returns the dedicated-mode engine for site. Explicit
// dbConfig.postgresEngine always wins. Otherwise: if a PerconaPGCluster
// already exists for this site (pre-upgrade dedicated-mode sites created
// before this field existed), grandfather it in as "percona" so upgrading the
// operator cannot silently swap a live cluster out from under a site. Only a
// genuinely new dedicated-mode site with no existing cluster defaults to the
// new "stackgres" default.
func (p *PostgresProvider) resolvePostgresEngine(ctx context.Context, site *vyogotechv1.FrappeSite) (string, error) {
	if e := site.Spec.DBConfig.PostgresEngine; e != "" {
		return e, nil
	}
	if p.client != nil {
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(PerconaPGClusterGVK)
		err := p.client.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-postgres", site.Name), Namespace: site.Namespace}, existing)
		if err == nil {
			return "percona", nil // grandfather an already-provisioned cluster
		}
		if !errors.IsNotFound(err) {
			return "", err
		}
	}
	return "stackgres", nil // new site, new default
}

// getDelegate returns the polymorphic Provider implementation appropriate for this site
func (p *PostgresProvider) getDelegate(ctx context.Context, site *vyogotechv1.FrappeSite) (Provider, error) {
	mode := site.Spec.DBConfig.Mode
	if mode == "" || mode == "shared" {
		if p.shared != nil {
			return p.shared, nil
		}
		return NewSharedPostgresProvider(p.client, p.scheme), nil
	}

	if mode == "dedicated" {
		engine, err := p.resolvePostgresEngine(ctx, site)
		if err != nil {
			return nil, err
		}
		switch engine {
		case "percona":
			if p.percona != nil {
				return p.percona, nil
			}
			return NewPerconaPostgresProvider(p.client, p.scheme), nil
		case "stackgres":
			if p.stackgres != nil {
				return p.stackgres, nil
			}
			return NewStackGresPostgresProvider(p.client, p.scheme), nil
		default:
			return nil, fmt.Errorf("unsupported postgres engine: %s", engine)
		}
	}

	return nil, fmt.Errorf("unsupported database mode: %s", mode)
}

func (p *PostgresProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseInfo, error) {
	d, err := p.getDelegate(ctx, site)
	if err != nil {
		return nil, err
	}
	return d.EnsureDatabase(ctx, site)
}

func (p *PostgresProvider) IsReady(ctx context.Context, site *vyogotechv1.FrappeSite) (bool, error) {
	d, err := p.getDelegate(ctx, site)
	if err != nil {
		return false, err
	}
	return d.IsReady(ctx, site)
}

func (p *PostgresProvider) GetCredentials(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseCredentials, error) {
	d, err := p.getDelegate(ctx, site)
	if err != nil {
		return nil, err
	}
	return d.GetCredentials(ctx, site)
}

func (p *PostgresProvider) Cleanup(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	d, err := p.getDelegate(ctx, site)
	if err != nil {
		return err
	}
	return d.Cleanup(ctx, site)
}

// Forwarders for backward compatibility with unit tests and external callers
func (p *PostgresProvider) hashString(s string) string {
	return hashString(s)
}

func (p *PostgresProvider) generateDBName(site *vyogotechv1.FrappeSite) string {
	return generateDBName(site)
}

func (p *PostgresProvider) generateDBUser(site *vyogotechv1.FrappeSite) string {
	return generateDBUser(site)
}

func (p *PostgresProvider) generatePGUserName(site *vyogotechv1.FrappeSite) string {
	return generatePGUserName(site)
}

func (p *PostgresProvider) getSharedHostPort(ctx context.Context, site *vyogotechv1.FrappeSite) (string, string, error) {
	return getSharedHostPort(site)
}
