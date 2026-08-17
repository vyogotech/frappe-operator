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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"strings"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	PerconaPGClusterGVK = schema.GroupVersionKind{
		Group:   "pgv2.percona.com",
		Version: "v2",
		Kind:    "PerconaPGCluster",
	}
)

// Percona PostgreSQL Operator component images for dedicated-mode clusters.
// Overridable via env so operators can pin/mirror images without a rebuild.
// Defaults track Percona PostgreSQL Operator v2.3.1 (PostgreSQL 16).
const (
	defaultPerconaPGVersion       = 16
	defaultPerconaPostgresImage   = "percona/percona-postgresql-operator:2.3.1-ppg16-postgres"
	defaultPerconaPGBouncerImage  = "percona/percona-postgresql-operator:2.3.1-ppg16-pgbouncer"
	defaultPerconaPGBackRestImage = "percona/percona-postgresql-operator:2.3.1-ppg16-pgbackrest"
	defaultDedicatedStorageSize   = "2Gi"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type PostgresProvider struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewPostgresProvider(client client.Client, scheme *runtime.Scheme) Provider {
	return &PostgresProvider{
		client: client,
		scheme: scheme,
	}
}

func (p *PostgresProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseInfo, error) {
	logger := log.FromContext(ctx)
	mode := site.Spec.DBConfig.Mode
	if mode == "" {
		mode = "shared"
	}

	dbName := p.generateDBName(site)

	var host, port string
	var err error

	if mode == "shared" {
		// Shared mode provisions via psql, where underscore-form identifiers are valid.
		host, port, err = p.ensureSharedPostgres(ctx, site, dbName, p.generateDBUser(site))
	} else if mode == "dedicated" {
		// Dedicated mode goes through the Percona CRD, which requires a DNS-label user.
		host, port, err = p.ensureDedicatedPostgres(ctx, site, dbName, p.generatePGUserName(site))
	} else {
		return nil, fmt.Errorf("unsupported database mode: %s", mode)
	}

	if err != nil {
		return nil, err
	}

	logger.Info("Database ensured", "mode", mode, "host", host, "dbName", dbName)

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: "postgres",
	}, nil
}

func (p *PostgresProvider) IsReady(ctx context.Context, site *vyogotechv1.FrappeSite) (bool, error) {
	logger := log.FromContext(ctx)
	mode := site.Spec.DBConfig.Mode
	if mode == "" {
		mode = "shared"
	}

	if mode == "shared" {
		// Check if provision job succeeded
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
	} else if mode == "dedicated" {
		cluster := &unstructured.Unstructured{}
		cluster.SetGroupVersionKind(PerconaPGClusterGVK)
		err := p.client.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-postgres", site.Name), Namespace: site.Namespace}, cluster)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		status, found, err := unstructured.NestedString(cluster.Object, "status", "state")
		if err != nil || !found {
			return false, nil
		}
		if status == "ready" {
			logger.Info("Dedicated Postgres cluster is ready")
			return true, nil
		}
		return false, nil
	}

	return false, fmt.Errorf("unknown mode: %s", mode)
}

func (p *PostgresProvider) GetCredentials(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseCredentials, error) {
	secretName := fmt.Sprintf("%s-db-password", site.Name)
	mode := site.Spec.DBConfig.Mode
	if mode == "" {
		mode = "shared"
	}

	if mode == "dedicated" {
		// Percona Operator generates <cluster>-pguser-<username> secret
		clusterName := fmt.Sprintf("%s-postgres", site.Name)
		dbUser := p.generatePGUserName(site)
		secretName = fmt.Sprintf("%s-pguser-%s", clusterName, dbUser)
	}

	secret := &corev1.Secret{}
	err := p.client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get password secret %s: %w", secretName, err)
	}

	passwordKey := "password"
	if mode == "dedicated" {
		// Percona operator uses standard keys
		passwordKey = "password"
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return nil, fmt.Errorf("password key '%s' not found in secret %s", passwordKey, secretName)
	}

	dbUser := p.generateDBUser(site)
	if mode == "dedicated" {
		dbUser = p.generatePGUserName(site)
		if userBytes, ok := secret.Data["user"]; ok {
			dbUser = string(userBytes)
		}
	}

	return &DatabaseCredentials{
		Username:   dbUser,
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

func (p *PostgresProvider) Cleanup(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	if site.Spec.DeletionPolicy != "Delete" {
		return nil
	}

	mode := site.Spec.DBConfig.Mode
	if mode == "" {
		mode = "shared"
	}

	if mode == "shared" {
		// Create cleanup job
		dbName := p.generateDBName(site)
		dbUser := p.generateDBUser(site)
		host, port, _ := p.getSharedHostPort(ctx, site)

		jobName := fmt.Sprintf("%s-db-delete", site.Name)

		// Setup provisioner credentials mount
		provisionerSecretName := "frappe-postgres-provisioner"
		if site.Spec.DBConfig.PostgresRef != nil && site.Spec.DBConfig.PostgresRef.Name != "" {
			provisionerSecretName = site.Spec.DBConfig.PostgresRef.Name
		}

		script := fmt.Sprintf(`
export PGPASSWORD="$(cat /tmp/creds/password)"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "DROP DATABASE IF EXISTS \"%s\";"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "DROP ROLE IF EXISTS \"%s\";"
`, host, port, dbName, host, port, dbUser)

		container := resources.NewContainerBuilder("pg-delete", "postgres:15-alpine").
			WithCommand("sh", "-c").
			WithArgs(script).
			WithVolumeMount("creds", "/tmp/creds").
			Build()

		job := resources.NewJobBuilder(jobName, site.Namespace).
			WithLabels(map[string]string{"app": "frappe", "site": site.Name}).
			WithContainer(container).
			WithSecretVolume("creds", provisionerSecretName, resources.Int32Ptr(0444)).
			MustBuild()

		err := p.client.Create(ctx, job)
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}

		// Manually delete the password secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-db-password", site.Name),
				Namespace: site.Namespace,
			},
		}
		_ = p.client.Delete(ctx, secret)

		// Delete the provision job if it exists
		provJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-db-provision", site.Name),
				Namespace: site.Namespace,
			},
		}
		_ = p.client.Delete(ctx, provJob, client.PropagationPolicy(metav1.DeletePropagationBackground))

	} else if mode == "dedicated" {
		cluster := &unstructured.Unstructured{}
		cluster.SetGroupVersionKind(PerconaPGClusterGVK)
		cluster.SetName(fmt.Sprintf("%s-postgres", site.Name))
		cluster.SetNamespace(site.Namespace)
		if err := p.client.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete dedicated postgres cluster: %w", err)
		}
	}

	return nil
}

