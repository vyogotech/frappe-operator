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

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	"github.com/vyogotech/frappe-operator/pkg/backoff"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	frappeSiteFinalizer      = "vyogo.tech/site-finalizer"
	requeueBackoffBase       = 10 * time.Second
	requeueBackoffMax        = 5 * time.Minute
	requeueAttemptAnnotation = "frappe.vyogo.tech/requeue-attempt"
)

// FrappeSiteReconciler reconciles a FrappeSite object
type FrappeSiteReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	Recorder                record.EventRecorder
	IsOpenShift             bool
	MaxConcurrentReconciles int
}

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
	startTime := time.Now()

	site := &vyogotechv1alpha1.FrappeSite{}
	if err := r.Get(ctx, req.NamespacedName, site); err != nil {
		if !errors.IsNotFound(err) {
			ReconciliationErrors.WithLabelValues("frappesite", "fetch_error").Inc()
		}
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

	// Early-exit guard
	if site.Status.Phase == vyogotechv1alpha1.FrappeSitePhaseReady && site.Status.ObservedGeneration == site.Generation {
		logger.V(1).Info("Site is Ready and spec unchanged, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if site.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(site, frappeSiteFinalizer) {
			logger.Info("Deleting site", "site", site.Name)
			r.Recorder.Event(site, corev1.EventTypeNormal, "Deleting", "FrappeSite deletion started")

			r.setCondition(site, metav1.Condition{
				Type:    "Terminating",
				Status:  metav1.ConditionTrue,
				Reason:  "Deleting",
				Message: "Site is being deleted",
			})
			if err := r.updateStatus(ctx, site); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.deleteSite(ctx, site); err != nil {
				logger.Error(err, "Failed to delete site, will requeue")
				r.setCondition(site, metav1.Condition{
					Type:    "Terminating",
					Status:  metav1.ConditionTrue,
					Reason:  "DeletionInProgress",
					Message: fmt.Sprintf("Site deletion in progress: %v", err),
				})
				_ = r.updateStatus(ctx, site)

				attempt := r.getRequeueAttempt(site)
				_ = r.patchRequeueAttempt(ctx, site, attempt+1)
				return ctrl.Result{RequeueAfter: backoff.ExponentialBackoff(15*time.Second, attempt, requeueBackoffMax)}, nil
			}

			// Cleanup remaining resources if any
			// (Secret and Ingress cleanup already partially handled by site finalizer or owner refs)

			logger.Info("FrappeSite cleanup complete, removing finalizer")
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
		return ctrl.Result{}, err
	}

	// Validate and Get Bench
	if site.Spec.BenchRef == nil {
		return r.failReconciliation(ctx, site, "benchRef is required", "ValidationFailed")
	}

	bench := &vyogotechv1alpha1.FrappeBench{}
	benchKey := types.NamespacedName{Name: site.Spec.BenchRef.Name, Namespace: site.Spec.BenchRef.Namespace}
	if benchKey.Namespace == "" {
		benchKey.Namespace = site.Namespace
	}

	if err := r.Get(ctx, benchKey, bench); err != nil {
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhasePending
		r.setCondition(site, metav1.Condition{
			Type:    "BenchReady",
			Status:  metav1.ConditionFalse,
			Reason:  "BenchNotFound",
			Message: fmt.Sprintf("Failed to get referenced bench: %v", err),
		})
		_ = r.updateStatus(ctx, site)
		attempt := r.getRequeueAttempt(site)
		_ = r.patchRequeueAttempt(ctx, site, attempt+1)
		return ctrl.Result{RequeueAfter: backoff.ExponentialBackoff(30*time.Second, attempt, requeueBackoffMax)}, nil
	}

	if bench.Status.Phase != "Ready" {
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhasePending
		r.setCondition(site, metav1.Condition{
			Type:    "BenchReady",
			Status:  metav1.ConditionFalse,
			Reason:  "BenchNotReady",
			Message: fmt.Sprintf("Bench %s is not ready", bench.Name),
		})
		_ = r.updateStatus(ctx, site)
		attempt := r.getRequeueAttempt(site)
		_ = r.patchRequeueAttempt(ctx, site, attempt+1)
		return ctrl.Result{RequeueAfter: backoff.ExponentialBackoff(requeueBackoffBase, attempt, requeueBackoffMax)}, nil
	}

	r.setCondition(site, metav1.Condition{
		Type:    "BenchReady",
		Status:  metav1.ConditionTrue,
		Reason:  "BenchReady",
		Message: "Referenced bench is ready",
	})

	// Resolve Domain and DB Config
	domain, domainSource := r.resolveDomain(ctx, site, bench)
	site.Status.ResolvedDomain = domain
	site.Status.DomainSource = domainSource
	dbConfig := r.resolveDBConfig(site, bench)

	// Provision Database
	dbProvider, err := database.NewProvider(dbConfig, r.Client, r.Scheme)
	if err != nil {
		return r.failReconciliation(ctx, site, fmt.Sprintf("Failed to create database provider: %v", err), "DatabaseProviderFailed")
	}

	dbReady, err := dbProvider.IsReady(ctx, site)
	if err != nil || !dbReady {
		if err == nil {
			_, err = dbProvider.EnsureDatabase(ctx, site)
		}
		if err != nil {
			return r.failReconciliation(ctx, site, fmt.Sprintf("Database provisioning failed: %v", err), "DatabaseFailed")
		}
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseProvisioning
		r.setCondition(site, metav1.Condition{
			Type:    "DatabaseReady",
			Status:  metav1.ConditionFalse,
			Reason:  "Provisioning",
			Message: "Database is being provisioned",
		})
		_ = r.updateStatus(ctx, site)
		attempt := r.getRequeueAttempt(site)
		_ = r.patchRequeueAttempt(ctx, site, attempt+1)
		return ctrl.Result{RequeueAfter: backoff.ExponentialBackoff(requeueBackoffBase, attempt, requeueBackoffMax)}, nil
	}

	r.setCondition(site, metav1.Condition{
		Type:    "DatabaseReady",
		Status:  metav1.ConditionTrue,
		Reason:  "DatabaseReady",
		Message: "Database is ready",
	})

	dbInfo, _ := dbProvider.EnsureDatabase(ctx, site)
	dbCreds, _ := dbProvider.GetCredentials(ctx, site)
	site.Status.DatabaseName = dbInfo.Name
	site.Status.DatabaseCredentialsSecret = dbCreds.SecretName

	// Initialize Site
	siteReady, err := r.ensureSiteInitialized(ctx, site, bench, domain, dbInfo, dbCreds)
	if err != nil {
		return r.failReconciliation(ctx, site, fmt.Sprintf("Site initialization failed: %v", err), "SiteInitializationFailed")
	}

	if !siteReady {
		site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseProvisioning
		_ = r.updateStatus(ctx, site)
		attempt := r.getRequeueAttempt(site)
		_ = r.patchRequeueAttempt(ctx, site, attempt+1)
		return ctrl.Result{RequeueAfter: backoff.ExponentialBackoff(requeueBackoffBase, attempt, requeueBackoffMax)}, nil
	}

	// External Access (Ingress/Route)
	if site.Spec.Ingress == nil || site.Spec.Ingress.Enabled == nil || *site.Spec.Ingress.Enabled {
		if r.IsOpenShift && (site.Spec.RouteConfig == nil || site.Spec.RouteConfig.Enabled == nil || *site.Spec.RouteConfig.Enabled) {
			if err := r.ensureRoute(ctx, site, bench, domain); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			if err := r.ensureIngress(ctx, site, bench, domain); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Finalize status
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseReady
	site.Status.ObservedGeneration = site.Generation
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
		Type:   "Progressing",
		Status: metav1.ConditionFalse,
		Reason: "Complete",
	})

	if err := r.updateStatus(ctx, site); err != nil {
		return ctrl.Result{}, err
	}

	ResourceTotal.WithLabelValues("frappesite", site.Namespace).Inc()
	ReconciliationDuration.WithLabelValues("frappesite", "success").Observe(time.Since(startTime).Seconds())
	return ctrl.Result{}, nil
}

func (r *FrappeSiteReconciler) failReconciliation(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, msg, reason string) (ctrl.Result, error) {
	site.Status.Phase = vyogotechv1alpha1.FrappeSitePhaseFailed
	r.setCondition(site, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	r.Recorder.Event(site, corev1.EventTypeWarning, reason, msg)
	_ = r.updateStatus(ctx, site)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *FrappeSiteReconciler) setCondition(site *vyogotechv1alpha1.FrappeSite, condition metav1.Condition) {
	condition.ObservedGeneration = site.Generation
	meta.SetStatusCondition(&site.Status.Conditions, condition)
}

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
		site.ResourceVersion = latest.ResourceVersion
		return nil
	})
}

func (r *FrappeSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("frappesite-controller")
	}
	opts := controller.Options{}
	if r.MaxConcurrentReconciles > 0 {
		opts.MaxConcurrentReconciles = r.MaxConcurrentReconciles
	}
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(opts).
		For(&vyogotechv1alpha1.FrappeSite{}).
		Owns(&batchv1.Job{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
