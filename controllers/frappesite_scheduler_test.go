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
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFrappeSite_EnableScheduler_InitSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	tests := []struct {
		name     string
		enabled  *bool
		expected string
	}{
		{name: "nil_enableScheduler", enabled: nil, expected: ""},
		{name: "true_enableScheduler", enabled: func() *bool { b := true; return &b }(), expected: "true"},
		{name: "false_enableScheduler", enabled: func() *bool { b := false; return &b }(), expected: "false"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			site := &vyogotechv1.FrappeSite{
				ObjectMeta: metav1.ObjectMeta{Name: "site-" + tc.name, Namespace: "default"},
				Spec: vyogotechv1.FrappeSiteSpec{
					SiteName:        "site-" + tc.name + ".local",
					EnableScheduler: tc.enabled,
				},
			}
			bench := &vyogotechv1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
			}
			dbInfo := &database.DatabaseInfo{Host: "db", Port: "3306", Name: "db"}
			dbCreds := &database.DatabaseCredentials{Username: "u", Password: "p"}

			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).Build()
			r := &FrappeSiteReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

			err := r.ensureInitSecrets(context.TODO(), site, bench, "domain.local", dbInfo, dbCreds, "pass", "cache", "queue")
			if err != nil {
				t.Fatalf("ensureInitSecrets failed: %v", err)
			}

			secret := &corev1.Secret{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s-init-secrets", site.Name), Namespace: "default"}, secret)
			if err != nil {
				t.Fatalf("get secret failed: %v", err)
			}

			val := string(secret.Data["enable_scheduler"])
			if val != tc.expected {
				t.Errorf("expected enable_scheduler %q, got %q", tc.expected, val)
			}
		})
	}
}

func TestFrappeSite_EnsureSchedulerState(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))

	trueVal := true
	falseVal := false

	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "15"},
	}

	t.Run("no-op when EnableScheduler is nil", func(t *testing.T) {
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
			Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "s1.local"},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).Build()
		r := &FrappeSiteReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
		if err := r.ensureSchedulerState(context.TODO(), site, bench); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("no-op when desired matches status", func(t *testing.T) {
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "s2", Namespace: "default"},
			Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "s2.local", EnableScheduler: &trueVal},
			Status:     vyogotechv1.FrappeSiteStatus{SchedulerEnabled: &trueVal},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).Build()
		r := &FrappeSiteReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
		if err := r.ensureSchedulerState(context.TODO(), site, bench); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("creates sync job and completes enabling scheduler", func(t *testing.T) {
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "s3", Namespace: "default"},
			Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "s3.local", EnableScheduler: &trueVal},
		}
		recorder := record.NewFakeRecorder(10)
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).WithStatusSubresource(site, &batchv1.Job{}).Build()
		r := &FrappeSiteReconciler{Client: client, Scheme: scheme, Recorder: recorder}
		ctx := context.TODO()

		// First call creates the job and returns in-progress error
		err := r.ensureSchedulerState(ctx, site, bench)
		if err == nil || !strings.Contains(err.Error(), "waiting for completion") {
			t.Fatalf("expected waiting for completion error, got: %v", err)
		}

		job := &batchv1.Job{}
		jobKey := types.NamespacedName{Name: "s3-sched-sync", Namespace: "default"}
		if err := client.Get(ctx, jobKey, job); err != nil {
			t.Fatalf("expected sync job to exist: %v", err)
		}

		// Verify job command runs enable-scheduler
		cmd := strings.Join(job.Spec.Template.Spec.Containers[0].Command, " ")
		if !strings.Contains(cmd, "bench --site 's3.local' enable-scheduler") {
			t.Errorf("expected command to contain enable-scheduler, got: %s", cmd)
		}

		// Mark job succeeded
		job.Status.Succeeded = 1
		if err := client.Status().Update(ctx, job); err != nil {
			t.Fatalf("failed to update job status: %v", err)
		}

		// Second call notices success, updates status, deletes job, returns nil
		if err := r.ensureSchedulerState(ctx, site, bench); err != nil {
			t.Fatalf("expected nil error on completion, got: %v", err)
		}

		if site.Status.SchedulerEnabled == nil || !*site.Status.SchedulerEnabled {
			t.Errorf("expected SchedulerEnabled to be true, got %v", site.Status.SchedulerEnabled)
		}

		// Verify event was recorded
		select {
		case ev := <-recorder.Events:
			if !strings.Contains(ev, "SchedulerConfigured") {
				t.Errorf("expected SchedulerConfigured event, got: %s", ev)
			}
		default:
			t.Error("expected SchedulerConfigured event to be recorded")
		}
	})

	t.Run("creates sync job and completes disabling scheduler", func(t *testing.T) {
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "s4", Namespace: "default"},
			Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "s4.local", EnableScheduler: &falseVal},
			Status:     vyogotechv1.FrappeSiteStatus{SchedulerEnabled: &trueVal},
		}
		recorder := record.NewFakeRecorder(10)
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).WithStatusSubresource(site, &batchv1.Job{}).Build()
		r := &FrappeSiteReconciler{Client: client, Scheme: scheme, Recorder: recorder}
		ctx := context.TODO()

		err := r.ensureSchedulerState(ctx, site, bench)
		if err == nil {
			t.Fatalf("expected waiting for completion error, got nil")
		}

		job := &batchv1.Job{}
		jobKey := types.NamespacedName{Name: "s4-sched-sync", Namespace: "default"}
		if err := client.Get(ctx, jobKey, job); err != nil {
			t.Fatalf("expected sync job to exist: %v", err)
		}

		cmd := strings.Join(job.Spec.Template.Spec.Containers[0].Command, " ")
		if !strings.Contains(cmd, "bench --site 's4.local' disable-scheduler") {
			t.Errorf("expected command to contain disable-scheduler, got: %s", cmd)
		}

		job.Status.Succeeded = 1
		if err := client.Status().Update(ctx, job); err != nil {
			t.Fatalf("failed to update job status: %v", err)
		}

		if err := r.ensureSchedulerState(ctx, site, bench); err != nil {
			t.Fatalf("expected nil error on completion, got: %v", err)
		}

		if site.Status.SchedulerEnabled == nil || *site.Status.SchedulerEnabled {
			t.Errorf("expected SchedulerEnabled to be false, got %v", site.Status.SchedulerEnabled)
		}
	})
}
