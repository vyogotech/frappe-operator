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

// SiteQuotaSpec defines the desired state of SiteQuota
type SiteQuotaSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// MaxDBSizeMB specifies maximum database size limit in megabytes.
	// +optional
	MaxDBSizeMB *int64 `json:"maxDbSizeMb,omitempty"`

	// MaxStorageMB specifies maximum file attachment storage limit in megabytes.
	// +optional
	MaxStorageMB *int64 `json:"maxStorageMb,omitempty"`

	// MaxUsers specifies maximum active users limit.
	// +optional
	MaxUsers *int32 `json:"maxUsers,omitempty"`

	// ActionOnExceeded specifies action when quota is exceeded: "Warn", "ReadOnly", or "Maintenance".
	// +optional
	// +kubebuilder:default="Warn"
	ActionOnExceeded string `json:"actionOnExceeded,omitempty"`
}

// SiteQuotaStatus defines the observed state of SiteQuota
type SiteQuotaStatus struct {
	// Phase represents the current state (Normal, Warning, QuotaExceeded, Failed).
	Phase string `json:"phase,omitempty"`

	// CurrentDBSizeMB is the observed DB size in megabytes.
	CurrentDBSizeMB int64 `json:"currentDbSizeMb,omitempty"`

	// CurrentStorageMB is the observed storage usage in megabytes.
	CurrentStorageMB int64 `json:"currentStorageMb,omitempty"`

	// CurrentUsers is the observed count of active users.
	CurrentUsers int32 `json:"currentUsers,omitempty"`

	// QuotaExceeded indicates whether any quota limit is currently breached.
	QuotaExceeded bool `json:"quotaExceeded,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteQuota's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Exceeded",type="boolean",JSONPath=".status.quotaExceeded"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteQuota is the Schema for the sitequotas API
type SiteQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteQuotaSpec   `json:"spec,omitempty"`
	Status SiteQuotaStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteQuotaList contains a list of SiteQuota
type SiteQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteQuota{}, &SiteQuotaList{})
}
