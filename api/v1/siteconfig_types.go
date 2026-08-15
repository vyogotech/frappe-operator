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

// SiteConfigSpec defines the desired state of SiteConfig
type SiteConfigSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// MaintenanceMode determines whether maintenance mode is enabled for the site.
	// +optional
	MaintenanceMode *bool `json:"maintenanceMode,omitempty"`

	// MaxFileSize specifies the maximum upload file size in bytes (max_file_size in site_config.json).
	// +optional
	MaxFileSize *int64 `json:"maxFileSize,omitempty"`

	// EncryptionKeySecretRef specifies a K8s secret holding an encryption_key to inject into site_config.json.
	// +optional
	EncryptionKeySecretRef *corev1.LocalObjectReference `json:"encryptionKeySecretRef,omitempty"`

	// CustomConfig defines arbitrary key-value pairs to set in site_config.json.
	// Values that parse as JSON (objects, arrays, numbers, booleans) are applied as such;
	// everything else is applied as a string.
	// +optional
	CustomConfig map[string]string `json:"customConfig,omitempty"`

	// ObjectStorage, when set, configures the site to offload File attachments to an
	// S3-compatible object store via the cloud_storage app (baked into the bench image).
	// The operator assembles the cloud_storage_settings block in site_config.json, sourcing
	// the access key/secret from CredentialsSecretRef so they never appear in the CR.
	// +optional
	ObjectStorage *ObjectStorageConfig `json:"objectStorage,omitempty"`
}

// ObjectStorageConfig configures S3-compatible File offload via the cloud_storage app.
type ObjectStorageConfig struct {
	// Bucket is the S3 bucket name.
	// +kubebuilder:validation:Required
	Bucket string `json:"bucket"`

	// EndpointURL is the S3 endpoint (e.g. "https://s3.amazonaws.com", "http://minio:9000").
	// +kubebuilder:validation:Required
	EndpointURL string `json:"endpointUrl"`

	// Region is the S3 region (e.g. "us-east-1").
	// +optional
	Region string `json:"region,omitempty"`

	// Folder is the key prefix within the bucket. Defaults to the site name, giving each
	// site its own prefix when a bucket is shared across tenants.
	// +optional
	Folder string `json:"folder,omitempty"`

	// PresignedURLExpirySeconds is the TTL for presigned download URLs (cloud_storage
	// "expiration"). Defaults to 120.
	// +optional
	PresignedURLExpirySeconds *int64 `json:"presignedUrlExpirySeconds,omitempty"`

	// CredentialsSecretRef references a Secret holding the S3 credentials.
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// AccessKeyKey is the Secret key holding the access key. Defaults to "access_key".
	// +optional
	AccessKeyKey string `json:"accessKeyKey,omitempty"`

	// SecretKeyKey is the Secret key holding the secret key. Defaults to "secret".
	// +optional
	SecretKeyKey string `json:"secretKeyKey,omitempty"`
}

// SiteConfigStatus defines the observed state of SiteConfig
type SiteConfigStatus struct {
	// Phase represents the current state of the SiteConfig (Pending, Ready, Failed).
	Phase string `json:"phase,omitempty"`

	// AppliedKeys lists the configuration keys successfully merged into site_config.json.
	AppliedKeys []string `json:"appliedKeys,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteConfig's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteConfig is the Schema for the siteconfigs API
type SiteConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteConfigSpec   `json:"spec,omitempty"`
	Status SiteConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteConfigList contains a list of SiteConfig
type SiteConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteConfig{}, &SiteConfigList{})
}
