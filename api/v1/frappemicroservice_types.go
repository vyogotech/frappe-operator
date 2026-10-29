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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalSiteReference names a FrappeSite in the same namespace as the referring
// object. It deliberately has no namespace field: a microservice can only bind
// to a site in its own namespace (Secrets, Ingress TLS secrets and same-host
// OpenShift Routes are all namespace-local).
type LocalSiteReference struct {
	// Name of the FrappeSite.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// MicroserviceRouting exposes the microservice on the bound site's domain.
type MicroserviceRouting struct {
	// Paths are URL path prefixes on the site's domain that are routed to this
	// microservice instead of the monolith. Matching uses Ingress `Prefix`
	// semantics: `/orders` matches `/orders` and `/orders/...`, not
	// `/orders-report`.
	//
	// Validation only looks at this object. Two microservices claiming the same
	// path on one site are not detected by the operator; the ingress controller
	// (or an API gateway in front of it) decides which one wins.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MaxLength=256
	// +listType=set
	// +kubebuilder:validation:XValidation:rule="self.all(p, p.matches('^(/[A-Za-z0-9._~%-]+)+$'))",message="each path must be an absolute path made of non-empty segments of [A-Za-z0-9._~%-], with no trailing slash; '/' is not allowed"
	// +kubebuilder:validation:XValidation:rule="self.all(p, !p.matches('/[.][.]?(/|$)'))",message="paths must not contain dot segments ('.' or '..')"
	// +kubebuilder:validation:XValidation:rule="self.all(p, !['/api/method/login', '/login', '/app', '/assets', '/files', '/private', '/socket.io'].exists(r, p == r || p.startsWith(r + '/') || r.startsWith(p + '/')))",message="paths must not overlap reserved monolith routes: /api/method/login, /login, /app, /assets, /files, /private, /socket.io"
	Paths []string `json:"paths"`
}

// FrappeMicroserviceSpec defines the desired state of FrappeMicroservice.
type FrappeMicroserviceSpec struct {
	// SiteRef is the FrappeSite (in this namespace) the microservice binds to.
	// The operator generates the site binding Secret from it. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="siteRef is immutable"
	SiteRef LocalSiteReference `json:"siteRef"`

	// Image of the microservice, typically built on frappe-microservice-lib.
	// +kubebuilder:validation:XValidation:rule="has(self.repository) && self.repository != ''",message="image.repository is required"
	Image ImageConfig `json:"image"`

	// Port the container listens on and the Service exposes. The operator passes
	// it to the container as PORT.
	// +kubebuilder:default=8000
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// Routing exposes path prefixes on the site's domain. When omitted, the
	// microservice is only reachable in-cluster through its ClusterIP Service.
	// +optional
	Routing *MicroserviceRouting `json:"routing,omitempty"`

	// Replicas of the microservice Deployment.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources for the microservice container.
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// Env adds environment variables to the container. Operator-managed binding
	// keys (FRAPPE_SITE, CENTRAL_SITE_URL, PORT, DB_*) cannot be set here.
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:XValidation:rule="self.all(e, !(e.name in ['FRAPPE_SITE', 'CENTRAL_SITE_URL', 'PORT']) && !e.name.startsWith('DB_'))",message="env must not set operator-managed binding keys: FRAPPE_SITE, CENTRAL_SITE_URL, PORT, DB_*"
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// FrappeMicroservicePhase is a coarse summary of the microservice state.
// +kubebuilder:validation:Enum=Pending;Ready;Failed
type FrappeMicroservicePhase string

const (
	FrappeMicroservicePhasePending FrappeMicroservicePhase = "Pending"
	FrappeMicroservicePhaseReady   FrappeMicroservicePhase = "Ready"
	FrappeMicroservicePhaseFailed  FrappeMicroservicePhase = "Failed"
)

// Condition types reported on FrappeMicroservice.
const (
	// MicroserviceConditionSiteReady: the bound site is Ready and its binding
	// Secret is in sync.
	MicroserviceConditionSiteReady = "SiteReady"
	// MicroserviceConditionAvailable: the Deployment has the desired number of
	// ready replicas and the Service exists.
	MicroserviceConditionAvailable = "Available"
	// MicroserviceConditionRouted: only set when spec.routing is present. On
	// OpenShift it is True once every Route is admitted; on Kubernetes once the
	// Ingress has a load balancer address.
	MicroserviceConditionRouted = "Routed"
)

// FrappeMicroserviceStatus defines the observed state of FrappeMicroservice.
type FrappeMicroserviceStatus struct {
	// Phase is Ready when SiteReady and Available are True and, if routed,
	// Routed is True.
	// +optional
	Phase FrappeMicroservicePhase `json:"phase,omitempty"`

	// ReadyReplicas reported by the Deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// ServiceURL is the in-cluster URL of the microservice.
	// +optional
	ServiceURL string `json:"serviceURL,omitempty"`

	// ExternalURLs lists the routed URLs on the site's domain.
	// +optional
	ExternalURLs []string `json:"externalURLs,omitempty"`

	// ObservedGeneration is the generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions: SiteReady, Available, Routed.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=fms
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas"
//+kubebuilder:printcolumn:name="Service URL",type="string",JSONPath=".status.serviceURL"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FrappeMicroservice runs a frappe-microservice-lib image next to an existing
// FrappeSite: a site binding Secret, a Deployment, a ClusterIP Service and,
// when spec.routing is set, an Ingress (or OpenShift Routes) on the site's
// domain.
type FrappeMicroservice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FrappeMicroserviceSpec   `json:"spec,omitempty"`
	Status FrappeMicroserviceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FrappeMicroserviceList contains a list of FrappeMicroservice
type FrappeMicroserviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FrappeMicroservice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FrappeMicroservice{}, &FrappeMicroserviceList{})
}
