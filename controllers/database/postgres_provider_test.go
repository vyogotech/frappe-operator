/*
Copyright 2023 Vyogo Technologies.
*/

package database

import (
	"context"
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPostgresProvider_CredentialsAndNames(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-site",
			Namespace: "default",
		},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "mysite.com",
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-site-db-password",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("securepassword123"),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(secret).Build()
	config := vyogotechv1.DatabaseConfig{
		Provider: "postgres",
	}

	p := NewPostgresProvider(config, client, scheme).(*PostgresProvider)
	ctx := context.Background()

	// 1. Verify DB name generation matches PostgreSQL constraints
	dbName := p.generateDBName(site)
	if dbName == "" {
		t.Fatal("generated dbName should not be empty")
	}
	if dbName[0] != '_' {
		t.Errorf("expected dbName to start with underscore, got: %s", dbName)
	}

	// 2. Verify Credentials retrieval
	creds, err := p.GetCredentials(ctx, site)
	if err != nil {
		t.Fatalf("GetCredentials failed: %v", err)
	}
	if creds.Username != dbName {
		t.Errorf("expected username to match dbName, got %s", creds.Username)
	}
	if creds.Password != "securepassword123" {
		t.Errorf("expected password to match secret value, got %s", creds.Password)
	}

	// 3. Verify superuser fallback resolution
	host, port, user, _, _, err := p.getSuperuserCredentials(ctx, site)
	if err != nil {
		t.Fatalf("getSuperuserCredentials failed: %v", err)
	}
	if host != "frappe-postgres" {
		t.Errorf("expected default host to be frappe-postgres, got %s", host)
	}
	if port != "5432" {
		t.Errorf("expected default port to be 5432, got %s", port)
	}
	if user != "postgres" {
		t.Errorf("expected default user to be postgres, got %s", user)
	}
}
