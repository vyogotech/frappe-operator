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
	"fmt"

	routev1 "github.com/openshift/api/route/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// Shared resolution of a site's external routing settings. The FrappeSite
// Ingress/Route and the FrappeMicroservice Ingress/Routes that share its host
// must agree on all of these, so both controllers go through these helpers.

// siteIngressEnabled reports whether the site is exposed externally at all.
func siteIngressEnabled(site *vyogotechv1.FrappeSite) bool {
	return site.Spec.Ingress == nil || site.Spec.Ingress.Enabled == nil || *site.Spec.Ingress.Enabled
}

// siteUsesRoute reports whether the site is exposed with an OpenShift Route
// (rather than an Ingress).
func siteUsesRoute(isOpenShift bool, site *vyogotechv1.FrappeSite) bool {
	if !isOpenShift {
		return false
	}
	rc := site.Spec.RouteConfig
	return rc == nil || rc.Enabled == nil || *rc.Enabled
}

// siteIngressClassName returns the IngressClass the site's Ingress uses.
func siteIngressClassName(site *vyogotechv1.FrappeSite) string {
	if site.Spec.IngressClassName != "" {
		return site.Spec.IngressClassName
	}
	if site.Spec.Ingress != nil && site.Spec.Ingress.ClassName != "" {
		return site.Spec.Ingress.ClassName
	}
	return "nginx"
}

// siteTLSSecretName returns the TLS Secret the site's Ingress references.
func siteTLSSecretName(site *vyogotechv1.FrappeSite) string {
	if site.Spec.TLS.SecretName != "" {
		return site.Spec.TLS.SecretName
	}
	return fmt.Sprintf("%s-tls", site.Name)
}

// siteRouteHost returns the host of the site's OpenShift Route.
func siteRouteHost(site *vyogotechv1.FrappeSite, domain string) string {
	if site.Spec.RouteConfig != nil && site.Spec.RouteConfig.Host != "" {
		return site.Spec.RouteConfig.Host
	}
	return domain
}

// siteRouteTermination returns the TLS termination of the site's Route.
func siteRouteTermination(site *vyogotechv1.FrappeSite) routev1.TLSTerminationType {
	if site.Spec.RouteConfig != nil {
		switch site.Spec.RouteConfig.TLSTermination {
		case "passthrough":
			return routev1.TLSTerminationPassthrough
		case "reencrypt":
			return routev1.TLSTerminationReencrypt
		}
	}
	return routev1.TLSTerminationEdge
}
