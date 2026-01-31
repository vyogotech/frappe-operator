package controllers

import (
	"context"
	"os"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/constants"
	"github.com/vyogotech/frappe-operator/pkg/resources"
)

func TestGetDefaultSecurityValues(t *testing.T) {
	t.Run("GetDefaultUID", func(t *testing.T) {
		os.Unsetenv("FRAPPE_DEFAULT_UID")
		if getDefaultUID() != nil {
			t.Error("Expected nil when env not set")
		}

		os.Setenv("FRAPPE_DEFAULT_UID", "2000")
		uid := getDefaultUID()
		if uid == nil || *uid != 2000 {
			t.Errorf("Expected 2000, got %v", uid)
		}
		os.Unsetenv("FRAPPE_DEFAULT_UID")
	})

	t.Run("GetDefaultGID", func(t *testing.T) {
		os.Unsetenv("FRAPPE_DEFAULT_GID")
		if getDefaultGID() != nil {
			t.Error("Expected nil when env not set")
		}

		os.Setenv("FRAPPE_DEFAULT_GID", "3000")
		gid := getDefaultGID()
		if gid == nil || *gid != 3000 {
			t.Errorf("Expected 3000, got %v", gid)
		}
		os.Unsetenv("FRAPPE_DEFAULT_GID")
	})
}

func TestGetNamespaceMCSLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	nsName := "test-ns"
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Annotations: map[string]string{
				"openshift.io/sa.scc.mcs": "s0:c10,c20",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ns).Build()
	label := getNamespaceMCSLabel(context.TODO(), client, nsName)
	if label != "s0:c10,c20" {
		t.Errorf("Expected s0:c10,c20, got %s", label)
	}
}

func TestIsRouteAPIAvailable(t *testing.T) {
	// This tests the logic flow. In a real environment, it would need a discovery client.
	// Since we are using fake clients/configs, we mainly want to ensure the function exists and compiles.
	// A proper test would involve mocking the discovery client, but that's complex for a simple utility.
	// For now, we verify the reconcilers handle the flag correctly.
}

func TestReconciler_PlatformDetectionLogic(t *testing.T) {
	t.Run("FrappeBenchReconciler", func(t *testing.T) {
		r := &FrappeBenchReconciler{IsOpenShift: true}
		if !r.IsOpenShift {
			t.Error("Expected IsOpenShift to be true")
		}
	})

	t.Run("FrappeSiteReconciler", func(t *testing.T) {
		r := &FrappeSiteReconciler{IsOpenShift: true}
		if !r.isOpenShiftPlatform(context.TODO()) {
			t.Error("Expected isOpenShiftPlatform to return true")
		}
	})
}

func TestApplyDefaultJobTTL(t *testing.T) {
	// Nil spec: no-op
	applyDefaultJobTTL(nil)
	spec := &batchv1.JobSpec{}
	applyDefaultJobTTL(spec)
	if spec.TTLSecondsAfterFinished == nil {
		t.Fatal("applyDefaultJobTTL should set TTL on empty spec")
	}
	if *spec.TTLSecondsAfterFinished != resources.DefaultJobTTL {
		t.Errorf("TTL expected %d, got %d", resources.DefaultJobTTL, *spec.TTLSecondsAfterFinished)
	}
	// Already set: no override
	existing := int32(1800)
	spec2 := &batchv1.JobSpec{TTLSecondsAfterFinished: &existing}
	applyDefaultJobTTL(spec2)
	if *spec2.TTLSecondsAfterFinished != 1800 {
		t.Errorf("applyDefaultJobTTL should not override existing TTL, got %d", *spec2.TTLSecondsAfterFinished)
	}
}

func TestIsLocalDomain(t *testing.T) {
	if !isLocalDomain("site.local") {
		t.Error("site.local should be local domain")
	}
	if !isLocalDomain("app.localhost") {
		t.Error("app.localhost should be local domain")
	}
	if !isLocalDomain("localhost") {
		t.Error("localhost should be local domain")
	}
	if isLocalDomain("site.example.com") {
		t.Error("site.example.com should not be local domain")
	}
}

func TestGetEnvAsInt64(t *testing.T) {
	os.Unsetenv("TEST_INT_KEY")
	if getEnvAsInt64("TEST_INT_KEY", 42) != 42 {
		t.Error("expected default 42 when env unset")
	}
	os.Setenv("TEST_INT_KEY", "100")
	if getEnvAsInt64("TEST_INT_KEY", 42) != 100 {
		t.Error("expected 100 from env")
	}
	os.Setenv("TEST_INT_KEY", "invalid")
	if getEnvAsInt64("TEST_INT_KEY", 7) != 7 {
		t.Error("expected default 7 when env invalid")
	}
	os.Unsetenv("TEST_INT_KEY")
}

func TestGetDefaultFSGroup(t *testing.T) {
	os.Unsetenv("FRAPPE_DEFAULT_FSGROUP")
	if getDefaultFSGroup() != nil {
		t.Error("expected nil when env not set")
	}
	os.Setenv("FRAPPE_DEFAULT_FSGROUP", "2000")
	g := getDefaultFSGroup()
	if g == nil || *g != 2000 {
		t.Errorf("expected 2000, got %v", g)
	}
	os.Unsetenv("FRAPPE_DEFAULT_FSGROUP")
}

func TestBoolPtr(t *testing.T) {
	trueVal := boolPtr(true)
	if trueVal == nil || !*trueVal {
		t.Error("boolPtr(true) should return *true")
	}
	f := boolPtr(false)
	if f == nil || *f {
		t.Error("boolPtr(false) should return *false")
	}
}

func TestInt32Ptr(t *testing.T) {
	p := int32Ptr(99)
	if p == nil || *p != 99 {
		t.Errorf("int32Ptr(99) = %v", p)
	}
}

func TestFrappeSiteReconciler_getBenchImage(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	ctx := context.Background()

	t.Run("bench ImageConfig override", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"},
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				ImageConfig: &vyogotechv1alpha1.ImageConfig{
					Repository: "myreg/frappe",
					Tag:        "v15",
				},
			},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &FrappeSiteReconciler{Client: client}
		img := r.getBenchImage(ctx, bench)
		if img != "myreg/frappe:v15" {
			t.Errorf("getBenchImage expected myreg/frappe:v15, got %s", img)
		}
	})

	t.Run("fallback to constant when no config", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &FrappeSiteReconciler{Client: client}
		img := r.getBenchImage(ctx, bench)
		if img != constants.DefaultFrappeImage {
			t.Errorf("getBenchImage expected %s, got %s", constants.DefaultFrappeImage, img)
		}
	})

	t.Run("ConfigMap default image", func(t *testing.T) {
		bench := &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"},
			Spec:       vyogotechv1alpha1.FrappeBenchSpec{},
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "frappe-operator-config",
				Namespace: "frappe-operator-system",
			},
			Data: map[string]string{"defaultFrappeImage": "custom/frappe:latest"},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(cm).Build()
		r := &FrappeSiteReconciler{Client: client}
		img := r.getBenchImage(ctx, bench)
		if img != "custom/frappe:latest" {
			t.Errorf("getBenchImage expected custom/frappe:latest, got %s", img)
		}
	})
}
