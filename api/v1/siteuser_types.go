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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SiteUserSpec defines the desired state of SiteUser
type SiteUserSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// Email specifies the user's primary email address (acts as Frappe User name).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=3
	Email string `json:"email"`

	// FirstName specifies the user's first name.
	// +kubebuilder:validation:Required
	FirstName string `json:"firstName"`

	// LastName specifies the user's last name.
	// +optional
	LastName string `json:"lastName,omitempty"`

	// UserType specifies "System User" or "Website User". Defaults to "System User".
	// +optional
	// +kubebuilder:default="System User"
	UserType string `json:"userType,omitempty"`

	// SendPasswordReset determines whether to trigger a password reset email upon creation.
	// +optional
	SendPasswordReset bool `json:"sendPasswordReset,omitempty"`

	// Roles lists the Frappe roles to assign to this user.
	// +optional
	Roles []string `json:"roles,omitempty"`

	// APIKeySecretRef specifies an optional Secret to store the auto-generated API Key and API Secret.
	// +optional
	APIKeySecretRef *corev1.LocalObjectReference `json:"apiKeySecretRef,omitempty"`
}

// SiteUserStatus defines the observed state of SiteUser
type SiteUserStatus struct {
	// Phase represents the current state of the SiteUser (Pending, Provisioning, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// AssignedRoles lists the roles currently assigned in Frappe.
	AssignedRoles []string `json:"assignedRoles,omitempty"`

	// APIKeysGenerated indicates whether Frappe API keys were generated and exported to the Secret.
	APIKeysGenerated bool `json:"apiKeysGenerated,omitempty"`

	// SecretName stores the name of the Secret holding the API key/secret.
	SecretName string `json:"secretName,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteUser's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Email",type="string",JSONPath=".spec.email"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteUser is the Schema for the siteusers API
type SiteUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteUserSpec   `json:"spec,omitempty"`
	Status SiteUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteUserList contains a list of SiteUser
type SiteUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteUser{}, &SiteUserList{})
}
