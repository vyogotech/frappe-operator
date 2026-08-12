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
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch

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

	appliedKeys := []string{}

	if siteConfig.Spec.MaintenanceMode != nil {
		appliedKeys = append(appliedKeys, "maintenance_mode")
	}
	if siteConfig.Spec.MaxFileSize != nil {
		appliedKeys = append(appliedKeys, "max_file_size")
	}
	if siteConfig.Spec.EncryptionKeySecretRef != nil && siteConfig.Spec.EncryptionKeySecretRef.Name != "" {
		appliedKeys = append(appliedKeys, "encryption_key")
	}
	for k := range siteConfig.Spec.CustomConfig {
		appliedKeys = append(appliedKeys, k)
	}

	siteConfig.Status.Phase = "Ready"
	siteConfig.Status.AppliedKeys = appliedKeys
	siteConfig.Status.ObservedGeneration = siteConfig.Generation
	r.setCondition(siteConfig, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "ConfigApplied",
		Message: "Site configuration successfully applied",
	})
	_ = r.updateStatus(ctx, siteConfig)

	return ctrl.Result{}, nil
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
		Complete(r)
}
