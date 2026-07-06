package v1

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

	// ComponentAutoscaling defines scaling configuration for all components
	// Map keys are component names: nginx, gunicorn, socketio, scheduler, worker-default, worker-long, worker-short
	// +optional
	ComponentAutoscaling map[string]*ComponentAutoscaling `json:"componentAutoscaling,omitempty"`

	// ComponentResources defines resource requirements for each component
	// +optional
	ComponentResources *ComponentResources `json:"componentResources,omitempty"`

	// RedisConfig defines Redis/Dragonfly configuration
	// +optional
	RedisConfig *RedisConfig `json:"redisConfig,omitempty"`

	// StorageClassName allows overriding the storage class for bench PVC
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// StorageSize for the bench PVC (e.g., "10Gi")
	// +optional
	// +kubebuilder:default="10Gi"
	StorageSize string `json:"storageSize,omitempty"`

	// DBConfig defines default database configuration for all sites in this bench
	// +optional
	DBConfig *DatabaseConfig `json:"dbConfig,omitempty"`

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

	// Security defines security context settings for all pods in this bench
	// +optional
	Security *SecurityConfig `json:"security,omitempty"`

	// SiteReconcileConcurrency suggests max concurrent site reconciles for sites on this bench.
	// Operator uses max(operatorConfig.maxConcurrentSiteReconciles, max across all benches).
	// Only applied at operator startup; change requires operator restart.
	// +optional
	SiteReconcileConcurrency *int32 `json:"siteReconcileConcurrency,omitempty"`

	// PodConfig defines advanced pod configuration for all bench components
	// +optional
	PodConfig *PodConfig `json:"podConfig,omitempty"`

	// Observability defines APM integration configuration (Prometheus/OpenTelemetry)
	// +optional
	Observability *ObservabilityConfig `json:"observability,omitempty"`
}

// ObservabilityConfig defines configuration for OpenTelemetry / Prometheus APM integration
type ObservabilityConfig struct {
	// EnableTelemetry enables the injection of the Prometheus/OTel sidecar container
	// +optional
	EnableTelemetry bool `json:"enableTelemetry,omitempty"`

	// OtelExporterEndpoint specifies the destination endpoint for OpenTelemetry traces/metrics
	// +optional
	OtelExporterEndpoint string `json:"otelExporterEndpoint,omitempty"`
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

	// ComponentScaling reports scaling status for all components
	// Map keys match component names from spec.componentAutoscaling
	// +optional
	ComponentScaling map[string]*ComponentScalingStatus `json:"componentScaling,omitempty"`

	// InitializedImage records the image tag that was used during the last successful initialization
	// +optional
	InitializedImage string `json:"initializedImage,omitempty"`
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
