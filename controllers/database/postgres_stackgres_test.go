/*
Copyright 2024 Vyogo Technologies.

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

package database

import (
	"context"
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestPostgresProvider_StackGresCustomResources(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	site.Spec.DBConfig.PostgresEngine = "stackgres"
	site.Spec.DBConfig.Resources = &vyogotechv1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1500m"),
			corev1.ResourceMemory: resource.MustParse("3Gi"),
		},
	}

	_, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase with custom resources: %v", err)
	}

	// Bespoke profile should be created
	profile := &unstructured.Unstructured{}
	profile.SetGroupVersionKind(SGInstanceProfileGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres-profile", Namespace: "default"}, profile); err != nil {
		t.Fatalf("expected custom SGInstanceProfile: %v", err)
	}

	cpu, _, _ := unstructured.NestedString(profile.Object, "spec", "cpu")
	mem, _, _ := unstructured.NestedString(profile.Object, "spec", "memory")
	if cpu != "1500m" || mem != "3Gi" {
		t.Errorf("custom profile sizing mismatch: cpu=%v, mem=%v", cpu, mem)
	}

	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, cluster); err != nil {
		t.Fatalf("expected SGCluster: %v", err)
	}

	profileRef, _, _ := unstructured.NestedString(cluster.Object, "spec", "sgInstanceProfile")
	if profileRef != "pgsite-postgres-profile" {
		t.Errorf("cluster should reference custom profile, got %v", profileRef)
	}
}

func TestPostgresProvider_StackGresLifecycle(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	site.Spec.DBConfig.PostgresEngine = "stackgres"
	site.Spec.DeletionPolicy = "Delete"

	// 1. EnsureDatabase
	info, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if info.Host != "pgsite-postgres.default.svc.cluster.local" || info.Port != "5432" {
		t.Fatalf("unexpected DatabaseInfo: %+v", info)
	}

	// 2. IsReady before cluster is ready -> false
	ready, err := p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady: %v", err)
	}
	if ready {
		t.Error("expected IsReady=false before SGCluster reports running")
	}

	// Simulate SGCluster becoming ready
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, cluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	cluster.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "True",
			},
		},
	}
	if err := p.client.Update(ctx, cluster); err != nil {
		t.Fatalf("update cluster status: %v", err)
	}

	// Create superuser secret named after cluster (simulating StackGres operator)
	superSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pgsite-postgres",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"superuser-password": []byte("superpass123"),
			"superuser-username":     []byte("postgres"),
		},
	}
	if err := p.client.Create(ctx, superSecret); err != nil {
		t.Fatalf("create superuser secret: %v", err)
	}

	// IsReady should now launch the provision job
	ready, err = p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady during provision: %v", err)
	}
	if ready {
		t.Error("expected IsReady=false while provision job running")
	}

	// Mark provision job succeeded
	provJob := &batchv1.Job{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-db-provision", Namespace: "default"}, provJob); err != nil {
		t.Fatalf("expected provision job: %v", err)
	}
	provJob.Status.Succeeded = 1
	if err := p.client.Status().Update(ctx, provJob); err != nil {
		t.Fatalf("update provision job status: %v", err)
	}

	// IsReady should now launch the configure job
	ready, err = p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady during configure: %v", err)
	}
	if ready {
		t.Error("expected IsReady=false while configure job running")
	}

	// Mark configure job succeeded
	confJob := &batchv1.Job{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-db-configure", Namespace: "default"}, confJob); err != nil {
		t.Fatalf("expected configure job: %v", err)
	}
	confJob.Status.Succeeded = 1
	if err := p.client.Status().Update(ctx, confJob); err != nil {
		t.Fatalf("update configure job status: %v", err)
	}

	// Now IsReady should be true!
	ready, err = p.IsReady(ctx, site)
	if err != nil || !ready {
		t.Errorf("expected IsReady=true after configure job succeeded, got %v err=%v", ready, err)
	}

	// 3. GetCredentials
	creds, err := p.GetCredentials(ctx, site)
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}
	wantUser := p.generatePGUserName(site)
	if creds.SecretName != "pgsite-db-password" || creds.Password == "" || creds.Username != wantUser {
		t.Errorf("unexpected credentials: %+v, want user %q", creds, wantUser)
	}

	// 4. Cleanup (Delete policy)
	if err := p.Cleanup(ctx, site); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Assert cluster was deleted
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, cluster); err == nil {
		t.Error("expected SGCluster to be deleted on Cleanup")
	}
}