// ensureSharedPostgres generates credentials and spawns a Job to execute psql
func (p *PostgresProvider) ensureSharedPostgres(ctx context.Context, site *vyogotechv1.FrappeSite, dbName, dbUser string) (string, string, error) {
	// 1. Create or get site credentials secret
	passwordSecretName := fmt.Sprintf("%s-db-password", site.Name)
	passwordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      passwordSecretName,
			Namespace: site.Namespace,
		},
	}

	err := p.client.Get(ctx, types.NamespacedName{Name: passwordSecretName, Namespace: site.Namespace}, passwordSecret)
	var sitePassword string
	if errors.IsNotFound(err) {
		sitePassword = p.generatePassword(16)
		passwordSecret.StringData = map[string]string{
			"password": sitePassword,
		}
		// NO OwnerReference so Retain works
		if err := p.client.Create(ctx, passwordSecret); err != nil {
			return "", "", err
		}
	} else if err == nil {
		sitePassword = string(passwordSecret.Data["password"])
		if len(passwordSecret.GetOwnerReferences()) > 0 {
			passwordSecret.SetOwnerReferences(nil)
			_ = p.client.Update(ctx, passwordSecret)
		}
	} else {
		return "", "", err
	}

	// 2. Resolve Postgres Host
	host, port, err := p.getSharedHostPort(ctx, site)
	if err != nil {
		return "", "", err
	}

	// 3. Create Provisioning Job
	jobName := fmt.Sprintf("%s-db-provision", site.Name)
	job := &batchv1.Job{}
	err = p.client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if errors.IsNotFound(err) {
		provisionerSecretName := "frappe-postgres-provisioner"
		if site.Spec.DBConfig.PostgresRef != nil && site.Spec.DBConfig.PostgresRef.Name != "" {
			provisionerSecretName = site.Spec.DBConfig.PostgresRef.Name
		}

		script := fmt.Sprintf(`
export PGPASSWORD="$(cat /tmp/creds/password)"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "CREATE ROLE \"%s\" WITH LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE;"
psql -h "%s" -p "%s" -U "$(cat /tmp/creds/user)" -d postgres -c "CREATE DATABASE \"%s\" OWNER \"%s\";"
`, host, port, dbUser, sitePassword, host, port, dbName, dbUser)

		container := resources.NewContainerBuilder("pg-provision", "postgres:15-alpine").
			WithCommand("sh", "-c").
			WithArgs(script).
			WithVolumeMount("creds", "/tmp/creds").
			Build()

		job = resources.NewJobBuilder(jobName, site.Namespace).
			WithLabels(map[string]string{"app": "frappe", "site": site.Name}).
			WithContainer(container).
			WithSecretVolume("creds", provisionerSecretName, resources.Int32Ptr(0444)).
			MustBuild()

		if err := p.client.Create(ctx, job); err != nil {
			return "", "", err
		}
	}

	return host, port, nil
}

