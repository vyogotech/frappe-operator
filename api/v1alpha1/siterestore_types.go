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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupSource defines where to retrieve a backup file from
type BackupSource struct {
	// LocalPath is the path relative to the bench root (e.g., "sites/site1.local/private/backups/xyz.sql")
	// +optional
	LocalPath string `json:"localPath,omitempty"`

	// S3 specifies a file in S3-compatible storage
	// +optional
	S3 *S3DownloadConfig `json:"s3,omitempty"`
}

// S3DownloadConfig defines how to download a specific file from S3
type S3DownloadConfig struct {
	// S3 connection details
	S3Config `json:",inline"`

	// Key is the path/name of the file in the bucket
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// SiteRestoreSpec defines the desired state of SiteRestore
type SiteRestoreSpec struct {
	// Site is the name of the Frappe site to restore
	// +kubebuilder:validation:Required
	Site string `json:"site"`

	// BenchRef references the bench where the site should be restored
	// +kubebuilder:validation:Required
	BenchRef NamespacedName `json:"benchRef"`

	// DatabaseBackupSource specifies where to get the SQL backup from
	// +kubebuilder:validation:Required
	DatabaseBackupSource BackupSource `json:"databaseBackupSource"`

	// PublicFilesSource specifies where to get the public files backup from
	// +optional
	PublicFilesSource *BackupSource `json:"publicFilesSource,omitempty"`

	// PrivateFilesSource specifies where to get the private files backup from
	// +optional
	PrivateFilesSource *BackupSource `json:"privateFilesSource,omitempty"`

	// AdminPasswordSecretRef references a secret key containing the new admin password
	// +optional
	AdminPasswordSecretRef *corev1.SecretKeySelector `json:"adminPasswordSecretRef,omitempty"`

	// Force bypasses the warning if a site downgrade is detected
	// +optional
	// +kubebuilder:default=false
	Force bool `json:"force,omitempty"`
}

// SiteRestoreStatus defines the observed state of SiteRestore
type SiteRestoreStatus struct {
	// Phase indicates the current phase of the restore
	// +optional
	Phase string `json:"phase,omitempty"`

	// CompletionTime is the timestamp when the restore finished
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// RestoreJob is the name of the restore job
	// +optional
	RestoreJob string `json:"restoreJob,omitempty"`

	// Message provides additional information about the restore status
	// +optional
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Site",type=string,JSONPath=`.spec.site`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteRestore is the Schema for the siterestores API
type SiteRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteRestoreSpec   `json:"spec,omitempty"`
	Status SiteRestoreStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteRestoreList contains a list of SiteRestore
type SiteRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteRestore{}, &SiteRestoreList{})
}
