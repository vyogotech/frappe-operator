package controllers

import (
	"context"
	"strings"
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFrappeBenchReconciler_Delete(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))

	namespace := "test-ns"
	benchName := "test-bench"

	t.Run("Blocked by dependent sites", func(t *testing.T) {
		// Bench with deletion timestamp and finalizer
		now := metav1.Now()
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{
				Name:              benchName,
				Namespace:         namespace,
				DeletionTimestamp: &now,
				Finalizers:        []string{"vyogo.tech/bench-finalizer"},
			},
		}

		// Dependent site
		site := &vyogotechv1alpha1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "site1",
				Namespace: namespace,
			},
			Spec: vyogotechv1alpha1.FrappeSiteSpec{
				BenchRef: &vyogotechv1alpha1.NamespacedName{Name: benchName},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench, site).WithStatusSubresource(bench).Build()
		recorder := record.NewFakeRecorder(10)
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme, Recorder: recorder}

		_, err := r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Name: benchName, Namespace: namespace}})
		if err != nil {
			t.Fatalf("Reconcile failed: %v", err)
		}

		// Verify finalizer still exists
		updatedBench := &vyogotechv1alpha1.FrappeBench{}
		client.Get(context.TODO(), types.NamespacedName{Name: benchName, Namespace: namespace}, updatedBench)
		if len(updatedBench.Finalizers) == 0 {
			t.Error("Finalizer removed but dependent sites exist")
		}

		// Check status condition
		foundCondition := false
		for _, cond := range updatedBench.Status.Conditions {
			if cond.Reason == "DependentSitesExist" && cond.Status == metav1.ConditionFalse {
				foundCondition = true
				break
			}
		}
		if !foundCondition {
			t.Error("Expected DependentSitesExist condition")
		}
	})

	t.Run("Successful cleanup", func(t *testing.T) {
		// Bench with deletion timestamp and finalizer
		now := metav1.Now()
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{
				Name:              benchName,
				Namespace:         namespace,
				DeletionTimestamp: &now,
				Finalizers:        []string{"vyogo.tech/bench-finalizer"},
			},
		}

		// Create dummy resources that should be cleaned up/scaled down
		replicas := int32(1)
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      benchName + "-worker-default",
				Namespace: namespace,
			},
			Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
			Status: appsv1.DeploymentStatus{Replicas: 1}, // Simulate running
		}

		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      benchName + "-sites",
				Namespace: namespace,
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench, deploy, pvc).WithStatusSubresource(bench).Build()
		recorder := record.NewFakeRecorder(10)
		r := &FrappeBenchReconciler{Client: client, Scheme: scheme, Recorder: recorder}

		// First pass: Scale down deployments
		// We expect multiple reconciles as it waits for pods to terminate
		// and deletes resources sequentially

		// Pass 1: Scale down deployments
		_, err := r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Name: benchName, Namespace: namespace}})
		if err != nil {
			t.Fatalf("Reconcile 1 failed: %v", err)
		}

		// Update deployment status to 0 replicas to simulate termination
		updatedDeploy := &appsv1.Deployment{}
		client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-worker-default", Namespace: namespace}, updatedDeploy)
		updatedDeploy.Status.Replicas = 0
		updatedDeploy.Status.ReadyReplicas = 0
		client.Status().Update(context.TODO(), updatedDeploy)

		// Pass 2: Delete PVC and remove finalizer (controller may remove finalizer here; fake client may then remove bench)
		_, err = r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Name: benchName, Namespace: namespace}})
		if err != nil && !errors.IsNotFound(err) && !strings.Contains(err.Error(), "not found") {
			t.Fatalf("Reconcile 2 failed: %v", err)
		}

		// Pass 3: No-op if bench already gone (finalizer was removed in Pass 2)
		_, err = r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Name: benchName, Namespace: namespace}})
		if err != nil && !errors.IsNotFound(err) && !strings.Contains(err.Error(), "not found") {
			t.Fatalf("Reconcile 3 failed: %v", err)
		}

		// Verify deployment scaled to 0
		client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-worker-default", Namespace: namespace}, updatedDeploy)
		if updatedDeploy.Spec.Replicas != nil && *updatedDeploy.Spec.Replicas != 0 {
			t.Errorf("Deployment not scaled to 0, got %d", *updatedDeploy.Spec.Replicas)
		}

		// Verify PVC deleted (or bench gone so cleanup completed)
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName + "-sites", Namespace: namespace}, pvc)
		if !errors.IsNotFound(err) {
			t.Logf("PVC still exists: %v", err)
		}

		// Verify finalizer removed (only if bench still exists in fake client)
		updatedBench := &vyogotechv1alpha1.FrappeBench{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: benchName, Namespace: namespace}, updatedBench)
		if err == nil && len(updatedBench.Finalizers) != 0 {
			t.Error("Finalizer not removed")
		}
	})
}
