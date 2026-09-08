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
	SGClusterGVK = schema.GroupVersionKind{
		Group:   "stackgres.io",
		Version: "v1",
		Kind:    "SGCluster",
	}
	SGInstanceProfileGVK = schema.GroupVersionKind{
		Group:   "stackgres.io",
		Version: "v1",
		Kind:    "SGInstanceProfile",
	}
	SGPostgresConfigGVK = schema.GroupVersionKind{
		Group:   "stackgres.io",
		Version: "v1",
		Kind:    "SGPostgresConfig",
	}
	SGPoolingConfigGVK = schema.GroupVersionKind{
		Group:   "stackgres.io",
		Version: "v1",
		Kind:    "SGPoolingConfig",
	}
)

const (
	defaultStackGresPostgresVersion = "16"
	defaultStackGresProfileName     = "frappe-postgres-dedicated-default"
	defaultStackGresConfigName      = "frappe-postgres-dedicated-defaults"
	defaultStackGresPoolingName     = "frappe-postgres-dedicated-pooling"
)

// StackGresPostgresProvider implements database.Provider for dedicated StackGres clusters
type StackGresPostgresProvider struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewStackGresPostgresProvider(client client.Client, scheme *runtime.Scheme) *StackGresPostgresProvider {
	return &StackGresPostgresProvider{
		client: client,
		scheme: scheme,
	}
}

func (p *StackGresPostgresProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseInfo, error) {
	logger := log.FromContext(ctx)
	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	dbName := generateDBName(site)

	// 1. Ensure password secret exists for the site
	if _, err := ensurePasswordSecret(ctx, p.client, site); err != nil {
		return nil, err
	}

	// 2. Ensure supporting StackGres configurations (PostgresConfig, PoolingConfig, InstanceProfile)
	profileName, err := p.ensureConfigurations(ctx, site)
	if err != nil {
		return nil, err
	}

	// 3. Storage size
	storageSize := defaultDedicatedStorageSize
	if site.Spec.DBConfig.StorageSize != nil {
		storageSize = site.Spec.DBConfig.StorageSize.String()
	}

	// 4. Create or update SGCluster CR
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "stackgres.io/v1",
			"kind":       "SGCluster",
			"metadata": map[string]interface{}{
				"name":      clusterName,
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"postgres": map[string]interface{}{
					"version": defaultStackGresPostgresVersion,
				},
				"instances": int64(1),
				"sgInstanceProfile": profileName,
				"configurations": map[string]interface{}{
					"sgPostgresConfig": defaultStackGresConfigName,
					"sgPoolingConfig":  defaultStackGresPoolingName,
				},
				"pods": map[string]interface{}{
					"persistentVolume": map[string]interface{}{
						"size": storageSize,
					},
				},
			},
		},
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(SGClusterGVK)
	err = p.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: site.Namespace}, existing)
	if errors.IsNotFound(err) {
		if err := p.client.Create(ctx, cluster); err != nil {
			return nil, fmt.Errorf("failed to create SGCluster: %w", err)
		}
		logger.Info("Created dedicated SGCluster", "name", clusterName)
	} else if err != nil {
		return nil, fmt.Errorf("failed to get SGCluster: %w", err)
	}

	port := "5432"
	if site.Spec.DBConfig.Port != "" {
		port = site.Spec.DBConfig.Port
	}
	host := fmt.Sprintf("%s.%s.svc.cluster.local", clusterName, site.Namespace)
	if site.Spec.DBConfig.Host != "" {
		host = site.Spec.DBConfig.Host
	}

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: "postgres",
	}, nil
}

