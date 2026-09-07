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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SiteAPIKeySpec defines the desired state of SiteAPIKey
type SiteAPIKeySpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// User specifies the Frappe user email for whom the API key is generated (e.g. "Administrator" or "integration@acme.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// SecretName specifies the target K8s Secret where api_key and api_secret will be stored.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`
}

// SiteAPIKeyStatus defines the observed state of SiteAPIKey
type SiteAPIKeyStatus struct {
	// Phase represents the current state (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// APIKeyGenerated indicates whether the API key was generated.
	APIKeyGenerated bool `json:"apiKeyGenerated,omitempty"`

	// SecretCreated indicates whether the target K8s Secret was created.
	SecretCreated bool `json:"secretCreated,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteAPIKey's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="User",type="string",JSONPath=".spec.user"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteAPIKey is the Schema for the siteapikeys API
type SiteAPIKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteAPIKeySpec   `json:"spec,omitempty"`
	Status SiteAPIKeyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteAPIKeyList contains a list of SiteAPIKey
type SiteAPIKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteAPIKey `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteAPIKey{}, &SiteAPIKeyList{})
}
