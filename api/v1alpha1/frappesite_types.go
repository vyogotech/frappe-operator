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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Note: Common types (NamespacedName, TLSConfig, DatabaseConfig, etc.) are defined in shared_types.go

// FrappeSiteSpec defines the desired state of FrappeSite
type FrappeSiteSpec struct {
	// BenchRef references the FrappeBench this site belongs to
	// +kubebuilder:validation:Required
	BenchRef *NamespacedName `json:"benchRef"`

	// SiteName is the Frappe site name - MUST match the domain that will receive traffic
	// This is what Frappe uses to route requests based on HTTP Host header
	// Example: "erp.customer.com" or "customer1.myplatform.com"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	SiteName string `json:"siteName"`

	// AdminPasswordSecretRef references the Secret containing admin password
	// +optional
	AdminPasswordSecretRef *corev1.SecretReference `json:"adminPasswordSecretRef,omitempty"`

	// DBConfig defines database configuration for this site
	// +optional
	DBConfig DatabaseConfig `json:"dbConfig,omitempty"`

	// Domain is the external domain for ingress
	// MUST match siteName (defaults to siteName if not specified)
	// +optional
	Domain string `json:"domain,omitempty"`

	// TLS configuration
	// +optional
	TLS TLSConfig `json:"tls,omitempty"`

	// IngressClassName specifies the ingress class
	// +optional
	IngressClassName string `json:"ingressClassName,omitempty"`

	// Ingress configuration
	// +optional
	Ingress *IngressConfig `json:"ingress,omitempty"`
}

// FrappeSitePhase represents the current phase
type FrappeSitePhase string

const (
	FrappeSitePhasePending      FrappeSitePhase = "Pending"
	FrappeSitePhaseProvisioning FrappeSitePhase = "Provisioning"
	FrappeSitePhaseReady        FrappeSitePhase = "Ready"
	FrappeSitePhaseFailed       FrappeSitePhase = "Failed"
)

// FrappeSiteStatus defines the observed state of FrappeSite
type FrappeSiteStatus struct {
	// Phase is the current phase
	// +optional
	Phase FrappeSitePhase `json:"phase,omitempty"`

	// BenchReady indicates if the referenced bench is ready
	// +optional
	BenchReady bool `json:"benchReady,omitempty"`

	// SiteURL is the accessible URL
	// +optional
	SiteURL string `json:"siteURL,omitempty"`

	// DBConnectionSecret is the name of the Secret with DB credentials
	// +optional
	DBConnectionSecret string `json:"dbConnectionSecret,omitempty"`

	// ResolvedDomain is the final domain after resolution
	// +optional
	ResolvedDomain string `json:"resolvedDomain,omitempty"`

	// DomainSource indicates how domain was determined
	// Values: explicit, bench-suffix, auto-detected, sitename-default
	// +optional
	DomainSource string `json:"domainSource,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// FrappeSite is the Schema for the frappesites API
type FrappeSite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FrappeSiteSpec   `json:"spec,omitempty"`
	Status FrappeSiteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FrappeSiteList contains a list of FrappeSite
type FrappeSiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FrappeSite `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FrappeSite{}, &FrappeSiteList{})
}
