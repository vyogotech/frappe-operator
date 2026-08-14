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

func TestSiteAppReconciler_Reconcile_SuccessAndUninstall(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	appInstalled := false
	appUninstalled := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/method/frappe.installer.install_app" {
			appInstalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "Installed"}`))
			return
		}
		if r.URL.Path == "/api/method/frappe.installer.uninstall_app" {
			appUninstalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "Uninstalled"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
		Spec: vyogotechv1.SiteAppSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
			AppName: "erpnext_australian_localisation",
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "site1-admin", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("adminpass")},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteApp, site, adminSecret).WithStatusSubresource(siteApp).Build()
	r := &SiteAppReconciler{
		Client:       client,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(10),
		FrappeClient: NewFrappeClient(ts.URL, "Administrator", "adminpass"),
	}

	ctx := context.Background()

	// 1. First Reconcile adds finalizer and installs app
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if !appInstalled {
		t.Error("expected app to be installed via REST API")
	}

	updatedApp := &vyogotechv1.SiteApp{}
	err = client.Get(ctx, types.NamespacedName{Name: "app1", Namespace: "default"}, updatedApp)
	if err != nil {
		t.Fatalf("failed to fetch updated site app: %v", err)
	}

	if updatedApp.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updatedApp.Status.Phase)
	}

	// 2. Test Deletion Finalizer
	err = client.Delete(ctx, updatedApp)
	if err != nil {
		t.Fatalf("failed to delete site app: %v", err)
	}

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile deletion failed: %v", err)
	}

	if !appUninstalled {
		t.Error("expected app to be uninstalled via REST API upon deletion")
	}
}

func TestSiteAppReconciler_Reconcile_ValidationAndNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing AppName", func(t *testing.T) {
		siteApp := &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
			Spec:       vyogotechv1.SiteAppSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteApp).WithStatusSubresource(siteApp).Build()
		r := &SiteAppReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing AppName")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteApp := &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
			Spec: vyogotechv1.SiteAppSpec{
				SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
				AppName: "payments",
			},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		adminSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "site1-admin", Namespace: "default"},
			Data:       map[string][]byte{"password": []byte("adminpass")},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteApp, site, adminSecret).WithStatusSubresource(siteApp).Build()
		r := &SiteAppReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}
