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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func setupTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(vyogotechv1.AddToScheme(s))
	return s
}

func newTestPool(namespace, name string, minShards, maxShards, maxSites, watermark int32) *vyogotechv1.FrappeBenchPool {
	return &vyogotechv1.FrappeBenchPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vyogotechv1.FrappeBenchPoolSpec{
			FrappeVersion: "16",
			ShardingPolicy: vyogotechv1.ShardingPolicy{
				MinShards:        minShards,
				MaxShards:        maxShards,
				MaxSitesPerShard: maxSites,
				WatermarkPercent: watermark,
				BufferWarmShards: 1,
				ShardPrefix:      "standard-v16",
			},
			Template: vyogotechv1.FrappeBenchSpec{
				FrappeVersion: "16",
				StorageSize:   "50Gi",
				Apps: []vyogotechv1.AppSource{
					{Name: "erpnext", Source: "fpm"},
					{Name: "hrms", Source: "fpm"},
				},
			},
		},
	}
}

func TestFrappeBenchPool_InitialScaleUp(t *testing.T) {
	scheme := setupTestScheme()
	namespace := "bench-v16"
	pool := newTestPool(namespace, "v16-pool", 2, 5, 25, 80)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pool).
		WithStatusSubresource(&vyogotechv1.FrappeBenchPool{}, &vyogotechv1.FrappeBench{}).
		Build()

	r := &FrappeBenchPoolReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	res, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Errorf("Expected RequeueAfter to be set")
	}

	// Verify first shard was created
	var benches vyogotechv1.FrappeBenchList
	if err := c.List(context.TODO(), &benches, client.InNamespace(namespace)); err != nil {
		t.Fatalf("Failed to list benches: %v", err)
	}
	if len(benches.Items) != 1 {
		t.Fatalf("Expected 1 bench created, found %d", len(benches.Items))
	}

	firstBench := benches.Items[0]
	if firstBench.Name != "standard-v16-001" {
		t.Errorf("Expected bench name standard-v16-001, got %s", firstBench.Name)
	}
	if firstBench.Labels["vyogo.tech/bench-tier"] != "standard" {
		t.Errorf("Expected bench-tier label standard, got %s", firstBench.Labels["vyogo.tech/bench-tier"])
	}
}

func TestFrappeBenchPool_ScaleOnWatermark(t *testing.T) {
	scheme := setupTestScheme()
	namespace := "bench-v16"
	pool := newTestPool(namespace, "v16-pool", 1, 5, 25, 80) // 80% of 25 = 20 sites

	bench1 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standard-v16-001",
			Namespace: namespace,
			Labels: map[string]string{
				"vyogo.tech/pool":       pool.Name,
				"vyogo.tech/bench-tier": "standard",
			},
		},
		Spec: vyogotechv1.FrappeBenchSpec{
			FrappeVersion: "16",
		},
		Status: vyogotechv1.FrappeBenchStatus{
			Phase: "Ready",
		},
	}

	// Add 20 sites referencing standard-v16-001
	objs := []client.Object{pool, bench1}
	for i := 1; i <= 20; i++ {
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("site-%02d", i),
				Namespace: namespace,
			},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: fmt.Sprintf("site-%02d.test.com", i),
				BenchRef: &vyogotechv1.NamespacedName{
					Name:      "standard-v16-001",
					Namespace: namespace,
				},
			},
		}
		objs = append(objs, site)
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&vyogotechv1.FrappeBenchPool{}, &vyogotechv1.FrappeBench{}).
		Build()

	r := &FrappeBenchPoolReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify standard-v16-002 was provisioned
	var benches vyogotechv1.FrappeBenchList
	if err := c.List(context.TODO(), &benches, client.InNamespace(namespace)); err != nil {
		t.Fatalf("Failed to list benches: %v", err)
	}
	if len(benches.Items) != 2 {
		t.Fatalf("Expected 2 benches, found %d", len(benches.Items))
	}

	var found002 bool
	for _, b := range benches.Items {
		if b.Name == "standard-v16-002" {
			found002 = true
			break
		}
	}
	if !found002 {
		t.Errorf("Expected standard-v16-002 to be created")
	}
}

