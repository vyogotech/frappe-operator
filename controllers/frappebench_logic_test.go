package controllers

import (
	"context"
	"fmt"
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFrappeBenchReconciler_getBenchImage(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))

	namespace := "test-ns"

	t.Run("Override in spec", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace},
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				ImageConfig: &vyogotechv1alpha1.ImageConfig{
					Repository: "custom/frappe",
					Tag:        "v1.0",
				},
			},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &FrappeBenchReconciler{Client: client}
		image := r.getBenchImage(context.TODO(), bench)
		if image != "custom/frappe:v1.0" {
			t.Errorf("Expected custom/frappe:v1.0, got %s", image)
		}
	})

	t.Run("Default image with version", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace},
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				FrappeVersion: "v15",
			},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &FrappeBenchReconciler{Client: client}
		image := r.getBenchImage(context.TODO(), bench)
		if image != "docker.io/frappe/erpnext:v15" {
			t.Errorf("Expected docker.io/frappe/erpnext:v15, got %s", image)
		}
	})

	t.Run("ConfigMap default", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}

		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "frappe-operator-config",
				Namespace: "frappe-operator-system",
			},
			Data: map[string]string{
				"defaultFrappeImage": "myrepo/frappe:latest",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(configMap).Build()
		r := &FrappeBenchReconciler{Client: client}

		image := r.getBenchImage(context.TODO(), bench)
		if image != "myrepo/frappe:latest" {
			t.Errorf("Expected myrepo/frappe:latest, got %s", image)
		}
	})

	t.Run("Fallback to constant", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: namespace},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &FrappeBenchReconciler{Client: client}
		image := r.getBenchImage(context.TODO(), bench)
		if image != constants.DefaultFrappeImage {
			t.Errorf("Expected %s, got %s", constants.DefaultFrappeImage, image)
		}
	})
}

func TestFrappeBenchReconciler_isGitEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))

	t.Run("Bench override true", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &FrappeBenchReconciler{Client: client}
		enabled := true
		bench := &vyogotechv1alpha1.FrappeBench{
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				GitConfig: &vyogotechv1alpha1.GitConfig{
					Enabled: &enabled,
				},
			},
		}
		if !r.isGitEnabled(nil, bench) {
			t.Error("Expected git to be enabled")
		}
	})

	t.Run("ConfigMap default true", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &FrappeBenchReconciler{Client: client}
		bench := &vyogotechv1alpha1.FrappeBench{}
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				"gitEnabled": "true",
			},
		}
		if !r.isGitEnabled(cm, bench) {
			t.Error("Expected git to be enabled from CM")
		}
	})
}

func TestFrappeBenchReconciler_getOperatorConfig_NilClient(t *testing.T) {
	r := &FrappeBenchReconciler{Client: nil}
	_, err := r.getOperatorConfig(context.TODO(), "default")
	if err == nil {
		t.Error("Expected error for nil client, got nil")
	}
	if err.Error() != "client not initialized" {
		t.Errorf("Expected 'client not initialized', got '%v'", err.Error())
	}
}

func TestFrappeBenchReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))

	namespace := "frappe-system"
	benchName := "main-bench"

	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{
			Name:      benchName,
			Namespace: namespace,
		},
		Spec: vyogotechv1alpha1.FrappeBenchSpec{
			FrappeVersion: "v15",
		},
	}

	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}, Provisioner: "kubernetes.io/no-provisioner"}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench, sc).WithStatusSubresource(bench).Build()
	recorder := record.NewFakeRecorder(10)
	r := &FrappeBenchReconciler{
		Client:   client,
		Scheme:   scheme,
		Recorder: recorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      benchName,
			Namespace: namespace,
		},
	}

	// First pass: Should add finalizer and set Progressing condition
	result, err := r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	// Accept either an immediate completion or a requeue during provisioning
	if !result.IsZero() && result.RequeueAfter == 0 {
		t.Errorf("Unexpected reconcile result: %v", result)
	}

	// Verify finalizer
	updatedBench := &vyogotechv1alpha1.FrappeBench{}
	err = client.Get(context.TODO(), req.NamespacedName, updatedBench)
	if err != nil {
		t.Fatalf("Failed to get bench: %v", err)
	}
	foundFinalizer := false
	for _, f := range updatedBench.Finalizers {
		if f == "vyogo.tech/bench-finalizer" {
			foundFinalizer = true
			break
		}
	}
	if !foundFinalizer {
		t.Error("Finalizer not added")
	}

	// Second pass: Should create storage (PVC)
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("Second reconcile failed: %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s-sites", benchName), Namespace: namespace}, pvc)
	if err != nil {
		t.Errorf("PVC not created: %v", err)
	}
}
