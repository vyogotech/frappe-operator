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
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const frappeSiteFinalizer = "vyogo.tech/site-finalizer"

// FrappeSiteReconciler reconciles a FrappeSite object
type FrappeSiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;ingressclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets;services;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.mariadb.com,resources=mariadbs;databases;users;grants,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *FrappeSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	site := &vyogotechv1alpha1.FrappeSite{}
	if err := r.Get(ctx, req.NamespacedName, site); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling FrappeSite", "site", site.Name, "siteName", site.Spec.SiteName)

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(site, frappeSiteFinalizer) {
		controllerutil.AddFinalizer(site, frappeSiteFinalizer)
		if err := r.Update(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set progressing condition
	r.setCondition(site, metav1.Condition{
		Type:    "Progressing",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciling",
		Message: "Starting site reconciliation",
	})
	if err := r.updateStatus(ctx, site); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if site.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(site, frappeSiteFinalizer) {
			logger.Info("Deleting site", "site", site.Name)

			// Set deletion condition
			r.setCondition(site, metav1.Condition{
				Type:    "Terminating",
				Status:  metav1.ConditionTrue,
				Reason:  "Deleting",
				Message: "Site is being deleted",
			})

			// Implement site deletion job
			if err := r.deleteSite(ctx, site); err != nil {
				logger.Error(err, "Failed to delete site")
				r.setCondition(site, metav1.Condition{
					Type:    "Degraded",
					Status:  metav1.ConditionTrue,
					Reason:  "DeletionFailed",
					Message: fmt.Sprintf("Site deletion failed: %v", err),
				})
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(site, frappeSiteFinalizer)
			if err := r.Update(ctx, site); err != nil {
				return ctrl.Result{}, err
			}

		}
		return ctrl.Result{}, nil
	}

	// Validate benchRef
	if site.Spec.BenchRef == nil {
		logger.Error(nil, "BenchRef is required")
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
		r.setCondition(site, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "ValidationFailed",
			Message: "benchRef is required",
		})
		r.setCondition(site, metav1.Condition{
			Type:    "Degraded",
			Status:  metav1.ConditionTrue,
			Reason:  "ValidationFailed",
			Message: "benchRef is required",
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, fmt.Errorf("benchRef is required")
	}

	// Get the referenced bench
	bench := &vyogotechv1alpha1.FrappeBench{}
	benchKey := types.NamespacedName{
		Name:      site.Spec.BenchRef.Name,
		Namespace: site.Spec.BenchRef.Namespace,
	}
	if benchKey.Namespace == "" {
		benchKey.Namespace = site.Namespace
	}

	if err := r.Get(ctx, benchKey, bench); err != nil {
		logger.Error(err, "Failed to get referenced bench", "bench", benchKey.Name)
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhasePending
		site.Status.BenchReady = false
		r.setCondition(site, metav1.Condition{
			Type:    "BenchReady",
			Status:  metav1.ConditionFalse,
			Reason:  "BenchNotFound",
			Message: fmt.Sprintf("Failed to get referenced bench: %v", err),
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if bench is ready
	if bench.Status.Phase != "Ready" {
		logger.Info("Referenced bench is not ready yet", "bench", bench.Name, "phase", bench.Status.Phase)
		site.Status.BenchReady = false
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhasePending
		r.setCondition(site, metav1.Condition{
			Type:    "BenchReady",
			Status:  metav1.ConditionFalse,
			Reason:  "BenchNotReady",
			Message: fmt.Sprintf("Bench %s is not ready (phase: %s)", bench.Name, bench.Status.Phase),
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	site.Status.BenchReady = true
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseProvisioning
	r.setCondition(site, metav1.Condition{
		Type:    "BenchReady",
		Status:  metav1.ConditionTrue,
		Reason:  "BenchReady",
		Message: "Referenced bench is ready",
	})

	// Resolve the final domain for the site (with smart auto-detection)
	domain, domainSource := r.resolveDomain(ctx, site, bench)

	// Update status with resolved domain
	site.Status.ResolvedDomain = domain
	site.Status.DomainSource = domainSource

	// Resolve DB config (merging site and bench defaults)
	dbConfig := r.resolveDBConfig(site, bench)

	// 0. Provision database using database provider
	dbProvider, err := database.NewProvider(dbConfig, r.Client, r.Scheme)
	if err != nil {
		logger.Error(err, "Failed to create database provider")
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{}, err
	}

	// Check if database is ready
	dbReady, err := dbProvider.IsReady(ctx, site)
	if err != nil {
		logger.Error(err, "Failed to check database readiness")
		site.Status.DatabaseReady = false
		r.setCondition(site, metav1.Condition{
			Type:    "DatabaseReady",
			Status:  metav1.ConditionFalse,
			Reason:  "DatabaseCheckFailed",
			Message: fmt.Sprintf("Failed to check database readiness: %v", err),
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	if !dbReady {
		logger.Info("Database not ready, provisioning...")
		site.Status.DatabaseReady = false
		r.setCondition(site, metav1.Condition{
			Type:    "DatabaseReady",
			Status:  metav1.ConditionFalse,
			Reason:  "Provisioning",
			Message: "Database is being provisioned",
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}

		// Ensure database resources are created
		dbInfo, err := dbProvider.EnsureDatabase(ctx, site)
		if err != nil {
			logger.Error(err, "Failed to ensure database")
			site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
			r.setCondition(site, metav1.Condition{
				Type:    "DatabaseReady",
				Status:  metav1.ConditionFalse,
				Reason:  "ProvisioningFailed",
				Message: fmt.Sprintf("Database provisioning failed: %v", err),
			})
			if err := r.updateStatus(ctx, site); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		logger.Info("Database provisioning initiated",
			"provider", dbInfo.Provider,
			"dbName", dbInfo.Name)

		// Requeue to check readiness
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Database is ready - get credentials
	site.Status.DatabaseReady = true
	r.setCondition(site, metav1.Condition{
		Type:    "DatabaseReady",
		Status:  metav1.ConditionTrue,
		Reason:  "DatabaseReady",
		Message: "Database is ready and accessible",
	})

	dbInfo, err := dbProvider.EnsureDatabase(ctx, site)
	if err != nil {
		return ctrl.Result{}, err
	}

	dbCreds, err := dbProvider.GetCredentials(ctx, site)
	if err != nil {
		logger.Error(err, "Failed to get database credentials")
		return ctrl.Result{}, err
	}

	// Update status with database info
	site.Status.DatabaseName = dbInfo.Name
	site.Status.DatabaseCredentialsSecret = dbCreds.SecretName
	if err := r.updateStatus(ctx, site); err != nil {
		return ctrl.Result{}, err
	}

	// 1. Ensure site is initialized with database credentials
	siteReady, err := r.ensureSiteInitialized(ctx, site, bench, domain, dbInfo, dbCreds)
	if err != nil {
		logger.Error(err, "Failed to initialize site")
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
		r.setCondition(site, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteInitializationFailed",
			Message: fmt.Sprintf("Site initialization failed: %v", err),
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	if !siteReady {
		logger.Info("Site initialization in progress", "site", site.Name)
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseProvisioning
		r.setCondition(site, metav1.Condition{
			Type:    "Progressing",
			Status:  metav1.ConditionTrue,
			Reason:  "SiteInitializing",
			Message: "Site initialization is in progress",
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// 2. Ensure Ingress (enabled by default, can be disabled)
	createIngress := true
	if site.Spec.Ingress != nil && site.Spec.Ingress.Enabled != nil && !*site.Spec.Ingress.Enabled {
		createIngress = false
		logger.Info("Ingress creation disabled by user", "site", site.Name)
	}

	if createIngress {
		// Check if we're on OpenShift and should create Routes instead
		if r.isOpenShiftPlatform(ctx) && (site.Spec.RouteConfig == nil || site.Spec.RouteConfig.Enabled == nil || *site.Spec.RouteConfig.Enabled) {
			if err := r.ensureRoute(ctx, site, bench, domain); err != nil {
				logger.Error(err, "Failed to ensure Route")
				return ctrl.Result{}, err
			}
		} else {
			if err := r.ensureIngress(ctx, site, bench, domain); err != nil {
				logger.Error(err, "Failed to ensure Ingress")
				return ctrl.Result{}, err
			}
		}
	}

	// 3. Update final status
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseReady
	site.Status.SiteURL = fmt.Sprintf("http://%s", domain)
	if site.Spec.TLS.Enabled {
		site.Status.SiteURL = fmt.Sprintf("https://%s", domain)
	}

	r.setCondition(site, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "SiteReady",
		Message: fmt.Sprintf("Site is ready at %s", site.Status.SiteURL),
	})
	r.setCondition(site, metav1.Condition{
		Type:    "Progressing",
		Status:  metav1.ConditionFalse,
		Reason:  "Complete",
		Message: "Site provisioning is complete",
	})

	if err := r.updateStatus(ctx, site); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("FrappeSite reconciled successfully", "site", site.Name, "domain", domain)
	return ctrl.Result{}, nil
}

// setCondition sets a condition on the FrappeSite
func (r *FrappeSiteReconciler) setCondition(site *vyogotechv1alpha1.FrappeSite, condition metav1.Condition) {
	condition.LastTransitionTime = metav1.Now()
	condition.ObservedGeneration = site.Generation

	// Find existing condition
	for i := range site.Status.Conditions {
		if site.Status.Conditions[i].Type == condition.Type {
			// Only update if something changed
			if site.Status.Conditions[i].Status != condition.Status ||
				site.Status.Conditions[i].Reason != condition.Reason ||
				site.Status.Conditions[i].Message != condition.Message {
				site.Status.Conditions[i] = condition
			}
			return
		}
	}

	// Add new condition
	site.Status.Conditions = append(site.Status.Conditions, condition)
}

// updateStatus updates the FrappeSite status with proper error handling
func (r *FrappeSiteReconciler) updateStatus(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	if err := r.Status().Update(ctx, site); err != nil {
		if errors.IsConflict(err) {
			// Requeue on conflict
			return fmt.Errorf("status update conflict, will requeue: %w", err)
		}
		return fmt.Errorf("failed to update status: %w", err)
	}
	return nil
}

// resolveDBConfig merges site-specific database configuration with bench-level defaults
func (r *FrappeSiteReconciler) resolveDBConfig(site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench) vyogotechv1alpha1.DatabaseConfig {
	config := site.Spec.DBConfig

	if bench.Spec.DBConfig == nil {
		return config
	}

	// Use bench-level defaults for any empty fields in site config
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
	logger := log.FromContext(ctx)

	// Priority 1: Explicit domain in FrappeSite spec
	if site.Spec.Domain != "" {
		logger.Info("Using explicit domain from FrappeSite spec", "domain", site.Spec.Domain)
		return site.Spec.Domain, "explicit"
	}

	// Priority 2: Bench domain config with suffix
	if bench.Spec.DomainConfig != nil && bench.Spec.DomainConfig.Suffix != "" {
		domain := site.Spec.SiteName + bench.Spec.DomainConfig.Suffix
		logger.Info("Using bench domain suffix", "domain", domain, "suffix", bench.Spec.DomainConfig.Suffix)
		return domain, "bench-suffix"
	}

	// Priority 3: Auto-detect from Ingress Controller (if enabled)
	autoDetect := true
	if bench.Spec.DomainConfig != nil && bench.Spec.DomainConfig.AutoDetect != nil {
		autoDetect = *bench.Spec.DomainConfig.AutoDetect
	}

	if autoDetect {
		detector := &DomainDetector{Client: r.Client}
		suffix, err := detector.DetectDomainSuffix(ctx, site.Namespace)
		if err == nil && suffix != "" {
			// Skip auto-detection for local domains
			if !isLocalDomain(site.Spec.SiteName) {
				domain := site.Spec.SiteName + suffix
				logger.Info("Auto-detected domain suffix", "domain", domain, "suffix", suffix)
				return domain, "auto-detected"
			}
		}
		logger.V(1).Info("Auto-detection skipped or failed, falling back to siteName", "error", err)
	}

	// Priority 4: Use siteName as-is (for .local, .localhost, etc.)
	logger.Info("Using siteName as final domain", "domain", site.Spec.SiteName)
	return site.Spec.SiteName, "sitename-default"
}

// ensureSiteInitialized creates a Job to run bench new-site
func (r *FrappeSiteReconciler) ensureSiteInitialized(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string, dbInfo *database.DatabaseInfo, dbCreds *database.DatabaseCredentials) (bool, error) {
	logger := log.FromContext(ctx)

	jobName := fmt.Sprintf("%s-init", site.Name)
	job := &batchv1.Job{}

	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err == nil {
		// Job exists, check if it completed
		if job.Status.Succeeded > 0 {
			logger.Info("Site initialization job completed", "job", jobName)
			return true, nil
		}
		if job.Status.Failed > 0 {
			logger.Error(nil, "Site initialization job failed", "job", jobName)
			return false, fmt.Errorf("site initialization job failed")
		}
		// Job is still running
		return false, nil
	}

	if !errors.IsNotFound(err) {
		return false, err
	}

	// Create the initialization job
	logger.Info("Creating site initialization job",
		"job", jobName,
		"domain", domain,
		"dbProvider", dbInfo.Provider,
		"dbName", dbInfo.Name)

	// Database credentials are provided by the database provider (secure, no hardcoded values)
	dbHost := dbInfo.Host
	dbPort := dbInfo.Port
	dbName := dbInfo.Name
	dbUser := dbCreds.Username
	dbPassword := dbCreds.Password
	dbProvider := dbInfo.Provider

	// Get or generate admin password
	var adminPassword string
	var adminPasswordSecret *corev1.Secret

	if site.Spec.AdminPasswordSecretRef != nil {
		// Fetch from provided secret
		adminPasswordSecret = &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      site.Spec.AdminPasswordSecretRef.Name,
			Namespace: site.Spec.AdminPasswordSecretRef.Namespace,
		}, adminPasswordSecret)
		if err != nil {
			return false, fmt.Errorf("failed to get admin password secret: %w", err)
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
			return false, fmt.Errorf("failed to check for generated secret: %w", err)
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
				return false, err
			}

			if err := r.Create(ctx, adminPasswordSecret); err != nil {
				return false, fmt.Errorf("failed to create admin password secret: %w", err)
			}

			logger.Info("Generated admin password", "secret", generatedSecretName)
		} else {
			// Use existing generated password
			adminPassword = string(adminPasswordSecret.Data["password"])
			logger.Info("Using existing generated password", "secret", generatedSecretName)
		}
	}

	// Create the init script using environment variables to prevent shell injection
	initScript := `#!/bin/bash
set -e

cd /home/frappe/frappe-bench

echo "Creating Frappe site: $SITE_NAME"
echo "Domain: $DOMAIN"

# Validate environment variables exist and are not empty
if [[ -z "$SITE_NAME" || -z "$DOMAIN" || -z "$ADMIN_PASSWORD" || -z "$BENCH_NAME" || -z "$DB_PROVIDER" ]]; then
    echo "ERROR: Required environment variables not set"
    exit 1
fi

# Run bench new-site with provider-specific database configuration
if [[ "$DB_PROVIDER" == "mariadb" ]] || [[ "$DB_PROVIDER" == "postgres" ]]; then
    # For MariaDB and PostgreSQL: use pre-provisioned database with dedicated credentials
    if [[ -z "$DB_HOST" || -z "$DB_PORT" || -z "$DB_NAME" || -z "$DB_USER" || -z "$DB_PASSWORD" ]]; then
        echo "ERROR: Database connection variables not set for $DB_PROVIDER"
        exit 1
    fi

    echo "Creating site with $DB_PROVIDER database (pre-provisioned)"
    
    # Check if bench version supports --db-user flag
    DB_USER_FLAG=""
    if bench new-site --help | grep -q " --db-user"; then
        echo "Detected support for --db-user flag"
        DB_USER_FLAG="--db-user=$DB_USER"
    elif [[ "$DB_USER" != "$DB_NAME" ]]; then
        echo "WARNING: Your bench version does not support --db-user. Using DB_NAME as username."
    else
        echo "Bench version does not support --db-user, but DB_USER matches DB_NAME. Proceeding."
    fi

    bench new-site \
      --db-type="$DB_PROVIDER" \
      --db-name="$DB_NAME" \
      --db-host="$DB_HOST" \
      --db-port="$DB_PORT" \
      $DB_USER_FLAG \
      --db-password="$DB_PASSWORD" \
      --no-setup-db \
      --admin-password="$ADMIN_PASSWORD" \
      --install-app=erpnext \
      --verbose \
      "$SITE_NAME"

elif [[ "$DB_PROVIDER" == "sqlite" ]]; then
    # For SQLite: file-based database, no external connection needed
    echo "Creating site with SQLite database (file-based)"
    bench new-site "$SITE_NAME" \
      --db-type=sqlite \
      --admin-password="$ADMIN_PASSWORD" \
      --install-app=erpnext \
      --verbose

else
    echo "ERROR: Unsupported database provider: $DB_PROVIDER"
    exit 1
fi

echo "Site $SITE_NAME created successfully!"

# Update site_config.json with domain and Redis configuration using Python
echo "Updating site_config.json with domain and Redis"
python3 << 'PYTHON_SCRIPT'
import json
import os

# Get values from environment variables
site_name = os.environ['SITE_NAME']
domain = os.environ['DOMAIN']
bench_name = os.environ['BENCH_NAME']

site_path = f"/home/frappe/frappe-bench/sites/{site_name}"
config_file = os.path.join(site_path, "site_config.json")

# Read existing config
with open(config_file, 'r') as f:
    config = json.load(f)

# Update with resolved domain
config['host_name'] = domain

# Add Redis configuration for this site
config['redis_cache'] = f"redis://{bench_name}-redis-cache:6379"
config['redis_queue'] = f"redis://{bench_name}-redis-queue:6379"

# Write back
with open(config_file, 'w') as f:
    json.dump(config, f, indent=2)

print(f"Updated site_config.json for domain: {domain}")
print(f"Redis cache: {bench_name}-redis-cache:6379")
print(f"Redis queue: {bench_name}-redis-queue:6379")
PYTHON_SCRIPT

echo "Site initialization complete!"
`

	// Get bench PVC name
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: site.Namespace,
			Labels: map[string]string{
				"app":  "frappe",
				"site": site.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:   corev1.RestartPolicyNever,
					SecurityContext: r.getPodSecurityContext(bench),
					Containers: []corev1.Container{
						{
							Name:    "site-init",
							Image:   r.getBenchImage(bench),
							Command: []string{"bash", "-c"},
							Args:    []string{initScript},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
							SecurityContext: r.getContainerSecurityContext(bench),
							Env: []corev1.EnvVar{
								{
									Name:  "SITE_NAME",
									Value: site.Spec.SiteName,
								},
								{
									Name:  "DOMAIN",
									Value: domain,
								},
								{
									Name:  "DB_PROVIDER",
									Value: dbProvider,
								},
								{
									Name:  "DB_HOST",
									Value: dbHost,
								},
								{
									Name:  "DB_PORT",
									Value: dbPort,
								},
								{
									Name:  "DB_NAME",
									Value: dbName,
								},
								{
									Name:  "DB_USER",
									Value: dbUser,
								},
								{
									Name:  "DB_PASSWORD",
									Value: dbPassword,
								},
								{
									Name:  "ADMIN_PASSWORD",
									Value: adminPassword,
								},
								{
									Name:  "BENCH_NAME",
									Value: bench.Name,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(site, job, r.Scheme); err != nil {
		return false, err
	}

	if err := r.Create(ctx, job); err != nil {
		return false, err
	}

	logger.Info("Site initialization job created", "job", jobName)
	return false, nil // Not ready yet, job is running
}

// ensureIngress creates an Ingress for the site
func (r *FrappeSiteReconciler) ensureIngress(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string) error {
	logger := log.FromContext(ctx)

	ingressName := fmt.Sprintf("%s-ingress", site.Name)
	ingress := &networkingv1.Ingress{}

	err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: site.Namespace}, ingress)
	if err == nil {
		logger.Info("Ingress already exists", "ingress", ingressName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Ingress", "ingress", ingressName, "domain", domain)

	// Determine ingress class
	ingressClassName := "nginx" // Default
	if site.Spec.IngressClassName != "" {
		ingressClassName = site.Spec.IngressClassName
	}

	// Validate IngressClass exists and warn if missing
	ingressClass := &networkingv1.IngressClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: ingressClassName}, ingressClass); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("IngressClass not found - Ingress will be created but may not work until controller is installed",
				"class", ingressClassName,
				"hint", "Install NGINX Ingress Controller: kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/cloud/deploy.yaml")
		} else {
			logger.Error(err, "Failed to check IngressClass", "class", ingressClassName)
		}
	}

	pathType := networkingv1.PathTypePrefix
	nginxSvcName := fmt.Sprintf("%s-nginx", bench.Name)

	ingress = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: site.Namespace,
			Labels: map[string]string{
				"app":  "frappe",
				"site": site.Name,
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "100m",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: domain,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: nginxSvcName,
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Add TLS if enabled
	if site.Spec.TLS.Enabled {
		tlsSecretName := site.Spec.TLS.SecretName
		if tlsSecretName == "" {
			tlsSecretName = fmt.Sprintf("%s-tls", site.Name)
		}

		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{domain},
				SecretName: tlsSecretName,
			},
		}

		// Add cert-manager annotation if issuer is specified
		if site.Spec.TLS.Issuer != "" {
			if ingress.Annotations == nil {
				ingress.Annotations = make(map[string]string)
			}
			ingress.Annotations["cert-manager.io/cluster-issuer"] = site.Spec.TLS.Issuer
		}
	}

	// Merge additional annotations from site spec
	if site.Spec.Ingress != nil && site.Spec.Ingress.Annotations != nil {
		if ingress.Annotations == nil {
			ingress.Annotations = make(map[string]string)
		}
		for k, v := range site.Spec.Ingress.Annotations {
			ingress.Annotations[k] = v
		}
	}

	if err := controllerutil.SetControllerReference(site, ingress, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, ingress)
}

// getBenchImage returns the image to use from the bench
func (r *FrappeSiteReconciler) getBenchImage(bench *vyogotechv1alpha1.FrappeBench) string {
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		image := bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			image = fmt.Sprintf("%s:%s", image, bench.Spec.ImageConfig.Tag)
		}
		return image
	}
	// Use bench's FrappeVersion
	return fmt.Sprintf("frappe/erpnext:%s", bench.Spec.FrappeVersion)
}

// isLocalDomain checks if a domain is a local development domain
func isLocalDomain(domain string) bool {
	return strings.HasSuffix(domain, ".local") ||
		strings.HasSuffix(domain, ".localhost") ||
		domain == "localhost"
}

// generatePassword generates a random password of specified length
func (r *FrappeSiteReconciler) generatePassword(length int) string {
	// Use alphanumeric only to avoid bash escaping issues
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	password := make([]byte, length)
	for i := range password {
		// Use crypto/rand for secure random generation
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback to timestamp-based if crypto/rand fails (shouldn't happen)
			password[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		} else {
			password[i] = charset[n.Int64()]
		}
	}
	return string(password)
}

// isOpenShiftPlatform checks if we're running on OpenShift
func (r *FrappeSiteReconciler) isOpenShiftPlatform(ctx context.Context) bool {
	// Try to list Routes to check if API is available
	routeList := &routev1.RouteList{}
	err := r.List(ctx, routeList)

	// If we can list Routes successfully, we're on OpenShift
	return err == nil
}

// ensureRoute creates an OpenShift Route for the site
func (r *FrappeSiteReconciler) ensureRoute(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string) error {
	logger := log.FromContext(ctx)

	routeName := fmt.Sprintf("%s-route", site.Name)
	route := &routev1.Route{}

	err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: site.Namespace}, route)
	if err == nil {
		logger.Info("Route already exists", "route", routeName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating OpenShift Route", "route", routeName, "domain", domain)

	nginxSvcName := fmt.Sprintf("%s-nginx", bench.Name)

	// Determine TLS termination
	tlsTermination := routev1.TLSTerminationEdge
	if site.Spec.RouteConfig != nil && site.Spec.RouteConfig.TLSTermination != "" {
		switch site.Spec.RouteConfig.TLSTermination {
		case "passthrough":
			tlsTermination = routev1.TLSTerminationPassthrough
		case "reencrypt":
			tlsTermination = routev1.TLSTerminationReencrypt
		}
	}

	route = &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: site.Namespace,
			Labels: map[string]string{
				"app":  "frappe",
				"site": site.Name,
			},
		},
		Spec: routev1.RouteSpec{
			Host: domain,
			Path: "",
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: nginxSvcName,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(8080),
			},
			TLS: &routev1.TLSConfig{
				Termination:                   tlsTermination,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
			WildcardPolicy: routev1.WildcardPolicyNone,
		},
	}

	// Add TLS certificate if specified
	if site.Spec.TLS.Enabled {
		if site.Spec.TLS.SecretName != "" {
			route.Spec.TLS.Certificate = "" // Will be set by certificate controller
			route.Spec.TLS.Key = ""
		}
	}

	// Add additional annotations from site spec
	if site.Spec.RouteConfig != nil && site.Spec.RouteConfig.Annotations != nil {
		if route.Annotations == nil {
			route.Annotations = make(map[string]string)
		}
		for k, v := range site.Spec.RouteConfig.Annotations {
			route.Annotations[k] = v
		}
	}

	if err := controllerutil.SetControllerReference(site, route, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, route); err != nil {
		return fmt.Errorf("failed to create Route: %w", err)
	}

	return nil
}

// deleteSite implements the site deletion logic
func (r *FrappeSiteReconciler) deleteSite(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	logger := log.FromContext(ctx)

	// Get the referenced bench
	bench := &vyogotechv1alpha1.FrappeBench{}
	benchKey := types.NamespacedName{
		Name:      site.Spec.BenchRef.Name,
		Namespace: site.Spec.BenchRef.Namespace,
	}
	if benchKey.Namespace == "" {
		benchKey.Namespace = site.Namespace
	}

	if err := r.Get(ctx, benchKey, bench); err != nil {
		return fmt.Errorf("failed to get referenced bench for deletion: %w", err)
	}

	// Create deletion job to run bench drop-site
	jobName := fmt.Sprintf("%s-delete", site.Name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: site.Namespace,
			Labels: map[string]string{
				"app":  "frappe",
				"site": site.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:   corev1.RestartPolicyNever,
					SecurityContext: r.getPodSecurityContext(bench),
					Containers: []corev1.Container{
						{
							Name:    "site-delete",
							Image:   r.getBenchImage(bench),
							Command: []string{"bash", "-c"},
							Args: []string{
								fmt.Sprintf(`#!/bin/bash
set -e

cd /home/frappe/frappe-bench

echo "Dropping Frappe site: %s"
bench drop-site %s --yes

echo "Site %s dropped successfully!"
`, site.Spec.SiteName, site.Spec.SiteName, site.Spec.SiteName),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
							SecurityContext: r.getContainerSecurityContext(bench),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("%s-sites", bench.Name),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(site, job, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to create site deletion job: %w", err)
	}

	// Wait for job to complete
	logger.Info("Waiting for site deletion job to complete", "job", jobName)
	for i := 0; i < 60; i++ { // Max 10 minutes
		time.Sleep(10 * time.Second)

		err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
		if err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("site deletion job disappeared")
			}
			return err
		}

		if job.Status.Succeeded > 0 {
			logger.Info("Site deletion job completed successfully")
			return nil
		}

		if job.Status.Failed > 0 {
			return fmt.Errorf("site deletion job failed")
		}
	}

	return fmt.Errorf("timeout waiting for site deletion job to complete")
}

// SetupWithManager sets up the controller with the Manager
func (r *FrappeSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1alpha1.FrappeSite{}).
		Owns(&batchv1.Job{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}

func (r *FrappeSiteReconciler) getPodSecurityContext(bench *vyogotechv1alpha1.FrappeBench) *corev1.PodSecurityContext {
	if bench.Spec.Security != nil && bench.Spec.Security.PodSecurityContext != nil {
		return bench.Spec.Security.PodSecurityContext
	}
	return &corev1.PodSecurityContext{
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func (r *FrappeSiteReconciler) getContainerSecurityContext(bench *vyogotechv1alpha1.FrappeBench) *corev1.SecurityContext {
	if bench.Spec.Security != nil && bench.Spec.Security.SecurityContext != nil {
		return bench.Spec.Security.SecurityContext
	}
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		ReadOnlyRootFilesystem: boolPtr(false),
	}
}
