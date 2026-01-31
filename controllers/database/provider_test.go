/*
Copyright 2023 Vyogo Technologies.
*/

package database

import (
	"context"
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("mariadb", func(t *testing.T) {
		config := vyogotechv1alpha1.DatabaseConfig{Mode: "shared", Provider: "mariadb"}
		p, err := NewProvider(config, client, scheme)
		if err != nil {
			t.Fatalf("NewProvider(mariadb) error: %v", err)
		}
		if p == nil {
			t.Fatal("NewProvider(mariadb) returned nil provider")
		}
		// MariaDB returns *MariaDBProvider
		_ = p
	})

	t.Run("external from ConnectionSecretRef", func(t *testing.T) {
		config := vyogotechv1alpha1.DatabaseConfig{
			ConnectionSecretRef: &corev1.SecretReference{Name: "db", Namespace: "default"},
		}
		p, err := NewProvider(config, client, scheme)
		if err != nil {
			t.Fatalf("NewProvider(external) error: %v", err)
		}
		if p == nil {
			t.Fatal("NewProvider(external) returned nil provider")
		}
		// external returns CircuitBreakerProvider wrapping ExternalProvider
		_ = p
	})

	t.Run("external explicit", func(t *testing.T) {
		config := vyogotechv1alpha1.DatabaseConfig{Provider: "external", Host: "rds.example.com"}
		p, err := NewProvider(config, client, scheme)
		if err != nil {
			t.Fatalf("NewProvider(external) error: %v", err)
		}
		if p == nil {
			t.Fatal("NewProvider(external) returned nil provider")
		}
		_ = p
	})

	t.Run("sqlite", func(t *testing.T) {
		config := vyogotechv1alpha1.DatabaseConfig{Provider: "sqlite"}
		p, err := NewProvider(config, client, scheme)
		if err != nil {
			t.Fatalf("NewProvider(sqlite) error: %v", err)
		}
		if p == nil {
			t.Fatal("NewProvider(sqlite) returned nil provider")
		}
		_ = p
	})

	t.Run("postgres returns error", func(t *testing.T) {
		config := vyogotechv1alpha1.DatabaseConfig{Provider: "postgres"}
		p, err := NewProvider(config, client, scheme)
		if err == nil {
			t.Fatal("NewProvider(postgres) expected error")
		}
		if p != nil {
			t.Error("NewProvider(postgres) should return nil provider")
		}
		if err != nil && err.Error() == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("unknown provider returns error", func(t *testing.T) {
		config := vyogotechv1alpha1.DatabaseConfig{Provider: "unknown"}
		p, err := NewProvider(config, client, scheme)
		if err == nil {
			t.Fatal("NewProvider(unknown) expected error")
		}
		if p != nil {
			t.Error("NewProvider(unknown) should return nil provider")
		}
	})
}

func TestMariaDBProvider_IsReady_NoDatabaseCR(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := NewMariaDBProvider(client, scheme)
	ctx := context.Background()
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "test.local",
			DBConfig: vyogotechv1alpha1.DatabaseConfig{Mode: "shared"},
		},
	}
	ready, err := p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady: %v", err)
	}
	if ready {
		t.Error("IsReady should be false when Database CR does not exist")
	}
}
