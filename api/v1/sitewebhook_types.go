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

// SiteWebhookSpec defines the desired state of SiteWebhook
type SiteWebhookSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// WebhookDocType specifies the DocType that triggers this webhook (e.g. "Sales Order", "Customer").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	WebhookDocType string `json:"webhookDoctype"`

	// WebhookEvent specifies the document event ("on_change", "on_update", "on_submit", "on_cancel", "on_trash").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="on_change";"on_update";"on_submit";"on_cancel";"on_trash"
	WebhookEvent string `json:"webhookEvent"`

	// RequestURL specifies the target endpoint URL to post webhook payloads.
	// +kubebuilder:validation:Required
	RequestURL string `json:"requestUrl"`

	// RequestStructure specifies the HTTP request content type ("JSON", "Form URL-Encoded"). Defaults to "JSON".
	// +optional
	// +kubebuilder:default="JSON"
	RequestStructure string `json:"requestStructure,omitempty"`

	// Enabled specifies whether the webhook is enabled. Defaults to true.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
}

// SiteWebhookStatus defines the observed state of SiteWebhook
type SiteWebhookStatus struct {
	// Phase represents the current state of the SiteWebhook (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the resource state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="DocType",type="string",JSONPath=".spec.webhookDoctype"
//+kubebuilder:printcolumn:name="Event",type="string",JSONPath=".spec.webhookEvent"
//+kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.requestUrl"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteWebhook is the Schema for the sitewebhooks API
type SiteWebhook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteWebhookSpec   `json:"spec,omitempty"`
	Status SiteWebhookStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteWebhookList contains a list of SiteWebhook
type SiteWebhookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteWebhook `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteWebhook{}, &SiteWebhookList{})
}
