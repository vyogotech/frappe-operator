/*
Copyright 2023 Vyogo Technologies.
*/

package controllers

import (
	"context"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFrappeSiteReconciler_ensureIngress(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "site.local",
			BenchRef: &vyogotechv1alpha1.NamespacedName{Name: "bench"},
		},
	}
	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"},
		Spec:       vyogotechv1alpha1.FrappeBenchSpec{FrappeVersion: "15"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).Build()
	r := &FrappeSiteReconciler{Client: client, Scheme: scheme}
	ctx := context.Background()
	err := r.ensureIngress(ctx, site, bench, "site.example.com")
	if err != nil {
		t.Fatalf("ensureIngress: %v", err)
	}
	ingress := &networkingv1.Ingress{}
	err = client.Get(ctx, types.NamespacedName{Name: "site-ingress", Namespace: "default"}, ingress)
	if err != nil {
		t.Fatalf("Get Ingress: %v", err)
	}
	if ingress.Spec.Rules[0].Host != "site.example.com" {
		t.Errorf("expected host site.example.com, got %s", ingress.Spec.Rules[0].Host)
	}
	if ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name != "bench-nginx" {
		t.Errorf("expected backend bench-nginx, got %s", ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name)
	}
}

func TestFrappeSiteReconciler_ensureIngress_Disabled(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	enabled := false
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "site.local",
			BenchRef: &vyogotechv1alpha1.NamespacedName{Name: "bench"},
			Ingress:  &vyogotechv1alpha1.IngressConfig{Enabled: &enabled},
		},
	}
	bench := &vyogotechv1alpha1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).Build()
	r := &FrappeSiteReconciler{Client: client, Scheme: scheme}
	ctx := context.Background()
	err := r.ensureIngress(ctx, site, bench, "site.example.com")
	if err != nil {
		t.Fatalf("ensureIngress (disabled): %v", err)
	}
	ingress := &networkingv1.Ingress{}
	err = client.Get(ctx, types.NamespacedName{Name: "site-ingress", Namespace: "default"}, ingress)
	if err == nil {
		t.Error("Ingress should not be created when disabled")
	}
}

func TestFrappeSiteReconciler_ensureRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	site := &vyogotechv1alpha1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
		Spec: vyogotechv1alpha1.FrappeSiteSpec{
			SiteName: "site.local",
			BenchRef: &vyogotechv1alpha1.NamespacedName{Name: "bench"},
		},
	}
	bench := &vyogotechv1alpha1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"},
		Spec:       vyogotechv1alpha1.FrappeBenchSpec{FrappeVersion: "15"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).Build()
	r := &FrappeSiteReconciler{Client: client, Scheme: scheme}
	ctx := context.Background()
	err := r.ensureRoute(ctx, site, bench, "site.example.com")
	if err != nil {
		t.Fatalf("ensureRoute: %v", err)
	}
	route := &routev1.Route{}
	err = client.Get(ctx, types.NamespacedName{Name: "site-route", Namespace: "default"}, route)
	if err != nil {
		t.Fatalf("Get Route: %v", err)
	}
	if route.Spec.Host != "site.example.com" {
		t.Errorf("expected host site.example.com, got %s", route.Spec.Host)
	}
	if route.Spec.To.Name != "bench-nginx" {
		t.Errorf("expected to.Name bench-nginx, got %s", route.Spec.To.Name)
	}
}
