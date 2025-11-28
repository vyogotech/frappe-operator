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
	"github.com/vyogotech/frappe-operator/controllers/database"
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
			logger.Info("Deleting site", "site", site.Name)
			// TODO: Implement site deletion job (bench drop-site)
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

	// Check if bench is ready (has pods running)
	// For now, we'll assume bench is ready if it exists
	site.Status.BenchReady = true
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseProvisioning

	// Resolve the final domain for the site (with smart auto-detection)
	domain, domainSource := r.resolveDomain(ctx, site, bench)

	// Update status with resolved domain
	site.Status.ResolvedDomain = domain
	site.Status.DomainSource = domainSource

	// 0. Provision database using database provider
	dbProvider, err := database.NewProvider(site.Spec.DBConfig.Provider, r.Client, r.Scheme)
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
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{}, err
	}

	if !dbReady {
		logger.Info("Database not ready, provisioning...")
		site.Status.DatabaseReady = false
		_ = r.Status().Update(ctx, site)

		// Ensure database resources are created
		dbInfo, err := dbProvider.EnsureDatabase(ctx, site)
		if err != nil {
			logger.Error(err, "Failed to ensure database")
			site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
			_ = r.Status().Update(ctx, site)
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
	_ = r.Status().Update(ctx, site)

	// 1. Ensure site is initialized with database credentials
	siteReady, err := r.ensureSiteInitialized(ctx, site, bench, domain, dbInfo, dbCreds)
	if err != nil {
		logger.Error(err, "Failed to initialize site")
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{}, err
	}

	if !siteReady {
		logger.Info("Site initialization in progress", "site", site.Name)
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseProvisioning
		_ = r.Status().Update(ctx, site)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// 2. Ensure Ingress (enabled by default, can be disabled)
	createIngress := true
	if site.Spec.Ingress != nil && site.Spec.Ingress.Enabled != nil && !*site.Spec.Ingress.Enabled {
		createIngress = false
		logger.Info("Ingress creation disabled by user", "site", site.Name)
	}
	
	if createIngress {
		if err := r.ensureIngress(ctx, site, bench, domain); err != nil {
			logger.Error(err, "Failed to ensure Ingress")
			return ctrl.Result{}, err
		}
	}

	// 3. Update final status
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseReady
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseReady
	site.Status.SiteURL = fmt.Sprintf("http://%s", domain)
	if site.Spec.TLS.Enabled {
		site.Status.SiteURL = fmt.Sprintf("https://%s", domain)
	}

	if err := r.Status().Update(ctx, site); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("FrappeSite reconciled successfully", "site", site.Name, "domain", domain)
	return ctrl.Result{}, nil
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
    bench new-site "$SITE_NAME" \
      --db-type="$DB_PROVIDER" \
      --db-name="$DB_NAME" \
      --db-host="$DB_HOST" \
      --db-port="$DB_PORT" \
      --db-user="$DB_USER" \
      --db-password="$DB_PASSWORD" \
      --no-setup-db \
      --admin-password="$ADMIN_PASSWORD" \
      --install-app=erpnext \
      --verbose

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
					RestartPolicy: corev1.RestartPolicyNever,
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

// SetupWithManager sets up the controller with the Manager
func (r *FrappeSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1alpha1.FrappeSite{}).
		Owns(&batchv1.Job{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
