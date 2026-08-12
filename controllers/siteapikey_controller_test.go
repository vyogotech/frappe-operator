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

func TestSiteAPIKeyReconciler_Reconcile_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	siteKey := &vyogotechv1.SiteAPIKey{
		ObjectMeta: metav1.ObjectMeta{Name: "key1", Namespace: "default"},
		Spec: vyogotechv1.SiteAPIKeySpec{
			SiteRef:    &vyogotechv1.NamespacedName{Name: "site1"},
			User:       "Administrator",
			SecretName: "admin-api-secret",
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Status: vyogotechv1.FrappeSiteStatus{
			Phase: vyogotechv1.FrappeSitePhaseReady,
			SiteURL: "http://central.atxinvox.com.au",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteKey, site).WithStatusSubresource(siteKey).Build()
	r := &SiteAPIKeyReconciler{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "key1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	updatedKey := &vyogotechv1.SiteAPIKey{}
	err = client.Get(ctx, types.NamespacedName{Name: "key1", Namespace: "default"}, updatedKey)
	if err != nil {
		t.Fatalf("failed to fetch updated site api key: %v", err)
	}

	if updatedKey.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updatedKey.Status.Phase)
	}

	// Verify Secret was created
	secret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Name: "admin-api-secret", Namespace: "default"}, secret)
	if err != nil {
		t.Fatalf("failed to fetch created Secret: %v", err)
	}
	if string(secret.Data["user"]) != "Administrator" {
		t.Errorf("expected secret user Administrator, got %s", string(secret.Data["user"]))
	}
}

func TestSiteAPIKeyReconciler_Reconcile_ValidationAndNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing SecretName", func(t *testing.T) {
		siteKey := &vyogotechv1.SiteAPIKey{
			ObjectMeta: metav1.ObjectMeta{Name: "k1", Namespace: "default"},
			Spec: vyogotechv1.SiteAPIKeySpec{
				SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
				User:    "Administrator",
			},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteKey).WithStatusSubresource(siteKey).Build()
		r := &SiteAPIKeyReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "k1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing SecretName")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteKey := &vyogotechv1.SiteAPIKey{
			ObjectMeta: metav1.ObjectMeta{Name: "k1", Namespace: "default"},
			Spec: vyogotechv1.SiteAPIKeySpec{
				SiteRef:    &vyogotechv1.NamespacedName{Name: "site1"},
				User:       "Administrator",
				SecretName: "sec1",
			},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteKey, site).WithStatusSubresource(siteKey).Build()
		r := &SiteAPIKeyReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "k1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}
