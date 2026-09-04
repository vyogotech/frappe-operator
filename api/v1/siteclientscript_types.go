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

// SiteClientScriptSpec defines the desired state of SiteClientScript
type SiteClientScriptSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// DT specifies the target DocType for the JavaScript client script.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DT string `json:"dt"`

	// Script specifies the JavaScript client script.
	// +kubebuilder:validation:Required
	Script string `json:"script"`

	// Enabled specifies whether the client script is enabled. Defaults to true.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
}

// SiteClientScriptStatus defines the observed state of SiteClientScript
type SiteClientScriptStatus struct {
	// Phase represents the current state of the SiteClientScript (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the resource state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="DocType",type="string",JSONPath=".spec.dt"
//+kubebuilder:printcolumn:name="Enabled",type="boolean",JSONPath=".spec.enabled"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteClientScript is the Schema for the siteclientscripts API
type SiteClientScript struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteClientScriptSpec   `json:"spec,omitempty"`
	Status SiteClientScriptStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteClientScriptList contains a list of SiteClientScript
type SiteClientScriptList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteClientScript `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteClientScript{}, &SiteClientScriptList{})
}
