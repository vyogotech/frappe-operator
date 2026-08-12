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

// SiteClientScriptReconciler reconciles a SiteClientScript object
type SiteClientScriptReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	FrappeClient *FrappeClient
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=siteclientscripts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteclientscripts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch

func (r *SiteClientScriptReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Reconciling SiteClientScript", "name", req.Name)

	clientScript := &vyogotechv1.SiteClientScript{}
	if err := r.Get(ctx, req.NamespacedName, clientScript); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if clientScript.Spec.SiteRef == nil || clientScript.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, clientScript, "siteRef.name is required", "ValidationFailed")
	}
	if clientScript.Spec.DT == "" {
		return r.failReconciliation(ctx, clientScript, "dt is required", "ValidationFailed")
	}
	if clientScript.Spec.Script == "" {
		return r.failReconciliation(ctx, clientScript, "script is required", "ValidationFailed")
	}

	siteNamespace := clientScript.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = clientScript.Namespace
	}

	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: clientScript.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		clientScript.Status.Phase = "Pending"
		r.setCondition(clientScript, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, clientScript)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		clientScript.Status.Phase = "Pending"
		r.setCondition(clientScript, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, clientScript)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	r.setCondition(clientScript, metav1.Condition{
		Type:    "SiteReady",
		Status:  metav1.ConditionTrue,
		Reason:  "SiteReady",
		Message: "Referenced FrappeSite is ready",
	})

	adminPassword, err := r.getAdminPassword(ctx, site)
	if err != nil {
		return r.failReconciliation(ctx, clientScript, fmt.Sprintf("Failed to fetch admin password: %v", err), "AdminPasswordFailed")
	}

	baseURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", site.Name, site.Namespace)
	hostHeader := site.Status.ResolvedDomain
	if site.Spec.BenchRef != nil && site.Spec.BenchRef.Name != "" {
		baseURL = fmt.Sprintf("http://%s-nginx.%s.svc.cluster.local:8080", site.Spec.BenchRef.Name, site.Namespace)
	} else if site.Status.ResolvedDomain != "" {
		baseURL = fmt.Sprintf("http://%s", site.Status.ResolvedDomain)
	}

	frappeClient := r.FrappeClient
	if frappeClient == nil {
		c := NewFrappeClient(baseURL, "Administrator", adminPassword)
		c.HostHeader = hostHeader
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

	err = frappeClient.EnsureClientScript(ctx, clientScript.Name, clientScript.Spec.DT, clientScript.Spec.Script, clientScript.Spec.Enabled)
	if err != nil {
		return r.failReconciliation(ctx, clientScript, fmt.Sprintf("Failed to sync client script with Frappe: %v", err), "ClientScriptSyncFailed")
	}

	clientScript.Status.Phase = "Ready"
	clientScript.Status.ObservedGeneration = clientScript.Generation
	r.setCondition(clientScript, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "ClientScriptSynced",
		Message: "Client Script successfully synced with Frappe",
	})
	_ = r.updateStatus(ctx, clientScript)

	return ctrl.Result{}, nil
}

func (r *SiteClientScriptReconciler) getAdminPassword(ctx context.Context, site *vyogotechv1.FrappeSite) (string, error) {
	secretName := fmt.Sprintf("%s-admin-password", site.Name)
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, secret); err != nil {
		return "", err
	}
	password := string(secret.Data["password"])
	if password == "" {
		return "", fmt.Errorf("password key missing or empty in secret %s", secretName)
	}
	return password, nil
}

func (r *SiteClientScriptReconciler) failReconciliation(ctx context.Context, clientScript *vyogotechv1.SiteClientScript, msg, reason string) (ctrl.Result, error) {
	clientScript.Status.Phase = "Failed"
	r.setCondition(clientScript, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	_ = r.updateStatus(ctx, clientScript)
	return ctrl.Result{}, fmt.Errorf("%s: %s", reason, msg)
}

func (r *SiteClientScriptReconciler) setCondition(clientScript *vyogotechv1.SiteClientScript, condition metav1.Condition) {
	condition.ObservedGeneration = clientScript.Generation
	meta.SetStatusCondition(&clientScript.Status.Conditions, condition)
}

func (r *SiteClientScriptReconciler) updateStatus(ctx context.Context, clientScript *vyogotechv1.SiteClientScript) error {
	return r.Status().Update(ctx, clientScript)
}

func (r *SiteClientScriptReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteClientScript{}).
		Complete(r)
}
