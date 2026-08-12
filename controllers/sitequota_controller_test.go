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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSiteQuotaReconciler_Reconcile_Normal(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	maxDB := int64(500)
	maxStorage := int64(1000)
	maxUsers := int32(20)

	siteQuota := &vyogotechv1.SiteQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "quota1", Namespace: "default"},
		Spec: vyogotechv1.SiteQuotaSpec{
			SiteRef:      &vyogotechv1.NamespacedName{Name: "site1"},
			MaxDBSizeMB:  &maxDB,
			MaxStorageMB: &maxStorage,
			MaxUsers:     &maxUsers,
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteQuota, site).WithStatusSubresource(siteQuota).Build()
	r := &SiteQuotaReconciler{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "quota1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	updatedQuota := &vyogotechv1.SiteQuota{}
	err = client.Get(ctx, types.NamespacedName{Name: "quota1", Namespace: "default"}, updatedQuota)
	if err != nil {
		t.Fatalf("failed to fetch updated site quota: %v", err)
	}

	if updatedQuota.Status.Phase != "Normal" {
		t.Errorf("expected phase Normal, got %s", updatedQuota.Status.Phase)
	}
	if updatedQuota.Status.QuotaExceeded {
		t.Error("expected QuotaExceeded to be false")
	}
}

func TestSiteQuotaReconciler_Reconcile_Exceeded(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	maxDB := int64(50) // 50 MB limit, but 120 MB observed

	siteQuota := &vyogotechv1.SiteQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "quota1", Namespace: "default"},
		Spec: vyogotechv1.SiteQuotaSpec{
			SiteRef:     &vyogotechv1.NamespacedName{Name: "site1"},
			MaxDBSizeMB: &maxDB,
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteQuota, site).WithStatusSubresource(siteQuota).Build()
	r := &SiteQuotaReconciler{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "quota1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RequeueAfter != 60*time.Second {
		t.Errorf("expected RequeueAfter 60s, got %v", res.RequeueAfter)
	}

	updatedQuota := &vyogotechv1.SiteQuota{}
	err = client.Get(ctx, types.NamespacedName{Name: "quota1", Namespace: "default"}, updatedQuota)
	if err != nil {
		t.Fatalf("failed to fetch updated site quota: %v", err)
	}

	if updatedQuota.Status.Phase != "QuotaExceeded" {
		t.Errorf("expected phase QuotaExceeded, got %s", updatedQuota.Status.Phase)
	}
	if !updatedQuota.Status.QuotaExceeded {
		t.Error("expected QuotaExceeded to be true")
	}
}
