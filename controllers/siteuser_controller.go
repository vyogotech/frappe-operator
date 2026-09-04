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

// SiteUserReconciler reconciles a SiteUser object
type SiteUserReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	FrappeClient *FrappeClient // Optional injected client for testing
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=siteusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteusers/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *SiteUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	siteUser := &vyogotechv1.SiteUser{}
	if err := r.Get(ctx, req.NamespacedName, siteUser); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteUser.Spec.SiteRef == nil || siteUser.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteUser, "siteRef.name is required", "ValidationFailed")
	}
	if siteUser.Spec.Email == "" {
		return r.failReconciliation(ctx, siteUser, "email is required", "ValidationFailed")
	}

	siteNamespace := siteUser.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteUser.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteUser.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteUser.Status.Phase = "Pending"
		r.setCondition(siteUser, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteUser)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteUser.Status.Phase = "Pending"
		r.setCondition(siteUser, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteUser)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	r.setCondition(siteUser, metav1.Condition{
		Type:    "SiteReady",
		Status:  metav1.ConditionTrue,
		Reason:  "SiteReady",
		Message: "Referenced FrappeSite is ready",
	})

	// Fetch admin password for REST authentication
	adminPassword, err := r.getAdminPassword(ctx, site)
	if err != nil {
		return r.failReconciliation(ctx, siteUser, fmt.Sprintf("Failed to fetch admin password: %v", err), "AdminPasswordFailed")
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

	// Ensure User exists and roles are assigned
	err = frappeClient.EnsureUser(ctx, siteUser.Spec.Email, siteUser.Spec.FirstName, siteUser.Spec.LastName, siteUser.Spec.UserType, siteUser.Spec.Roles, siteUser.Spec.SendPasswordReset)
	if err != nil {
		return r.failReconciliation(ctx, siteUser, fmt.Sprintf("Failed to sync user with Frappe: %v", err), "UserSyncFailed")
	}

	// Handle API Key Secret generation if requested
	secretName := ""
	apiKeysGenerated := false
	if siteUser.Spec.APIKeySecretRef != nil && siteUser.Spec.APIKeySecretRef.Name != "" {
		secretName = siteUser.Spec.APIKeySecretRef.Name
		apiKey, apiSecret, err := frappeClient.GenerateAPIKeys(ctx, siteUser.Spec.Email)
		if err != nil {
			logger.Error(err, "Failed to generate API keys in Frappe", "user", siteUser.Spec.Email)
		} else {
			// Write to K8s Secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: siteUser.Namespace,
					Labels: map[string]string{
						"app":  "frappe",
						"user": siteUser.Name,
					},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"api_key":    []byte(apiKey),
					"api_secret": []byte(apiSecret),
					"email":      []byte(siteUser.Spec.Email),
				},
			}
			_ = controllerutil.SetControllerReference(siteUser, secret, r.Scheme)

			var existing corev1.Secret
			err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: siteUser.Namespace}, &existing)
			if err != nil && errors.IsNotFound(err) {
				if err := r.Create(ctx, secret); err == nil {
					apiKeysGenerated = true
				}
			} else if err == nil {
				existing.Data = secret.Data
				if err := r.Update(ctx, &existing); err == nil {
					apiKeysGenerated = true
				}
			}
		}
	}

	siteUser.Status.Phase = "Ready"
	siteUser.Status.AssignedRoles = siteUser.Spec.Roles
	siteUser.Status.APIKeysGenerated = apiKeysGenerated
	siteUser.Status.SecretName = secretName
	siteUser.Status.ObservedGeneration = siteUser.Generation
	r.setCondition(siteUser, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "UserSynced",
		Message: "User and roles successfully synced with Frappe",
	})
	_ = r.updateStatus(ctx, siteUser)

	return ctrl.Result{}, nil
}

func (r *SiteUserReconciler) getAdminPassword(ctx context.Context, site *vyogotechv1.FrappeSite) (string, error) {
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

func (r *SiteUserReconciler) failReconciliation(ctx context.Context, siteUser *vyogotechv1.SiteUser, msg, reason string) (ctrl.Result, error) {
	siteUser.Status.Phase = "Failed"
	r.setCondition(siteUser, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteUser, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteUser)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteUserReconciler) setCondition(siteUser *vyogotechv1.SiteUser, condition metav1.Condition) {
	condition.ObservedGeneration = siteUser.Generation
	meta.SetStatusCondition(&siteUser.Status.Conditions, condition)
}

func (r *SiteUserReconciler) updateStatus(ctx context.Context, siteUser *vyogotechv1.SiteUser) error {
	latest := &vyogotechv1.SiteUser{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteUser.Name, Namespace: siteUser.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteUser.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("siteuser-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteUser{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