// ensureDedicatedPostgres creates a PerconaPGCluster CR
func (p *PostgresProvider) ensureDedicatedPostgres(ctx context.Context, site *vyogotechv1.FrappeSite, dbName, dbUser string) (string, string, error) {
	clusterName := fmt.Sprintf("%s-postgres", site.Name)

	// Storage size for the data volume and the pgBackRest repo volume.
	storageSize := defaultDedicatedStorageSize
	if site.Spec.DBConfig.StorageSize != nil {
		storageSize = site.Spec.DBConfig.StorageSize.String()
	}

	pvcSpec := func(size string) map[string]interface{} {
		return map[string]interface{}{
			"accessModes": []interface{}{"ReadWriteOnce"},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"storage": size,
				},
			},
		}
	}

	// A complete, CRD-valid PerconaPGCluster: the schema requires postgresVersion,
	// instances[].dataVolumeClaimSpec and backups.pgbackrest.repos. We provision a
	// single instance with a PVC-backed pgBackRest repo (repo1) so backups work
	// out of the box, and a pgBouncer proxy that Frappe connects through.
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "pgv2.percona.com/v2",
			"kind":       "PerconaPGCluster",
			"metadata": map[string]interface{}{
				"name":      clusterName,
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"image":           envOr("FRAPPE_PERCONA_POSTGRES_IMAGE", defaultPerconaPostgresImage),
				"postgresVersion": int64(defaultPerconaPGVersion),
				"instances": []interface{}{
					map[string]interface{}{
						"name":                "instance1",
						"replicas":            int64(1),
						"dataVolumeClaimSpec": pvcSpec(storageSize),
					},
				},
				"users": []interface{}{
					map[string]interface{}{
						"name": dbUser,
						"databases": []interface{}{
							dbName,
						},
					},
				},
				"proxy": map[string]interface{}{
					"pgBouncer": map[string]interface{}{
						"image":    envOr("FRAPPE_PERCONA_PGBOUNCER_IMAGE", defaultPerconaPGBouncerImage),
						"replicas": int64(1),
					},
				},
				"backups": map[string]interface{}{
					"pgbackrest": map[string]interface{}{
						"image": envOr("FRAPPE_PERCONA_PGBACKREST_IMAGE", defaultPerconaPGBackRestImage),
						"repos": []interface{}{
							map[string]interface{}{
								"name": "repo1",
								"volume": map[string]interface{}{
									"volumeClaimSpec": pvcSpec(storageSize),
								},
							},
						},
					},
				},
			},
		},
	}

	// No OwnerReference for Retain policy
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(PerconaPGClusterGVK)
	err := p.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: site.Namespace}, existing)

	if errors.IsNotFound(err) {
		if err := p.client.Create(ctx, cluster); err != nil {
			return "", "", err
		}
	} else if err == nil {
		if len(existing.GetOwnerReferences()) > 0 {
			existing.SetOwnerReferences(nil)
			_ = p.client.Update(ctx, existing)
		}
	} else {
		return "", "", err
	}

	host := fmt.Sprintf("%s-pgbouncer.%s.svc.cluster.local", clusterName, site.Namespace)
	port := "5432"

	return host, port, nil
}

func (p *PostgresProvider) getSharedHostPort(ctx context.Context, site *vyogotechv1.FrappeSite) (string, string, error) {
	host := "frappe-postgres-pgbouncer" // Default for Percona shared cluster
	port := "5432"

	if site.Spec.DBConfig.PostgresRef != nil && site.Spec.DBConfig.PostgresRef.Name != "" {
		host = site.Spec.DBConfig.PostgresRef.Name + "-pgbouncer"
	}

	ns := site.Namespace
	if site.Spec.DBConfig.PostgresRef != nil && site.Spec.DBConfig.PostgresRef.Namespace != "" {
		ns = site.Spec.DBConfig.PostgresRef.Namespace
	}

	return fmt.Sprintf("%s.%s.svc.cluster.local", host, ns), port, nil
}

func (p *PostgresProvider) generateDBName(site *vyogotechv1.FrappeSite) string {
	hash := p.hashString(site.Namespace + "/" + site.Name)[:8]
	safeName := p.sanitizeName(site.Spec.SiteName)
	dbName := fmt.Sprintf("_%s_%s", hash, safeName)
	if len(dbName) > 63 {
		dbName = dbName[:63]
	}
	return dbName
}

func (p *PostgresProvider) generateDBUser(site *vyogotechv1.FrappeSite) string {
	return p.generateDBName(site)
}

// generatePGUserName returns a PostgreSQL role name for dedicated (Percona) mode.
// The Percona CRD constrains spec.users[].name to a DNS-label
// (^[a-z0-9]([-a-z0-9]*[a-z0-9])?$) because it derives the credential Secret name
// (<cluster>-pguser-<name>) from it — so the underscore-prefixed generateDBName form
// is invalid here. We use a stable, lowercase, label-safe name derived from the site.
func (p *PostgresProvider) generatePGUserName(site *vyogotechv1.FrappeSite) string {
	return "u" + p.hashString(site.Namespace + "/" + site.Name)[:8]
}

func (p *PostgresProvider) hashString(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum32())
}

func (p *PostgresProvider) sanitizeName(name string) string {
	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	safe := reg.ReplaceAllString(name, "_")
	safe = strings.Trim(safe, "_")
	return safe
}

func (p *PostgresProvider) generatePassword(length int) string {
	b := make([]byte, length/2)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never fails in practice; fall back deterministically-unique
		// enough to avoid an empty password if it ever does.
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", length)))
	}
	return hex.EncodeToString(b)
}
