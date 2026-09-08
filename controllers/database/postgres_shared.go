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
	"github.com/vyogotech/frappe-operator/pkg/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SharedPostgresProvider implements database.Provider for shared PostgreSQL instances
type SharedPostgresProvider struct {
	config vyogotechv1.DatabaseConfig
	client client.Client
	scheme *runtime.Scheme
}

func NewSharedPostgresProvider(config vyogotechv1.DatabaseConfig, client client.Client, scheme *runtime.Scheme) *SharedPostgresProvider {
	return &SharedPostgresProvider{
		config: config,
		client: client,
		scheme: scheme,
	}
}

func (p *SharedPostgresProvider) getDBConfig(site *vyogotechv1.FrappeSite) vyogotechv1.DatabaseConfig {
	cfg := p.config
	if site == nil {
		return cfg
	}
	if cfg.Host == "" {
		cfg.Host = site.Spec.DBConfig.Host
	}
	if cfg.Port == "" {
		cfg.Port = site.Spec.DBConfig.Port
	}
	if cfg.PostgresRef == nil {
		cfg.PostgresRef = site.Spec.DBConfig.PostgresRef
	}
	return cfg
}

func (p *SharedPostgresProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseInfo, error) {
	logger := log.FromContext(ctx)
	dbName := generateDBName(site)
	dbUser := generateDBUser(site)

	// 1. Create or get site credentials secret
	sitePassword, err := ensurePasswordSecret(ctx, p.client, site)
	if err != nil {
		return nil, err
	}

	// 2. Resolve Postgres Host
	cfg := p.getDBConfig(site)
	host, port, err := getSharedHostPortWithConfig(cfg, site)
	if err != nil {
		return nil, err
	}

	// 3. Create Provisioning Job
	jobName := fmt.Sprintf("%s-db-provision", site.Name)
	job := &batchv1.Job{}
	err = p.client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if errors.IsNotFound(err) {
		provisionerSecretName := "frappe-postgres-provisioner"
		if cfg.PostgresRef != nil && cfg.PostgresRef.Name != "" {
			provisionerSecretName = cfg.PostgresRef.Name
		}

		script := fmt.Sprintf(`
export PGPASSWORD="$(cat /tmp/creds/password)"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "CREATE ROLE \"%s\" WITH LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE;"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "CREATE DATABASE \"%s\" OWNER \"%s\";"
`, host, port, dbUser, sitePassword, host, port, dbName, dbUser)

		container := resources.NewContainerBuilder("pg-provision", "postgres:15-alpine").
			WithCommand("sh", "-c").
			WithResources(vyogotechv1.ResolveJobResources(nil, vyogotechv1.JobKindMaintenance)).
			WithArgs(script).
			WithVolumeMount("creds", "/tmp/creds").
			Build()

		job = resources.NewJobBuilder(jobName, site.Namespace).
			WithLabels(map[string]string{"app": "frappe", "site": site.Name}).
			WithContainer(container).
			WithSecretVolume("creds", provisionerSecretName, resources.Int32Ptr(0444)).
			MustBuild()

		if err := p.client.Create(ctx, job); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	logger.Info("Shared database ensured", "host", host, "dbName", dbName)

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: "postgres",
	}, nil
}

func (p *SharedPostgresProvider) IsReady(ctx context.Context, site *vyogotechv1.FrappeSite) (bool, error) {
	logger := log.FromContext(ctx)
	jobName := fmt.Sprintf("%s-db-provision", site.Name)
	job := &batchv1.Job{}
	err := p.client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if job.Status.Succeeded > 0 {
		logger.Info("Database provision job succeeded")
		return true, nil
	}
	if job.Status.Failed > 0 {
		logger.Error(nil, "Database provision job failed")
		return false, fmt.Errorf("database provision job failed")
	}
	return false, nil
}

func (p *SharedPostgresProvider) GetCredentials(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseCredentials, error) {
	secretName := fmt.Sprintf("%s-db-password", site.Name)
	secret := &corev1.Secret{}
	err := p.client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get password secret %s: %w", secretName, err)
	}

	password, ok := secret.Data["password"]
	if !ok {
		if strPw, found := secret.StringData["password"]; found {
			password = []byte(strPw)
		} else {
			return nil, fmt.Errorf("password key 'password' not found in secret %s", secretName)
		}
	}

	return &DatabaseCredentials{
		Username:   generateDBUser(site),
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

func (p *SharedPostgresProvider) Cleanup(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	if site.Spec.DeletionPolicy != "Delete" {
		return nil
	}

	dbName := generateDBName(site)
	dbUser := generateDBUser(site)
	cfg := p.getDBConfig(site)
	host, port, err := getSharedHostPortWithConfig(cfg, site)
	if err != nil {
		return err
	}

	jobName := fmt.Sprintf("%s-db-delete", site.Name)
	provisionerSecretName := "frappe-postgres-provisioner"
	if cfg.PostgresRef != nil && cfg.PostgresRef.Name != "" {
		provisionerSecretName = cfg.PostgresRef.Name
	}

	script := fmt.Sprintf(`
export PGPASSWORD="$(cat /tmp/creds/password)"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "DROP DATABASE IF EXISTS \"%s\";"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "DROP ROLE IF EXISTS \"%s\";"
`, host, port, dbName, host, port, dbUser)

	container := resources.NewContainerBuilder("pg-delete", "postgres:15-alpine").
		WithCommand("sh", "-c").
		WithResources(vyogotechv1.ResolveJobResources(nil, vyogotechv1.JobKindMaintenance)).
		WithArgs(script).
		WithVolumeMount("creds", "/tmp/creds").
		Build()

	job := resources.NewJobBuilder(jobName, site.Namespace).
		WithLabels(map[string]string{"app": "frappe", "site": site.Name}).
		WithContainer(container).
		WithSecretVolume("creds", provisionerSecretName, resources.Int32Ptr(0444)).
		MustBuild()

	err = p.client.Create(ctx, job)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-password", site.Name),
			Namespace: site.Namespace,
		},
	}
	_ = p.client.Delete(ctx, secret)

	provJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-provision", site.Name),
			Namespace: site.Namespace,
		},
	}
	_ = p.client.Delete(ctx, provJob, client.PropagationPolicy(metav1.DeletePropagationBackground))

	return nil
}
