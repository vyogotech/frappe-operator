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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MariaDB Operator GVKs
var (
	MariaDBGVK = schema.GroupVersionKind{
		Group:   "k8s.mariadb.com",
		Version: "v1alpha1",
		Kind:    "MariaDB",
	}
	DatabaseGVK = schema.GroupVersionKind{
		Group:   "k8s.mariadb.com",
		Version: "v1alpha1",
		Kind:    "Database",
	}
	UserGVK = schema.GroupVersionKind{
		Group:   "k8s.mariadb.com",
		Version: "v1alpha1",
		Kind:    "User",
	}
	GrantGVK = schema.GroupVersionKind{
		Group:   "k8s.mariadb.com",
		Version: "v1alpha1",
		Kind:    "Grant",
	}
)

// MariaDBProviderUnstructured implements database provisioning using unstructured objects
type MariaDBProviderUnstructured struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewMariaDBProvider creates a new MariaDB provider using unstructured objects
func NewMariaDBProvider(client client.Client, scheme *runtime.Scheme) Provider {
	return &MariaDBProviderUnstructured{
		client: client,
		scheme: scheme,
	}
}

// EnsureDatabase ensures database, user, and grant CRs exist
func (p *MariaDBProviderUnstructured) EnsureDatabase(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseInfo, error) {
	logger := log.FromContext(ctx)

	// Generate database and user names
	dbName := p.generateDBName(site)
	dbUser := p.generateDBUser(site)

	// Determine MariaDB instance to use
	mariadbName, mariadbNamespace, err := p.getMariaDBInstance(ctx, site)
	if err != nil {
		return nil, err
	}

	logger.Info("Using MariaDB instance",
		"mariadb", mariadbName,
		"namespace", mariadbNamespace,
		"dbName", dbName,
		"dbUser", dbUser)

	// 1. Ensure Database CR
	if err := p.ensureDatabaseCR(ctx, site, mariadbName, mariadbNamespace, dbName); err != nil {
		return nil, fmt.Errorf("failed to ensure Database CR: %w", err)
	}

	// 2. Ensure User CR with password secret
	_, err = p.ensureUserCR(ctx, site, mariadbName, mariadbNamespace, dbUser)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure User CR: %w", err)
	}

	// 3. Ensure Grant CR
	if err := p.ensureGrantCR(ctx, site, mariadbName, mariadbNamespace, dbName, dbUser); err != nil {
		return nil, fmt.Errorf("failed to ensure Grant CR: %w", err)
	}

	// Get MariaDB service for connection details
	host, port, err := p.getMariaDBConnection(ctx, mariadbName, mariadbNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get MariaDB connection info: %w", err)
	}

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: "mariadb",
	}, nil
}

