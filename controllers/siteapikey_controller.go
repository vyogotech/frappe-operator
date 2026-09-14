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
	"k8s.io/apimachinery/pkg/api/errors"
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

// SiteAPIKeyReconciler reconciles a SiteAPIKey object
type SiteAPIKeyReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	FrappeClient *FrappeClient // Optional injected client for testing
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=siteapikeys,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteapikeys/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteapikeys/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *SiteAPIKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	siteAPIKey := &vyogotechv1.SiteAPIKey{}
	if err := r.Get(ctx, req.NamespacedName, siteAPIKey); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteAPIKey.Spec.SiteRef == nil || siteAPIKey.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteAPIKey, "siteRef.name is required", "ValidationFailed")
	}
	if siteAPIKey.Spec.User == "" {
		return r.failReconciliation(ctx, siteAPIKey, "user is required", "ValidationFailed")
	}
	if siteAPIKey.Spec.SecretName == "" {
		return r.failReconciliation(ctx, siteAPIKey, "secretName is required", "ValidationFailed")
	}

	siteNamespace := siteAPIKey.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteAPIKey.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteAPIKey.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteAPIKey.Status.Phase = "Pending"
		r.setCondition(siteAPIKey, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteAPIKey)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteAPIKey.Status.Phase = "Pending"
		r.setCondition(siteAPIKey, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteAPIKey)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Already minted: the Secret exists with a real key and we recorded it. Do
	// not call generate_keys again — every call rotates the user's api_secret.
	existing := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteAPIKey.Spec.SecretName, Namespace: siteAPIKey.Namespace}, existing); err == nil &&
		siteAPIKey.Status.APIKeyGenerated && len(existing.Data["api_key"]) > 0 && !strings.HasPrefix(string(existing.Data["api_key"]), "k8s_ak_") {
		siteAPIKey.Status.Phase = "Ready"
		siteAPIKey.Status.SecretCreated = true
		siteAPIKey.Status.ObservedGeneration = siteAPIKey.Generation
		_ = r.updateStatus(ctx, siteAPIKey)
		return ctrl.Result{}, nil
	}

	// Mint the key pair through Frappe (`frappe.core.doctype.user.user.generate_keys`)
	// as Administrator, exactly as a human would in Desk. The values written to
	// the Secret are the ones Frappe will accept in `Authorization: token k:s`.
	adminPassword, err := siteAdminPassword(ctx, r.Client, site)
	if err != nil {
		return r.failReconciliation(ctx, siteAPIKey, fmt.Sprintf("Failed to fetch admin password: %v", err), "AdminPasswordFailed")
	}
	frappeClient := r.FrappeClient
	if frappeClient == nil {
		baseURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", site.Name, site.Namespace)
		if site.Spec.BenchRef != nil && site.Spec.BenchRef.Name != "" {
			baseURL = fmt.Sprintf("http://%s-nginx.%s.svc.cluster.local:8080", site.Spec.BenchRef.Name, site.Namespace)
		}
		c := NewFrappeClient(baseURL, "Administrator", adminPassword)
		c.HostHeader = site.Status.ResolvedDomain
		if c.HostHeader == "" {
			c.HostHeader = site.Spec.SiteName
		}
		frappeClient = c
	}
	apiKeyStr, apiSecretStr, err := frappeClient.GenerateAPIKeys(ctx, siteAPIKey.Spec.User)
	if err != nil {
		return r.failReconciliation(ctx, siteAPIKey, fmt.Sprintf("Failed to generate API keys in Frappe: %v", err), "KeyGenerationFailed")
	}
	if apiKeyStr == "" || apiSecretStr == "" {
		return r.failReconciliation(ctx, siteAPIKey, "Frappe returned an empty api_key/api_secret", "KeyGenerationFailed")
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      siteAPIKey.Spec.SecretName,
			Namespace: siteAPIKey.Namespace,
			Labels:    map[string]string{"app": "frappe", "site": site.Name},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"api_key":    []byte(apiKeyStr),
			"api_secret": []byte(apiSecretStr),
			"user":       []byte(siteAPIKey.Spec.User),
			"site_url":   []byte(site.Status.SiteURL),
		},
	}
	_ = controllerutil.SetControllerReference(siteAPIKey, secret, r.Scheme)
	var existingSecret corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: siteAPIKey.Spec.SecretName, Namespace: siteAPIKey.Namespace}, &existingSecret)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, secret); err != nil {
			return r.failReconciliation(ctx, siteAPIKey, fmt.Sprintf("Failed to create K8s Secret: %v", err), "SecretCreationFailed")
		}
	} else if err == nil {
		existingSecret.Data = secret.Data
		existingSecret.Labels = secret.Labels
		if err := r.Update(ctx, &existingSecret); err != nil {
			return r.failReconciliation(ctx, siteAPIKey, fmt.Sprintf("Failed to update K8s Secret: %v", err), "SecretUpdateFailed")
		}
	} else {
		return r.failReconciliation(ctx, siteAPIKey, fmt.Sprintf("Failed to read K8s Secret: %v", err), "SecretReadFailed")
	}
	siteAPIKey.Status.Phase = "Ready"
	siteAPIKey.Status.APIKeyGenerated = true
	siteAPIKey.Status.SecretCreated = true
	siteAPIKey.Status.ObservedGeneration = siteAPIKey.Generation
	r.setCondition(siteAPIKey, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "APIKeyCreated",
		Message: fmt.Sprintf("API key generated and saved to Secret %s", siteAPIKey.Spec.SecretName),
	})
	_ = r.updateStatus(ctx, siteAPIKey)

	return ctrl.Result{}, nil
}

func (r *SiteAPIKeyReconciler) failReconciliation(ctx context.Context, siteAPIKey *vyogotechv1.SiteAPIKey, msg, reason string) (ctrl.Result, error) {
	siteAPIKey.Status.Phase = "Failed"
	r.setCondition(siteAPIKey, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteAPIKey, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteAPIKey)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteAPIKeyReconciler) setCondition(siteAPIKey *vyogotechv1.SiteAPIKey, condition metav1.Condition) {
	condition.ObservedGeneration = siteAPIKey.Generation
	meta.SetStatusCondition(&siteAPIKey.Status.Conditions, condition)
}

func (r *SiteAPIKeyReconciler) updateStatus(ctx context.Context, siteAPIKey *vyogotechv1.SiteAPIKey) error {
	latest := &vyogotechv1.SiteAPIKey{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteAPIKey.Name, Namespace: siteAPIKey.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteAPIKey.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteAPIKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("siteapikey-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteAPIKey{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
