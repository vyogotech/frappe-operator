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

func TestSiteCronReconciler_Reconcile_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	siteCron := &vyogotechv1.SiteCron{
		ObjectMeta: metav1.ObjectMeta{Name: "cron1", Namespace: "default"},
		Spec: vyogotechv1.SiteCronSpec{
			SiteRef:  &vyogotechv1.NamespacedName{Name: "site1"},
			Schedule: "0 2 * * *",
			Method:   "saas_platform.api.sync_nightly_sales",
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"},
		},
		Status: vyogotechv1.FrappeSiteStatus{
			Phase:          vyogotechv1.FrappeSitePhaseReady,
			ResolvedDomain: "central.atxinvox.com.au",
		},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
		Spec: vyogotechv1.FrappeBenchSpec{
			ImageConfig: &vyogotechv1.ImageConfig{Repository: "ghcr.io/vyogotech/central-bench", Tag: "latest"},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteCron, site, bench).WithStatusSubresource(siteCron).Build()
	r := &SiteCronReconciler{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "cron1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	updatedCron := &vyogotechv1.SiteCron{}
	err = client.Get(ctx, types.NamespacedName{Name: "cron1", Namespace: "default"}, updatedCron)
	if err != nil {
		t.Fatalf("failed to fetch updated site cron: %v", err)
	}

	if updatedCron.Status.Phase != "Active" {
		t.Errorf("expected phase Active, got %s", updatedCron.Status.Phase)
	}

	// Verify K8s CronJob was created
	cronJob := &batchv1.CronJob{}
	err = client.Get(ctx, types.NamespacedName{Name: updatedCron.Status.CronJobName, Namespace: "default"}, cronJob)
	if err != nil {
		t.Fatalf("failed to fetch created CronJob: %v", err)
	}
	if cronJob.Spec.Schedule != "0 2 * * *" {
		t.Errorf("expected schedule 0 2 * * *, got %s", cronJob.Spec.Schedule)
	}
}

func TestSiteCronReconciler_Reconcile_ValidationAndNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing Method", func(t *testing.T) {
		siteCron := &vyogotechv1.SiteCron{
			ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "default"},
			Spec: vyogotechv1.SiteCronSpec{
				SiteRef:  &vyogotechv1.NamespacedName{Name: "site1"},
				Schedule: "0 2 * * *",
			},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteCron).WithStatusSubresource(siteCron).Build()
		r := &SiteCronReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "c1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing Method")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteCron := &vyogotechv1.SiteCron{
			ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "default"},
			Spec: vyogotechv1.SiteCronSpec{
				SiteRef:  &vyogotechv1.NamespacedName{Name: "site1"},
				Schedule: "0 2 * * *",
				Method:   "saas_platform.api.sync",
			},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteCron, site).WithStatusSubresource(siteCron).Build()
		r := &SiteCronReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "c1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}
