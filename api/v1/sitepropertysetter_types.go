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

// SitePropertySetterSpec defines the desired state of SitePropertySetter
type SitePropertySetterSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// DocType specifies the target DocType (e.g. "Customer", "Supplier").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DocType string `json:"docType"`

	// FieldName specifies the field name to override (leave empty if setting DocType-level property).
	// +optional
	FieldName string `json:"fieldName,omitempty"`

	// Property specifies the property to set (e.g. "reqd", "read_only", "hidden", "label", "default").
	// +kubebuilder:validation:Required
	Property string `json:"property"`

	// PropertyType specifies the data type of the property ("Check", "Data", "Select", "Int", etc.). Defaults to "Check".
	// +optional
	// +kubebuilder:default="Check"
	PropertyType string `json:"propertyType,omitempty"`

	// Value specifies the new property value (e.g. "1", "0", or text string).
	// +kubebuilder:validation:Required
	Value string `json:"value"`
}

// SitePropertySetterStatus defines the observed state of SitePropertySetter
type SitePropertySetterStatus struct {
	// Phase represents the current state of the SitePropertySetter (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the resource state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="DocType",type="string",JSONPath=".spec.docType"
//+kubebuilder:printcolumn:name="Field",type="string",JSONPath=".spec.fieldName"
//+kubebuilder:printcolumn:name="Property",type="string",JSONPath=".spec.property"
//+kubebuilder:printcolumn:name="Value",type="string",JSONPath=".spec.value"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SitePropertySetter is the Schema for the sitepropertysetters API
type SitePropertySetter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SitePropertySetterSpec   `json:"spec,omitempty"`
	Status SitePropertySetterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SitePropertySetterList contains a list of SitePropertySetter
type SitePropertySetterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SitePropertySetter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SitePropertySetter{}, &SitePropertySetterList{})
}
