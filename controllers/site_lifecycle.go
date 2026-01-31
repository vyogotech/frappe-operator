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

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureAdminPassword gets or generates the admin password for the site
func (r *FrappeSiteReconciler) ensureAdminPassword(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, error) {
	logger := log.FromContext(ctx)
	var adminPassword string
	var adminPasswordSecret *corev1.Secret

	if site.Spec.AdminPasswordSecretRef != nil {
		// Fetch from provided secret
		adminPasswordSecret = &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      site.Spec.AdminPasswordSecretRef.Name,
			Namespace: site.Spec.AdminPasswordSecretRef.Namespace,
		}
		if secretKey.Namespace == "" {
			secretKey.Namespace = site.Namespace
		}
		err := r.Get(ctx, secretKey, adminPasswordSecret)
		if err != nil {
			return "", fmt.Errorf("failed to get admin password secret: %w", err)
		}
		adminPassword = string(adminPasswordSecret.Data["password"])
		logger.Info("Using provided admin password", "secret", site.Spec.AdminPasswordSecretRef.Name)
	} else {
		// Check if we already generated a secret
		generatedSecretName := fmt.Sprintf("%s-admin", site.Name)
		adminPasswordSecret = &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      generatedSecretName,
			Namespace: site.Namespace,
		}, adminPasswordSecret)

		if err != nil && !errors.IsNotFound(err) {
			return "", fmt.Errorf("failed to check for generated secret: %w", err)
		}

		if errors.IsNotFound(err) {
			// Generate new random password
			adminPassword = r.generatePassword(16)

			// Create secret to store it
			adminPasswordSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generatedSecretName,
					Namespace: site.Namespace,
					Labels: map[string]string{
						"app":  "frappe",
						"site": site.Name,
					},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"password": []byte(adminPassword),
				},
			}

			if err := controllerutil.SetControllerReference(site, adminPasswordSecret, r.Scheme); err != nil {
				return "", err
			}

			if err := r.Create(ctx, adminPasswordSecret); err != nil {
				return "", fmt.Errorf("failed to create admin password secret: %w", err)
			}

			logger.Info("Generated admin password", "secret", generatedSecretName)
		} else {
			// Use existing generated password
			adminPassword = string(adminPasswordSecret.Data["password"])
			logger.Info("Using existing generated password", "secret", generatedSecretName)
		}
	}
	return adminPassword, nil
}

// ensureInitSecrets creates a Secret containing all initialization credentials
func (r *FrappeSiteReconciler) ensureInitSecrets(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string, dbInfo *database.DatabaseInfo, dbCreds *database.DatabaseCredentials, adminPassword string) error {
	logger := log.FromContext(ctx)

	secretName := fmt.Sprintf("%s-init-secrets", site.Name)

	// Get DB_PROVIDER from database info
	dbProvider := "mariadb" // default
	if site.Spec.DBConfig.Provider != "" {
		dbProvider = site.Spec.DBConfig.Provider
	}

	// Get apps to install if specified
	appsToInstall := ""
	if len(site.Spec.Apps) > 0 {
		var validApps []string
		for _, app := range site.Spec.Apps {
			isValid := true
			for _, char := range app {
				if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
					(char >= '0' && char <= '9') || char == '_' || char == '-') {
					isValid = false
					break
				}
			}

			if !isValid {
				r.Recorder.Event(site, corev1.EventTypeWarning, "InvalidAppName",
					fmt.Sprintf("App '%s' contains invalid characters and will be skipped", app))
			} else {
				validApps = append(validApps, app)
			}
		}

		if len(validApps) > 0 {
			appsToInstall = strings.Join(validApps, " ")
			r.Recorder.Event(site, corev1.EventTypeNormal, "AppsRequested",
				fmt.Sprintf("Requested %d app(s): %v - will check availability in container", len(validApps), validApps))
		}
	}

	// Build secret data with all credentials as individual files
	secretData := map[string][]byte{
		"site_name":       []byte(site.Spec.SiteName),
		"domain":          []byte(domain),
		"admin_password":  []byte(adminPassword),
		"bench_name":      []byte(bench.Name),
		"db_provider":     []byte(dbProvider),
		"apps_to_install": []byte(appsToInstall),
	}

	// Add database credentials
	if dbInfo != nil {
		secretData["db_host"] = []byte(dbInfo.Host)
		secretData["db_port"] = []byte(dbInfo.Port)
		secretData["db_name"] = []byte(dbInfo.Name)
	}
	if dbCreds != nil {
		secretData["db_user"] = []byte(dbCreds.Username)
		secretData["db_password"] = []byte(dbCreds.Password)
	}

	// Create or update the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: site.Namespace,
			Labels: map[string]string{
				"app":  "frappe",
				"site": site.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}

	if err := controllerutil.SetControllerReference(site, secret, r.Scheme); err != nil {
		return err
	}

	var existing corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, &existing)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, secret); err != nil {
			return err
		}
		logger.Info("Created initialization secret", "secret", secretName)
	} else if err != nil {
		return err
	} else {
		existing.Data = secretData
		if err := r.Update(ctx, &existing); err != nil {
			return err
		}
		logger.Info("Updated initialization secret", "secret", secretName)
	}

	return nil
}

