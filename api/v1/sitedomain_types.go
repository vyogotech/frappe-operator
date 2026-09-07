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

// SiteDomainTLSSpec defines TLS certificate settings for custom domains
type SiteDomainTLSSpec struct {
	// Enabled determines whether to generate an automated TLS certificate.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// IssuerRef references a cert-manager ClusterIssuer or Issuer.
	// +optional
	IssuerRef *corev1.TypedLocalObjectReference `json:"issuerRef,omitempty"`

	// SecretName specifies the target K8s Secret to store the TLS cert/key.
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// SiteDomainSpec defines the desired state of SiteDomain
type SiteDomainSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// Domain specifies the custom domain name (e.g. "erp.acmecorp.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=3
	Domain string `json:"domain"`

	// TLS defines cert-manager SSL/TLS certificate configuration.
	// +optional
	TLS *SiteDomainTLSSpec `json:"tls,omitempty"`

	// RedirectPrimary determines whether to 301-redirect old site traffic to this custom domain.
	// +optional
	RedirectPrimary bool `json:"redirectPrimary,omitempty"`
}

// SiteDomainStatus defines the observed state of SiteDomain
type SiteDomainStatus struct {
	// Phase represents the current state (Pending, DNSUnreachable, Provisioning, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// DNSConfigured indicates whether public DNS lookup resolves to the cluster ingress.
	DNSConfigured bool `json:"dnsConfigured,omitempty"`

	// TLSCertificateIssued indicates whether cert-manager issued a valid SSL cert.
	TLSCertificateIssued bool `json:"tlsCertificateIssued,omitempty"`

	// IngressName stores the name of the generated Kubernetes Ingress.
	IngressName string `json:"ingressName,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteDomain's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Domain",type="string",JSONPath=".spec.domain"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteDomain is the Schema for the sitedomains API
type SiteDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteDomainSpec   `json:"spec,omitempty"`
	Status SiteDomainStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteDomainList contains a list of SiteDomain
type SiteDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteDomain{}, &SiteDomainList{})
}