// IsReady checks if all database resources are ready
func (p *MariaDBProviderUnstructured) IsReady(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (bool, error) {
	logger := log.FromContext(ctx)

	// Check Database CR
	database := &unstructured.Unstructured{}
	database.SetGroupVersionKind(DatabaseGVK)
	dbKey := types.NamespacedName{
		Name:      fmt.Sprintf("%s-db", site.Name),
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, dbKey, database); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if !p.isResourceReady(database) {
		logger.Info("Database not ready yet", "database", database.GetName())
		return false, nil
	}

	// Check User CR
	user := &unstructured.Unstructured{}
	user.SetGroupVersionKind(UserGVK)
	userKey := types.NamespacedName{
		Name:      fmt.Sprintf("%s-user", site.Name),
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, userKey, user); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if !p.isResourceReady(user) {
		logger.Info("User not ready yet", "user", user.GetName())
		return false, nil
	}

	// Check Grant CR
	grant := &unstructured.Unstructured{}
	grant.SetGroupVersionKind(GrantGVK)
	grantKey := types.NamespacedName{
		Name:      fmt.Sprintf("%s-grant", site.Name),
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, grantKey, grant); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if !p.isResourceReady(grant) {
		logger.Info("Grant not ready yet", "grant", grant.GetName())
		return false, nil
	}

	logger.Info("All database resources ready")
	return true, nil
}

// GetCredentials retrieves database credentials from the User's password secret
func (p *MariaDBProviderUnstructured) GetCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseCredentials, error) {
	// Get the User CR to find password secret
	user := &unstructured.Unstructured{}
	user.SetGroupVersionKind(UserGVK)
	userKey := types.NamespacedName{
		Name:      fmt.Sprintf("%s-user", site.Name),
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, userKey, user); err != nil {
		return nil, fmt.Errorf("failed to get User CR: %w", err)
	}

	// Extract username from spec
	dbUser, _, err := unstructured.NestedString(user.Object, "spec", "name")
	if err != nil || dbUser == "" {
		return nil, fmt.Errorf("failed to get username from User CR: %w", err)
	}

	// Extract password secret reference
	passwordSecretName, _, err := unstructured.NestedString(user.Object, "spec", "passwordSecretKeyRef", "name")
	if err != nil || passwordSecretName == "" {
		return nil, fmt.Errorf("failed to get password secret name from User CR: %w", err)
	}

	passwordSecretKey, _, err := unstructured.NestedString(user.Object, "spec", "passwordSecretKeyRef", "key")
	if err != nil || passwordSecretKey == "" {
		passwordSecretKey = "password" // Default
	}

	// Get the password from the secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      passwordSecretName,
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get password secret: %w", err)
	}

	password, ok := secret.Data[passwordSecretKey]
	if !ok {
		return nil, fmt.Errorf("password key '%s' not found in secret", passwordSecretKey)
	}

	return &DatabaseCredentials{
		Username:   dbUser,
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

// Cleanup removes database resources
func (p *MariaDBProviderUnstructured) Cleanup(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	// Resources will be automatically cleaned up via owner references
	return nil
}

// Helper functions

func (p *MariaDBProviderUnstructured) getMariaDBInstance(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
	// Check if user specified a MariaDB reference
	if site.Spec.DBConfig.MariaDBRef != nil {
		ns := site.Spec.DBConfig.MariaDBRef.Namespace
		if ns == "" {
			ns = site.Namespace
		}
		return site.Spec.DBConfig.MariaDBRef.Name, ns, nil
	}

	// Mode determines how we find/create MariaDB instance
	mode := site.Spec.DBConfig.Mode
	if mode == "" {
		mode = "shared" // Default
	}

	switch mode {
	case "shared":
		return p.findOrCreateSharedMariaDB(ctx, site)
	case "dedicated":
		return p.createDedicatedMariaDB(ctx, site)
	default:
		return "", "", fmt.Errorf("unsupported database mode: %s", mode)
	}
}

func (p *MariaDBProviderUnstructured) findOrCreateSharedMariaDB(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
	mariadbName := "frappe-mariadb"
	mariadb := &unstructured.Unstructured{}
	mariadb.SetGroupVersionKind(MariaDBGVK)

	err := p.client.Get(ctx, types.NamespacedName{
		Name:      mariadbName,
		Namespace: site.Namespace,
	}, mariadb)

	if err == nil {
		return mariadbName, site.Namespace, nil
	}

	if !errors.IsNotFound(err) {
		return "", "", err
	}

	return "", "", fmt.Errorf("shared MariaDB instance '%s' not found in namespace '%s'. Please create a MariaDB CR or specify dbConfig.mariadbRef", mariadbName, site.Namespace)
}

func (p *MariaDBProviderUnstructured) createDedicatedMariaDB(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
	mariadbName := fmt.Sprintf("%s-mariadb", site.Name)

	// Check if already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(MariaDBGVK)
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      mariadbName,
		Namespace: site.Namespace,
	}, existing)

	if err == nil {
		return mariadbName, site.Namespace, nil
	}

	if !errors.IsNotFound(err) {
		return "", "", err
	}

	// Create dedicated MariaDB instance
	storageSize := "10Gi"
	if site.Spec.DBConfig.StorageSize != nil {
		storageSize = site.Spec.DBConfig.StorageSize.String()
	}

	// Generate root password
	rootPassword := p.generatePassword(32)
	rootSecretName := fmt.Sprintf("%s-mariadb-root", site.Name)

	// Create root password secret
	rootSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rootSecretName,
			Namespace: site.Namespace,
		},
		StringData: map[string]string{
			"password": rootPassword,
		},
	}
	if err := controllerutil.SetControllerReference(site, rootSecret, p.scheme); err != nil {
		return "", "", err
	}
	if err := p.client.Create(ctx, rootSecret); err != nil && !errors.IsAlreadyExists(err) {
		return "", "", fmt.Errorf("failed to create root password secret: %w", err)
	}

	mariadb := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.mariadb.com/v1alpha1",
			"kind":       "MariaDB",
			"metadata": map[string]interface{}{
				"name":      mariadbName,
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"rootPasswordSecretKeyRef": map[string]interface{}{
					"name": rootSecretName,
					"key":  "password",
				},
				"image": "mariadb:10.11",
				"storage": map[string]interface{}{
					"size": storageSize,
				},
				"replicas": 1,
				"port":     3306,
			},
		},
	}

	if err := controllerutil.SetControllerReference(site, mariadb, p.scheme); err != nil {
		return "", "", err
	}

	if err := p.client.Create(ctx, mariadb); err != nil {
		return "", "", fmt.Errorf("failed to create MariaDB instance: %w", err)
	}

	return mariadbName, site.Namespace, nil
}

func (p *MariaDBProviderUnstructured) ensureDatabaseCR(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, mariadbName, mariadbNamespace, dbName string) error {
	database := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.mariadb.com/v1alpha1",
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-db", site.Name),
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"mariaDbRef": map[string]interface{}{
					"name":      mariadbName,
					"namespace": mariadbNamespace,
				},
				"name":         dbName,
				"characterSet": "utf8mb4",
				"collate":      "utf8mb4_unicode_ci",
			},
		},
	}

	if err := controllerutil.SetControllerReference(site, database, p.scheme); err != nil {
		return err
	}

	// Check if exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(DatabaseGVK)
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      database.GetName(),
		Namespace: database.GetNamespace(),
	}, existing)

	if errors.IsNotFound(err) {
		return p.client.Create(ctx, database)
	}

	return err
}

