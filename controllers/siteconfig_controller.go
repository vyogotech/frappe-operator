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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// SiteConfigReconciler reconciles a SiteConfig object
type SiteConfigReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	FrappeClient *FrappeClient // Optional injected client for testing
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=siteconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites;frappebenches,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *SiteConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	siteConfig := &vyogotechv1.SiteConfig{}
	if err := r.Get(ctx, req.NamespacedName, siteConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteConfig.Spec.SiteRef == nil || siteConfig.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteConfig, "siteRef.name is required", "ValidationFailed")
	}

	siteNamespace := siteConfig.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteConfig.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteConfig.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteConfig.Status.Phase = "Pending"
		r.setCondition(siteConfig, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteConfig)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteConfig.Status.Phase = "Pending"
		r.setCondition(siteConfig, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteConfig)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	r.setCondition(siteConfig, metav1.Condition{
		Type:    "SiteReady",
		Status:  metav1.ConditionTrue,
		Reason:  "SiteReady",
		Message: "Referenced FrappeSite is ready",
	})

	// Resolve the referenced bench (for the image + sites PVC) and the bench --site target.
	if site.Spec.BenchRef == nil || site.Spec.BenchRef.Name == "" {
		return r.failReconciliation(ctx, siteConfig, "referenced FrappeSite has no benchRef; cannot apply config", "BenchRefMissing")
	}
	bench := &vyogotechv1.FrappeBench{}
	if err := r.Get(ctx, types.NamespacedName{Name: site.Spec.BenchRef.Name, Namespace: site.Namespace}, bench); err != nil {
		return r.failReconciliation(ctx, siteConfig, fmt.Sprintf("Referenced FrappeBench %s not found: %v", site.Spec.BenchRef.Name, err), "BenchNotFound")
	}
	benchImage := "frappe/erpnext:latest"
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		benchImage = bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			benchImage = fmt.Sprintf("%s:%s", benchImage, bench.Spec.ImageConfig.Tag)
		}
	}
	domain := site.Status.ResolvedDomain
	if domain == "" {
		domain = site.Spec.SiteName
	}

	job, appliedKeys := buildConfigJob(siteConfig, bench, domain, benchImage)
	if job == nil {
		// Nothing to apply — converge to Ready with no keys.
		siteConfig.Status.Phase = "Ready"
		siteConfig.Status.AppliedKeys = nil
		siteConfig.Status.ObservedGeneration = siteConfig.Generation
		r.setCondition(siteConfig, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "NoConfig", Message: "No configuration keys to apply"})
		_ = r.updateStatus(ctx, siteConfig)
		return ctrl.Result{}, nil
	}
	if err := controllerutil.SetControllerReference(siteConfig, job, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// The Job is named per-generation, so a spec change produces a fresh apply Job.
	existing := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, job); err != nil {
			return r.failReconciliation(ctx, siteConfig, fmt.Sprintf("failed to create config apply Job: %v", err), "JobCreateFailed")
		}
		siteConfig.Status.Phase = "Applying"
		r.setCondition(siteConfig, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Applying", Message: "Applying site configuration"})
		_ = r.updateStatus(ctx, siteConfig)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if existing.Status.Succeeded > 0 {
		siteConfig.Status.Phase = "Ready"
		siteConfig.Status.AppliedKeys = appliedKeys
		siteConfig.Status.ObservedGeneration = siteConfig.Generation
		r.setCondition(siteConfig, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "ConfigApplied", Message: "Site configuration successfully applied"})
		_ = r.updateStatus(ctx, siteConfig)
		return ctrl.Result{}, nil
	}
	for _, c := range existing.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return r.failReconciliation(ctx, siteConfig, "config apply Job failed; check the Job logs", "ApplyFailed")
		}
	}

	// Still applying.
	siteConfig.Status.Phase = "Applying"
	_ = r.updateStatus(ctx, siteConfig)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *SiteConfigReconciler) failReconciliation(ctx context.Context, siteConfig *vyogotechv1.SiteConfig, msg, reason string) (ctrl.Result, error) {
	siteConfig.Status.Phase = "Failed"
	r.setCondition(siteConfig, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteConfig, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteConfig)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteConfigReconciler) setCondition(siteConfig *vyogotechv1.SiteConfig, condition metav1.Condition) {
	condition.ObservedGeneration = siteConfig.Generation
	meta.SetStatusCondition(&siteConfig.Status.Conditions, condition)
}

func (r *SiteConfigReconciler) updateStatus(ctx context.Context, siteConfig *vyogotechv1.SiteConfig) error {
	latest := &vyogotechv1.SiteConfig{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteConfig.Name, Namespace: siteConfig.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteConfig.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("siteconfig-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteConfig{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