func TestFrappeBenchPool_RespectMaxShards(t *testing.T) {
	scheme := setupTestScheme()
	namespace := "bench-v16"
	pool := newTestPool(namespace, "v16-pool", 1, 2, 25, 80) // MaxShards: 2

	bench1 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-v16-001", Namespace: namespace},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
	}
	bench2 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-v16-002", Namespace: namespace},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
	}

	// 25 sites on bench1, 25 sites on bench2 (both full)
	objs := []client.Object{pool, bench1, bench2}
	for i := 1; i <= 25; i++ {
		objs = append(objs, &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("s1-%d", i), Namespace: namespace},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: fmt.Sprintf("s1-%d.test.com", i),
				BenchRef: &vyogotechv1.NamespacedName{Name: "standard-v16-001", Namespace: namespace},
			},
		})
		objs = append(objs, &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("s2-%d", i), Namespace: namespace},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: fmt.Sprintf("s2-%d.test.com", i),
				BenchRef: &vyogotechv1.NamespacedName{Name: "standard-v16-002", Namespace: namespace},
			},
		})
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&vyogotechv1.FrappeBenchPool{}, &vyogotechv1.FrappeBench{}).
		Build()

	r := &FrappeBenchPoolReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Shard count must not exceed 2
	var benches vyogotechv1.FrappeBenchList
	if err := c.List(context.TODO(), &benches, client.InNamespace(namespace)); err != nil {
		t.Fatalf("Failed to list benches: %v", err)
	}
	if len(benches.Items) != 2 {
		t.Fatalf("Expected strictly 2 benches (capped by MaxShards), found %d", len(benches.Items))
	}
}

func TestFrappeBenchPool_InFlightGuard(t *testing.T) {
	scheme := setupTestScheme()
	namespace := "bench-v16"
	pool := newTestPool(namespace, "v16-pool", 2, 5, 25, 80)

	// bench1 is full (25 sites), bench2 is still Initializing
	bench1 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-v16-001", Namespace: namespace},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
	}
	bench2 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-v16-002", Namespace: namespace},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Initializing"},
	}

	objs := []client.Object{pool, bench1, bench2}
	for i := 1; i <= 25; i++ {
		objs = append(objs, &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("s1-%d", i), Namespace: namespace},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: fmt.Sprintf("s1-%d.test.com", i),
				BenchRef: &vyogotechv1.NamespacedName{Name: "standard-v16-001", Namespace: namespace},
			},
		})
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&vyogotechv1.FrappeBenchPool{}, &vyogotechv1.FrappeBench{}).
		Build()

	r := &FrappeBenchPoolReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	res, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Reconciler should wait (short requeue) without creating bench 003 yet
	if res.RequeueAfter != 15*time.Second {
		t.Errorf("Expected 15s requeue for in-flight shard, got %v", res.RequeueAfter)
	}

	var benches vyogotechv1.FrappeBenchList
	if err := c.List(context.TODO(), &benches, client.InNamespace(namespace)); err != nil {
		t.Fatalf("Failed to list benches: %v", err)
	}
	if len(benches.Items) != 2 {
		t.Fatalf("Expected strictly 2 benches while shard is in-flight, found %d", len(benches.Items))
	}
}