func (p *StackGresPostgresProvider) ensureConfigurations(ctx context.Context, site *vyogotechv1.FrappeSite) (string, error) {
	// Shared postgres configuration
	pgConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "stackgres.io/v1",
			"kind":       "SGPostgresConfig",
			"metadata": map[string]interface{}{
				"name":      defaultStackGresConfigName,
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"postgresVersion": defaultStackGresPostgresVersion,
				"postgresql.conf": map[string]interface{}{
					"max_connections": "300",
					"shared_buffers":  "1GB",
					"work_mem":        "8MB",
				},
			},
		},
	}
	if err := createIfAbsent(ctx, p.client, pgConfig); err != nil {
		return "", err
	}

	// Shared pooling configuration
	poolingConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "stackgres.io/v1",
			"kind":       "SGPoolingConfig",
			"metadata": map[string]interface{}{
				"name":      defaultStackGresPoolingName,
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"pgBouncer": map[string]interface{}{
					"pgbouncer.ini": map[string]interface{}{
						"pgbouncer": map[string]interface{}{
							"pool_mode":         "transaction",
							"max_client_conn":   "1000",
							"default_pool_size": "50",
						},
					},
				},
			},
		},
	}
	if err := createIfAbsent(ctx, p.client, poolingConfig); err != nil {
		return "", err
	}

	// Sizing profile: custom if resources are set, otherwise namespace-shared default
	profileName := defaultStackGresProfileName
	cpu := "2"
	memory := "4Gi"
	if site.Spec.DBConfig.Resources != nil {
		profileName = fmt.Sprintf("%s-postgres-profile", site.Name)
		if cpuQty, ok := site.Spec.DBConfig.Resources.Requests[corev1.ResourceCPU]; ok {
			cpu = cpuQty.String()
		} else if cpuQty, ok := site.Spec.DBConfig.Resources.Limits[corev1.ResourceCPU]; ok {
			cpu = cpuQty.String()
		}
		if memQty, ok := site.Spec.DBConfig.Resources.Requests[corev1.ResourceMemory]; ok {
			memory = memQty.String()
		} else if memQty, ok := site.Spec.DBConfig.Resources.Limits[corev1.ResourceMemory]; ok {
			memory = memQty.String()
		}
	}

	profile := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "stackgres.io/v1",
			"kind":       "SGInstanceProfile",
			"metadata": map[string]interface{}{
				"name":      profileName,
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"cpu":    cpu,
				"memory": memory,
			},
		},
	}
	if err := createIfAbsent(ctx, p.client, profile); err != nil {
		return "", err
	}

	return profileName, nil
}

func (p *StackGresPostgresProvider) IsReady(ctx context.Context, site *vyogotechv1.FrappeSite) (bool, error) {
	logger := log.FromContext(ctx)
	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	err := p.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: site.Namespace}, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if !p.isClusterReady(cluster) {
		return false, nil
	}

	// 1. Run database provisioning Job connecting as StackGres superuser
	provisioned, err := p.ensureProvisioned(ctx, site)
	if err != nil {
		return false, err
	}
	if !provisioned {
		logger.Info("Dedicated StackGres cluster ready; waiting on database provision job")
		return false, nil
	}

	// 2. Run configure Job (schema ownership and search_path fix)
	host := fmt.Sprintf("%s.%s.svc.cluster.local", clusterName, site.Namespace)
	if site.Spec.DBConfig.Host != "" {
		host = site.Spec.DBConfig.Host
	}
	superSecretName := clusterName
	configured, err := ensureDedicatedConfigured(ctx, p.client, site, host, superSecretName)
	if err != nil {
		return false, err
	}
	if !configured {
		logger.Info("Dedicated StackGres database provisioned; waiting on schema configure job")
		return false, nil
	}

	logger.Info("Dedicated StackGres cluster is ready and configured")
	return true, nil
}

func (p *StackGresPostgresProvider) isClusterReady(cluster *unstructured.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(cluster.Object, "status", "conditions")
	if err == nil && found {
		for _, c := range conditions {
			condMap, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if condMap["type"] == "Ready" && condMap["status"] == "True" {
				return true
			}
		}
	}
	phase, _, _ := unstructured.NestedString(cluster.Object, "status", "phase")
	if strings.EqualFold(phase, "Running") || strings.EqualFold(phase, "Ready") {
		return true
	}
	state, _, _ := unstructured.NestedString(cluster.Object, "status", "state")
	if strings.EqualFold(state, "Running") || strings.EqualFold(state, "Ready") {
		return true
	}
	return false
}

