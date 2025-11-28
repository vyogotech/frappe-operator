package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ResourceRequirements defines compute resource requirements
type ResourceRequirements struct {
	// Requests describes the minimum amount of compute resources required
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`
	// Limits describes the maximum amount of compute resources allowed
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`
}

// DatabaseConfig defines database configuration for a Frappe site
type DatabaseConfig struct {
	// Provider: mariadb, postgres, sqlite
	// +kubebuilder:validation:Enum=mariadb;postgres;sqlite
	// +kubebuilder:default=mariadb
	// +optional
	Provider string `json:"provider,omitempty"`

	// Mode: shared (one DB instance, multiple site databases) or dedicated (one DB instance per site)
	// +kubebuilder:validation:Enum=shared;dedicated
	// +kubebuilder:default=shared
	// +optional
	Mode string `json:"mode,omitempty"`

	// MariaDBRef references an existing MariaDB CR (for shared/dedicated modes)
	// If not specified in shared mode, operator uses/creates a default MariaDB instance
	// If not specified in dedicated mode, operator creates a per-site MariaDB instance
	// +optional
	MariaDBRef *NamespacedName `json:"mariadbRef,omitempty"`

	// PostgresRef references an existing PostgreSQL cluster (future)
	// +optional
	PostgresRef *NamespacedName `json:"postgresRef,omitempty"`

	// StorageSize for dedicated database mode
	// +optional
	StorageSize *resource.Quantity `json:"storageSize,omitempty"`

	// Resources for dedicated database mode
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// Host is the database hostname (legacy, for backward compatibility)
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the database port (legacy, for backward compatibility)
	// +optional
	Port string `json:"port,omitempty"`

	// ConnectionSecretRef references a Secret containing database credentials (legacy)
	// +optional
	ConnectionSecretRef *corev1.SecretReference `json:"connectionSecretRef,omitempty"`
}

