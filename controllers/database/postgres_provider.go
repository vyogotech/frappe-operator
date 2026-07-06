/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
you may obtain a copy of the License at

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
	"database/sql"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	_ "github.com/lib/pq"
	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PostgresProvider implements database provisioning for PostgreSQL
type PostgresProvider struct {
	config vyogotechv1.DatabaseConfig
	client client.Client
	scheme *runtime.Scheme
}

// NewPostgresProvider creates a new PostgreSQL provider
func NewPostgresProvider(config vyogotechv1.DatabaseConfig, client client.Client, scheme *runtime.Scheme) Provider {
	return &PostgresProvider{
		config: config,
		client: client,
		scheme: scheme,
	}
}

// EnsureDatabase provisions PostgreSQL database using SQL commands as superuser
func (p *PostgresProvider) EnsureDatabase(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseInfo, error) {
	logger := log.FromContext(ctx)

	// Generate database and user names
	dbName := p.generateDBName(site)
	dbUser := p.generateDBUser(site)

	// Create site password secret if it doesn't exist
	passwordSecretName := fmt.Sprintf("%s-db-password", site.Name)
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

	dbPassword := ""
	if errors.IsNotFound(err) {
		dbPassword = p.generatePassword(16)
		passwordSecret.StringData = map[string]string{
			"password": dbPassword,
		}
		if err := controllerutil.SetControllerReference(site, passwordSecret, p.scheme); err != nil {
			return nil, err
		}
		if err := p.client.Create(ctx, passwordSecret); err != nil {
			return nil, fmt.Errorf("failed to create site DB password secret: %w", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		dbPassword = string(passwordSecret.Data["password"])
	}

	// Get superuser credentials for connection
	host, port, superUser, superPassword, sslMode, err := p.getSuperuserCredentials(ctx, site)
	if err != nil {
		return nil, fmt.Errorf("failed to get superuser credentials: %w", err)
	}

	logger.Info("Provisioning PostgreSQL database",
		"host", host,
		"port", port,
		"dbName", dbName,
		"dbUser", dbUser)

	// Establish connection to Postgres default db to run DDL
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=%s&connect_timeout=10",
		superUser, superPassword, host, port, sslMode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// 1. Create User/Role if not exists
	var userExists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", dbUser).Scan(&userExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if database user exists: %w", err)
	}

	if !userExists {
		// DDL names cannot be parameterized, but dbUser is sanitized so safe from injection
		createUserSQL := fmt.Sprintf("CREATE USER %s WITH LOGIN PASSWORD '%s'", dbUser, dbPassword)
		if _, err := db.ExecContext(ctx, createUserSQL); err != nil {
			return nil, fmt.Errorf("failed to create database user: %w", err)
		}
		logger.Info("Created PostgreSQL user", "user", dbUser)
	}

	// 2. Create Database if not exists
	var dbExists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&dbExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if database exists: %w", err)
	}

	if !dbExists {
		createDBSQL := fmt.Sprintf("CREATE DATABASE %s OWNER %s", dbName, dbUser)
		if _, err := db.ExecContext(ctx, createDBSQL); err != nil {
			return nil, fmt.Errorf("failed to create database: %w", err)
		}
		logger.Info("Created PostgreSQL database", "database", dbName)
	}

	// 3. Grant privileges
	grantSQL := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", dbName, dbUser)
	if _, err := db.ExecContext(ctx, grantSQL); err != nil {
		return nil, fmt.Errorf("failed to grant database privileges: %w", err)
	}

	return &DatabaseInfo{
		Host:     host,
		Port:     port,
		Name:     dbName,
		Provider: "postgres",
	}, nil
}

// IsReady checks if the PostgreSQL database exists and is accessible
func (p *PostgresProvider) IsReady(ctx context.Context, site *vyogotechv1.FrappeSite) (bool, error) {
	dbName := p.generateDBName(site)
	host, port, superUser, superPassword, sslMode, err := p.getSuperuserCredentials(ctx, site)
	if err != nil {
		return false, err
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=%s&connect_timeout=5",
		superUser, superPassword, host, port, sslMode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return false, nil
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return false, nil
	}

	var dbExists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&dbExists)
	if err != nil {
		return false, err
	}

	return dbExists, nil
}

// GetCredentials retrieves site-specific PostgreSQL credentials
func (p *PostgresProvider) GetCredentials(ctx context.Context, site *vyogotechv1.FrappeSite) (*DatabaseCredentials, error) {
	passwordSecretName := fmt.Sprintf("%s-db-password", site.Name)
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      passwordSecretName,
		Namespace: site.Namespace,
	}
	if err := p.client.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get password secret: %w", err)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("password key not found in secret %s", passwordSecretName)
	}

	dbUser := p.generateDBUser(site)
	return &DatabaseCredentials{
		Username:   dbUser,
		Password:   string(password),
		SecretName: secret.Name,
	}, nil
}