func (p *MariaDBProviderUnstructured) ensureUserCR(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, mariadbName, mariadbNamespace, dbUser string) (string, error) {
	passwordSecretName := fmt.Sprintf("%s-db-password", site.Name)

	// Create password secret if it doesn't exist
	passwordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      passwordSecretName,
			Namespace: site.Namespace,
		},
	}

	err := p.client.Get(ctx, types.NamespacedName{
		Name:      passwordSecretName,
		Namespace: site.Namespace,
	}, passwordSecret)

	if errors.IsNotFound(err) {
		passwordSecret.StringData = map[string]string{
			"password": p.generatePassword(16),
		}
		if err := controllerutil.SetControllerReference(site, passwordSecret, p.scheme); err != nil {
			return "", err
		}
		if err := p.client.Create(ctx, passwordSecret); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	// Create User CR
	user := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.mariadb.com/v1alpha1",
			"kind":       "User",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-user", site.Name),
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"mariaDbRef": map[string]interface{}{
					"name":      mariadbName,
					"namespace": mariadbNamespace,
				},
				"name": dbUser,
				"passwordSecretKeyRef": map[string]interface{}{
					"name": passwordSecretName,
					"key":  "password",
				},
				"maxUserConnections": 100,
			},
		},
	}

	if err := controllerutil.SetControllerReference(site, user, p.scheme); err != nil {
		return "", err
	}

	// Check if exists
	existingUser := &unstructured.Unstructured{}
	existingUser.SetGroupVersionKind(UserGVK)
	err = p.client.Get(ctx, types.NamespacedName{
		Name:      user.GetName(),
		Namespace: user.GetNamespace(),
	}, existingUser)

	if errors.IsNotFound(err) {
		if err := p.client.Create(ctx, user); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	return passwordSecretName, nil
}

func (p *MariaDBProviderUnstructured) ensureGrantCR(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, mariadbName, mariadbNamespace, dbName, dbUser string) error {
	grant := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.mariadb.com/v1alpha1",
			"kind":       "Grant",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-grant", site.Name),
				"namespace": site.Namespace,
			},
			"spec": map[string]interface{}{
				"mariaDbRef": map[string]interface{}{
					"name":      mariadbName,
					"namespace": mariadbNamespace,
				},
				"privileges":  []string{"ALL PRIVILEGES"},
				"database":    dbName,
				"table":       "*",
				"username":    dbUser,
				"grantOption": true,
			},
		},
	}

	if err := controllerutil.SetControllerReference(site, grant, p.scheme); err != nil {
		return err
	}

	// Check if exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(GrantGVK)
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      grant.GetName(),
		Namespace: grant.GetNamespace(),
	}, existing)

	if errors.IsNotFound(err) {
		return p.client.Create(ctx, grant)
	}

	return err
}

func (p *MariaDBProviderUnstructured) getMariaDBConnection(ctx context.Context, mariadbName, mariadbNamespace string) (string, string, error) {
	// MariaDB Operator creates a service with the same name as the MariaDB CR
	host := fmt.Sprintf("%s.%s.svc.cluster.local", mariadbName, mariadbNamespace)
	port := "3306"
	return host, port, nil
}

func (p *MariaDBProviderUnstructured) isResourceReady(obj *unstructured.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(condMap, "type")
		condStatus, _, _ := unstructured.NestedString(condMap, "status")

		if condType == "Ready" && condStatus == "True" {
			return true
		}
	}

	return false
}

func (p *MariaDBProviderUnstructured) generateDBName(site *vyogotechv1alpha1.FrappeSite) string {
	hash := p.hashString(site.Namespace + "/" + site.Name)[:8]
	safeName := p.sanitizeName(site.Spec.SiteName)
	dbName := fmt.Sprintf("_%s_%s", hash, safeName)
	if len(dbName) > 64 {
		dbName = dbName[:64]
	}
	return dbName
}

func (p *MariaDBProviderUnstructured) generateDBUser(site *vyogotechv1alpha1.FrappeSite) string {
	safeName := p.sanitizeName(site.Name)
	userName := fmt.Sprintf("%s_user", safeName)
	if len(userName) > 32 {
		userName = userName[:32]
	}
	return userName
}

func (p *MariaDBProviderUnstructured) sanitizeName(name string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	sanitized := reg.ReplaceAllString(name, "_")
	reg2 := regexp.MustCompile(`_{2,}`)
	sanitized = reg2.ReplaceAllString(sanitized, "_")
	sanitized = strings.Trim(sanitized, "_")
	return sanitized
}

func (p *MariaDBProviderUnstructured) hashString(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum32())
}

func (p *MariaDBProviderUnstructured) generatePassword(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", metav1.Now().Unix())
	}
	return hex.EncodeToString(bytes)
}
