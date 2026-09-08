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
	"context"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureIngress creates an Ingress for the site
func (r *FrappeSiteReconciler) ensureIngress(ctx context.Context, site *vyogotechv1.FrappeSite, bench *vyogotechv1.FrappeBench, domain string) error {
	logger := log.FromContext(ctx)

	// Check if Ingress is disabled
	if site.Spec.Ingress != nil && site.Spec.Ingress.Enabled != nil && !*site.Spec.Ingress.Enabled {
		logger.Info("Ingress creation disabled by user", "site", site.Name)
		return nil
	}

	ingressName := fmt.Sprintf("%s-ingress", site.Name)
	ingress := &networkingv1.Ingress{}

	err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: site.Namespace}, ingress)
	if err == nil {
		logger.Info("Ingress already exists", "ingress", ingressName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Ingress", "ingress", ingressName, "domain", domain)

	// Determine ingress class
	ingressClassName := "nginx" // Default
	if site.Spec.IngressClassName != "" {
		ingressClassName = site.Spec.IngressClassName
	} else if site.Spec.Ingress != nil && site.Spec.Ingress.ClassName != "" {
		ingressClassName = site.Spec.Ingress.ClassName
	}

	// Validate IngressClass existence (optional/warning)
	// (Skipping for brevity in this refactored version, but keeping logic if needed)

	nginxSvcName := fmt.Sprintf("%s-nginx", bench.Name)
	pathType := networkingv1.PathTypePrefix

	// TLS is per-site opt-in (spec.tls.enabled) unless the operator-wide
	// FRAPPE_ENFORCE_HTTPS policy is on, in which case every site is HTTPS-only.
	// See controllers/tls_policy.go.
	tls := effectiveTLS(r.EnforceHTTPS, site)

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-body-size": "100m",
	}
	if tls {
		annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
		annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] = "true"
	}

	builder := resources.NewIngressBuilder(ingressName, site.Namespace).
		WithLabels(map[string]string{
			"app":  "frappe",
			"site": site.Name,
		}).
		WithAnnotations(annotations).
		WithClassName(ingressClassName).
		WithRule(domain, "/", pathType, nginxSvcName, 8080).
		WithOwner(site, r.Scheme)

	if tls {
		tlsSecretName := site.Spec.TLS.SecretName
		if tlsSecretName == "" {
			tlsSecretName = fmt.Sprintf("%s-tls", site.Name)
		}
		builder.WithTLS([]string{domain}, tlsSecretName)

		issuer := site.Spec.TLS.Issuer
		if issuer == "" {
			issuer = r.DefaultClusterIssuer
		}
		if issuer != "" {
			builder.WithAnnotations(map[string]string{
				"cert-manager.io/cluster-issuer": issuer,
			})
		}
	}

	// Merge additional annotations from site spec
	if site.Spec.Ingress != nil && site.Spec.Ingress.Annotations != nil {
		builder.WithAnnotations(site.Spec.Ingress.Annotations)
	}

	ingress, err = builder.Build()
	if err != nil {
		return err
	}

	// Under an enforced HTTPS policy, a site must not be able to opt back out
	// of the mandatory redirect via its own ingress annotations.
	if r.EnforceHTTPS {
		if stripped := stripInsecureOverrides(ingress.Annotations); len(stripped) > 0 {
			logger.Info("Stripped insecure ingress annotation overrides under enforced HTTPS policy", "annotations", stripped)
			r.Recorder.Eventf(site, corev1.EventTypeWarning, "TLSPolicyEnforced",
				"HTTPS is enforced operator-wide; ignoring annotations: %v", stripped)
		}
	}

	if err := r.Create(ctx, ingress); err != nil {
		return fmt.Errorf("failed to create Ingress: %w", err)
	}

	logger.Info("Ingress created successfully", "ingress", ingressName, "host", domain)
	return nil
}

// ensureRoute creates an OpenShift Route for the site
func (r *FrappeSiteReconciler) ensureRoute(ctx context.Context, site *vyogotechv1.FrappeSite, bench *vyogotechv1.FrappeBench, domain string) error {
	logger := log.FromContext(ctx)

	routeName := fmt.Sprintf("%s-route", site.Name)
	route := &routev1.Route{}

	err := r.Get(ctx, types.NamespacedName{Name: routeName, Namespace: site.Namespace}, route)
	if err == nil {
		logger.Info("Route already exists", "route", routeName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating OpenShift Route", "route", routeName, "domain", domain)

	nginxSvcName := fmt.Sprintf("%s-nginx", bench.Name)

	routeHost := domain
	if site.Spec.RouteConfig != nil && site.Spec.RouteConfig.Host != "" {
		routeHost = site.Spec.RouteConfig.Host
	}

	// Determine TLS termination
	tlsTermination := routev1.TLSTerminationEdge
	if site.Spec.RouteConfig != nil && site.Spec.RouteConfig.TLSTermination != "" {
		switch site.Spec.RouteConfig.TLSTermination {
		case "passthrough":
			tlsTermination = routev1.TLSTerminationPassthrough
		case "reencrypt":
			tlsTermination = routev1.TLSTerminationReencrypt
		}
	}

	route = &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: site.Namespace,
			Labels: map[string]string{
				"app":  "frappe",
				"site": site.Name,
			},
		},
		Spec: routev1.RouteSpec{
			Host: routeHost,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: nginxSvcName,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(8080),
			},
			TLS: &routev1.TLSConfig{
				Termination:                   tlsTermination,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
			WildcardPolicy: routev1.WildcardPolicyNone,
		},
	}

	// Add additional annotations from site spec
	if site.Spec.RouteConfig != nil && site.Spec.RouteConfig.Annotations != nil {
		if route.Annotations == nil {
			route.Annotations = make(map[string]string)
		}
		for k, v := range site.Spec.RouteConfig.Annotations {
			route.Annotations[k] = v
		}
	}

	if err := controllerutil.SetControllerReference(site, route, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, route); err != nil {
		return fmt.Errorf("failed to create Route: %w", err)
	}

	return nil
}
