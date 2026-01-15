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

	// Validate IngressClass exists and warn if missing
	ingressClass := &networkingv1.IngressClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: ingressClassName}, ingressClass); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("IngressClass not found - Ingress will be created but may not work until controller is installed",
				"class", ingressClassName,
				"hint", "Install NGINX Ingress Controller: kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/cloud/deploy.yaml")
		} else {
			logger.Error(err, "Failed to check IngressClass", "class", ingressClassName)
		}
	}

	pathType := networkingv1.PathTypePrefix
	nginxSvcName := fmt.Sprintf("%s-nginx", bench.Name)

	ingress = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: site.Namespace,
			Labels: map[string]string{
				"app":  "frappe",
				"site": site.Name,
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "100m",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: domain,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: nginxSvcName,
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Add TLS if enabled
	if site.Spec.TLS.Enabled {
		tlsSecretName := site.Spec.TLS.SecretName
		if tlsSecretName == "" {
			tlsSecretName = fmt.Sprintf("%s-tls", site.Name)
		}

		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{domain},
				SecretName: tlsSecretName,
			},
		}

		// Add cert-manager annotation if issuer is specified
		if site.Spec.TLS.Issuer != "" {
			if ingress.Annotations == nil {
				ingress.Annotations = make(map[string]string)
			}
			ingress.Annotations["cert-manager.io/cluster-issuer"] = site.Spec.TLS.Issuer
		}
	}

	// Merge additional annotations from site spec
	if site.Spec.Ingress != nil && site.Spec.Ingress.Annotations != nil {
		if ingress.Annotations == nil {
			ingress.Annotations = make(map[string]string)
		}
		for k, v := range site.Spec.Ingress.Annotations {
			ingress.Annotations[k] = v
		}
	}

	if err := controllerutil.SetControllerReference(site, ingress, r.Scheme); err != nil {
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
			Path: "",
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

	// Add TLS certificate if specified
	if site.Spec.TLS.Enabled {
		if site.Spec.TLS.SecretName != "" {
			route.Spec.TLS.Certificate = "" // Will be set by certificate controller
			route.Spec.TLS.Key = ""
		}
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

	// Update status with Route hostname after creation
	if route.Spec.Host != "" {
		logger.Info("Route created with hostname", "host", route.Spec.Host)
	} else if len(route.Status.Ingress) > 0 {
		logger.Info("Route created, hostname will be assigned", "pendingHost", route.Status.Ingress[0].Host)
	}

	return nil
}
