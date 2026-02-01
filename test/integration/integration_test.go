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

// Package integration contains integration tests for the Frappe Operator
// These tests require a Kubernetes cluster (Kind, Minikube, or real cluster)
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	k8sClient client.Client
	testCtx   context.Context
	cancel    context.CancelFunc
)

// TestMain sets up the test environment
func TestMain(m *testing.M) {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION_TEST") != "true" {
		fmt.Println("Skipping integration tests. Set INTEGRATION_TEST=true to run.")
		os.Exit(0)
	}

	// Setup
	testCtx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Printf("Failed to get kubeconfig: %v\n", err)
		os.Exit(1)
	}

	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = vyogotechv1alpha1.AddToScheme(s)

	k8sClient, err = client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	cancel()
	os.Exit(code)
}

// skipIfNoCluster skips the test if no cluster is available
func skipIfNoCluster(t *testing.T) {
	t.Helper()
	if _, err := rest.InClusterConfig(); err != nil {
		if _, err := config.GetConfig(); err != nil {
			t.Skip("No Kubernetes cluster available")
		}
	}
}

// createTestNamespace creates a namespace for testing
func createTestNamespace(t *testing.T, name string) {
	t.Helper()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"test": "integration",
			},
		},
	}
	if err := k8sClient.Create(testCtx, ns); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create namespace: %v", err)
	}
}

// cleanupTestNamespace deletes a test namespace
func cleanupTestNamespace(t *testing.T, name string) {
	t.Helper()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_ = k8sClient.Delete(testCtx, ns)
}

// TestBenchCreation tests creating a FrappeBench
func TestBenchCreation(t *testing.T) {
	skipIfNoCluster(t)

	namespace := "test-bench-creation"
	createTestNamespace(t, namespace)
	defer cleanupTestNamespace(t, namespace)

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bench",
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "version-15",
			Apps: []vyogotechv1alpha1.AppSource{
				{
					Name:   "frappe",
					Source: "image",
				},
			},
		},
	}

	// Create
	if err := k8sClient.Create(testCtx, bench); err != nil {
		t.Fatalf("Failed to create bench: %v", err)
	}

	// Wait for phase update
	var createdBench vyogotechv1alpha1.FrappeBench
	err := waitFor(t, 30*time.Second, func() bool {
		if err := k8sClient.Get(testCtx, types.NamespacedName{Name: "test-bench", Namespace: namespace}, &createdBench); err != nil {
			return false
		}
		return createdBench.Status.Phase != ""
	})
	if err != nil {
		t.Logf("Bench status: %+v", createdBench.Status)
	}

	// Verify bench was created
	if err := k8sClient.Get(testCtx, types.NamespacedName{Name: "test-bench", Namespace: namespace}, &createdBench); err != nil {
		t.Fatalf("Failed to get created bench: %v", err)
	}

	t.Logf("Bench created with phase: %s", createdBench.Status.Phase)
}

// TestBenchValidation tests webhook validation for FrappeBench
func TestBenchValidation(t *testing.T) {
	skipIfNoCluster(t)

	namespace := "test-bench-validation"
	createTestNamespace(t, namespace)
	defer cleanupTestNamespace(t, namespace)

	// Test missing required fields
	invalidBench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-bench",
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			// Missing FrappeVersion and Apps
		},
	}

	err := k8sClient.Create(testCtx, invalidBench)
	if err == nil {
		t.Error("Expected validation error for missing required fields")
	} else {
		t.Logf("Expected validation error: %v", err)
	}
}

// TestSiteCreation tests creating a FrappeSite
func TestSiteCreation(t *testing.T) {
	skipIfNoCluster(t)

	namespace := "test-site-creation"
	createTestNamespace(t, namespace)
	defer cleanupTestNamespace(t, namespace)

	// Create bench first
	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site-test-bench",
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "version-15",
			Apps: []vyogotechv1alpha1.AppSource{
				{Name: "frappe", Source: "image"},
			},
		},
	}
	if err := k8sClient.Create(testCtx, bench); err != nil {
		t.Fatalf("Failed to create bench: %v", err)
	}

	// Create site
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-site",
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "test.local",
			BenchRef: &vyogotechv1alpha1.NamespacedName{
				Name:      "site-test-bench",
				Namespace: namespace,
			},
			DBConfig: vyogotechv1alpha1.DatabaseConfig{
				Provider: "mariadb",
				Mode:     "shared",
			},
		},
	}

	if err := k8sClient.Create(testCtx, site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Verify site was created
	var createdSite vyogotechv1alpha1.FrappeSite
	if err := k8sClient.Get(testCtx, types.NamespacedName{Name: "test-site", Namespace: namespace}, &createdSite); err != nil {
		t.Fatalf("Failed to get created site: %v", err)
	}

	t.Logf("Site created with phase: %s", createdSite.Status.Phase)
}

// TestSiteValidation tests webhook validation for FrappeSite
func TestSiteValidation(t *testing.T) {
	skipIfNoCluster(t)

	namespace := "test-site-validation"
	createTestNamespace(t, namespace)
	defer cleanupTestNamespace(t, namespace)

	// Test missing benchRef
	invalidSite := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-site",
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "test.local",
			// Missing BenchRef
		},
	}

	err := k8sClient.Create(testCtx, invalidSite)
	if err == nil {
		t.Error("Expected validation error for missing benchRef")
	} else {
		t.Logf("Expected validation error: %v", err)
	}
}

// TestResourceDefaults tests that default resources are applied
func TestResourceDefaults(t *testing.T) {
	defaults := vyogotechv1alpha1.DefaultComponentResources()

	if defaults.Gunicorn == nil {
		t.Error("Expected Gunicorn defaults to be set")
	}
	if defaults.Nginx == nil {
		t.Error("Expected Nginx defaults to be set")
	}
	if defaults.Scheduler == nil {
		t.Error("Expected Scheduler defaults to be set")
	}
	if defaults.WorkerDefault == nil {
		t.Error("Expected WorkerDefault defaults to be set")
	}

	// Test production defaults
	prodDefaults := vyogotechv1alpha1.ProductionComponentResources()
	prodMem := prodDefaults.Gunicorn.Limits[corev1.ResourceMemory]
	defMem := defaults.Gunicorn.Limits[corev1.ResourceMemory]
	if (&prodMem).String() == (&defMem).String() {
		t.Error("Production defaults should have higher memory than default")
	}
}

// TestMergeResources tests resource merging
func TestMergeResources(t *testing.T) {
	defaults := vyogotechv1alpha1.DefaultComponentResources()

	// User overrides only Gunicorn
	userResources := vyogotechv1alpha1.ComponentResources{
		Gunicorn: &vyogotechv1alpha1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: vyogotechv1alpha1.MustParseQuantity("500m"),
			},
		},
	}

	merged := userResources.MergeWithDefaults(defaults)

	// Gunicorn should be user's value
	mergedCPU := merged.Gunicorn.Requests[corev1.ResourceCPU]
	if (&mergedCPU).String() != "500m" {
		t.Errorf("Expected merged Gunicorn CPU to be 500m, got %s", (&mergedCPU).String())
	}

	// Nginx should be default
	if merged.Nginx == nil {
		t.Error("Expected Nginx to use default value")
	}
}

// waitFor waits for a condition to be true
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("condition not met within %v", timeout)
}
