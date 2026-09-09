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
	"time"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

// perconaStatusGracePeriod is how long a PerconaPGCluster may exist without the
// Percona operator ever writing a status before we treat it as stalled and
// surface an error. The operator normally sets status within seconds, so a
// cluster still blank after this window almost always means the Percona
// operator is not reconciling it at all - most commonly because it is deployed
// in single-namespace mode (WATCH_NAMESPACE) and is not watching this
// namespace, or is missing RBAC for it. Without this check the site would sit
// in Provisioning forever with no explanation.
const perconaStatusGracePeriod = 5 * time.Minute

const (
	defaultPerconaPGVersion = 16
	// Fully qualified on purpose. An unqualified name like
	// "percona/percona-postgresql-operator:..." is resolved through the
	// cluster's unqualified-search-registries list, which on OpenShift does not
	// start with Docker Hub: CRI-O tries registry.connect.redhat.com first and
	// the pull fails with "name unknown: Image not found", leaving every
	// dedicated Percona cluster stuck in ImagePullBackOff.
	defaultPerconaPostgresImage   = "docker.io/percona/percona-postgresql-operator:2.3.1-ppg16-postgres"
	defaultPerconaPGBouncerImage  = "docker.io/percona/percona-postgresql-operator:2.3.1-ppg16-pgbouncer"
	defaultPerconaPGBackRestImage = "docker.io/percona/percona-postgresql-operator:2.3.1-ppg16-pgbackrest"
)

// PerconaPostgresProvider implements database.Provider for dedicated Percona PostgreSQL clusters
type PerconaPostgresProvider struct {
	config vyogotechv1.DatabaseConfig
	client client.Client
	scheme *runtime.Scheme
}

func NewPerconaPostgresProvider(config vyogotechv1.DatabaseConfig, client client.Client, scheme *runtime.Scheme) *PerconaPostgresProvider {
	return &PerconaPostgresProvider{
		config: config,
		client: client,
		scheme: scheme,
	}
}

func (p *PerconaPostgresProvider) getDBConfig(site *vyogotechv1.FrappeSite) vyogotechv1.DatabaseConfig {
	cfg := p.config
	if site == nil {
		return cfg
	}
	if cfg.StorageSize == nil {
		cfg.StorageSize = site.Spec.DBConfig.StorageSize
	}
	if cfg.Resources == nil {
		cfg.Resources = site.Spec.DBConfig.Resources
	}
	if cfg.Host == "" {
		cfg.Host = site.Spec.DBConfig.Host
	}
	if cfg.Port == "" {
		cfg.Port = site.Spec.DBConfig.Port
	}
	return cfg
}

func (p *PerconaPostgresProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseInfo, error) {
	logger := log.FromContext(ctx)
	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	dbName := generateDBName(site)
	dbUser := generatePGUserName(site)

	cfg := p.getDBConfig(site)
	storageSize := defaultDedicatedStorageSize
	if cfg.StorageSize != nil {
		storageSize = cfg.StorageSize.String()
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
						"name": "postgres",
					},
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

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(PerconaPGClusterGVK)
	err := p.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: site.Namespace}, existing)
	if errors.IsNotFound(err) {
		if err := p.client.Create(ctx, cluster); err != nil {
			return nil, fmt.Errorf("failed to create PerconaPGCluster: %w", err)
		}
		logger.Info("Created dedicated PerconaPGCluster", "name", clusterName)
	} else if err != nil {
		return nil, fmt.Errorf("failed to get PerconaPGCluster: %w", err)
	}

	host := fmt.Sprintf("%s-primary.%s.svc.cluster.local", clusterName, site.Namespace)
	port := "5432"
	if cfg.Host != "" {
		host = cfg.Host
	}
	if cfg.Port != "" {
		port = cfg.Port
	}

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: "postgres",
	}, nil
}

func (p *PerconaPostgresProvider) IsReady(ctx context.Context, site *vyogotechv1.FrappeSite) (bool, error) {
	logger := log.FromContext(ctx)
	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(PerconaPGClusterGVK)
	err := p.client.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: site.Namespace}, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	status, found, err := unstructured.NestedString(cluster.Object, "status", "state")
	if err != nil || !found || status == "" {
		// No status yet. Normal right after creation, but if it stays blank the
		// Percona operator is not reconciling this cluster at all - report that
		// rather than requeueing silently forever.
		if age := time.Since(cluster.GetCreationTimestamp().Time); age > perconaStatusGracePeriod {
			return false, fmt.Errorf(
				"PerconaPGCluster %s/%s has had no status for %s: the Percona operator does not appear to be reconciling it "+
					"(check that its WATCH_NAMESPACE covers namespace %q and that it has RBAC for perconapgclusters there)",
				site.Namespace, clusterName, age.Round(time.Second), site.Namespace)
		}
		logger.Info("Dedicated Percona Postgres cluster has no status yet; waiting", "cluster", clusterName)
		return false, nil
	}
	if isPerconaFailureState(status) {
		return false, fmt.Errorf("PerconaPGCluster %s/%s reported state %q", site.Namespace, clusterName, status)
	}
	if status != "ready" {
		logger.Info("Dedicated Percona Postgres cluster not ready yet", "cluster", clusterName, "state", status)
		return false, nil
	}

	host := fmt.Sprintf("%s-primary.%s.svc.cluster.local", clusterName, site.Namespace)
	superSecretName := fmt.Sprintf("%s-pguser-postgres", clusterName)
	configured, err := ensureDedicatedConfigured(ctx, p.client, site, host, superSecretName)
	if err != nil {
		return false, err
	}
	if !configured {
		logger.Info("Dedicated Percona Postgres cluster ready; waiting on schema configure job")
		return false, nil
	}

	logger.Info("Dedicated Percona Postgres cluster is ready and configured")
	return true, nil
}

func (p *PerconaPostgresProvider) GetCredentials(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseCredentials, error) {
	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	dbUser := generatePGUserName(site)
	secretName := fmt.Sprintf("%s-pguser-%s", clusterName, dbUser)

	secret := &corev1.Secret{}
	err := p.client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get password secret %s: %w", secretName, err)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("password key 'password' not found in secret %s", secretName)
	}

	if userBytes, ok := secret.Data["user"]; ok {
		dbUser = string(userBytes)
	}

	return &DatabaseCredentials{
		Username:   dbUser,
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

func (p *PerconaPostgresProvider) Cleanup(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	if site.Spec.DeletionPolicy != "Delete" {
		return nil
	}

	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(PerconaPGClusterGVK)
	cluster.SetName(fmt.Sprintf("%s-postgres", site.Name))
	cluster.SetNamespace(site.Namespace)
	if err := p.client.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete dedicated postgres cluster: %w", err)
	}
	return nil
}

// isPerconaFailureState reports whether a PerconaPGCluster status.state value
// represents a terminal or error condition rather than normal progress. Percona
// reports free-form states, so match conservatively on the error-ish ones and
// treat anything else as "still working".
func isPerconaFailureState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "error", "failed", "failing", "unhealthy":
		return true
	}
	return false
}
