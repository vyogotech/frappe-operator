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

func TestPostgresProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := NewPostgresProvider(client, scheme)
	ctx := context.Background()
	site := &vyogotechv1alpha1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"}}
	site.Spec.DBConfig.Mode = "shared"

	// Create the secret beforehand to bypass fake client StringData conversion issues
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "site-db-password", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("testpass")},
	}
	if err := client.Create(ctx, secret); err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	info, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase unexpected error: %v", err)
	}
	if info.Provider != "postgres" {
		t.Errorf("expected provider postgres, got %s", info.Provider)
	}

	ready, err := p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady unexpected error: %v", err)
	}
	if ready {
		t.Error("expected IsReady to be false initially")
	}

	creds, err := p.GetCredentials(ctx, site)
	if err != nil {
		t.Fatalf("GetCredentials unexpected error: %v", err)
	}
	if creds.SecretName == "" {
		t.Error("expected non-empty secret name")
	}

	site.Spec.DeletionPolicy = "Delete"
	err = p.Cleanup(ctx, site)
	if err != nil {
		t.Fatalf("Cleanup unexpected error: %v", err)
	}
}