// Cleanup drops the database and user on site deletion
func (p *PostgresProvider) Cleanup(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	logger := log.FromContext(ctx)
	dbName := p.generateDBName(site)
	dbUser := p.generateDBUser(site)

	host, port, superUser, superPassword, sslMode, err := p.getSuperuserCredentials(ctx, site)
	if err != nil {
		return err
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=%s&connect_timeout=5",
		superUser, superPassword, host, port, sslMode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil // Ignore connection errors during cleanup
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil
	}

	// Terminate active connections to the database to allow dropping it
	terminateSQL := fmt.Sprintf(`
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = '%s' AND pid <> pg_backend_pid()`, dbName)
	_, _ = db.ExecContext(ctx, terminateSQL)

	// Drop database
	dropDBSQL := fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)
	if _, err := db.ExecContext(ctx, dropDBSQL); err != nil {
		logger.Error(err, "Failed to drop PostgreSQL database", "database", dbName)
	} else {
		logger.Info("Dropped PostgreSQL database", "database", dbName)
	}

	// Drop user
	dropUserSQL := fmt.Sprintf("DROP USER IF EXISTS %s", dbUser)
	if _, err := db.ExecContext(ctx, dropUserSQL); err != nil {
		logger.Error(err, "Failed to drop PostgreSQL user", "user", dbUser)
	} else {
		logger.Info("Dropped PostgreSQL user", "user", dbUser)
	}

	return nil
}

// Helper methods

func (p *PostgresProvider) getSuperuserCredentials(ctx context.Context, site *vyogotechv1.FrappeSite) (string, string, string, string, string, error) {
	host := p.config.Host
	port := p.config.Port
	if host == "" {
		host = "frappe-postgres" // Default service name
	}
	if port == "" {
		port = "5432"
	}
	sslMode := "disable"

	username := "postgres"
	password := ""

	if p.config.ConnectionSecretRef != nil {
		secret := &corev1.Secret{}
		err := p.client.Get(ctx, types.NamespacedName{
			Name:      p.config.ConnectionSecretRef.Name,
			Namespace: site.Namespace,
		}, secret)
		if err != nil {
			return "", "", "", "", "", err
		}

		if u, ok := secret.Data["username"]; ok {
			username = string(u)
		}
		if pwd, ok := secret.Data["password"]; ok {
			password = string(pwd)
		}
		if h, ok := secret.Data["host"]; ok {
			host = string(h)
		}
		if pt, ok := secret.Data["port"]; ok {
			port = string(pt)
		}
		if ssl, ok := secret.Data["sslmode"]; ok {
			sslMode = string(ssl)
		}
	} else {
		// Fallback to default superuser secret
		secret := &corev1.Secret{}
		err := p.client.Get(ctx, types.NamespacedName{
			Name:      "frappe-postgres-superuser",
			Namespace: site.Namespace,
		}, secret)
		if err == nil {
			if u, ok := secret.Data["username"]; ok {
				username = string(u)
			}
			if pwd, ok := secret.Data["password"]; ok {
				password = string(pwd)
			}
		}
	}

	return host, port, username, password, sslMode, nil
}

func (p *PostgresProvider) generateDBName(site *vyogotechv1.FrappeSite) string {
	hash := p.hashString(site.Namespace + "/" + site.Name)[:8]
	safeName := p.sanitizeName(site.Spec.SiteName)
	// PostgreSQL identifiers should start with a letter/underscore and are case-insensitive
	dbName := fmt.Sprintf("_%s_%s", hash, safeName)
	if len(dbName) > 63 {
		dbName = dbName[:63]
	}
	return strings.ToLower(dbName)
}

func (p *PostgresProvider) generateDBUser(site *vyogotechv1.FrappeSite) string {
	return p.generateDBName(site)
}

func (p *PostgresProvider) sanitizeName(name string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	sanitized := reg.ReplaceAllString(name, "_")
	reg2 := regexp.MustCompile(`_{2,}`)
	sanitized = reg2.ReplaceAllString(sanitized, "_")
	sanitized = strings.Trim(sanitized, "_")
	return sanitized
}

func (p *PostgresProvider) hashString(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}

func (p *PostgresProvider) generatePassword(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", metav1.Now().Unix())
	}
	return hex.EncodeToString(bytes)
}
