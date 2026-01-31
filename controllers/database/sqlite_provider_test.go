/*
Copyright 2023 Vyogo Technologies.
*/

package database

import (
	"context"
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSQLiteProvider_EnsureDatabase(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := NewSQLiteProvider(client, scheme)
	ctx := context.Background()
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
		Spec:       vyogotechv1alpha1.FrappeSiteSpec{SiteName: "test.local"},
	}
	info, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if info.Provider != "sqlite" {
		t.Errorf("expected provider sqlite, got %s", info.Provider)
	}
	// SQLite uses literal "site" for Name in DatabaseInfo
	if info.Name != "site" {
		t.Errorf("expected name site, got %s", info.Name)
	}
}

func TestSQLiteProvider_IsReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := NewSQLiteProvider(client, scheme)
	ctx := context.Background()
	site := &vyogotechv1alpha1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"}}
	ready, err := p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady: %v", err)
	}
	if !ready {
		t.Error("SQLite IsReady should be true")
	}
}

func TestSQLiteProvider_GetCredentials(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := NewSQLiteProvider(client, scheme)
	ctx := context.Background()
	site := &vyogotechv1alpha1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"}}
	creds, err := p.GetCredentials(ctx, site)
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("GetCredentials returned nil")
	}
}

func TestSQLiteProvider_Cleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := NewSQLiteProvider(client, scheme)
	ctx := context.Background()
	site := &vyogotechv1alpha1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"}}
	err := p.Cleanup(ctx, site)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}
