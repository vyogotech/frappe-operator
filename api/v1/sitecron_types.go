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

// SiteCronSpec defines the desired state of SiteCron
type SiteCronSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// Schedule specifies the cron schedule string (e.g. "0 2 * * *").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=5
	Schedule string `json:"schedule"`

	// Method specifies the Frappe Python method to execute (e.g. "saas_platform.api.sync_nightly_sales").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=3
	Method string `json:"method"`

	// TimeoutSeconds specifies execution timeout in seconds. Defaults to 3600.
	// +optional
	// +kubebuilder:default=3600
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`

	// Suspended determines whether the cron schedule is currently paused.
	// +optional
	Suspended bool `json:"suspended,omitempty"`
}

// SiteCronStatus defines the observed state of SiteCron
type SiteCronStatus struct {
	// Phase represents the current state (Pending, Active, Suspended, Failed).
	Phase string `json:"phase,omitempty"`

	// CronJobName stores the generated Kubernetes CronJob name.
	CronJobName string `json:"cronJobName,omitempty"`

	// LastScheduleTime is the last time the cron job was scheduled.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteCron's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.schedule"
//+kubebuilder:printcolumn:name="Method",type="string",JSONPath=".spec.method"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteCron is the Schema for the sitecrons API
type SiteCron struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteCronSpec   `json:"spec,omitempty"`
	Status SiteCronStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteCronList contains a list of SiteCron
type SiteCronList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteCron `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteCron{}, &SiteCronList{})
}
