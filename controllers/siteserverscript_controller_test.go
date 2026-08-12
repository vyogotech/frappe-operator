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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func TestSiteServerScriptReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = vyogotechv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "ok"}`))
	}))
	defer mockServer.Close()

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-site",
			Namespace: "default",
		},
		Status: vyogotechv1.FrappeSiteStatus{
			Phase: vyogotechv1.FrappeSitePhaseReady,
		},
	}

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-site-admin-password",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("adminpass"),
		},
	}

	serverScript := &vyogotechv1.SiteServerScript{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server-script",
			Namespace: "default",
		},
		Spec: vyogotechv1.SiteServerScriptSpec{
			SiteRef: &vyogotechv1.NamespacedName{
				Name: "test-site",
			},
			ScriptType:       "DocType Event",
			ReferenceDocType: "Sales Order",
			EventType:        "Before Submit",
			Script:           "frappe.msgprint('Validating order')",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(site, adminSecret, serverScript).
		WithStatusSubresource(serverScript).
		Build()

	frappeClient := NewFrappeClient(mockServer.URL, "Administrator", "adminpass")
	frappeClient.SID = "mock-sid"

	r := &SiteServerScriptReconciler{
		Client:       client,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(10),
		FrappeClient: frappeClient,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-server-script",
			Namespace: "default",
		},
	}

	res, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue {
		t.Errorf("expected no requeue, got %v", res)
	}

	updated := &vyogotechv1.SiteServerScript{}
	if err := client.Get(context.Background(), req.NamespacedName, updated); err != nil {
		t.Fatalf("failed to fetch updated ServerScript: %v", err)
	}

	if updated.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updated.Status.Phase)
	}
}
