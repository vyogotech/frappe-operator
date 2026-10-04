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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSiteUserReconciler_Reconcile_ValidationAndSiteWaiting(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing SiteRef", func(t *testing.T) {
		siteUser := &vyogotechv1.SiteUser{
			ObjectMeta: metav1.ObjectMeta{Name: "user1", Namespace: "default"},
			Spec:       vyogotechv1.SiteUserSpec{Email: "user1@example.com"},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteUser).WithStatusSubresource(siteUser).Build()
		r := &SiteUserReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "user1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing SiteRef")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteUser := &vyogotechv1.SiteUser{
			ObjectMeta: metav1.ObjectMeta{Name: "user1", Namespace: "default"},
			Spec: vyogotechv1.SiteUserSpec{
				SiteRef:   &vyogotechv1.NamespacedName{Name: "site1"},
				Email:     "user1@example.com",
				FirstName: "User",
			},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteUser, site).WithStatusSubresource(siteUser).Build()
		r := &SiteUserReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "user1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}

func TestSiteUserReconciler_Reconcile_SuccessWithAPIKeys(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/resource/User/bot%40example.com" || r.URL.Path == "/api/resource/User/bot@example.com" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data": {"name": "bot@example.com"}}`))
			return
		}
		if r.URL.Path == "/api/method/frappe.core.doctype.user.user.generate_keys" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": {"api_key": "mykey", "api_secret": "mysecret"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	siteUser := &vyogotechv1.SiteUser{
		ObjectMeta: metav1.ObjectMeta{Name: "bot-user", Namespace: "default"},
		Spec: vyogotechv1.SiteUserSpec{
			SiteRef:         &vyogotechv1.NamespacedName{Name: "site1"},
			Email:           "bot@example.com",
			FirstName:       "Bot",
			Roles:           []string{"System Manager"},
			APIKeySecretRef: &corev1.LocalObjectReference{Name: "bot-api-keys"},
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady, ResolvedDomain: ts.Listener.Addr().String()},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "site1-admin", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("adminpass")},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteUser, site, adminSecret).WithStatusSubresource(siteUser).Build()
	r := &SiteUserReconciler{
		Client:       client,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(10),
		FrappeClient: NewFrappeClient(ts.URL, "Administrator", "adminpass"),
	}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "bot-user", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify SiteUser status updated
	updatedUser := &vyogotechv1.SiteUser{}
	err = client.Get(ctx, types.NamespacedName{Name: "bot-user", Namespace: "default"}, updatedUser)
	if err != nil {
		t.Fatalf("failed to fetch updated user: %v", err)
	}

	if updatedUser.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updatedUser.Status.Phase)
	}
	if !updatedUser.Status.APIKeysGenerated {
		t.Error("expected apiKeysGenerated to be true")
	}

	// Verify K8s Secret created
	keySecret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Name: "bot-api-keys", Namespace: "default"}, keySecret)
	if err != nil {
		t.Fatalf("failed to fetch created API key secret: %v", err)
	}
	if string(keySecret.Data["api_key"]) != "mykey" || string(keySecret.Data["api_secret"]) != "mysecret" {
		t.Errorf("expected mykey/mysecret in secret, got %s/%s", keySecret.Data["api_key"], keySecret.Data["api_secret"])
	}
}

func TestSiteUserReconciler_PasswordSecretRefAndFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	var receivedPassword string
	var deletedUser string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/resource/User/pwduser@example.com" {
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": {"name": "pwduser@example.com"}}`))
				return
			}
			if r.Method == http.MethodPut {
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if p, ok := body["new_password"].(string); ok {
					receivedPassword = p
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": {"name": "pwduser@example.com"}}`))
				return
			}
			if r.Method == http.MethodDelete {
				deletedUser = "pwduser@example.com"
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"message": "ok"}`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	pwdSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pwduser-secret", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("SuperSecurePass123!")},
	}
	siteUser := &vyogotechv1.SiteUser{
		ObjectMeta: metav1.ObjectMeta{Name: "pwd-user", Namespace: "default"},
		Spec: vyogotechv1.SiteUserSpec{
			SiteRef:   &vyogotechv1.NamespacedName{Name: "site1"},
			Email:     "pwduser@example.com",
			FirstName: "PwdUser",
			PasswordSecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "pwduser-secret"},
				Key:                  "password",
			},
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady, ResolvedDomain: ts.Listener.Addr().String()},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "site1-admin", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("adminpass")},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteUser, site, adminSecret, pwdSecret).WithStatusSubresource(siteUser).Build()
	r := &SiteUserReconciler{
		Client:       client,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(10),
		FrappeClient: NewFrappeClient(ts.URL, "Administrator", "adminpass"),
	}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "pwd-user", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if receivedPassword != "SuperSecurePass123!" {
		t.Errorf("expected received password 'SuperSecurePass123!', got %q", receivedPassword)
	}

	// Verify finalizer added
	updated := &vyogotechv1.SiteUser{}
	_ = client.Get(ctx, types.NamespacedName{Name: "pwd-user", Namespace: "default"}, updated)
	if len(updated.Finalizers) == 0 || updated.Finalizers[0] != siteUserFinalizer {
		t.Fatalf("expected finalizer %q, got %v", siteUserFinalizer, updated.Finalizers)
	}

	// Now delete
	_ = client.Delete(ctx, updated)

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "pwd-user", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile delete failed: %v", err)
	}

	if deletedUser != "pwduser@example.com" {
		t.Errorf("expected deletedUser to be pwduser@example.com, got %q", deletedUser)
	}

	// Verify finalizer removed
	afterDelete := &vyogotechv1.SiteUser{}
	_ = client.Get(ctx, types.NamespacedName{Name: "pwd-user", Namespace: "default"}, afterDelete)
	if len(afterDelete.Finalizers) != 0 {
		t.Errorf("expected finalizer to be removed, got %v", afterDelete.Finalizers)
	}
}
