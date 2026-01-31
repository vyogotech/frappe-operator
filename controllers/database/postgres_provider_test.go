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

func TestPostgresProvider_NotImplemented(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := NewPostgresProvider(client, scheme)
	ctx := context.Background()
	site := &vyogotechv1alpha1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"}}

	_, err := p.EnsureDatabase(ctx, site)
	if err == nil {
		t.Fatal("EnsureDatabase expected error")
	}
	if err != nil && err.Error() == "" {
		t.Error("expected non-empty error")
	}

	_, err = p.IsReady(ctx, site)
	if err == nil {
		t.Fatal("IsReady expected error")
	}

	_, err = p.GetCredentials(ctx, site)
	if err == nil {
		t.Fatal("GetCredentials expected error")
	}

	err = p.Cleanup(ctx, site)
	if err == nil {
		t.Fatal("Cleanup expected error")
	}
}
