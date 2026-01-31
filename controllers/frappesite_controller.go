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
	"strings"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const frappeSiteFinalizer = "vyogo.tech/site-finalizer"

// FrappeSiteReconciler reconciles a FrappeSite object
type FrappeSiteReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	Recorder                 record.EventRecorder
	IsOpenShift              bool
	MaxConcurrentReconciles  int // from operator config and/or bench-level siteReconcileConcurrency
}

// int32Ptr returns a pointer to the passed int32 value

//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;ingressclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets;services;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.mariadb.com,resources=mariadbs;databases;users;grants,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *FrappeSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	site := &vyogotechv1alpha1.FrappeSite{}
	if err := r.Get(ctx, req.NamespacedName, site); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling FrappeSite", "site", site.Name, "siteName", site.Spec.SiteName)
	r.Recorder.Event(site, corev1.EventTypeNormal, "Reconciling", "Starting FrappeSite reconciliation")

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(site, frappeSiteFinalizer) {
		controllerutil.AddFinalizer(site, frappeSiteFinalizer)
		if err := r.Update(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(site, corev1.EventTypeNormal, "FinalizerAdded", "Finalizer added to FrappeSite")
	}

	// Handle deletion
	if site.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(site, frappeSiteFinalizer) {
			logger.Info("Deleting site", "site", site.Name)
			r.Recorder.Event(site, corev1.EventTypeNormal, "Deleting", "FrappeSite deletion started")

			// Set terminating condition
			r.setCondition(site, metav1.Condition{
				Type:    "Terminating",
				Status:  metav1.ConditionTrue,
				Reason:  "Deleting",
				Message: "Site is being deleted",
			})
			if err := r.updateStatus(ctx, site); err != nil {
				return ctrl.Result{}, err
			}

			// Implement site deletion job
			if err := r.deleteSite(ctx, site); err != nil {
				logger.Error(err, "Failed to delete site, will requeue")
				r.Recorder.Event(site, corev1.EventTypeWarning, "DeletionInProgress", fmt.Sprintf("Site deletion in progress: %v", err))
				r.setCondition(site, metav1.Condition{
					Type:    "Terminating",
					Status:  metav1.ConditionTrue,
					Reason:  "DeletionInProgress",
					Message: fmt.Sprintf("Site deletion in progress: %v", err),
				})
				if statusErr := r.updateStatus(ctx, site); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// Requeue to check deletion job status
				return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
			}

			// Clean up Ingress/Route
			ingressName := fmt.Sprintf("%s-ingress", site.Name)
			ingress := &networkingv1.Ingress{}
			if err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: site.Namespace}, ingress); err == nil {
				if err := r.Delete(ctx, ingress); err != nil {
					logger.Error(err, "Failed to delete Ingress", "ingress", ingressName)
				} else {
					r.Recorder.Event(site, corev1.EventTypeNormal, "IngressDeleted", "Ingress resource deleted")
				}
			}

			routeName := fmt.Sprintf("%s-route", site.Name)
			route := &routev1.Route{}
			if err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: site.Namespace}, route); err == nil {
				if err := r.Delete(ctx, route); err != nil {
					logger.Error(err, "Failed to delete Route", "route", routeName)
				} else {
					r.Recorder.Event(site, corev1.EventTypeNormal, "RouteDeleted", "Route resource deleted")
				}
			}

			// Clean up admin password secret
			secretName := fmt.Sprintf("%s-admin", site.Name)
			secret := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, secret); err == nil {
				if err := r.Delete(ctx, secret); err != nil {
					logger.Error(err, "Failed to delete admin password secret", "secret", secretName)
				} else {
					r.Recorder.Event(site, corev1.EventTypeNormal, "SecretDeleted", "Admin password secret deleted")
				}
			}

			logger.Info("FrappeSite cleanup complete, removing finalizer")
			r.Recorder.Event(site, corev1.EventTypeNormal, "Deleted", "FrappeSite cleanup completed")
			controllerutil.RemoveFinalizer(site, frappeSiteFinalizer)
			if err := r.Update(ctx, site); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
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

	// Validate benchRef
	if site.Spec.BenchRef == nil {
		logger.Error(nil, "BenchRef is required")
		r.Recorder.Event(site, corev1.EventTypeWarning, "ValidationFailed", "benchRef is required")
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
		r.Recorder.Event(site, corev1.EventTypeWarning, "DatabaseProviderFailed", fmt.Sprintf("Failed to create database provider: %v", err))
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
		r.setCondition(site, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "DatabaseProviderFailed",
			Message: fmt.Sprintf("Failed to create database provider: %v", err),
		})
		if err := r.updateStatus(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// Check if database is ready
	dbReady, err := dbProvider.IsReady(ctx, site)
	if err != nil {
		logger.Error(err, "Failed to check database readiness")
		r.Recorder.Event(site, corev1.EventTypeWarning, "DatabaseCheckFailed", fmt.Sprintf("Failed to check database readiness: %v", err))
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
		r.Recorder.Event(site, corev1.EventTypeNormal, "DatabaseProvisioning", "Database is being provisioned")
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
			r.Recorder.Event(site, corev1.EventTypeWarning, "DatabaseProvisioningFailed", fmt.Sprintf("Database provisioning failed: %v", err))
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
		r.Recorder.Event(site, corev1.EventTypeNormal, "DatabaseProvisioning", fmt.Sprintf("Database provisioning initiated: %s", dbInfo.Name))

		// Requeue to check readiness
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Database is ready - get credentials
	site.Status.DatabaseReady = true
	r.Recorder.Event(site, corev1.EventTypeNormal, "DatabaseReady", "Database is ready and accessible")
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
		r.Recorder.Event(site, corev1.EventTypeWarning, "SiteInitializationFailed", fmt.Sprintf("Site initialization failed: %v", err))
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
		r.Recorder.Event(site, corev1.EventTypeNormal, "SiteInitializing", "Site initialization in progress")
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
	r.Recorder.Event(site, corev1.EventTypeNormal, "SiteInitialized", "Site initialization completed successfully")

	// 2. Ensure Ingress (enabled by default, can be disabled)
	createIngress := true
	if site.Spec.Ingress != nil && site.Spec.Ingress.Enabled != nil && !*site.Spec.Ingress.Enabled {
		createIngress = false
		logger.Info("Ingress creation disabled by user", "site", site.Name)
	}

	var routeHost string
	if createIngress {
		logger.Info("External access enabled, checking platform", "site", site.Name)
		// Check if we're on OpenShift and should create Routes instead
		if r.isOpenShiftPlatform(ctx) && (site.Spec.RouteConfig == nil || site.Spec.RouteConfig.Enabled == nil || *site.Spec.RouteConfig.Enabled) {
			if err := r.ensureRoute(ctx, site, bench, domain); err != nil {
				logger.Error(err, "Failed to ensure Route")
				r.Recorder.Event(site, corev1.EventTypeWarning, "RouteFailed", fmt.Sprintf("Failed to create Route: %v", err))
				return ctrl.Result{}, err
			}
			// Get Route hostname for status
			routeName := fmt.Sprintf("%s-route", site.Name)
			route := &routev1.Route{}
			if err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: site.Namespace}, route); err == nil {
				routeHost = route.Spec.Host
				if routeHost == "" && len(route.Status.Ingress) > 0 {
					routeHost = route.Status.Ingress[0].Host
				}
			}
			r.Recorder.Event(site, corev1.EventTypeNormal, "RouteCreated", fmt.Sprintf("OpenShift Route created: %s", routeHost))
		} else {
			if err := r.ensureIngress(ctx, site, bench, domain); err != nil {
				logger.Error(err, "Failed to ensure Ingress")
				r.Recorder.Event(site, corev1.EventTypeWarning, "IngressFailed", fmt.Sprintf("Failed to create Ingress: %v", err))
				return ctrl.Result{}, err
			}
			r.Recorder.Event(site, corev1.EventTypeNormal, "IngressCreated", fmt.Sprintf("Ingress created for domain: %s", domain))
		}
	}

	// 3. Update final status
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseReady
	if routeHost != "" {
		// Use Route hostname if available
		site.Status.SiteURL = fmt.Sprintf("http://%s", routeHost)
		if site.Spec.TLS.Enabled {
			site.Status.SiteURL = fmt.Sprintf("https://%s", routeHost)
		}
	} else {
		site.Status.SiteURL = fmt.Sprintf("http://%s", domain)
		if site.Spec.TLS.Enabled {
			site.Status.SiteURL = fmt.Sprintf("https://%s", domain)
		}
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

	r.Recorder.Event(site, corev1.EventTypeNormal, "SiteReady", fmt.Sprintf("FrappeSite is ready at %s", site.Status.SiteURL))
	logger.Info("FrappeSite reconciled successfully", "site", site.Name, "domain", domain)
	return ctrl.Result{}, nil
}

