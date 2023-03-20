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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SiteDashboardSpec defines the desired state of SiteDashboard
type SiteDashboardSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of SiteDashboard. Edit sitedashboard_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// SiteDashboardStatus defines the observed state of SiteDashboard
type SiteDashboardStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SiteDashboard is the Schema for the sitedashboards API
type SiteDashboard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteDashboardSpec   `json:"spec,omitempty"`
	Status SiteDashboardStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteDashboardList contains a list of SiteDashboard
type SiteDashboardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteDashboard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteDashboard{}, &SiteDashboardList{})
}
