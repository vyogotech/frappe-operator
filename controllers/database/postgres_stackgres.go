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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
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
	SGScriptGVK = schema.GroupVersionKind{
		Group:   "stackgres.io",
		Version: "v1",
		Kind:    "SGScript",
	}
)

const (
	defaultStackGresPostgresVersion = "16"
	defaultStackGresProfileName     = "frappe-postgres-dedicated-default"
	defaultStackGresConfigName      = "frappe-postgres-dedicated-defaults"
	defaultStackGresPoolingName     = "frappe-postgres-dedicated-pooling"
)

const sgPostgresConfigTemplate = `
apiVersion: stackgres.io/v1
kind: SGPostgresConfig
metadata:
  name: %s
  namespace: %s
spec:
  postgresVersion: "%s"
  postgresql.conf:
    max_connections: '300'
    shared_buffers: '1GB'
    work_mem: '8MB'
`

const sgPoolingConfigTemplate = `
apiVersion: stackgres.io/v1
kind: SGPoolingConfig
metadata:
  name: %s
  namespace: %s
spec:
  pgBouncer:
    pgbouncer.ini:
      pgbouncer:
        default_pool_size: '50'
        max_client_conn: '1000'
        pool_mode: transaction
`

const sgInstanceProfileTemplate = `
apiVersion: stackgres.io/v1
kind: SGInstanceProfile
metadata:
  name: %s
  namespace: %s
spec:
  cpu: "%s"
  memory: "%s"
`

const sgScriptSecretTemplate = `
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
stringData:
  create-user.sql: |
    CREATE ROLE "%s" WITH LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE;
  create-db.sql: |
    CREATE DATABASE "%s" OWNER "%s";
  configure-schema.sql: |
    ALTER SCHEMA public OWNER TO "%s";
    GRANT ALL ON SCHEMA public TO "%s";
`

const sgScriptTemplate = `
apiVersion: stackgres.io/v1
kind: SGScript
metadata:
  name: %s
  namespace: %s
spec:
  managedVersions: true
  scripts:
    - name: create-user
      scriptFrom:
        secretKeyRef:
          name: %s
          key: create-user.sql
    - name: create-db
      scriptFrom:
        secretKeyRef:
          name: %s
          key: create-db.sql
    - name: configure-schema
      database: %s
      scriptFrom:
        secretKeyRef:
          name: %s
          key: configure-schema.sql
`

const sgClusterTemplate = `
apiVersion: stackgres.io/v1
kind: SGCluster
metadata:
  name: %s
  namespace: %s
spec:
  instances: 1
  postgres:
    version: "%s"
  sgInstanceProfile: %s
  configurations:
    sgPostgresConfig: %s
    sgPoolingConfig: %s
  pods:
    persistentVolume:
      size: %s
  managedSql:
    scripts:
      - sgScript: %s
`

// StackGresPostgresProvider implements database.Provider for dedicated StackGres clusters
type StackGresPostgresProvider struct {
	config vyogotechv1.DatabaseConfig
	client client.Client
	scheme *runtime.Scheme
}

func NewStackGresPostgresProvider(config vyogotechv1.DatabaseConfig, client client.Client, scheme *runtime.Scheme) *StackGresPostgresProvider {
	return &StackGresPostgresProvider{
		config: config,
		client: client,
		scheme: scheme,
	}
}

func (p *StackGresPostgresProvider) getDBConfig(site *vyogotechv1.FrappeSite) vyogotechv1.DatabaseConfig {
	cfg := p.config
	if site == nil {
		return cfg
	}
	if cfg.Port == "" {
		cfg.Port = site.Spec.DBConfig.Port
	}
	if cfg.Host == "" {
		cfg.Host = site.Spec.DBConfig.Host
	}
	if cfg.StorageSize == nil {
		cfg.StorageSize = site.Spec.DBConfig.StorageSize
	}
	if cfg.Resources == nil {
		cfg.Resources = site.Spec.DBConfig.Resources
	}
	return cfg
}

