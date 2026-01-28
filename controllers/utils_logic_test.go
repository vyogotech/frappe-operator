package controllers

import (
	"context"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsPlatformOpenShift(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	t.Run("Nil client", func(t *testing.T) {
		if isPlatformOpenShift(context.TODO(), nil) {
			t.Error("Expected false for nil client")
		}
	})

	t.Run("Standard k8s (not openshift)", func(t *testing.T) {
		// Mock client that returns error for RouteList should be considered not OpenShift
		// Actually, if it's not registered in scheme, it will fail to list.
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		if isPlatformOpenShift(context.TODO(), client) {
			t.Error("Expected false for standard k8s")
		}
	})
}

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