// resolveDBConfig merges site-specific database configuration with bench-level defaults
func (r *FrappeSiteReconciler) resolveDBConfig(site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench) vyogotechv1alpha1.DatabaseConfig {
	config := site.Spec.DBConfig

	if bench.Spec.DBConfig == nil {
		if config.Provider == "" {
			config.Provider = "mariadb"
		}
		return config
	}

	if config.Provider == "" {
		config.Provider = bench.Spec.DBConfig.Provider
	}
	if config.Mode == "" {
		config.Mode = bench.Spec.DBConfig.Mode
	}
	if config.MariaDBRef == nil {
		config.MariaDBRef = bench.Spec.DBConfig.MariaDBRef
	}
	if config.PostgresRef == nil {
		config.PostgresRef = bench.Spec.DBConfig.PostgresRef
	}
	if config.Host == "" {
		config.Host = bench.Spec.DBConfig.Host
	}
	if config.Port == "" {
		config.Port = bench.Spec.DBConfig.Port
	}
	if config.ConnectionSecretRef == nil {
		config.ConnectionSecretRef = bench.Spec.DBConfig.ConnectionSecretRef
	}
	if config.StorageSize == nil {
		config.StorageSize = bench.Spec.DBConfig.StorageSize
	}
	if config.Resources == nil {
		config.Resources = bench.Spec.DBConfig.Resources
	}

	return config
}

// resolveDomain determines the final domain for the site with priority-based resolution
func (r *FrappeSiteReconciler) resolveDomain(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench) (string, string) {
	if site.Spec.Domain != "" {
		return site.Spec.Domain, "explicit"
	}

	if bench.Spec.DomainConfig != nil && bench.Spec.DomainConfig.Suffix != "" {
		domain := site.Spec.SiteName + bench.Spec.DomainConfig.Suffix
		return domain, "bench-suffix"
	}

	autoDetect := true
	if bench.Spec.DomainConfig != nil && bench.Spec.DomainConfig.AutoDetect != nil {
		autoDetect = *bench.Spec.DomainConfig.AutoDetect
	}

	if autoDetect {
		detector := &DomainDetector{Client: r.Client}
		suffix, err := detector.DetectDomainSuffix(ctx, site.Namespace)
		if err == nil && suffix != "" {
			domain := site.Spec.SiteName + suffix
			return domain, "auto-detected"
		}
	}

	return site.Spec.SiteName, "sitename-default"
}