// setCondition sets a condition on the FrappeSite using meta.SetStatusCondition
func (r *FrappeSiteReconciler) setCondition(site *vyogotechv1alpha1.FrappeSite, condition metav1.Condition) {
	condition.ObservedGeneration = site.Generation
	meta.SetStatusCondition(&site.Status.Conditions, condition)
}

// updateStatus updates the FrappeSite status with proper error handling and conflict retry
func (r *FrappeSiteReconciler) updateStatus(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &vyogotechv1alpha1.FrappeSite{}
		if err := r.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, latest); err != nil {
			return err
		}
		latest.Status = site.Status
		if err := r.Status().Update(ctx, latest); err != nil {
			return err
		}
		// Sync back resource version to avoid conflict in subsequent updates in same reconcile loop
		site.SetResourceVersion(latest.GetResourceVersion())
		return nil
	})
}

// resolveDBConfig merges site-specific database configuration with bench-level defaults
func (r *FrappeSiteReconciler) resolveDBConfig(site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench) vyogotechv1alpha1.DatabaseConfig {
	config := site.Spec.DBConfig

	if bench.Spec.DBConfig == nil {
		// Default provider to MariaDB if not specified anywhere
		if config.Provider == "" {
			config.Provider = "mariadb"
		}
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

// ensureInitSecrets creates a Secret containing all initialization credentials
// This function ensures credentials are mounted as files, not environment variables
func (r *FrappeSiteReconciler) ensureInitSecrets(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string, dbInfo *database.DatabaseInfo, dbCreds *database.DatabaseCredentials, adminPassword string) error {
	logger := log.FromContext(ctx)

	secretName := fmt.Sprintf("%s-init-secrets", site.Name)

	// Get DB_PROVIDER from database info
	dbProvider := "mariadb" // default
	if site.Spec.DBConfig.Provider != "" {
		dbProvider = site.Spec.DBConfig.Provider
	}

	// Get apps to install if specified
	// New approach: Pass all requested apps to the script, which will check actual availability
	// and gracefully skip any that aren't available
	appsToInstall := ""
	if len(site.Spec.Apps) > 0 {
		logger.Info("Apps specified for site", "apps", site.Spec.Apps, "count", len(site.Spec.Apps))

		var validApps []string
		var skippedApps []string

		// Basic validation: check app names are safe (alphanumeric, underscore, hyphen only)
		// This prevents shell injection but doesn't fail for missing apps
		for _, app := range site.Spec.Apps {
			// Check for valid characters to prevent shell injection
			isValid := true
			for _, char := range app {
				if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
					(char >= '0' && char <= '9') || char == '_' || char == '-') {
					isValid = false
					break
				}
			}

			if !isValid {
				skippedApps = append(skippedApps, app)
				logger.Info("Skipping app with invalid characters", "app", app)
				r.Recorder.Event(site, corev1.EventTypeWarning, "InvalidAppName",
					fmt.Sprintf("App '%s' contains invalid characters and will be skipped", app))
			} else {
				validApps = append(validApps, app)
			}
		}

		if len(skippedApps) > 0 {
			logger.Info("Some apps skipped due to invalid names", "skipped", skippedApps)
		}

		if len(validApps) == 0 {
			logger.Info("No valid apps to install after validation")
		} else {
			// Pass all valid apps to the script - it will check actual availability
			// Use strings.Join for efficient space-separated string construction
			appsToInstall = strings.Join(validApps, " ")
			logger.Info("Apps prepared for reconciliation", "apps", appsToInstall, "count", len(validApps))
			r.Recorder.Event(site, corev1.EventTypeNormal, "AppsRequested",
				fmt.Sprintf("Requested %d app(s): %v - will check availability in container", len(validApps), validApps))
		}
	} else {
		logger.Info("No apps specified - only frappe framework will be present")
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

	// Add database credentials if using external database
	if dbProvider == "mariadb" || dbProvider == "postgres" {
		if dbInfo != nil {
			secretData["db_host"] = []byte(dbInfo.Host)
			secretData["db_port"] = []byte(dbInfo.Port)
			secretData["db_name"] = []byte(dbInfo.Name)
		}
		if dbCreds != nil {
			secretData["db_user"] = []byte(dbCreds.Username)
			secretData["db_password"] = []byte(dbCreds.Password)
		}
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

	// Set controller reference
	if err := controllerutil.SetControllerReference(site, secret, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference for secret", "secret", secretName)
		return err
	}

	// Create or update secret
	var existing corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, &existing)
	if err != nil && errors.IsNotFound(err) {
		// Create new secret
		if err := r.Create(ctx, secret); err != nil {
			logger.Error(err, "Failed to create initialization secret", "secret", secretName)
			return err
		}
		logger.Info("Created initialization secret", "secret", secretName)
	} else if err != nil {
		logger.Error(err, "Failed to get initialization secret", "secret", secretName)
		return err
	} else {
		// Update existing secret
		existing.Data = secretData
		if err := r.Update(ctx, &existing); err != nil {
			logger.Error(err, "Failed to update initialization secret", "secret", secretName)
			return err
		}
		logger.Info("Updated initialization secret", "secret", secretName)
	}

	return nil
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
			logger.Info("Site initialization job completed successfully", "job", jobName)

			// Update status with requested apps
			// Note: Some apps may have been skipped if not available - check logs for details
			if len(site.Spec.Apps) > 0 {
				site.Status.InstalledApps = site.Spec.Apps
				site.Status.AppInstallationStatus = fmt.Sprintf("Completed app installation for %d requested app(s) - check logs for any skipped apps", len(site.Spec.Apps))
				logger.Info("App installation process completed", "requestedApps", site.Spec.Apps)
				r.Recorder.Event(site, corev1.EventTypeNormal, "AppsProcessed",
					fmt.Sprintf("Processed app installation for: %v - check job logs for any skipped apps", site.Spec.Apps))
			} else {
				site.Status.AppInstallationStatus = "No apps specified - only frappe framework installed"
				logger.Info("Site initialized with frappe framework only")
			}

			return true, nil
		}
		if job.Status.Failed > 0 {
			logger.Error(nil, "Site initialization job failed", "job", jobName, "failedCount", job.Status.Failed)
			r.Recorder.Event(site, corev1.EventTypeWarning, "SiteInitializationFailed",
				fmt.Sprintf("Site initialization job failed after %d attempt(s)", job.Status.Failed))

			// Try to get pod logs for error details
			podList := &corev1.PodList{}
			listOpts := []client.ListOption{
				client.InNamespace(site.Namespace),
				client.MatchingLabels{"job-name": jobName},
			}
			if err := r.List(ctx, podList, listOpts...); err == nil && len(podList.Items) > 0 {
				// Check the most recent pod for error messages
				pod := podList.Items[len(podList.Items)-1]
				if pod.Status.Phase == corev1.PodFailed {
					logger.Error(nil, "Site initialization pod failed",
						"pod", pod.Name,
						"phase", pod.Status.Phase,
						"reason", pod.Status.Reason,
						"message", pod.Status.Message)

					// Update status with failure information
					if len(site.Spec.Apps) > 0 {
						site.Status.AppInstallationStatus = fmt.Sprintf("Failed to install apps: %s", pod.Status.Message)
						r.Recorder.Event(site, corev1.EventTypeWarning, "AppInstallationFailed",
							fmt.Sprintf("Failed to install apps. Check pod %s logs for details", pod.Name))
					}
				}
			}

			return false, fmt.Errorf("site initialization job failed")
		}
		// Job is still running
		logger.Info("Site initialization job in progress", "job", jobName)
		if len(site.Spec.Apps) > 0 {
			site.Status.AppInstallationStatus = fmt.Sprintf("Installing %d app(s)...", len(site.Spec.Apps))
		}
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
		"dbName", dbInfo.Name,
		"apps", site.Spec.Apps,
		"appsCount", len(site.Spec.Apps))

	if len(site.Spec.Apps) > 0 {
		r.Recorder.Event(site, corev1.EventTypeNormal, "CreatingInitJob",
			fmt.Sprintf("Creating initialization job to install %d app(s): %v", len(site.Spec.Apps), site.Spec.Apps))
	} else {
		r.Recorder.Event(site, corev1.EventTypeNormal, "CreatingInitJob",
			"Creating initialization job (frappe framework only)")
	}

	// Get or generate admin password
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

	// Ensure initialization secret exists with all credentials
	if err := r.ensureInitSecrets(ctx, site, bench, domain, dbInfo, dbCreds, adminPassword); err != nil {
		logger.Error(err, "Failed to create initialization secret")
		return false, fmt.Errorf("failed to create init secret: %w", err)
	}

	// Create the init script using environment variables to prevent shell injection
	initScript := `#!/bin/bash
set -e
umask 0002

# Setup user for OpenShift compatibility (fixes getpwuid() error)
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  # Try to add user to /etc/passwd if writable (rarely the case on OpenShift, but good practice)
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench

# Read from secret files mounted at /tmp/site-secrets
SITE_NAME=$(cat /tmp/site-secrets/site_name)
DOMAIN=$(cat /tmp/site-secrets/domain)
ADMIN_PASSWORD=$(cat /tmp/site-secrets/admin_password)
BENCH_NAME=$(cat /tmp/site-secrets/bench_name)
DB_PROVIDER=$(cat /tmp/site-secrets/db_provider)
APPS_TO_INSTALL=$(cat /tmp/site-secrets/apps_to_install 2>/dev/null || echo "")

echo "Creating Frappe site: $SITE_NAME"
echo "Domain: $DOMAIN"

# If the site directory already exists, skip creation but update config
if [[ -d "/home/frappe/frappe-bench/sites/$SITE_NAME" ]]; then
    echo "Site $SITE_NAME already exists; skipping new-site and updating config."
    goto_update_config=1
else
    goto_update_config=0
fi

# Link apps.txt to site path for bench to find it
# The apps.txt is in the sites directory, but bench expects it in the root
echo "Debug: Current directory is $(pwd)"
echo "Debug: Contents of $(pwd):"
ls -la
if [ -f sites/apps.txt ]; then
    echo "Debug: sites/apps.txt found, creating link..."
    ln -sf sites/apps.txt apps.txt || cp sites/apps.txt apps.txt || echo "Warning: Failed to create apps.txt in root"
else
    echo "Warning: sites/apps.txt not found!"
fi
ls -l apps.txt || true

# Dynamically build the --install-app argument with validation and logging
INSTALL_APP_ARG=""
APPS_TO_INSTALL_COUNT=0
if [[ -n "$APPS_TO_INSTALL" ]]; then
	echo "=========================================="
	echo "App Installation Configuration"
	echo "=========================================="
	echo "Apps requested for installation: $APPS_TO_INSTALL"
	
	# Validate apps directory exists and list available apps
	if [[ -d "apps" ]]; then
		# Get list of available apps from apps directory
		AVAILABLE_APPS=$(ls -1 apps/ 2>/dev/null | grep -v "^frappe$" || true)
		echo "Available apps in bench (from filesystem):"
		if [[ -n "$AVAILABLE_APPS" ]]; then
			echo "$AVAILABLE_APPS"
		fi
		echo "frappe (framework - always available)"
		
		# Also check apps.txt if it exists
		if [[ -f "sites/apps.txt" ]]; then
			echo ""
			echo "Apps listed in apps.txt:"
			cat sites/apps.txt || true
		fi
	else
		echo "WARNING: apps directory not found in bench - this is unexpected"
	fi
	echo "------------------------------------------"
	
	# Build install arguments and validate each app
	# New approach: Skip apps that aren't available instead of failing
	SKIPPED_APPS=""
	for app in $APPS_TO_INSTALL; do
		# Check if app directory exists
		if [[ -d "apps/$app" ]]; then
			INSTALL_APP_ARG+=" --install-app=$app"
			APPS_TO_INSTALL_COUNT=$((APPS_TO_INSTALL_COUNT + 1))
			echo "✓ App '$app' found in bench and will be installed"
		else
			# Gracefully skip missing apps
			echo "⚠ WARNING: App '$app' not found in bench directory - skipping"
			echo "  The app may not be installed in this bench yet"
			if [[ -n "$SKIPPED_APPS" ]]; then
				SKIPPED_APPS="$SKIPPED_APPS, $app"
			else
				SKIPPED_APPS="$app"
			fi
		fi
	done
	
	if [[ -n "$SKIPPED_APPS" ]]; then
		echo "------------------------------------------"
		echo "⚠ Skipped apps (not available): $SKIPPED_APPS"
		echo "  These apps will not be installed on this site"
		echo "  To install them later, ensure they're available in the bench"
		echo "  and use: bench --site $SITE_NAME install-app <app_name>"
	fi
	
	echo "=========================================="
	echo "Total apps to install: $APPS_TO_INSTALL_COUNT"
	if [[ $APPS_TO_INSTALL_COUNT -gt 0 ]]; then
		echo "Install arguments: $INSTALL_APP_ARG"
	else
		echo "No apps will be installed (none available or none specified)"
	fi
	echo "=========================================="
else
	echo "=========================================="
	echo "No apps specified for installation"
	echo "Only frappe framework will be installed"
	echo "=========================================="
fi

# Run bench new-site with provider-specific database configuration
if [[ "$DB_PROVIDER" == "mariadb" ]] || [[ "$DB_PROVIDER" == "postgres" ]]; then
	# For MariaDB and PostgreSQL: use pre-provisioned database with dedicated credentials
	# These are mounted from secret volumes, not environment variables
	DB_HOST=$(cat /tmp/site-secrets/db_host)
	DB_PORT=$(cat /tmp/site-secrets/db_port)
	DB_NAME=$(cat /tmp/site-secrets/db_name)
	DB_USER=$(cat /tmp/site-secrets/db_user)
	DB_PASSWORD=$(cat /tmp/site-secrets/db_password)
    
	if [[ -z "$DB_HOST" || -z "$DB_PORT" || -z "$DB_NAME" || -z "$DB_USER" || -z "$DB_PASSWORD" ]]; then
		echo "ERROR: Database connection secret files not found for $DB_PROVIDER"
		exit 1
	fi

    if [[ "$goto_update_config" -eq 0 ]]; then
        echo "=========================================="
        echo "Creating Frappe Site"
        echo "=========================================="
        echo "Site Name: $SITE_NAME"
        echo "Database Provider: $DB_PROVIDER"
        echo "Database Name: $DB_NAME"
        echo "Database Host: $DB_HOST:$DB_PORT"
        echo "Apps to install: ${APPS_TO_INSTALL:-none}"
        echo "=========================================="
        
        # Check if bench version supports --db-user flag
        DB_USER_FLAG=""
        if bench new-site --help | grep -q " --db-user"; then
            echo "✓ Detected support for --db-user flag"
            DB_USER_FLAG="--db-user=$DB_USER"
        elif [[ "$DB_USER" != "$DB_NAME" ]]; then
            echo "⚠ WARNING: Your bench version does not support --db-user. Using DB_NAME as username."
        else
            echo "✓ Bench version does not support --db-user, but DB_USER matches DB_NAME. Proceeding."
        fi

        echo ""
        echo "Executing: bench new-site with apps: $INSTALL_APP_ARG"
        echo "------------------------------------------"
        
        # Capture both stdout and stderr, and exit code
        # Temporarily disable exit-on-error to capture the output
        SITE_CREATION_OUTPUT=""
        SITE_CREATION_EXIT_CODE=0
        set +e  # Don't exit on error yet, we want to capture it
        SITE_CREATION_OUTPUT=$(bench new-site \
          --db-type="$DB_PROVIDER" \
          --db-name="$DB_NAME" \
          --db-host="$DB_HOST" \
          --db-port="$DB_PORT" \
          $DB_USER_FLAG \
          --db-password="$DB_PASSWORD" \
          --no-setup-db \
          --admin-password="$ADMIN_PASSWORD" \
          $INSTALL_APP_ARG \
          --verbose \
          "$SITE_NAME" 2>&1)
        SITE_CREATION_EXIT_CODE=$?
        set -e  # Re-enable exit on error
        
        # Always print the output
        echo "$SITE_CREATION_OUTPUT"
        echo "------------------------------------------"
        
        if [[ $SITE_CREATION_EXIT_CODE -eq 0 ]]; then
            echo "✓ Site created successfully!"
            if [[ $APPS_TO_INSTALL_COUNT -gt 0 ]]; then
                echo "✓ Requested installation of $APPS_TO_INSTALL_COUNT app(s). See logs above for per-app status."
                
                # Log each app that was requested for installation
                echo "Apps requested for installation:"
                for app in $APPS_TO_INSTALL; do
                    echo "  - $app"
                done
            fi
        else
            echo "✗ ERROR: Site creation failed with exit code $SITE_CREATION_EXIT_CODE"
            
            # Try to extract error information
            if echo "$SITE_CREATION_OUTPUT" | grep -Eqi "error|traceback|exception|failed"; then
                echo "Error details found in output above"
            fi
            
            # Check for specific app installation failures with more patterns
            if echo "$SITE_CREATION_OUTPUT" | grep -Eqi "app.*not (found|installed)|no module named|cannot import|failed to install"; then
                echo "ERROR: App installation failed - one or more apps could not be found or imported"
            fi
            
            # If site exists, it's not a critical error, continue to config update
            if echo "$SITE_CREATION_OUTPUT" | grep -Eqi "site.*(already exists|exists already)"; then
                echo "⚠ Site already exists, will proceed to update configuration"
                # Don't exit - continue to update config
            else
                echo "CRITICAL ERROR: Site creation failed. Exiting."
                exit $SITE_CREATION_EXIT_CODE
            fi
        fi
        echo "=========================================="
    else
        echo "=========================================="
        echo "Site already exists - skipping site creation"
        echo "Will update site_config.json only"
        echo "Note: Apps cannot be installed after site creation"
        echo "=========================================="
    fi
else
    echo "ERROR: Unsupported DB provider: $DB_PROVIDER"
    exit 1
fi

# Create or update common_site_config.json
echo "Creating common_site_config.json..."
cat > sites/common_site_config.json <<EOF
{
  "redis_cache": "redis://${BENCH_NAME}-redis-cache:6379",
  "redis_queue": "redis://${BENCH_NAME}-redis-queue:6379",
  "socketio_port": 9000
}
EOF

# Sync assets from the image cache to the Persistent Volume
if [ -d "/home/frappe/assets_cache" ]; then
    echo "Syncing pre-built assets from image to PVC..."
    mkdir -p sites/assets
    cp -rn /home/frappe/assets_cache/* sites/assets/ || true
fi

echo "Site $SITE_NAME created successfully!"

# Update site_config.json with domain and Redis configuration using Python
echo "Updating site_config.json with domain and Redis"
python3 << 'PYTHON_SCRIPT'
import json, os

# Read from secret files mounted at /tmp/site-secrets
with open('/tmp/site-secrets/site_name', 'r') as f:
    site_name = f.read().strip()
with open('/tmp/site-secrets/domain', 'r') as f:
    domain = f.read().strip()
with open('/tmp/site-secrets/bench_name', 'r') as f:
    bench_name = f.read().strip()
with open('/tmp/site-secrets/db_host', 'r') as f:
    db_host = f.read().strip()
with open('/tmp/site-secrets/db_port', 'r') as f:
    db_port = f.read().strip()
with open('/tmp/site-secrets/db_name', 'r') as f:
    db_name = f.read().strip()
with open('/tmp/site-secrets/db_user', 'r') as f:
    db_user = f.read().strip()
with open('/tmp/site-secrets/db_password', 'r') as f:
    db_password = f.read().strip()
with open('/tmp/site-secrets/db_provider', 'r') as f:
    db_provider = f.read().strip()

site_path = f"/home/frappe/frappe-bench/sites/{site_name}"
config_file = os.path.join(site_path, "site_config.json")

# Read or initialize config
try:
    with open(config_file, 'r') as f:
        config = json.load(f)
except FileNotFoundError:
    config = {}

# Update with resolved domain
config['host_name'] = domain

# Add Redis configuration for this site
config['redis_cache'] = f"redis://{bench_name}-redis-cache:6379"
config['redis_queue'] = f"redis://{bench_name}-redis-queue:6379"

# Explicitly add database credentials for self-healing
config['db_name'] = db_name
config['db_user'] = db_user
config['db_password'] = db_password
config['db_host'] = db_host
config['db_type'] = db_provider

# Ensure directory exists
os.makedirs(site_path, exist_ok=True)

# Ensure logs directory exists
os.makedirs(os.path.join(site_path, "logs"), exist_ok=True)

# Write back
with open(config_file, 'w') as f:
    json.dump(config, f, indent=2)

print(f"Updated site_config.json for domain: {domain}")
print(f"Redis cache: {bench_name}-redis-cache:6379")
print(f"Redis queue: {bench_name}-redis-queue:6379")
PYTHON_SCRIPT

echo "Site initialization complete!"

# Exit success regardless of whether new-site ran
exit 0
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
					SecurityContext: r.getPodSecurityContext(ctx, bench),
					Containers: []corev1.Container{
						{
							Name:    "site-init",
							Image:   r.getBenchImage(ctx, bench),
							Command: []string{"bash", "-c"},
							Args:    []string{initScript},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
								{
									Name:      "site-secrets",
									MountPath: "/tmp/site-secrets",
								},
							},
							SecurityContext: r.getContainerSecurityContext(ctx, bench),
							// Removed: No environment variables for sensitive data
							Env: []corev1.EnvVar{},
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
						{
							Name: "site-secrets",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  fmt.Sprintf("%s-init-secrets", site.Name),
									DefaultMode: int32Ptr(0444), // Read-only for security, but allow all users to read
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

	jobToCreate := job.DeepCopy()
	jobToCreate.ResourceVersion = ""
	if err := r.Create(ctx, jobToCreate); err != nil {
		return false, err
	}

	logger.Info("Site initialization job created", "job", jobName)
	return false, nil // Not ready yet, job is running
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
		if errors.IsNotFound(err) {
			logger.Info("Referenced bench not found, assuming it's already deleted")
			return nil
		}
		return fmt.Errorf("failed to get referenced bench for deletion: %w", err)
	}

	// Create deletion job to run bench drop-site
	// Site user now has minimal privileges (cannot drop database) - use root credentials
	jobName := fmt.Sprintf("%s-delete", site.Name)
	job := &batchv1.Job{}

	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get deletion job: %w", err)
		}

		// Job doesn't exist, create it
		logger.Info("Creating site deletion job", "job", jobName)

		// Get MariaDB root credentials for deletion (site user has limited privileges)
		rootUser, rootPassword, err := r.getMariaDBRootCredentials(ctx, site)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("MariaDB instance not found, skipping site deletion job")
				return nil
			}
			return fmt.Errorf("failed to get MariaDB root credentials: %w", err)
		}

		// Create deletion secret with root credentials
		deletionSecretName := fmt.Sprintf("%s-deletion-secret", site.Name)
		deletionSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deletionSecretName,
				Namespace: site.Namespace,
				Labels: map[string]string{
					"app":  "frappe",
					"site": site.Name,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"db_root_user":     []byte(rootUser),
				"db_root_password": []byte(rootPassword),
				"site_name":        []byte(site.Spec.SiteName),
			},
		}

		if err := controllerutil.SetControllerReference(site, deletionSecret, r.Scheme); err != nil {
			return err
		}

		if err := r.Create(ctx, deletionSecret); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create deletion secret: %w", err)
			}
			// Update existing secret with current credentials
			var existing corev1.Secret
			if err := r.Get(ctx, types.NamespacedName{Name: deletionSecretName, Namespace: site.Namespace}, &existing); err != nil {
				return fmt.Errorf("failed to get existing deletion secret: %w", err)
			}
			existing.Data = deletionSecret.Data
			if err := r.Update(ctx, &existing); err != nil {
				return fmt.Errorf("failed to update deletion secret: %w", err)
			}
		}

		// Use root credentials from secret volume (not environment variables)
		deleteScript := `#!/bin/bash
set -e

# Setup user for OpenShift compatibility (fixes getpwuid() error)
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  # Try to add user to /etc/passwd if writable
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench

# Link apps.txt to site path for bench to find it
# The apps.txt is in the sites directory, but bench expects it in the root
if [ -f sites/apps.txt ]; then
    ln -sf sites/apps.txt apps.txt || cp sites/apps.txt apps.txt || echo "Warning: Failed to create apps.txt in root"
else
    echo "Warning: sites/apps.txt not found!"
fi

# Read credentials from mounted secret files
DB_ROOT_USER=$(cat /tmp/secrets/db_root_user)
DB_ROOT_PASSWORD=$(cat /tmp/secrets/db_root_password)
SITE_NAME=$(cat /tmp/secrets/site_name)

echo "Dropping Frappe site: $SITE_NAME"
echo "Using MariaDB root credentials from secret volume for secure deletion"

# Use root credentials to drop the site (site user cannot drop database)
bench drop-site "$SITE_NAME" --force --db-root-username "$DB_ROOT_USER" --db-root-password "$DB_ROOT_PASSWORD" --no-backup

echo "Site $SITE_NAME dropped successfully!"
`

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
						SecurityContext: r.getPodSecurityContext(ctx, bench),
						Containers: []corev1.Container{
							{
								Name:    "site-delete",
								Image:   r.getBenchImage(ctx, bench),
								Command: []string{"bash", "-c"},
								Args:    []string{deleteScript},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "sites",
										MountPath: "/home/frappe/frappe-bench/sites",
									},
									{
										Name:      "deletion-secret",
										MountPath: "/tmp/secrets",
										ReadOnly:  true,
									},
								},
								SecurityContext: r.getContainerSecurityContext(ctx, bench),
								Env:             []corev1.EnvVar{}, // No environment variables for sensitive data
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
							{
								Name: "deletion-secret",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName:  deletionSecretName,
										DefaultMode: int32Ptr(0400), // Read-only for security
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

		// Create a copy with no ResourceVersion so Create succeeds (required for fake client and real API)
		jobToCreate := job.DeepCopy()
		jobToCreate.ResourceVersion = ""

		if err := r.Create(ctx, jobToCreate); err != nil {
			return fmt.Errorf("failed to create site deletion job: %w", err)
		}

		// Job created, requeue to check status later
		return fmt.Errorf("site deletion job created, waiting for completion")
	}

	// Job exists, check its status
	if job.Status.Succeeded > 0 {
		logger.Info("Site deletion job completed successfully")
		// Job finished, now we can clean it up
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			return fmt.Errorf("failed to delete completed deletion job: %w", err)
		}
		return nil
	}

	if job.Status.Failed > 0 {
		// Job failed, log the error and don't remove the finalizer so it can be inspected
		return fmt.Errorf("site deletion job failed")
	}

	// Job is still running
	return fmt.Errorf("site deletion job is still running")
}

// SetupWithManager sets up the controller with the Manager
func (r *FrappeSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up event recorder
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("frappesite-controller")
	}

	opts := controller.Options{}
	if r.MaxConcurrentReconciles > 0 {
		opts.MaxConcurrentReconciles = r.MaxConcurrentReconciles
	}
	builder := ctrl.NewControllerManagedBy(mgr).
		WithOptions(opts).
		For(&vyogotechv1alpha1.FrappeSite{}).
		Owns(&batchv1.Job{}).
		Owns(&networkingv1.Ingress{})

	// Use the platform detection result provided via IsOpenShift
	if r.IsOpenShift {
		ctrl.Log.WithName("setup").Info("OpenShift platform detected - enabling Route support")
		builder.Owns(&routev1.Route{})
	}

	return builder.Complete(r)
}

// getMariaDBRootCredentials retrieves MariaDB root credentials for site deletion
// Returns (username, password, error). Only use these credentials in deletion jobs.
func (r *FrappeSiteReconciler) getMariaDBRootCredentials(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) (string, string, error) {
	logger := log.FromContext(ctx)

	// For dedicated mode, root secret is {site-name}-mariadb-root
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

	// For shared mode, need to get MariaDB CR and read its rootPasswordSecretKeyRef
	if site.Spec.DBConfig.Mode == "shared" {
		// Get the MariaDB instance name from site spec
		mariadbName := "frappe-mariadb"
		mariadbNamespace := site.Namespace
		if site.Spec.DBConfig.MariaDBRef != nil {
			mariadbName = site.Spec.DBConfig.MariaDBRef.Name
			if site.Spec.DBConfig.MariaDBRef.Namespace != "" {
				mariadbNamespace = site.Spec.DBConfig.MariaDBRef.Namespace
			}
		}

		// Get MariaDB CR using unstructured client
		mariadbCR := &unstructured.Unstructured{}
		mariadbCR.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "k8s.mariadb.com",
			Version: "v1alpha1",
			Kind:    "MariaDB",
		})
		err := r.Get(ctx, types.NamespacedName{Name: mariadbName, Namespace: mariadbNamespace}, mariadbCR)
		if err != nil {
			return "", "", err // Return raw error so caller can check errors.IsNotFound
		}

		// Extract rootPasswordSecretKeyRef from spec
		spec, found, err := unstructured.NestedMap(mariadbCR.Object, "spec")
		if err != nil || !found {
			return "", "", fmt.Errorf("failed to get spec from MariaDB CR: %w", err)
		}

		rootPasswordRef, found, err := unstructured.NestedMap(spec, "rootPasswordSecretKeyRef")
		if err != nil || !found {
			return "", "", fmt.Errorf("rootPasswordSecretKeyRef not found in MariaDB spec: %w", err)
		}

		rootSecretName, found, err := unstructured.NestedString(rootPasswordRef, "name")
		if err != nil || !found {
			return "", "", fmt.Errorf("secret name not found in rootPasswordSecretKeyRef: %w", err)
		}

		rootSecretKey, found, err := unstructured.NestedString(rootPasswordRef, "key")
		if err != nil || !found {
			// Default key is "password" if not specified
			rootSecretKey = "password"
			logger.Info("Using default key 'password' for root secret")
		}

		// Get the root password secret
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

func (r *FrappeSiteReconciler) getPodSecurityContext(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) *corev1.PodSecurityContext {
	if bench.Spec.Security != nil && bench.Spec.Security.PodSecurityContext != nil {
		return bench.Spec.Security.PodSecurityContext
	}
	// Default to 1001 (OpenShift standard) but allow override via environment
	defaultUID := getDefaultUID()
	defaultGID := getDefaultGID()
	defaultFSGroup := getDefaultFSGroup()

	secCtx := &corev1.PodSecurityContext{
		RunAsNonRoot: boolPtr(true),
		// RunAsUser:    defaultUID,
		// RunAsGroup:   defaultGID,
		FSGroup: defaultFSGroup,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	if !r.IsOpenShift {
		secCtx.RunAsUser = defaultUID
		secCtx.RunAsGroup = defaultGID
	} else {
		// On OpenShift, rely on SCC restricted-v2 to inject FSGroup
		// set FSGroup to 0 to trigger recursive relabeling - REMOVED for restricted-v2
		// Skip RunAsUser/RunAsGroup to allow SCC to assign them
		secCtx.FSGroup = nil
		secCtx.SupplementalGroups = nil
	}

	return secCtx
}

func (r *FrappeSiteReconciler) getContainerSecurityContext(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) *corev1.SecurityContext {
	if bench.Spec.Security != nil && bench.Spec.Security.SecurityContext != nil {
		return bench.Spec.Security.SecurityContext
	}
	// Default to 1001 (OpenShift standard) but allow override via environment
	defaultUID := getDefaultUID()
	defaultGID := getDefaultGID()

	secCtx := &corev1.SecurityContext{
		RunAsNonRoot: boolPtr(true),
		// RunAsUser:                defaultUID,
		// RunAsGroup:               defaultGID,
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		ReadOnlyRootFilesystem: boolPtr(false),
	}

	if !r.IsOpenShift {
		secCtx.RunAsUser = defaultUID
		secCtx.RunAsGroup = defaultGID
	}

	return secCtx
}
