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

	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/v25/api/v1alpha1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MariaDBProvider implements database provisioning using MariaDB Operator
type MariaDBProvider struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewMariaDBProvider creates a new MariaDB provider
func NewMariaDBProvider(client client.Client, scheme *runtime.Scheme) Provider {
	return &MariaDBProvider{
		client: client,
		scheme: scheme,
	}
}

// EnsureDatabase ensures database, user, and grant CRs exist
func (p *MariaDBProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseInfo, error) {
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
func (p *MariaDBProvider) IsReady(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (bool, error) {
	logger := log.FromContext(ctx)

	// Check Database CR
	database := &mariadbv1alpha1.Database{}
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

	if !p.isDatabaseReady(database) {
		logger.Info("Database not ready yet", "database", database.Name)
		return false, nil
	}

	// Check User CR
	user := &mariadbv1alpha1.User{}
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

	if !p.isUserReady(user) {
		logger.Info("User not ready yet", "user", user.Name)
		return false, nil
	}

	// Check Grant CR
	grant := &mariadbv1alpha1.Grant{}
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

	if !p.isGrantReady(grant) {
		logger.Info("Grant not ready yet", "grant", grant.Name)
		return false, nil
	}

	logger.Info("All database resources ready")
	return true, nil
}

// GetCredentials retrieves database credentials from the User's password secret
func (p *MariaDBProvider) GetCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (*DatabaseCredentials, error) {
	// Get the User CR to find password secret
	user := &mariadbv1alpha1.User{}
	userKey := types.NamespacedName{
		Name:      fmt.Sprintf("%s-user", site.Name),
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, userKey, user); err != nil {
		return nil, fmt.Errorf("failed to get User CR: %w", err)
	}

	if user.Spec.PasswordSecretKeyRef == nil {
		return nil, fmt.Errorf("user has no password secret reference")
	}

	// Get the password from the secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      user.Spec.PasswordSecretKeyRef.Name,
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get password secret: %w", err)
	}

	password, ok := secret.Data[user.Spec.PasswordSecretKeyRef.Key]
	if !ok {
		return nil, fmt.Errorf("password key '%s' not found in secret", user.Spec.PasswordSecretKeyRef.Key)
	}

	return &DatabaseCredentials{
		Username:   user.Spec.Name,
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

// Cleanup removes database resources
func (p *MariaDBProvider) Cleanup(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	// Resources will be automatically cleaned up via owner references
	// This is a placeholder for any additional cleanup logic
	return nil
}

// Helper functions

func (p *MariaDBProvider) getMariaDBInstance(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
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
		// Look for a default shared MariaDB instance
		// Convention: "frappe-mariadb" in the operator namespace or site namespace
		return p.findOrCreateSharedMariaDB(ctx, site)
	case "dedicated":
		// Create a dedicated MariaDB instance for this site
		return p.createDedicatedMariaDB(ctx, site)
	default:
		return "", "", fmt.Errorf("unsupported database mode: %s", mode)
	}
}

func (p *MariaDBProvider) findOrCreateSharedMariaDB(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
	// For v1.0.0, we'll require a pre-existing MariaDB instance
	// Look for "frappe-mariadb" in the site's namespace
	mariadbName := "frappe-mariadb"
	mariadb := &mariadbv1alpha1.MariaDB{}
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

	// MariaDB not found - return error with helpful message
	return "", "", fmt.Errorf("shared MariaDB instance '%s' not found in namespace '%s'. Please create a MariaDB CR or specify dbConfig.mariadbRef", mariadbName, site.Namespace)
}

func (p *MariaDBProvider) createDedicatedMariaDB(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
	mariadbName := fmt.Sprintf("%s-mariadb", site.Name)

	// Check if already exists
	existing := &mariadbv1alpha1.MariaDB{}
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
	storageSize := resource.MustParse("10Gi")
	if site.Spec.DBConfig.StorageSize != nil {
		storageSize = *site.Spec.DBConfig.StorageSize
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

	mariadb := &mariadbv1alpha1.MariaDB{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mariadbName,
			Namespace: site.Namespace,
		},
		Spec: mariadbv1alpha1.MariaDBSpec{
			RootPasswordSecretKeyRef: mariadbv1alpha1.GeneratedSecretKeyRef{
				SecretKeySelector: mariadbv1alpha1.SecretKeySelector{
					LocalObjectReference: mariadbv1alpha1.LocalObjectReference{
						Name: rootSecretName,
					},
					Key: "password",
				},
			},
			Storage: mariadbv1alpha1.Storage{
				Size: &storageSize,
			},
			Replicas: 1,
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

func (p *MariaDBProvider) ensureDatabaseCR(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, mariadbName, mariadbNamespace, dbName string) error {
	database := &mariadbv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-db", site.Name),
			Namespace: site.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, p.client, database, func() error {
		database.Spec = mariadbv1alpha1.DatabaseSpec{
			MariaDBRef: mariadbv1alpha1.MariaDBRef{
				ObjectReference: mariadbv1alpha1.ObjectReference{
					Name:      mariadbName,
					Namespace: mariadbNamespace,
				},
			},
			Name:         dbName,
			CharacterSet: "utf8mb4",
			Collate:      "utf8mb4_unicode_ci",
		}

		return controllerutil.SetControllerReference(site, database, p.scheme)
	})

	return err
}

func (p *MariaDBProvider) ensureUserCR(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, mariadbName, mariadbNamespace, dbUser string) (string, error) {
	passwordSecretName := fmt.Sprintf("%s-db-password", site.Name)

	// Create password secret if it doesn't exist
	passwordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      passwordSecretName,
			Namespace: site.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, p.client, passwordSecret, func() error {
		if passwordSecret.Data == nil {
			// Only generate password if secret doesn't exist
			passwordSecret.StringData = map[string]string{
				"password": p.generatePassword(16),
			}
		}
		return controllerutil.SetControllerReference(site, passwordSecret, p.scheme)
	})
	if err != nil {
		return "", err
	}

	// Create User CR
	user := &mariadbv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-user", site.Name),
			Namespace: site.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, p.client, user, func() error{
		user.Spec = mariadbv1alpha1.UserSpec{
			MariaDBRef: mariadbv1alpha1.MariaDBRef{
				ObjectReference: mariadbv1alpha1.ObjectReference{
					Name:      mariadbName,
					Namespace: mariadbNamespace,
				},
			},
			Name: dbUser,
			PasswordSecretKeyRef: &mariadbv1alpha1.SecretKeySelector{
				LocalObjectReference: mariadbv1alpha1.LocalObjectReference{
					Name: passwordSecretName,
				},
				Key: "password",
			},
			MaxUserConnections: 100,
		}

		return controllerutil.SetControllerReference(site, user, p.scheme)
	})

	return passwordSecretName, err
}

func (p *MariaDBProvider) ensureGrantCR(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, mariadbName, mariadbNamespace, dbName, dbUser string) error {
	grant := &mariadbv1alpha1.Grant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-grant", site.Name),
			Namespace: site.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, p.client, grant, func() error {
		grant.Spec = mariadbv1alpha1.GrantSpec{
			MariaDBRef: mariadbv1alpha1.MariaDBRef{
				ObjectReference: mariadbv1alpha1.ObjectReference{
					Name:      mariadbName,
					Namespace: mariadbNamespace,
				},
			},
			Privileges:  []string{"ALL PRIVILEGES"},
			Database:    dbName,
			Table:       "*",
			Username:    dbUser,
			GrantOption: true,
		}

		return controllerutil.SetControllerReference(site, grant, p.scheme)
	})

	return err
}

func (p *MariaDBProvider) getMariaDBConnection(ctx context.Context, mariadbName, mariadbNamespace string) (string, string, error) {
	// MariaDB Operator creates a service with the same name as the MariaDB CR
	host := fmt.Sprintf("%s.%s.svc.cluster.local", mariadbName, mariadbNamespace)
	port := "3306" // Default MariaDB port

	return host, port, nil
}

func (p *MariaDBProvider) isDatabaseReady(db *mariadbv1alpha1.Database) bool {
	for _, cond := range db.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (p *MariaDBProvider) isUserReady(user *mariadbv1alpha1.User) bool {
	for _, cond := range user.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (p *MariaDBProvider) isGrantReady(grant *mariadbv1alpha1.Grant) bool {
	for _, cond := range grant.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (p *MariaDBProvider) generateDBName(site *vyogotechv1alpha1.FrappeSite) string {
	// Generate: _<hash>_<sanitized-sitename>
	hash := p.hashString(site.Namespace + "/" + site.Name)[:8]
	safeName := p.sanitizeName(site.Spec.SiteName)
	// Limit total length to 64 characters (MySQL database name limit)
	dbName := fmt.Sprintf("_%s_%s", hash, safeName)
	if len(dbName) > 64 {
		dbName = dbName[:64]
	}
	return dbName
}

func (p *MariaDBProvider) generateDBUser(site *vyogotechv1alpha1.FrappeSite) string {
	// Generate: <sanitized-sitename>_user
	safeName := p.sanitizeName(site.Name)
	// Limit to 32 characters for MySQL username limit
	userName := fmt.Sprintf("%s_user", safeName)
	if len(userName) > 32 {
		userName = userName[:32]
	}
	return userName
}

func (p *MariaDBProvider) sanitizeName(name string) string {
	// Remove invalid characters for database/user names
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	sanitized := reg.ReplaceAllString(name, "_")
	// Remove consecutive underscores
	reg2 := regexp.MustCompile(`_{2,}`)
	sanitized = reg2.ReplaceAllString(sanitized, "_")
	// Trim underscores from edges
	sanitized = strings.Trim(sanitized, "_")
	return sanitized
}

func (p *MariaDBProvider) hashString(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum32())
}

func (p *MariaDBProvider) generatePassword(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based password if random fails
		return fmt.Sprintf("%d", metav1.Now().Unix())
	}
	return hex.EncodeToString(bytes)
}