// getMariaDBRootCredentials retrieves root credentials for database operations
func (r *FrappeSiteReconciler) getMariaDBRootCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
	if site.Spec.DBConfig.Mode == "dedicated" {
		secretName := fmt.Sprintf("%s-mariadb-root", site.Name)
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, secret)
		if err != nil {
			return "", "", fmt.Errorf("failed to get dedicated MariaDB root secret %s: %w", secretName, err)
		}
		password, ok := secret.Data["password"]
		if !ok {
			return "", "", fmt.Errorf("password key not found in secret %s", secretName)
		}
		return "root", string(password), nil
	}

	if site.Spec.DBConfig.Mode == "shared" {
		mariadbName := "frappe-mariadb"
		mariadbNamespace := site.Namespace
		if site.Spec.DBConfig.MariaDBRef != nil {
			mariadbName = site.Spec.DBConfig.MariaDBRef.Name
			if site.Spec.DBConfig.MariaDBRef.Namespace != "" {
				mariadbNamespace = site.Spec.DBConfig.MariaDBRef.Namespace
			}
		}

		mariadbCR := &unstructured.Unstructured{}
		mariadbCR.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "k8s.mariadb.com",
			Version: "v1alpha1",
			Kind:    "MariaDB",
		})
		err := r.Get(ctx, types.NamespacedName{Name: mariadbName, Namespace: mariadbNamespace}, mariadbCR)
		if err != nil {
			return "", "", err
		}

		spec, _, _ := unstructured.NestedMap(mariadbCR.Object, "spec")
		rootPasswordRef, _, _ := unstructured.NestedMap(spec, "rootPasswordSecretKeyRef")
		rootSecretName, _, _ := unstructured.NestedString(rootPasswordRef, "name")
		rootSecretKey, found, _ := unstructured.NestedString(rootPasswordRef, "key")
		if !found {
			rootSecretKey = "password"
		}

		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: rootSecretName, Namespace: mariadbNamespace}, secret)
		if err != nil {
			return "", "", fmt.Errorf("failed to get MariaDB root secret %s: %w", rootSecretName, err)
		}

		password, ok := secret.Data[rootSecretKey]
		if !ok {
			return "", "", fmt.Errorf("key %s not found in secret %s", rootSecretKey, rootSecretName)
		}
		return "root", string(password), nil
	}

	return "", "", fmt.Errorf("unsupported database mode: %s", site.Spec.DBConfig.Mode)
}

// getRequeueAttempt returns the current requeue attempt from the site annotation
func (r *FrappeSiteReconciler) getRequeueAttempt(site *vyogotechv1alpha1.FrappeSite) int {
	if site.Annotations == nil {
		return 0
	}
	v, ok := site.Annotations[requeueAttemptAnnotation]
	if !ok {
		return 0
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	if i < 0 {
		return 0
	}
	return i
}

// patchRequeueAttempt sets the requeue-attempt annotation on the site
func (r *FrappeSiteReconciler) patchRequeueAttempt(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, nextAttempt int) error {
	siteCopy := site.DeepCopy()
	if siteCopy.Annotations == nil {
		siteCopy.Annotations = make(map[string]string)
	}
	siteCopy.Annotations[requeueAttemptAnnotation] = strconv.Itoa(nextAttempt)
	return r.Patch(ctx, siteCopy, client.MergeFrom(site))
}

// getPodSecurityContext returns the pod-level security context (shared logic in security_context.go)
func (r *FrappeSiteReconciler) getPodSecurityContext(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) *corev1.PodSecurityContext {
	return PodSecurityContextForBench(ctx, r.Client, r.IsOpenShift, bench.Namespace, bench.Spec.Security)
}

// getContainerSecurityContext returns the container-level security context (shared logic in security_context.go)
func (r *FrappeSiteReconciler) getContainerSecurityContext(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) *corev1.SecurityContext {
	return ContainerSecurityContextForBench(r.IsOpenShift, bench.Spec.Security)
}
