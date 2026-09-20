/*
Copyright 2024 Vyogo Technologies.

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
	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// insecureRedirectAnnotations are the nginx-ingress annotations that, when set
// to a falsy value, would disable the HTTPS redirect for an Ingress. Under an
// enforced HTTPS policy these must never be allowed to take effect, regardless
// of who set them (site spec, custom-domain spec, or an operator default).
var insecureRedirectAnnotations = []string{
	"nginx.ingress.kubernetes.io/ssl-redirect",
	"nginx.ingress.kubernetes.io/force-ssl-redirect",
}

// effectiveTLS reports whether a given FrappeSite's primary Ingress/domain
// should be provisioned with TLS + a forced HTTPS redirect. This is the
// operator-wide `enforceHTTPS` setting (frappe-operator-config ConfigMap /
// FRAPPE_ENFORCE_HTTPS env) OR'd with the site's own opt-in
// (spec.tls.enabled). When enforce is false, behavior matches every release
// prior to the HTTPS-policy change: TLS is strictly per-site opt-in.
func effectiveTLS(enforce bool, site *vyogotechv1.FrappeSite) bool {
	if enforce {
		return true
	}
	if site == nil {
		return false
	}
	return site.Spec.TLS.Enabled
}

// isFalsy reports whether an annotation value means "off" for the purposes of
// the nginx-ingress ssl-redirect/force-ssl-redirect annotations.
func isFalsy(v string) bool {
	return v == "false" || v == "0"
}

// stripInsecureOverrides resets any insecureRedirectAnnotations entries in
// annotations that are set to a falsy value back to "true", mutating the map
// in place, and returns the keys that were reset (nil if none). Deleting the
// key instead of resetting it would not be enough: ingress-nginx's own
// default for force-ssl-redirect is "false", so simply removing an explicit
// "false" would silently leave HTTP reachable. Call this only when the
// operator-wide HTTPS policy is enforced, after merging in any user-supplied
// annotations, so a site cannot opt back out of the mandatory redirect.
func stripInsecureOverrides(annotations map[string]string) []string {
	var stripped []string
	for _, key := range insecureRedirectAnnotations {
		if v, ok := annotations[key]; ok && isFalsy(v) {
			annotations[key] = "true"
			stripped = append(stripped, key)
		}
	}
	return stripped
}
