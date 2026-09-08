/*
Copyright 2023 Vyogo Technologies.
*/

package controllers

import (
	"context"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestFrappeSiteReconciler_ensureIngress covers the three rows of the HTTPS
// policy table in controllers/tls_policy.go: TLS is per-site opt-in
// (spec.tls.enabled) unless the operator-wide EnforceHTTPS setting is on, in
// which case TLS is mandatory and any insecure annotation override is
// stripped rather than honored.
func TestFrappeSiteReconciler_ensureIngress(t *testing.T) {
	newSite := func(tlsEnabled bool, annotations map[string]string) *vyogotechv1.FrappeSite {
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: "site.local",
				BenchRef: &vyogotechv1.NamespacedName{Name: "bench"},
				TLS:      vyogotechv1.TLSConfig{Enabled: tlsEnabled},
			},
		}
		if annotations != nil {
			site.Spec.Ingress = &vyogotechv1.IngressConfig{Annotations: annotations}
		}
		return site
	}

	tests := []struct {
		name              string
		enforceHTTPS      bool
		tlsEnabled        bool
		siteAnnotations   map[string]string
		wantTLS           bool
		wantRedirect      string // "" means the annotation must be absent
		wantStrippedEvent bool
	}{
		{
			name:         "both off: plain HTTP, unchanged from every pre-policy release",
			enforceHTTPS: false,
			tlsEnabled:   false,
			wantTLS:      false,
			wantRedirect: "",
		},
		{
			name:         "per-site opt-in: TLS and redirect added",
			enforceHTTPS: false,
			tlsEnabled:   true,
			wantTLS:      true,
			wantRedirect: "true",
		},
		{
			name:              "enforced: TLS mandatory, insecure override stripped",
			enforceHTTPS:      true,
			tlsEnabled:        false,
			siteAnnotations:   map[string]string{"nginx.ingress.kubernetes.io/ssl-redirect": "false"},
			wantTLS:           true,
			wantRedirect:      "true",
			wantStrippedEvent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(vyogotechv1.AddToScheme(scheme))
			utilruntime.Must(networkingv1.AddToScheme(scheme))
			site := newSite(tt.tlsEnabled, tt.siteAnnotations)
			bench := &vyogotechv1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"},
				Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "15"},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, bench).Build()
			recorder := record.NewFakeRecorder(5)
			r := &FrappeSiteReconciler{Client: client, Scheme: scheme, Recorder: recorder, EnforceHTTPS: tt.enforceHTTPS}
			ctx := context.Background()
			if err := r.ensureIngress(ctx, site, bench, "site.example.com"); err != nil {
				t.Fatalf("ensureIngress: %v", err)
			}
			ingress := &networkingv1.Ingress{}
			if err := client.Get(ctx, types.NamespacedName{Name: "site-ingress", Namespace: "default"}, ingress); err != nil {
				t.Fatalf("Get Ingress: %v", err)
			}
			if ingress.Spec.Rules[0].Host != "site.example.com" {
				t.Errorf("expected host site.example.com, got %s", ingress.Spec.Rules[0].Host)
			}
			if ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name != "bench-nginx" {
				t.Errorf("expected backend bench-nginx, got %s", ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name)
			}
			hasTLS := len(ingress.Spec.TLS) > 0
			if hasTLS != tt.wantTLS {
				t.Errorf("expected TLS configured=%v, got %v", tt.wantTLS, hasTLS)
			}
			if hasTLS && ingress.Spec.TLS[0].Hosts[0] != "site.example.com" {
				t.Errorf("expected TLS host site.example.com, got %s", ingress.Spec.TLS[0].Hosts[0])
			}
			got, present := ingress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"]
			if tt.wantRedirect == "" && present {
				t.Errorf("expected no ssl-redirect annotation, got %q", got)
			} else if tt.wantRedirect != "" && got != tt.wantRedirect {
				t.Errorf("expected ssl-redirect %q, got %q", tt.wantRedirect, got)
			}
			select {
			case ev := <-recorder.Events:
				if !tt.wantStrippedEvent {
					t.Errorf("unexpected event: %s", ev)
				}
			default:
				if tt.wantStrippedEvent {
					t.Error("expected a TLSPolicyEnforced warning event, got none")
				}
			}
		})
	}
}

func TestFrappeSiteReconciler_ensureIngress_Disabled(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	enabled := false
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "site.local",
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench"},
			Ingress:  &vyogotechv1.IngressConfig{Enabled: &enabled},
		},
	}
	bench := &vyogotechv1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"}}
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
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "site.local",
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench"},
		},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "15"},
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
