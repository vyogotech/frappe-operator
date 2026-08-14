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

func TestSiteRoleReconciler_Reconcile_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	roleCreated := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/resource/Role/Auditor" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/api/resource/Role" && r.Method == http.MethodPost {
			roleCreated = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data": {"name": "Auditor"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	siteRole := &vyogotechv1.SiteRole{
		ObjectMeta: metav1.ObjectMeta{Name: "auditor-role", Namespace: "default"},
		Spec: vyogotechv1.SiteRoleSpec{
			SiteRef:    &vyogotechv1.NamespacedName{Name: "site1"},
			RoleName:   "Auditor",
			DeskAccess: true,
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

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteRole, site, adminSecret).WithStatusSubresource(siteRole).Build()
	r := &SiteRoleReconciler{
		Client:       client,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(10),
		FrappeClient: NewFrappeClient(ts.URL, "Administrator", "adminpass"),
	}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "auditor-role", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if !roleCreated {
		t.Error("expected role to be created in Frappe")
	}

	updatedRole := &vyogotechv1.SiteRole{}
	err = client.Get(ctx, types.NamespacedName{Name: "auditor-role", Namespace: "default"}, updatedRole)
	if err != nil {
		t.Fatalf("failed to get updated site role: %v", err)
	}

	if updatedRole.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updatedRole.Status.Phase)
	}
}

func TestSiteRoleReconciler_Reconcile_ValidationAndNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing RoleName", func(t *testing.T) {
		siteRole := &vyogotechv1.SiteRole{
			ObjectMeta: metav1.ObjectMeta{Name: "role1", Namespace: "default"},
			Spec:       vyogotechv1.SiteRoleSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteRole).WithStatusSubresource(siteRole).Build()
		r := &SiteRoleReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "role1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing RoleName")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteRole := &vyogotechv1.SiteRole{
			ObjectMeta: metav1.ObjectMeta{Name: "role1", Namespace: "default"},
			Spec: vyogotechv1.SiteRoleSpec{
				SiteRef:  &vyogotechv1.NamespacedName{Name: "site1"},
				RoleName: "Auditor",
			},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteRole, site).WithStatusSubresource(siteRole).Build()
		r := &SiteRoleReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "role1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}
