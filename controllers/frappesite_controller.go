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
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
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
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets;services;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.mariadb.com,resources=mariadbs;databases;users;grants,verbs=get;list;watch;create;update;patch;delete

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

	// Handle deletion
	if site.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(site, frappeSiteFinalizer) {
			// Run site deletion job (bench drop-site)
			logger.Info("Deleting site", "site", site.Name)
			// TODO: Implement site deletion job
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
		_ = r.Status().Update(ctx, site)
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
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if bench is ready
	if !bench.Status.Ready {
		logger.Info("Waiting for bench to be ready", "bench", bench.Name)
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhasePending
		site.Status.BenchReady = false
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	site.Status.BenchReady = true
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseProvisioning

	// Resolve the final domain for the site
	domain, domainSource, err := r.resolveDomain(ctx, site, bench)
	if err != nil {
		logger.Error(err, "Failed to resolve domain")
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{}, err
	}

	// Update status with resolved domain
	site.Status.ResolvedDomain = domain
	site.Status.DomainSource = domainSource

	// NOTE: Database and user creation is handled by bench new-site
	// No need to pre-provision via MariaDB Operator

	// 1. Ensure site is initialized (bench new-site creates DB and user automatically)
	siteReady, err := r.ensureSiteInitialized(ctx, site, bench, domain)
	if err != nil {
		logger.Error(err, "Failed to initialize site")
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{}, err
	}

	if !siteReady {
		logger.Info("Site initialization in progress")
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("Site initialized successfully", "site", site.Name)

	// 4. Update Frappe site config with correct domain
	if err := r.updateFrappeSiteConfig(ctx, site, bench, domain); err != nil {
		logger.Error(err, "Failed to update site config")
		return ctrl.Result{}, err
	}

	// 5. Create Ingress for site
	if err := r.reconcileIngress(ctx, site, bench, domain); err != nil {
		logger.Error(err, "Failed to reconcile Ingress")
		return ctrl.Result{}, err
	}

	// Update status to Ready
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseReady
	scheme := "https"
	if site.Spec.TLS.Enabled || (site.Spec.Ingress != nil && site.Spec.Ingress.TLS != nil && site.Spec.Ingress.TLS.Enabled) {
		scheme = "https"
	} else {
		scheme = "http"
	}
	site.Status.SiteURL = fmt.Sprintf("%s://%s", scheme, domain)
	if err := r.Status().Update(ctx, site); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("FrappeSite reconciliation completed", "site", site.Name)
	return ctrl.Result{}, nil
}

// ensureSiteInitialized creates and monitors the site init job
func (r *FrappeSiteReconciler) ensureSiteInitialized(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string) (bool, error) {
	logger := log.FromContext(ctx)
	jobName := fmt.Sprintf("%s-init", site.Name)

	// Check if init job already completed successfully
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err == nil {
		if job.Status.Succeeded > 0 {
			logger.Info("Site initialization already completed")
			return true, nil
		}
		if job.Status.Failed > 0 {
			logger.Error(nil, "Site initialization failed")
			return false, fmt.Errorf("site initialization job failed")
		}
		logger.Info("Site initialization in progress", "active", job.Status.Active)
		return false, nil
	}

	if !errors.IsNotFound(err) {
		return false, err
	}

	// Create site init job
	logger.Info("Creating site initialization job")
	job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: site.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, job, func() error {
		if err := controllerutil.SetControllerReference(site, job, r.Scheme); err != nil {
			return err
		}

		backoffLimit := int32(3)
		job.Spec = batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "init-site",
							Image:   "frappe/erpnext:latest",
							Command: []string{"bash", "-c"},
							Args: []string{
								fmt.Sprintf(`set -e
								cd /home/frappe/frappe-bench
								
								echo "=== Initializing site: %s ==="
								echo "Database host: $DB_HOST"
								echo "Database port: $DB_PORT"
								
								# Check if site already exists
								if [ -d "sites/%s" ] && [ -f "sites/%s/site_config.json" ]; then
								  echo "✅ Site %s already exists and is configured"
								  exit 0
								fi
								
								echo "Creating new site: %s"
								
								# Create site - let bench create the database and user automatically
								# MariaDB root password is provided via secret mount
								ROOT_PASSWORD=$(cat /run/secrets/mariadb-root/password)
								
								echo "$ROOT_PASSWORD" | bench new-site %s \
								  --mariadb-root-username root \
								  --db-host "$DB_HOST" \
								  --db-port "$DB_PORT" \
								  --mariadb-user-host-login-scope '%%' \
								  --admin-password admin || {
								    echo "❌ Site creation failed, cleaning up..."
								    rm -rf "sites/%s"
								    exit 1
								  }
								
								
								echo "✅ Site created successfully"
								echo "Verifying site directory..."
								ls -la sites/%s/
								echo "Site config:"
								cat sites/%s/site_config.json
								`, domain, domain, domain, domain, domain, domain, domain, domain, domain),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
								{
									Name:      "mariadb-root",
									MountPath: "/run/secrets/mariadb-root",
									ReadOnly:  true,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DB_HOST",
									Value: "frappe-shared-mariadb.default.svc.cluster.local",
								},
								{
									Name:  "DB_PORT",
									Value: "3306",
								},
							},
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
							Name: "mariadb-root",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "mariadb-root",
								},
							},
						},
					},
				},
			},
		}
		return nil
	}); err != nil {
		return false, fmt.Errorf("failed to create site init job: %w", err)
	}

	return false, nil
}

// ensureIngress creates or updates the Ingress for the site
// resolveDomain determines the final domain for the site
func (r *FrappeSiteReconciler) resolveDomain(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench) (string, string, error) {
	logger := log.FromContext(ctx)

	// Priority 1: Explicit domain in FrappeSite
	if site.Spec.Domain != "" {
		logger.Info("Using explicit domain from FrappeSite spec", "domain", site.Spec.Domain)
		return site.Spec.Domain, "explicit", nil
	}

	// Priority 2: Bench domain config with suffix
	if bench.Spec.DomainConfig != nil && bench.Spec.DomainConfig.Suffix != "" {
		domain := site.Spec.SiteName + bench.Spec.DomainConfig.Suffix
		logger.Info("Using bench domain suffix", "domain", domain, "suffix", bench.Spec.DomainConfig.Suffix)
		return domain, "bench-suffix", nil
	}

	// Priority 3: Auto-detect (if enabled)
	autoDetect := true
	if bench.Spec.DomainConfig != nil && bench.Spec.DomainConfig.AutoDetect != nil {
		autoDetect = *bench.Spec.DomainConfig.AutoDetect
	}

	if autoDetect {
		detector := &DomainDetector{Client: r.Client}
		suffix, err := detector.DetectDomainSuffix(ctx, site.Namespace)
		if err == nil && suffix != "" {
			domain := site.Spec.SiteName + suffix
			logger.Info("Auto-detected domain suffix", "domain", domain, "suffix", suffix)
			return domain, "auto-detected", nil
		}
		logger.V(1).Info("Auto-detection failed, falling back to siteName", "error", err)
	}

	// Priority 4: Use siteName as-is
	logger.Info("Using siteName as final domain", "domain", site.Spec.SiteName)
	return site.Spec.SiteName, "sitename-default", nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FrappeSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1alpha1.FrappeSite{}).
		Owns(&batchv1.Job{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