// EnsureDatabase provisions dedicated StackGres database resources (SGCluster, SGScript, SGInstanceProfile, Secret).
// In v1, resources are created if absent (create-if-absent). Modifications to resource sizing or storage
// post-creation require manual cluster maintenance or an upgrade migration.
func (p *StackGresPostgresProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseInfo, error) {
	logger := log.FromContext(ctx)
	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	dbName := generateDBName(site)
	dbUser := generateDBUser(site)
	cfg := p.getDBConfig(site)

	// 1. Ensure password secret exists for the site
	sitePasswordBytes, err := ensurePasswordSecret(ctx, p.client, site)
	if err != nil {
		return nil, err
	}
	sitePassword := string(sitePasswordBytes)
	escapedPassword := strings.ReplaceAll(sitePassword, "'", "''")

	// 2. Ensure supporting StackGres configurations (PostgresConfig, PoolingConfig, InstanceProfile)
	profileName, err := p.ensureConfigurations(ctx, site)
	if err != nil {
		return nil, err
	}

	// 3. Ensure provisioning SGScript and Secret
	scriptName := fmt.Sprintf("%s-script", clusterName)
	scriptSecretName := fmt.Sprintf("%s-script-secret", clusterName)

	secretYAML := fmt.Sprintf(sgScriptSecretTemplate, scriptSecretName, site.Namespace, dbUser, escapedPassword, dbName, dbUser, dbUser, dbUser)
	scriptSecret := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(secretYAML), scriptSecret); err != nil {
		return nil, fmt.Errorf("failed to parse SGScript secret template: %w", err)
	}
	if err := createIfAbsent(ctx, p.client, scriptSecret); err != nil {
		return nil, err
	}

	scriptYAML := fmt.Sprintf(sgScriptTemplate, scriptName, site.Namespace, scriptSecretName, scriptSecretName, dbName, scriptSecretName)
	sgScript := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(scriptYAML), sgScript); err != nil {
		return nil, fmt.Errorf("failed to parse SGScript template: %w", err)
	}
	if err := createIfAbsent(ctx, p.client, sgScript); err != nil {
		return nil, err
	}

	// 4. Create or update SGCluster CR
	storageSize := defaultDedicatedStorageSize
	if cfg.StorageSize != nil {
		storageSize = cfg.StorageSize.String()
	}

	clusterYAML := fmt.Sprintf(sgClusterTemplate, clusterName, site.Namespace, defaultStackGresPostgresVersion, profileName, defaultStackGresConfigName, defaultStackGresPoolingName, storageSize, scriptName)
	cluster := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(clusterYAML), cluster); err != nil {
		return nil, fmt.Errorf("failed to parse SGCluster template: %w", err)
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
	if cfg.Port != "" {
		port = cfg.Port
	}
	host := fmt.Sprintf("%s.%s.svc.cluster.local", clusterName, site.Namespace)
	if cfg.Host != "" {
		host = cfg.Host
	}

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: "postgres",
	}, nil
}

