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

// SiteServerScriptSpec defines the desired state of SiteServerScript
type SiteServerScriptSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// ScriptType specifies the Server Script type ("DocType Event", "API", "Permission Query").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="DocType Event";"API";"Permission Query"
	ScriptType string `json:"scriptType"`

	// ReferenceDocType specifies the DocType for "DocType Event" or "Permission Query" scripts.
	// +optional
	ReferenceDocType string `json:"referenceDocType,omitempty"`

	// EventType specifies the event hook ("Before Insert", "Before Save", "After Save", "Before Submit", "After Submit", "Before Cancel", "After Cancel", "Before Delete").
	// +optional
	EventType string `json:"eventType,omitempty"`

	// APIMethod specifies the route path for "API" type server scripts (e.g. "my_custom_endpoint").
	// +optional
	APIMethod string `json:"apiMethod,omitempty"`

	// Script specifies the Python script code.
	// +kubebuilder:validation:Required
	Script string `json:"script"`

	// Disabled specifies whether the script is disabled. Defaults to false.
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

// SiteServerScriptStatus defines the observed state of SiteServerScript
type SiteServerScriptStatus struct {
	// Phase represents the current state of the SiteServerScript (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the resource state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.scriptType"
//+kubebuilder:printcolumn:name="DocType",type="string",JSONPath=".spec.referenceDocType"
//+kubebuilder:printcolumn:name="Event",type="string",JSONPath=".spec.eventType"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteServerScript is the Schema for the siteserverscripts API
type SiteServerScript struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteServerScriptSpec   `json:"spec,omitempty"`
	Status SiteServerScriptStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteServerScriptList contains a list of SiteServerScript
type SiteServerScriptList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteServerScript `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteServerScript{}, &SiteServerScriptList{})
}
