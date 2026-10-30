/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
)

func TestSiteIngressClassName(t *testing.T) {
	cases := []struct {
		name string
		spec vyogotechv1.FrappeSiteSpec
		want string
	}{
		{"defaults to nginx", vyogotechv1.FrappeSiteSpec{}, "nginx"},
		{"ingress.className", vyogotechv1.FrappeSiteSpec{Ingress: &vyogotechv1.IngressConfig{ClassName: "traefik"}}, "traefik"},
		{"top-level ingressClassName wins", vyogotechv1.FrappeSiteSpec{
			IngressClassName: "haproxy",
			Ingress:          &vyogotechv1.IngressConfig{ClassName: "traefik"},
		}, "haproxy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := siteIngressClassName(&vyogotechv1.FrappeSite{Spec: tc.spec}); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSiteTLSSecretName(t *testing.T) {
	site := &vyogotechv1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "acme"}}
	if got := siteTLSSecretName(site); got != "acme-tls" {
		t.Fatalf("default: got %q", got)
	}
	site.Spec.TLS.SecretName = "wildcard-tls"
	if got := siteTLSSecretName(site); got != "wildcard-tls" {
		t.Fatalf("explicit: got %q", got)
	}
}

func TestSiteIngressEnabled(t *testing.T) {
	if !siteIngressEnabled(&vyogotechv1.FrappeSite{}) {
		t.Fatal("ingress is enabled by default")
	}
	off := &vyogotechv1.FrappeSite{Spec: vyogotechv1.FrappeSiteSpec{Ingress: &vyogotechv1.IngressConfig{Enabled: resources.BoolPtr(false)}}}
	if siteIngressEnabled(off) {
		t.Fatal("ingress.enabled=false disables it")
	}
}

func TestSiteRouteHostAndTermination(t *testing.T) {
	site := &vyogotechv1.FrappeSite{}
	if got := siteRouteHost(site, "erp.acme.com"); got != "erp.acme.com" {
		t.Fatalf("default host: got %q", got)
	}
	if got := siteRouteTermination(site); got != routev1.TLSTerminationEdge {
		t.Fatalf("default termination: got %q", got)
	}

	site.Spec.RouteConfig = &vyogotechv1.RouteConfig{Host: "apps.acme.com", TLSTermination: "reencrypt"}
	if got := siteRouteHost(site, "erp.acme.com"); got != "apps.acme.com" {
		t.Fatalf("override host: got %q", got)
	}
	if got := siteRouteTermination(site); got != routev1.TLSTerminationReencrypt {
		t.Fatalf("reencrypt: got %q", got)
	}
	site.Spec.RouteConfig.TLSTermination = "passthrough"
	if got := siteRouteTermination(site); got != routev1.TLSTerminationPassthrough {
		t.Fatalf("passthrough: got %q", got)
	}
}

func TestSiteUsesRoute(t *testing.T) {
	site := &vyogotechv1.FrappeSite{}
	if siteUsesRoute(false, site) {
		t.Fatal("Kubernetes never uses Routes")
	}
	if !siteUsesRoute(true, site) {
		t.Fatal("OpenShift uses Routes by default")
	}
	site.Spec.RouteConfig = &vyogotechv1.RouteConfig{Enabled: resources.BoolPtr(false)}
	if siteUsesRoute(true, site) {
		t.Fatal("routeConfig.enabled=false falls back to an Ingress")
	}
}
