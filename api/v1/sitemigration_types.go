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

// SiteMigrationSpec defines the desired state of SiteMigration
type SiteMigrationSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// SkipFixtures determines whether to skip fixture synchronization during migration.
	// +optional
	SkipFixtures bool `json:"skipFixtures,omitempty"`

	// Force determines whether to force migration even if schema is unchanged.
	// +optional
	Force bool `json:"force,omitempty"`
}

// SiteMigrationStatus defines the observed state of SiteMigration
type SiteMigrationStatus struct {
	// Phase represents the current state (Pending, Migrating, Succeeded, Failed).
	Phase string `json:"phase,omitempty"`

	// JobName stores the name of the Kubernetes Job executing bench migrate.
	JobName string `json:"jobName,omitempty"`

	// StartTime indicates when the migration started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime indicates when the migration completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteMigration's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Job",type="string",JSONPath=".status.jobName"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteMigration is the Schema for the sitemigrations API
type SiteMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteMigrationSpec   `json:"spec,omitempty"`
	Status SiteMigrationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteMigrationList contains a list of SiteMigration
type SiteMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteMigration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteMigration{}, &SiteMigrationList{})
}
