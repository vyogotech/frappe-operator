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

// SiteRoleSpec defines the desired state of SiteRole
type SiteRoleSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// RoleName specifies the name of the Frappe Role (e.g. "API Integrator").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RoleName string `json:"roleName"`

	// DeskAccess determines whether users with this role can log into Frappe Desk. Defaults to true.
	// +optional
	// +kubebuilder:default=true
	DeskAccess bool `json:"deskAccess,omitempty"`

	// Disabled determines whether the role is disabled in Frappe. Defaults to false.
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

// SiteRoleStatus defines the observed state of SiteRole
type SiteRoleStatus struct {
	// Phase represents the current state of the SiteRole (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteRole's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Role",type="string",JSONPath=".spec.roleName"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteRole is the Schema for the siteroles API
type SiteRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteRoleSpec   `json:"spec,omitempty"`
	Status SiteRoleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteRoleList contains a list of SiteRole
type SiteRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteRole{}, &SiteRoleList{})
}