func (p *StackGresPostgresProvider) ensureConfigurations(ctx context.Context, site *vyogotechv1.FrappeSite) (string, error) {
	cfg := p.getDBConfig(site)

	// Shared postgres configuration
	pgConfigYAML := fmt.Sprintf(sgPostgresConfigTemplate, defaultStackGresConfigName, site.Namespace, defaultStackGresPostgresVersion)
	pgConfig := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(pgConfigYAML), pgConfig); err != nil {
		return "", fmt.Errorf("failed to parse SGPostgresConfig template: %w", err)
	}
	if err := createIfAbsent(ctx, p.client, pgConfig); err != nil {
		return "", err
	}

	// Shared pooling configuration
	poolingConfigYAML := fmt.Sprintf(sgPoolingConfigTemplate, defaultStackGresPoolingName, site.Namespace)
	poolingConfig := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(poolingConfigYAML), poolingConfig); err != nil {
		return "", fmt.Errorf("failed to parse SGPoolingConfig template: %w", err)
	}
	if err := createIfAbsent(ctx, p.client, poolingConfig); err != nil {
		return "", err
	}

	// Sizing profile: custom if resources are set, otherwise namespace-shared default
	profileName := defaultStackGresProfileName
	cpu := "2"
	memory := "4Gi"
	if cfg.Resources != nil {
		profileName = fmt.Sprintf("%s-postgres-profile", site.Name)
		if cpuQty, ok := cfg.Resources.Requests[corev1.ResourceCPU]; ok {
			cpu = cpuQty.String()
		} else if cpuQty, ok := cfg.Resources.Limits[corev1.ResourceCPU]; ok {
			cpu = cpuQty.String()
		}
		if memQty, ok := cfg.Resources.Requests[corev1.ResourceMemory]; ok {
			memory = memQty.String()
		} else if memQty, ok := cfg.Resources.Limits[corev1.ResourceMemory]; ok {
			memory = memQty.String()
		}
	}

	profileYAML := fmt.Sprintf(sgInstanceProfileTemplate, profileName, site.Namespace, cpu, memory)
	profile := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(profileYAML), profile); err != nil {
		return "", fmt.Errorf("failed to parse SGInstanceProfile template: %w", err)
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

	// A cluster StackGres has marked Failed will never become ready on its own;
	// surface it instead of requeueing silently (mirrors the managedSql script
	// failure handling below).
	if msg, failed := stackGresFailure(cluster); failed {
		return false, fmt.Errorf("SGCluster %s/%s reported Failed: %s", site.Namespace, clusterName, msg)
	}

	if !p.isClusterReady(cluster) {
		return false, nil
	}

	scriptReady, err := p.isScriptReady(cluster)
	if err != nil {
		return false, err
	}
	if !scriptReady {
		logger.Info("Dedicated StackGres cluster ready; waiting on managedSql scripts")
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
			t, _ := condMap["type"].(string)
			s, _ := condMap["status"].(string)
			if (t == "Ready" || t == "Bootstrapped") && strings.EqualFold(s, "True") {
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

	podStatuses, found, err := unstructured.NestedSlice(cluster.Object, "status", "podStatuses")
	if err == nil && found && len(podStatuses) > 0 {
		for _, pod := range podStatuses {
			if podMap, ok := pod.(map[string]interface{}); ok {
				if isPrimary, ok := podMap["primary"].(bool); ok && isPrimary {
					return true
				}
			}
		}
	}

	return false
}

func (p *StackGresPostgresProvider) isScriptReady(cluster *unstructured.Unstructured) (bool, error) {
	scripts, found, err := unstructured.NestedSlice(cluster.Object, "status", "managedSql", "scripts")
	if err != nil || !found || len(scripts) == 0 {
		return false, nil
	}

	specScripts, specFound, _ := unstructured.NestedSlice(cluster.Object, "spec", "managedSql", "scripts")
	if specFound && len(specScripts) > 0 && len(scripts) < len(specScripts) {
		return false, nil
	}

	for i, sc := range scripts {
		scriptState, ok := sc.(map[string]interface{})
		if !ok {
			return false, nil
		}
		if failedAt, found := scriptState["failedAt"]; found && failedAt != nil {
			failureMsg := fmt.Sprintf("managedSql script entry %d failed at %v", i, failedAt)
			if innerScripts, found, _ := unstructured.NestedSlice(scriptState, "scripts"); found {
				for _, inner := range innerScripts {
					if innerMap, ok := inner.(map[string]interface{}); ok {
						if fail, hasFail := innerMap["failure"].(string); hasFail && fail != "" {
							failureMsg = fmt.Sprintf("%s: %s", failureMsg, fail)
						}
					}
				}
			}
			return false, fmt.Errorf("%s", failureMsg)
		}
		if innerScripts, found, _ := unstructured.NestedSlice(scriptState, "scripts"); found {
			for _, inner := range innerScripts {
				if innerMap, ok := inner.(map[string]interface{}); ok {
					if fail, hasFail := innerMap["failure"].(string); hasFail && fail != "" {
						return false, fmt.Errorf("managedSql script execution failed: %s", fail)
					}
				}
			}
		}
		if completedAt, found := scriptState["completedAt"]; !found || completedAt == nil {
			return false, nil
		}
	}

	return true, nil
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
		Username:   generateDBUser(site),
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

func (p *StackGresPostgresProvider) Cleanup(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	if site.Spec.DeletionPolicy != "Delete" {
		return nil
	}

	logger := log.FromContext(ctx)
	var errs []error

	clusterName := fmt.Sprintf("%s-postgres", site.Name)
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	cluster.SetName(clusterName)
	cluster.SetNamespace(site.Namespace)
	if err := p.client.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete SGCluster %s: %w", clusterName, err))
	}

	// Delete SGScript and Secret
	scriptName := fmt.Sprintf("%s-script", clusterName)
	sgScript := &unstructured.Unstructured{}
	sgScript.SetGroupVersionKind(SGScriptGVK)
	sgScript.SetName(scriptName)
	sgScript.SetNamespace(site.Namespace)
	if err := p.client.Delete(ctx, sgScript); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete SGScript", "name", scriptName)
		errs = append(errs, fmt.Errorf("failed to delete SGScript %s: %w", scriptName, err))
	}

	scriptSecretName := fmt.Sprintf("%s-script-secret", clusterName)
	scriptSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scriptSecretName,
			Namespace: site.Namespace,
		},
	}
	if err := p.client.Delete(ctx, scriptSecret); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete SGScript secret", "name", scriptSecretName)
		errs = append(errs, fmt.Errorf("failed to delete SGScript secret %s: %w", scriptSecretName, err))
	}

	// Delete custom profile if created
	cfg := p.getDBConfig(site)
	if cfg.Resources != nil {
		profile := &unstructured.Unstructured{}
		profile.SetGroupVersionKind(SGInstanceProfileGVK)
		profile.SetName(fmt.Sprintf("%s-postgres-profile", site.Name))
		profile.SetNamespace(site.Namespace)
		if err := p.client.Delete(ctx, profile); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete SGInstanceProfile", "name", profile.GetName())
			errs = append(errs, fmt.Errorf("failed to delete SGInstanceProfile %s: %w", profile.GetName(), err))
		}
	}

	// Delete password secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db-password", site.Name),
			Namespace: site.Namespace,
		},
	}
	if err := p.client.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete password secret", "name", secret.Name)
		errs = append(errs, fmt.Errorf("failed to delete password secret %s: %w", secret.Name, err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors for %s: %v", site.Name, errs)
	}
	return nil
}

// stackGresFailure reports whether the SGCluster carries a Failed=True
// condition, along with its message. StackGres sets this for configuration and
// bootstrap errors that will not clear without intervention.
func stackGresFailure(cluster *unstructured.Unstructured) (string, bool) {
	conditions, found, err := unstructured.NestedSlice(cluster.Object, "status", "conditions")
	if err != nil || !found {
		return "", false
	}
	for _, c := range conditions {
		condMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		t, _ := condMap["type"].(string)
		st, _ := condMap["status"].(string)
		if t == "Failed" && strings.EqualFold(st, "True") {
			msg, _ := condMap["message"].(string)
			if msg == "" {
				if reason, ok := condMap["reason"].(string); ok {
					msg = reason
				}
			}
			if msg == "" {
				msg = "no message reported"
			}
			return msg, true
		}
	}
	return "", false
}
