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

// ShardingPolicy controls capacity thresholds and scaling boundaries
type ShardingPolicy struct {
	// MinShards is the minimum number of shards to keep active (e.g. 1 or 2)
	// +kubebuilder:default=1
	MinShards int32 `json:"minShards"`

	// MaxShards prevents runaway scaling from consuming cluster node RAM/storage
	// +kubebuilder:validation:Required
	MaxShards int32 `json:"maxShards"`

	// MaxSitesPerShard matches Cloud Agent Settings (default 25)
	// +kubebuilder:default=25
	MaxSitesPerShard int32 `json:"maxSitesPerShard"`

	// WatermarkPercent triggers next shard when the active shard reaches this percentage (e.g. 80 = 20/25 sites)
	// +kubebuilder:default=80
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=100
	WatermarkPercent int32 `json:"watermarkPercent"`

	// BufferWarmShards ensures at least N empty/warm Ready shards exist at all times (e.g. 1)
	// +kubebuilder:default=1
	BufferWarmShards int32 `json:"bufferWarmShards"`

	// ShardPrefix is the bench name prefix (e.g. "standard-v16"). Defaults to standard-v{major}.
	// +optional
	ShardPrefix string `json:"shardPrefix,omitempty"`
}

// FrappeBenchPoolSpec defines the desired state of FrappeBenchPool
type FrappeBenchPoolSpec struct {
	// FrappeVersion must match the framework major, e.g. "16" or "17"
	// +kubebuilder:validation:Required
	FrappeVersion string `json:"frappeVersion"`

	// ShardingPolicy controls capacity thresholds and scaling boundaries
	// +kubebuilder:validation:Required
	ShardingPolicy ShardingPolicy `json:"shardingPolicy"`

	// Template is the FrappeBenchSpec stamped onto each generated shard
	// +kubebuilder:validation:Required
	Template FrappeBenchSpec `json:"template"`
}

// BenchShardInfo records the status and occupancy of a single shard
type BenchShardInfo struct {
	Name       string `json:"name"`
	Phase      string `json:"phase,omitempty"`
	SiteCount  int32  `json:"siteCount"`
	Capacity   int32  `json:"capacity"`
	AtCapacity bool   `json:"atCapacity"`
}

// FrappeBenchPoolStatus defines the observed state of FrappeBenchPool
type FrappeBenchPoolStatus struct {
	// TotalShards is the total count of shards belonging to this pool
	TotalShards int32 `json:"totalShards"`

	// ReadyShards is the count of shards in Ready phase
	ReadyShards int32 `json:"readyShards"`

	// TotalActiveSites is the count of sites assigned to shards in this pool
	TotalActiveSites int32 `json:"totalActiveSites"`

	// Shards lists observed status for each shard
	// +optional
	Shards []BenchShardInfo `json:"shards,omitempty"`

	// Conditions represent observations of the pool's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastScaleTime records when the last shard was provisioned
	// +optional
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed pool
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.frappeVersion`
//+kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyShards`
//+kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalShards`
//+kubebuilder:printcolumn:name="Sites",type=integer,JSONPath=`.status.totalActiveSites`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FrappeBenchPool is the Schema for the frappebenchpools API
type FrappeBenchPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FrappeBenchPoolSpec   `json:"spec,omitempty"`
	Status FrappeBenchPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FrappeBenchPoolList contains a list of FrappeBenchPool
type FrappeBenchPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FrappeBenchPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FrappeBenchPool{}, &FrappeBenchPoolList{})
}
