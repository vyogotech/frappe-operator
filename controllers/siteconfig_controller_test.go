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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSiteConfigReconciler_Reconcile_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	mMode := true
	maxSize := int64(50000000)

	siteConfig := &vyogotechv1.SiteConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "config1", Namespace: "default", Generation: 1},
		Spec: vyogotechv1.SiteConfigSpec{
			SiteRef:         &vyogotechv1.NamespacedName{Name: "site1"},
			MaintenanceMode: &mMode,
			MaxFileSize:     &maxSize,
			CustomConfig:    map[string]string{"allow_consecutive_logins": "1"},
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "site1.local", BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"}},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "15"},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteConfig, site, bench).WithStatusSubresource(siteConfig, &batchv1.Job{}).Build()
	r := &SiteConfigReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
	ctx := context.Background()
	key := types.NamespacedName{Name: "config1", Namespace: "default"}

	// First reconcile creates the apply Job and reports Applying (async, Job-based).
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	updated := &vyogotechv1.SiteConfig{}
	if err := client.Get(ctx, key, updated); err != nil {
		t.Fatalf("fetch config: %v", err)
	}
	if updated.Status.Phase != "Applying" {
		t.Fatalf("expected phase Applying after first reconcile, got %s", updated.Status.Phase)
	}
	job := &batchv1.Job{}
	if err := client.Get(ctx, types.NamespacedName{Name: "config1-apply-1", Namespace: "default"}, job); err != nil {
		t.Fatalf("expected apply Job config1-apply-1: %v", err)
	}

	// Simulate the Job completing, then reconcile again -> Ready with the applied keys.
	job.Status.Succeeded = 1
	if err := client.Status().Update(ctx, job); err != nil {
		t.Fatalf("update job status: %v", err)
	}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("Reconcile (post-job) failed: %v", err)
	}
	if err := client.Get(ctx, key, updated); err != nil {
		t.Fatalf("fetch config: %v", err)
	}
	if updated.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updated.Status.Phase)
	}
	if len(updated.Status.AppliedKeys) != 3 {
		t.Errorf("expected 3 applied keys (maintenance_mode, max_file_size, allow_consecutive_logins), got %d (%v)", len(updated.Status.AppliedKeys), updated.Status.AppliedKeys)
	}
}

func TestSiteConfigReconciler_Reconcile_ValidationAndNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing SiteRef", func(t *testing.T) {
		siteConfig := &vyogotechv1.SiteConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "config1", Namespace: "default"},
			Spec:       vyogotechv1.SiteConfigSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteConfig).WithStatusSubresource(siteConfig).Build()
		r := &SiteConfigReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "config1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing SiteRef")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteConfig := &vyogotechv1.SiteConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "config1", Namespace: "default"},
			Spec:       vyogotechv1.SiteConfigSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteConfig, site).WithStatusSubresource(siteConfig).Build()
		r := &SiteConfigReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "config1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}
