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
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureIngress creates an Ingress for the site
func (r *FrappeSiteReconciler) ensureIngress(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string) error {
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
	}

	// Validate IngressClass existence (optional/warning)
	// (Skipping for brevity in this refactored version, but keeping logic if needed)

	nginxSvcName := fmt.Sprintf("%s-nginx", bench.Name)
	pathType := networkingv1.PathTypePrefix

	builder := resources.NewIngressBuilder(ingressName, site.Namespace).
		WithLabels(map[string]string{
			"app":  "frappe",
			"site": site.Name,
		}).
		WithAnnotations(map[string]string{
			"nginx.ingress.kubernetes.io/proxy-body-size": "100m",
		}).
		WithClassName(ingressClassName).
		WithRule(domain, "/", pathType, nginxSvcName, 8080).
		WithOwner(site, r.Scheme)

	// Add TLS if enabled
	if site.Spec.TLS.Enabled {
		tlsSecretName := site.Spec.TLS.SecretName
		if tlsSecretName == "" {
			tlsSecretName = fmt.Sprintf("%s-tls", site.Name)
		}
		builder.WithTLS([]string{domain}, tlsSecretName)

		if site.Spec.TLS.Issuer != "" {
			builder.WithAnnotations(map[string]string{
				"cert-manager.io/cluster-issuer": site.Spec.TLS.Issuer,
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

	if err := r.Create(ctx, ingress); err != nil {
		return fmt.Errorf("failed to create Ingress: %w", err)
	}

	logger.Info("Ingress created successfully", "ingress", ingressName, "host", domain)
	return nil
}

// ensureRoute creates an OpenShift Route for the site
func (r *FrappeSiteReconciler) ensureRoute(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string) error {
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
			Host: domain,
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
