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
	"strings"
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

	// Assert SGScript created
	sgScript := &unstructured.Unstructured{}
	sgScript.SetGroupVersionKind(SGScriptGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres-script", Namespace: "default"}, sgScript); err != nil {
		t.Fatalf("expected SGScript: %v", err)
	}

	// 2. IsReady before cluster is ready -> false
	ready, err := p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady: %v", err)
	}
	if ready {
		t.Error("expected IsReady=false before SGCluster reports running")
	}

	// Simulate SGCluster becoming bootstrapped
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, cluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	cluster.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Bootstrapped",
				"status": "True",
			},
		},
	}
	if err := p.client.Update(ctx, cluster); err != nil {
		t.Fatalf("update cluster status: %v", err)
	}

	// IsReady should now check if the SGScript is ready. It will be false since managedSql is missing
	ready, err = p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady during script wait: %v", err)
	}
	if ready {
		t.Error("expected IsReady=false while script running")
	}

	// Mark SGScript completed in SGCluster status
	cluster.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Bootstrapped",
				"status": "True",
			},
		},
		"managedSql": map[string]interface{}{
			"scripts": []interface{}{
				map[string]interface{}{
					"id":          int64(0),
					"completedAt": "2024-01-01T00:00:00Z",
				},
			},
		},
	}
	if err := p.client.Update(ctx, cluster); err != nil {
		t.Fatalf("update cluster status for managedSql: %v", err)
	}

	// Now IsReady should be true!
	ready, err = p.IsReady(ctx, site)
	if err != nil || !ready {
		t.Errorf("expected IsReady=true after SGScript completed, got %v err=%v", ready, err)
	}

	// 3. GetCredentials
	creds, err := p.GetCredentials(ctx, site)
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}
	wantUser := generateDBUser(site)
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

	// Assert script was deleted
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres-script", Namespace: "default"}, sgScript); err == nil {
		t.Error("expected SGScript to be deleted on Cleanup")
	}
}

func TestPostgresProvider_StackGresScriptFailure(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	site.Spec.DBConfig.PostgresEngine = "stackgres"

	_, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}

	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, cluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}

	// Cluster is bootstrapped, but script execution failed with error
	cluster.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Bootstrapped",
				"status": "True",
			},
		},
		"managedSql": map[string]interface{}{
			"scripts": []interface{}{
				map[string]interface{}{
					"id":       int64(0),
					"failedAt": "2024-01-01T00:01:00Z",
					"scripts": []interface{}{
						map[string]interface{}{
							"failure": "syntax error at or near \\c",
						},
					},
				},
			},
		},
	}
	if err := p.client.Update(ctx, cluster); err != nil {
		t.Fatalf("update cluster: %v", err)
	}

	ready, err := p.IsReady(ctx, site)
	if ready {
		t.Errorf("expected IsReady=false on script failure")
	}
	if err == nil {
		t.Fatalf("expected error from IsReady on script failure, got nil")
	}
	if want := "syntax error"; !strings.Contains(err.Error(), want) {
		t.Errorf("expected error containing %q, got: %v", want, err)
	}
}