func TestFrappeBenchPool_ComputeNextShardName(t *testing.T) {
	r := &FrappeBenchPoolReconciler{}

	cases := []struct {
		existing []string
		prefix   string
		expected string
	}{
		{
			existing: []string{},
			prefix:   "standard-v16",
			expected: "standard-v16-001",
		},
		{
			existing: []string{"standard-v16-001"},
			prefix:   "standard-v16",
			expected: "standard-v16-002",
		},
		{
			existing: []string{"standard-v16-001", "standard-v16-002", "standard-v16-003"},
			prefix:   "standard-v16",
			expected: "standard-v16-004",
		},
		{
			existing: []string{"standard-v16-001", "standard-v16-009"},
			prefix:   "standard-v16",
			expected: "standard-v16-010",
		},
		{
			existing: []string{"standard-v17-001"},
			prefix:   "standard-v17",
			expected: "standard-v17-002",
		},
	}

	for _, tc := range cases {
		benches := make([]vyogotechv1.FrappeBench, len(tc.existing))
		for i, name := range tc.existing {
			benches[i] = vyogotechv1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: name}}
		}
		res := r.computeNextShardName(benches, tc.prefix)
		if res != tc.expected {
			t.Errorf("For prefix %s with existing %v: expected %s, got %s", tc.prefix, tc.existing, tc.expected, res)
		}
	}
}

func TestFrappeBenchPool_BufferWarmShards(t *testing.T) {
	scheme := setupTestScheme()
	namespace := "bench-v16"
	pool := newTestPool(namespace, "v16-pool", 1, 5, 25, 80)
	pool.Spec.ShardingPolicy.BufferWarmShards = 1

	// bench1 has 5 sites (below watermark 20, but not empty)
	bench1 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-v16-001", Namespace: namespace},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
	}

	objs := []client.Object{pool, bench1}
	for i := 1; i <= 5; i++ {
		objs = append(objs, &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("s1-%d", i), Namespace: namespace},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: fmt.Sprintf("s1-%d.test.com", i),
				BenchRef: &vyogotechv1.NamespacedName{Name: "standard-v16-001", Namespace: namespace},
			},
		})
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&vyogotechv1.FrappeBenchPool{}, &vyogotechv1.FrappeBench{}).
		Build()

	r := &FrappeBenchPoolReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Should provision standard-v16-002 as a buffer warm shard
	var benches vyogotechv1.FrappeBenchList
	if err := c.List(context.TODO(), &benches, client.InNamespace(namespace)); err != nil {
		t.Fatalf("Failed to list benches: %v", err)
	}
	if len(benches.Items) != 2 {
		t.Fatalf("Expected 2 benches (1 occupied + 1 warm buffer), found %d", len(benches.Items))
	}
}

func TestFrappeBenchPool_StatusUpdate(t *testing.T) {
	scheme := setupTestScheme()
	namespace := "bench-v16"
	pool := newTestPool(namespace, "v16-pool", 2, 5, 25, 80)
	pool.Spec.ShardingPolicy.BufferWarmShards = 0

	bench1 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-v16-001", Namespace: namespace},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
	}
	bench2 := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "standard-v16-002", Namespace: namespace},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
	}

	objs := []client.Object{pool, bench1, bench2}
	for i := 1; i <= 10; i++ {
		objs = append(objs, &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("s-%d", i), Namespace: namespace},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: fmt.Sprintf("s-%d.test.com", i),
				BenchRef: &vyogotechv1.NamespacedName{Name: "standard-v16-001", Namespace: namespace},
			},
		})
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&vyogotechv1.FrappeBenchPool{}, &vyogotechv1.FrappeBench{}).
		Build()

	r := &FrappeBenchPoolReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.TODO(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	var updatedPool vyogotechv1.FrappeBenchPool
	if err := c.Get(context.TODO(), types.NamespacedName{Name: pool.Name, Namespace: namespace}, &updatedPool); err != nil {
		t.Fatalf("Failed to get pool: %v", err)
	}

	if updatedPool.Status.TotalShards != 2 {
		t.Errorf("Expected TotalShards 2, got %d", updatedPool.Status.TotalShards)
	}
	if updatedPool.Status.ReadyShards != 2 {
		t.Errorf("Expected ReadyShards 2, got %d", updatedPool.Status.ReadyShards)
	}
	if updatedPool.Status.TotalActiveSites != 10 {
		t.Errorf("Expected TotalActiveSites 10, got %d", updatedPool.Status.TotalActiveSites)
	}
	if len(updatedPool.Status.Shards) != 2 {
		t.Errorf("Expected 2 shard status entries, got %d", len(updatedPool.Status.Shards))
	}
}

