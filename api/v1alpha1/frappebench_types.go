package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FrappeBenchSpec defines the desired state of FrappeBench
type FrappeBenchSpec struct {
	// FrappeVersion specifies the Frappe framework version
	// +kubebuilder:validation:Required
	FrappeVersion string `json:"frappeVersion"`

	// Apps to install with their sources
	// Supports FPM packages, Git repositories, and pre-built images
	// +optional
	Apps []AppSource `json:"apps,omitempty"`

	// AppsJSON is deprecated, use Apps instead
	// JSON array of app names (e.g., '["erpnext", "hrms"]')
	// +optional
	AppsJSON string `json:"appsJSON,omitempty"`

	// ImageConfig defines the container image configuration
	// +optional
	ImageConfig *ImageConfig `json:"imageConfig,omitempty"`

	// ComponentReplicas defines replica counts for each component
	// +optional
	ComponentReplicas *ComponentReplicas `json:"componentReplicas,omitempty"`

	// ComponentResources defines resource requirements for each component
	// +optional
	ComponentResources *ComponentResources `json:"componentResources,omitempty"`

	// RedisConfig defines Redis/Dragonfly configuration
	// +optional
	RedisConfig *RedisConfig `json:"redisConfig,omitempty"`

	// StorageClassName allows overriding the storage class for bench PVC
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// DomainConfig defines default domain behavior for sites on this bench
	// +optional
	DomainConfig *DomainConfig `json:"domainConfig,omitempty"`

	// FPMConfig for FPM repository configuration
	// Merged with operator-level FPM configuration
	// +optional
	FPMConfig *FPMConfig `json:"fpmConfig,omitempty"`

	// GitConfig controls Git-based app installation
	// Overrides operator-level Git configuration
	// +optional
	GitConfig *GitConfig `json:"gitConfig,omitempty"`
}

// FrappeBenchStatus defines the observed state of FrappeBench
type FrappeBenchStatus struct {
	// Phase represents the current phase of the bench
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the bench's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// InstalledApps lists the apps that have been successfully installed
	// +optional
	InstalledApps []string `json:"installedApps,omitempty"`

	// GitEnabled indicates whether Git is enabled for this bench
	// +optional
	GitEnabled bool `json:"gitEnabled,omitempty"`

	// FPMRepositories lists the configured FPM repositories
	// +optional
	FPMRepositories []string `json:"fpmRepositories,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed FrappeBench
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Apps",type=string,JSONPath=`.status.installedApps`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FrappeBench is the Schema for the frappebenches API
type FrappeBench struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FrappeBenchSpec   `json:"spec,omitempty"`
	Status FrappeBenchStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FrappeBenchList contains a list of FrappeBench
type FrappeBenchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FrappeBench `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FrappeBench{}, &FrappeBenchList{})
}
