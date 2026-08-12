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

// SiteCustomFieldSpec defines the desired state of SiteCustomField
type SiteCustomFieldSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// DT specifies the target DocType to extend (e.g. "Customer", "Sales Order").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DT string `json:"dt"`

	// FieldName specifies the name of the custom field (e.g. "custom_vat_number").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	FieldName string `json:"fieldname"`

	// Label specifies the human-readable label shown in Desk UI.
	// +kubebuilder:validation:Required
	Label string `json:"label"`

	// FieldType specifies the Frappe field type (Data, Select, Link, Int, Currency, Check, Text, etc.).
	// +kubebuilder:validation:Required
	FieldType string `json:"fieldtype"`

	// Options specifies field options (e.g. Link target DocType or select options list).
	// +optional
	Options string `json:"options,omitempty"`

	// InsertAfter specifies the field name after which this custom field will be placed.
	// +optional
	InsertAfter string `json:"insertAfter,omitempty"`

	// Reqd specifies whether the field is mandatory (1 or 0).
	// +optional
	Reqd int `json:"reqd,omitempty"`

	// ReadOnly specifies whether the field is read-only (1 or 0).
	// +optional
	ReadOnly int `json:"readOnly,omitempty"`

	// Hidden specifies whether the field is hidden (1 or 0).
	// +optional
	Hidden int `json:"hidden,omitempty"`

	// Default specifies the default value for the field.
	// +optional
	Default string `json:"default,omitempty"`
}

// SiteCustomFieldStatus defines the observed state of SiteCustomField
type SiteCustomFieldStatus struct {
	// Phase represents the current state of the SiteCustomField (Pending, Ready, Failed).
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
//+kubebuilder:printcolumn:name="Field",type="string",JSONPath=".spec.fieldname"
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.fieldtype"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteCustomField is the Schema for the sitecustomfields API
type SiteCustomField struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteCustomFieldSpec   `json:"spec,omitempty"`
	Status SiteCustomFieldStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteCustomFieldList contains a list of SiteCustomField
type SiteCustomFieldList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteCustomField `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteCustomField{}, &SiteCustomFieldList{})
}
