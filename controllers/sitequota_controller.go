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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// SiteQuotaReconciler reconciles a SiteQuota object
type SiteQuotaReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=sitequotas,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitequotas/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitequotas/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch

func (r *SiteQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	siteQuota := &vyogotechv1.SiteQuota{}
	if err := r.Get(ctx, req.NamespacedName, siteQuota); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteQuota.Spec.SiteRef == nil || siteQuota.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteQuota, "siteRef.name is required", "ValidationFailed")
	}

	siteNamespace := siteQuota.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteQuota.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteQuota.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteQuota.Status.Phase = "Pending"
		r.setCondition(siteQuota, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteQuota)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteQuota.Status.Phase = "Pending"
		r.setCondition(siteQuota, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteQuota)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Evaluate Quotas (Mock/Observed metrics)
	currentDBSize := int64(120)   // 120 MB observed
	currentStorage := int64(450) // 450 MB observed
	currentUsers := int32(5)     // 5 active users

	quotaExceeded := false
	exceededReason := ""

	if siteQuota.Spec.MaxDBSizeMB != nil && currentDBSize > *siteQuota.Spec.MaxDBSizeMB {
		quotaExceeded = true
		exceededReason = fmt.Sprintf("Database size (%d MB) exceeds limit (%d MB)", currentDBSize, *siteQuota.Spec.MaxDBSizeMB)
	}
	if siteQuota.Spec.MaxStorageMB != nil && currentStorage > *siteQuota.Spec.MaxStorageMB {
		quotaExceeded = true
		exceededReason = fmt.Sprintf("Storage size (%d MB) exceeds limit (%d MB)", currentStorage, *siteQuota.Spec.MaxStorageMB)
	}
	if siteQuota.Spec.MaxUsers != nil && currentUsers > *siteQuota.Spec.MaxUsers {
		quotaExceeded = true
		exceededReason = fmt.Sprintf("Active user count (%d) exceeds limit (%d)", currentUsers, *siteQuota.Spec.MaxUsers)
	}

	siteQuota.Status.CurrentDBSizeMB = currentDBSize
	siteQuota.Status.CurrentStorageMB = currentStorage
	siteQuota.Status.CurrentUsers = currentUsers
	siteQuota.Status.QuotaExceeded = quotaExceeded
	siteQuota.Status.ObservedGeneration = siteQuota.Generation

	if quotaExceeded {
		siteQuota.Status.Phase = "QuotaExceeded"
		r.setCondition(siteQuota, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "QuotaBreached",
			Message: exceededReason,
		})
		if r.Recorder != nil {
			r.Recorder.Event(siteQuota, corev1.EventTypeWarning, "QuotaBreached", exceededReason)
		}
	} else {
		siteQuota.Status.Phase = "Normal"
		r.setCondition(siteQuota, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "QuotaNormal",
			Message: "All resource usages are within allowed limits",
		})
	}

	_ = r.updateStatus(ctx, siteQuota)
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *SiteQuotaReconciler) failReconciliation(ctx context.Context, siteQuota *vyogotechv1.SiteQuota, msg, reason string) (ctrl.Result, error) {
	siteQuota.Status.Phase = "Failed"
	r.setCondition(siteQuota, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteQuota, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteQuota)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteQuotaReconciler) setCondition(siteQuota *vyogotechv1.SiteQuota, condition metav1.Condition) {
	condition.ObservedGeneration = siteQuota.Generation
	meta.SetStatusCondition(&siteQuota.Status.Conditions, condition)
}

func (r *SiteQuotaReconciler) updateStatus(ctx context.Context, siteQuota *vyogotechv1.SiteQuota) error {
	latest := &vyogotechv1.SiteQuota{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteQuota.Name, Namespace: siteQuota.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteQuota.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteQuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("sitequota-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteQuota{}).
		Complete(r)
}
