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

package resources

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// IngressBuilder provides a fluent interface for building Ingresses
type IngressBuilder struct {
	ingress *networkingv1.Ingress
	owner   metav1.Object
	scheme  *runtime.Scheme
}

// NewIngressBuilder creates a new IngressBuilder
func NewIngressBuilder(name, namespace string) *IngressBuilder {
	return &IngressBuilder{
		ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    make(map[string]string),
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{},
			},
		},
	}
}

// WithOwner sets the owner reference for garbage collection
func (b *IngressBuilder) WithOwner(owner metav1.Object, scheme *runtime.Scheme) *IngressBuilder {
	b.owner = owner
	b.scheme = scheme
	return b
}

// WithLabels sets labels on the ingress
func (b *IngressBuilder) WithLabels(labels map[string]string) *IngressBuilder {
	for k, v := range labels {
		b.ingress.Labels[k] = v
	}
	return b
}

// WithAnnotations sets annotations on the ingress
func (b *IngressBuilder) WithAnnotations(annotations map[string]string) *IngressBuilder {
	if b.ingress.Annotations == nil {
		b.ingress.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.ingress.Annotations[k] = v
	}
	return b
}

// WithClassName sets the ingress class name
func (b *IngressBuilder) WithClassName(className string) *IngressBuilder {
	if className != "" {
		b.ingress.Spec.IngressClassName = &className
	}
	return b
}

// WithTLS adds a TLS configuration
func (b *IngressBuilder) WithTLS(hosts []string, secretName string) *IngressBuilder {
	if secretName == "" {
		return b
	}
	b.ingress.Spec.TLS = append(b.ingress.Spec.TLS, networkingv1.IngressTLS{
		Hosts:      hosts,
		SecretName: secretName,
	})
	return b
}

// WithRule adds a simple HTTP rule
func (b *IngressBuilder) WithRule(host string, path string, pathType networkingv1.PathType, serviceName string, servicePort int32) *IngressBuilder {
	rule := networkingv1.IngressRule{
		Host: host,
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path:     path,
						PathType: &pathType,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: serviceName,
								Port: networkingv1.ServiceBackendPort{
									Number: servicePort,
								},
							},
						},
					},
				},
			},
		},
	}
	b.ingress.Spec.Rules = append(b.ingress.Spec.Rules, rule)
	return b
}

// Build returns the constructed Ingress
func (b *IngressBuilder) Build() (*networkingv1.Ingress, error) {
	if b.owner != nil && b.scheme != nil {
		if err := controllerutil.SetControllerReference(b.owner, b.ingress, b.scheme); err != nil {
			return nil, err
		}
	}
	return b.ingress, nil
}

// MustBuild returns the Ingress or panics on error
func (b *IngressBuilder) MustBuild() *networkingv1.Ingress {
	i, err := b.Build()
	if err != nil {
		panic(err)
	}
	return i
}
