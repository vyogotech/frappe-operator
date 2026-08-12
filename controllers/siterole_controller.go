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

// SiteRoleReconciler reconciles a SiteRole object
type SiteRoleReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	FrappeClient *FrappeClient // Optional injected client for testing
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=siteroles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteroles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteroles/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch

func (r *SiteRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	siteRole := &vyogotechv1.SiteRole{}
	if err := r.Get(ctx, req.NamespacedName, siteRole); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteRole.Spec.SiteRef == nil || siteRole.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteRole, "siteRef.name is required", "ValidationFailed")
	}
	if siteRole.Spec.RoleName == "" {
		return r.failReconciliation(ctx, siteRole, "roleName is required", "ValidationFailed")
	}

	siteNamespace := siteRole.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteRole.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteRole.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteRole.Status.Phase = "Pending"
		r.setCondition(siteRole, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteRole)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteRole.Status.Phase = "Pending"
		r.setCondition(siteRole, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteRole)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	r.setCondition(siteRole, metav1.Condition{
		Type:    "SiteReady",
		Status:  metav1.ConditionTrue,
		Reason:  "SiteReady",
		Message: "Referenced FrappeSite is ready",
	})

	// Fetch admin password for REST authentication
	adminPassword, err := r.getAdminPassword(ctx, site)
	if err != nil {
		return r.failReconciliation(ctx, siteRole, fmt.Sprintf("Failed to fetch admin password: %v", err), "AdminPasswordFailed")
	}

	// Determine Frappe Base URL & Host Header
	baseURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", site.Name, site.Namespace)
	hostHeader := site.Status.ResolvedDomain
	if site.Spec.BenchRef != nil && site.Spec.BenchRef.Name != "" {
		baseURL = fmt.Sprintf("http://%s-nginx.%s.svc.cluster.local:8080", site.Spec.BenchRef.Name, site.Namespace)
	} else if site.Status.ResolvedDomain != "" {
		baseURL = fmt.Sprintf("http://%s", site.Status.ResolvedDomain)
	}

	// Local dev fallback if running outside K8s cluster
	frappeClient := r.FrappeClient
	if frappeClient == nil {
		c := NewFrappeClient(baseURL, "Administrator", adminPassword)
		c.HostHeader = hostHeader
		// Check if cluster URL is resolvable, otherwise fallback to local port-forward
		if err := c.Authenticate(ctx); err != nil && (strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "connection refused")) {
			cLocal := NewFrappeClient("http://127.0.0.1:8080", "Administrator", adminPassword)
			cLocal.HostHeader = hostHeader
			if errLocal := cLocal.Authenticate(ctx); errLocal == nil {
				frappeClient = cLocal
			} else {
				frappeClient = c
			}
		} else {
			frappeClient = c
		}
	}

	// Ensure Role exists in Frappe
	err = frappeClient.EnsureRole(ctx, siteRole.Spec.RoleName, siteRole.Spec.DeskAccess, siteRole.Spec.Disabled)
	if err != nil {
		return r.failReconciliation(ctx, siteRole, fmt.Sprintf("Failed to sync role with Frappe: %v", err), "RoleSyncFailed")
	}

	siteRole.Status.Phase = "Ready"
	siteRole.Status.ObservedGeneration = siteRole.Generation
	r.setCondition(siteRole, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "RoleSynced",
		Message: "Role successfully synced with Frappe",
	})
	_ = r.updateStatus(ctx, siteRole)

	return ctrl.Result{}, nil
}

func (r *SiteRoleReconciler) getAdminPassword(ctx context.Context, site *vyogotechv1.FrappeSite) (string, error) {
	secretName := fmt.Sprintf("%s-admin", site.Name)
	if site.Spec.AdminPasswordSecretRef != nil && site.Spec.AdminPasswordSecretRef.Name != "" {
		secretName = site.Spec.AdminPasswordSecretRef.Name
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, secret)
	if err != nil {
		return "", err
	}
	password, ok := secret.Data["password"]
	if !ok {
		return "", fmt.Errorf("key 'password' not found in secret %s", secretName)
	}
	return string(password), nil
}

func (r *SiteRoleReconciler) failReconciliation(ctx context.Context, siteRole *vyogotechv1.SiteRole, msg, reason string) (ctrl.Result, error) {
	siteRole.Status.Phase = "Failed"
	r.setCondition(siteRole, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteRole, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteRole)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteRoleReconciler) setCondition(siteRole *vyogotechv1.SiteRole, condition metav1.Condition) {
	condition.ObservedGeneration = siteRole.Generation
	meta.SetStatusCondition(&siteRole.Status.Conditions, condition)
}

func (r *SiteRoleReconciler) updateStatus(ctx context.Context, siteRole *vyogotechv1.SiteRole) error {
	latest := &vyogotechv1.SiteRole{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteRole.Name, Namespace: siteRole.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteRole.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("siterole-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteRole{}).
		Complete(r)
}
