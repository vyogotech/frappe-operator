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

// SiteUserPermissionSpec defines the desired state of SiteUserPermission
type SiteUserPermissionSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// User specifies the target Frappe user email (e.g. "john.doe@example.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// Allow specifies the DocType to restrict (e.g. "Company", "Warehouse", "Territory").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Allow string `json:"allow"`

	// ForValue specifies the target document ID/value to allow (e.g. "Acme Corp", "Main Warehouse").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ForValue string `json:"forValue"`

	// ApplyToAllDocTypes specifies whether this restriction applies across all linked DocTypes. Defaults to true.
	// +optional
	// +kubebuilder:default=true
	ApplyToAllDocTypes bool `json:"applyToAllDoctypes,omitempty"`
}

// SiteUserPermissionStatus defines the observed state of SiteUserPermission
type SiteUserPermissionStatus struct {
	// Phase represents the current state of the SiteUserPermission (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the resource state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="User",type="string",JSONPath=".spec.user"
//+kubebuilder:printcolumn:name="Allow",type="string",JSONPath=".spec.allow"
//+kubebuilder:printcolumn:name="For Value",type="string",JSONPath=".spec.forValue"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteUserPermission is the Schema for the siteuserpermissions API
type SiteUserPermission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteUserPermissionSpec   `json:"spec,omitempty"`
	Status SiteUserPermissionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteUserPermissionList contains a list of SiteUserPermission
type SiteUserPermissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteUserPermission `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteUserPermission{}, &SiteUserPermissionList{})
}