// IngressConfig defines Ingress configuration
type IngressConfig struct {
	// Enabled controls whether Ingress is created
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ClassName specifies the Ingress class
	// +optional
	ClassName string `json:"className,omitempty"`

	// Annotations for the Ingress resource
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// TLS configuration
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

// TLSConfig defines TLS/SSL configuration
type TLSConfig struct {
	// Enabled controls whether TLS is enabled
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// SecretName containing TLS certificate
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// Issuer for cert-manager integration
	// +optional
	Issuer string `json:"issuer,omitempty"`
}

// DomainConfig defines domain resolution behavior
type DomainConfig struct {
	// Suffix to append to site names (e.g., ".myplatform.com")
	// +optional
	Suffix string `json:"suffix,omitempty"`

	// AutoDetect enables automatic domain detection from cluster
	// +optional
	// +kubebuilder:default=true
	AutoDetect *bool `json:"autoDetect,omitempty"`

	// IngressControllerRef references the Ingress Controller service
	// +optional
	IngressControllerRef *NamespacedName `json:"ingressControllerRef,omitempty"`
}

// NamespacedName represents a namespaced resource reference
type NamespacedName struct {
	// Name of the resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the resource
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ImageConfig defines container image configuration
type ImageConfig struct {
	// Repository is the base image repository
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the image tag
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy is the image pull policy
	// +optional
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// PullSecrets for private registries
	// +optional
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// ComponentReplicas defines replica counts for bench components
type ComponentReplicas struct {
	// Gunicorn replicas
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Gunicorn int32 `json:"gunicorn,omitempty"`

	// Nginx replicas
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Nginx int32 `json:"nginx,omitempty"`

	// Socketio replicas
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Socketio int32 `json:"socketio,omitempty"`

	// WorkerDefault replicas (DEPRECATED: use WorkerAutoscaling instead)
	// Kept for backward compatibility
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	WorkerDefault int32 `json:"workerDefault,omitempty"`

	// WorkerLong replicas (DEPRECATED: use WorkerAutoscaling instead)
	// Kept for backward compatibility
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	WorkerLong int32 `json:"workerLong,omitempty"`

	// WorkerShort replicas (DEPRECATED: use WorkerAutoscaling instead)
	// Kept for backward compatibility
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	WorkerShort int32 `json:"workerShort,omitempty"`
}

// ComponentResources defines resource requirements for bench components
type ComponentResources struct {
	// Gunicorn resources
	// +optional
	Gunicorn *ResourceRequirements `json:"gunicorn,omitempty"`

	// Nginx resources
	// +optional
	Nginx *ResourceRequirements `json:"nginx,omitempty"`

	// Scheduler resources
	// +optional
	Scheduler *ResourceRequirements `json:"scheduler,omitempty"`

	// Socketio resources
	// +optional
	Socketio *ResourceRequirements `json:"socketio,omitempty"`

	// WorkerDefault resources
	// +optional
	WorkerDefault *ResourceRequirements `json:"workerDefault,omitempty"`

	// WorkerLong resources
	// +optional
	WorkerLong *ResourceRequirements `json:"workerLong,omitempty"`

	// WorkerShort resources
	// +optional
	WorkerShort *ResourceRequirements `json:"workerShort,omitempty"`
}

// RedisConfig defines Redis/Dragonfly configuration
type RedisConfig struct {
	// Type: redis or dragonfly
	// +kubebuilder:validation:Enum=redis;dragonfly
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// Image is the Redis/Dragonfly container image
	// +optional
	Image string `json:"image,omitempty"`

	// MaxMemory sets maximum memory for cache eviction
	// +optional
	MaxMemory *resource.Quantity `json:"maxMemory,omitempty"`

	// Resources for Redis/Dragonfly
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// StorageSize for persistent storage
	// +optional
	StorageSize *resource.Quantity `json:"storageSize,omitempty"`

	// ConnectionSecretRef for external Redis
	// +optional
	ConnectionSecretRef *corev1.SecretReference `json:"connectionSecretRef,omitempty"`
}

// AppSource defines where an app comes from and how to install it
type AppSource struct {
	// Name of the app (e.g., "erpnext", "hrms")
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Source type: fpm, git, or image
	// fpm: Install from FPM package repository
	// git: Install from Git repository (requires Git enabled)
	// image: App is pre-installed in container image
	// +kubebuilder:validation:Enum=fpm;git;image
	// +kubebuilder:validation:Required
	Source string `json:"source"`

	// Version for FPM packages (e.g., "1.0.0")
	// Required when source is "fpm"
	// +optional
	Version string `json:"version,omitempty"`

	// Org is the organization for FPM packages (e.g., "frappe")
	// Required when source is "fpm"
	// +optional
	Org string `json:"org,omitempty"`

	// GitURL for git source (e.g., "https://github.com/frappe/erpnext")
	// Required when source is "git"
	// +optional
	GitURL string `json:"gitUrl,omitempty"`

	// GitBranch for git source (e.g., "version-15")
	// Optional, defaults to repository default branch
	// +optional
	GitBranch string `json:"gitBranch,omitempty"`
}

// FPMConfig defines FPM (Frappe Package Manager) repository configuration
type FPMConfig struct {
	// Repositories to add to FPM configuration
	// These are added to any operator-level default repositories
	// +optional
	Repositories []FPMRepository `json:"repositories,omitempty"`

	// DefaultRepo for publishing packages (optional)
	// +optional
	DefaultRepo string `json:"defaultRepo,omitempty"`
}

// FPMRepository defines an FPM package repository
type FPMRepository struct {
	// Name of the repository (e.g., "company-private", "frappe-community")
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// URL of the repository (e.g., "https://fpm.company.com")
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Priority for repository search order (lower number = higher priority)
	// Default: 50
	// +optional
	// +kubebuilder:default=50
	Priority int `json:"priority,omitempty"`

	// AuthSecretRef references a secret with FPM authentication credentials
	// Secret should contain keys: username, password
	// +optional
	AuthSecretRef *corev1.SecretReference `json:"authSecretRef,omitempty"`
}

// GitConfig defines Git installation configuration
type GitConfig struct {
	// Enabled controls whether Git-based app installation is allowed
	// Set to false in enterprise environments without Git access
	// If not specified, uses operator-level default
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// WorkerAutoscaling defines scaling configuration for a worker type
// Supports both KEDA-based autoscaling and static replica counts
type WorkerAutoscaling struct {
	// Enabled controls whether KEDA autoscaling is active
	// If false or KEDA not installed, uses StaticReplicas
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// StaticReplicas for non-autoscaled workers
	// Used when Enabled=false OR KEDA not available
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	StaticReplicas *int32 `json:"staticReplicas,omitempty"`

	// MinReplicas for KEDA (can be 0 for true serverless)
	// Only used when Enabled=true AND KEDA available
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas for KEDA
	// Only used when Enabled=true AND KEDA available
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// QueueLength triggers scaling when queue depth exceeds this value
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	QueueLength *int32 `json:"queueLength,omitempty"`

	// CooldownPeriod in seconds before scaling down
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=60
	CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`

	// PollingInterval in seconds for checking queue depth
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=30
	PollingInterval *int32 `json:"pollingInterval,omitempty"`
}

// WorkerAutoscalingConfig defines scaling per worker type
type WorkerAutoscalingConfig struct {
	// Short worker scaling configuration
	// +optional
	Short *WorkerAutoscaling `json:"short,omitempty"`

	// Long worker scaling configuration
	// +optional
	Long *WorkerAutoscaling `json:"long,omitempty"`

	// Default worker scaling configuration
	// +optional
	Default *WorkerAutoscaling `json:"default,omitempty"`
}