func (p *StackGresPostgresProvider) ensureProvisioned(ctx context.Context, site *vyogotechv1.FrappeSite) (bool, error) {
	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	dbName := generateDBName(site)
	dbUser := generatePGUserName(site)
	superSecretName := clusterName

	// Superuser secret generated by StackGres must exist
	superSecret := &corev1.Secret{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: superSecretName, Namespace: site.Namespace}, superSecret); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	sitePassword, err := ensurePasswordSecret(ctx, p.client, site)
	if err != nil {
		return false, err
	}

	jobName := fmt.Sprintf("%s-db-provision", site.Name)
	job := &batchv1.Job{}
	err = p.client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err == nil {
		if job.Status.Succeeded > 0 {
			return true, nil
		}
		if job.Status.Failed > 0 {
			return false, fmt.Errorf("dedicated StackGres database provision job failed")
		}
		return false, nil
	}
	if !errors.IsNotFound(err) {
		return false, err
	}

	host := fmt.Sprintf("%s.%s.svc.cluster.local", clusterName, site.Namespace)
	port := "5432"
	if site.Spec.DBConfig.Port != "" {
		port = site.Spec.DBConfig.Port
	}
	if site.Spec.DBConfig.Host != "" {
		host = site.Spec.DBConfig.Host
	}

	script := fmt.Sprintf(`set -e
if [ -f /tmp/creds/password ]; then
  export PGPASSWORD="$(cat /tmp/creds/password)"
elif [ -f /tmp/creds/superuser-password ]; then
  export PGPASSWORD="$(cat /tmp/creds/superuser-password)"
fi
if [ -f /tmp/creds/user ]; then
  SUPER="$(cat /tmp/creds/user)"
elif [ -f /tmp/creds/superuser-user ]; then
  SUPER="$(cat /tmp/creds/superuser-user)"
else
  SUPER="postgres"
fi
psql -v ON_ERROR_STOP=1 -h "%s" -p "%s" -U "$SUPER" -d postgres <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%s') THEN
    CREATE ROLE "%s" WITH LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE;
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE "%s" OWNER "%s"' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '%s')\gexec
SQL
`, host, port, dbUser, dbUser, sitePassword, dbName, dbUser, dbName)

	container := resources.NewContainerBuilder("pg-provision", "postgres:16-alpine").
		WithCommand("sh", "-c").
		WithResources(vyogotechv1.ResolveJobResources(nil, vyogotechv1.JobKindMaintenance)).
		WithArgs(script).
		WithVolumeMount("creds", "/tmp/creds").
		Build()

	provisionJob := resources.NewJobBuilder(jobName, site.Namespace).
		WithLabels(map[string]string{"app": "frappe", "site": site.Name}).
		WithContainer(container).
		WithSecretVolume("creds", superSecretName, resources.Int32Ptr(0444)).
		MustBuild()

	if err := p.client.Create(ctx, provisionJob); err != nil && !errors.IsAlreadyExists(err) {
		return false, err
	}
	return false, nil
}

func (p *StackGresPostgresProvider) GetCredentials(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseCredentials, error) {
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
		Username:   generatePGUserName(site),
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

func (p *StackGresPostgresProvider) Cleanup(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	if site.Spec.DeletionPolicy != "Delete" {
		return nil
	}

	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	cluster.SetName(clusterName)
	cluster.SetNamespace(site.Namespace)
	if err := p.client.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete dedicated StackGres cluster: %w", err)
	}

	// Delete custom profile if created
	if site.Spec.DBConfig.Resources != nil {
		profile := &unstructured.Unstructured{}
		profile.SetGroupVersionKind(SGInstanceProfileGVK)
		profile.SetName(fmt.Sprintf("%s-postgres-profile", site.Name))
		profile.SetNamespace(site.Namespace)
		_ = p.client.Delete(ctx, profile)
	}

	// Delete jobs and password secret
	provJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-provision", site.Name),
			Namespace: site.Namespace,
		},
	}
	_ = p.client.Delete(ctx, provJob, client.PropagationPolicy(metav1.DeletePropagationBackground))

	confJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-configure", site.Name),
			Namespace: site.Namespace,
		},
	}
	_ = p.client.Delete(ctx, confJob, client.PropagationPolicy(metav1.DeletePropagationBackground))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-password", site.Name),
			Namespace: site.Namespace,
		},
	}
	_ = p.client.Delete(ctx, secret)

	return nil
}
