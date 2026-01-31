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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ServiceBuilder provides a fluent interface for building Services
type ServiceBuilder struct {
	service *corev1.Service
	owner   metav1.Object
	scheme  *runtime.Scheme
}

// NewServiceBuilder creates a new ServiceBuilder
func NewServiceBuilder(name, namespace string) *ServiceBuilder {
	return &ServiceBuilder{
		service: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    make(map[string]string),
			},
			Spec: corev1.ServiceSpec{
				Selector: make(map[string]string),
				Ports:    []corev1.ServicePort{},
			},
		},
	}
}

// WithOwner sets the owner reference for garbage collection
func (b *ServiceBuilder) WithOwner(owner metav1.Object, scheme *runtime.Scheme) *ServiceBuilder {
	b.owner = owner
	b.scheme = scheme
	return b
}

// WithLabels sets labels on the service
func (b *ServiceBuilder) WithLabels(labels map[string]string) *ServiceBuilder {
	for k, v := range labels {
		b.service.Labels[k] = v
	}
	return b
}

// WithAnnotations sets annotations on the service
func (b *ServiceBuilder) WithAnnotations(annotations map[string]string) *ServiceBuilder {
	if b.service.Annotations == nil {
		b.service.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.service.Annotations[k] = v
	}
	return b
}

// WithSelector sets the selector labels
func (b *ServiceBuilder) WithSelector(selector map[string]string) *ServiceBuilder {
	for k, v := range selector {
		b.service.Spec.Selector[k] = v
	}
	return b
}

// WithPort adds a port to the service
func (b *ServiceBuilder) WithPort(name string, port, targetPort int32) *ServiceBuilder {
	return b.WithPortProtocol(name, port, targetPort, corev1.ProtocolTCP)
}

// WithPortProtocol adds a port with specified protocol
func (b *ServiceBuilder) WithPortProtocol(name string, port, targetPort int32, protocol corev1.Protocol) *ServiceBuilder {
	b.service.Spec.Ports = append(b.service.Spec.Ports, corev1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromInt(int(targetPort)),
		Protocol:   protocol,
	})
	return b
}

// WithNamedTargetPort adds a port using a named target
func (b *ServiceBuilder) WithNamedTargetPort(name string, port int32, targetPortName string) *ServiceBuilder {
	b.service.Spec.Ports = append(b.service.Spec.Ports, corev1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromString(targetPortName),
		Protocol:   corev1.ProtocolTCP,
	})
	return b
}

// WithType sets the service type
func (b *ServiceBuilder) WithType(svcType corev1.ServiceType) *ServiceBuilder {
	b.service.Spec.Type = svcType
	return b
}

// AsClusterIP sets the service as ClusterIP (default)
func (b *ServiceBuilder) AsClusterIP() *ServiceBuilder {
	return b.WithType(corev1.ServiceTypeClusterIP)
}

// AsNodePort sets the service as NodePort
func (b *ServiceBuilder) AsNodePort() *ServiceBuilder {
	return b.WithType(corev1.ServiceTypeNodePort)
}

// AsLoadBalancer sets the service as LoadBalancer
func (b *ServiceBuilder) AsLoadBalancer() *ServiceBuilder {
	return b.WithType(corev1.ServiceTypeLoadBalancer)
}

// AsHeadless creates a headless service
func (b *ServiceBuilder) AsHeadless() *ServiceBuilder {
	b.service.Spec.ClusterIP = corev1.ClusterIPNone
	return b
}

// WithClusterIP sets a specific cluster IP
func (b *ServiceBuilder) WithClusterIP(ip string) *ServiceBuilder {
	b.service.Spec.ClusterIP = ip
	return b
}

// WithSessionAffinity sets session affinity
func (b *ServiceBuilder) WithSessionAffinity(affinity corev1.ServiceAffinity) *ServiceBuilder {
	b.service.Spec.SessionAffinity = affinity
	return b
}

// WithExternalTrafficPolicy sets external traffic policy (for LoadBalancer/NodePort)
func (b *ServiceBuilder) WithExternalTrafficPolicy(policy corev1.ServiceExternalTrafficPolicyType) *ServiceBuilder {
	b.service.Spec.ExternalTrafficPolicy = policy
	return b
}

// Build returns the constructed Service
func (b *ServiceBuilder) Build() (*corev1.Service, error) {
	if b.owner != nil && b.scheme != nil {
		if err := controllerutil.SetControllerReference(b.owner, b.service, b.scheme); err != nil {
			return nil, err
		}
	}
	return b.service, nil
}

// MustBuild returns the Service or panics on error
func (b *ServiceBuilder) MustBuild() *corev1.Service {
	s, err := b.Build()
	if err != nil {
		panic(err)
	}
	return s
}
