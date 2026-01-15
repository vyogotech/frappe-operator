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

// SiteBackupSpec defines the desired state of SiteBackup
type SiteBackupSpec struct {
	// Site is the name of the Frappe site to backup
	// +kubebuilder:validation:Required
	Site string `json:"site"`

	// Schedule is a cron expression for scheduled backups (e.g., "0 2 * * *")
	// If empty, performs a one-time backup
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// WithFiles includes private and public files in the backup
	// +optional
	// +kubebuilder:default=false
	WithFiles bool `json:"withFiles,omitempty"`

	// Compress compresses the backup files
	// +optional
	// +kubebuilder:default=false
	Compress bool `json:"compress,omitempty"`

	// BackupPath specifies the path to save the backup files
	// If empty, uses the default site backup location
	// +optional
	BackupPath string `json:"backupPath,omitempty"`

	// BackupPathDB specifies the path to save the database file
	// +optional
	BackupPathDB string `json:"backupPathDB,omitempty"`

	// BackupPathConf specifies the path to save the config file
	// +optional
	BackupPathConf string `json:"backupPathConf,omitempty"`

	// BackupPathFiles specifies the path to save the public file
	// +optional
	BackupPathFiles string `json:"backupPathFiles,omitempty"`

	// BackupPathPrivateFiles specifies the path to save the private file
	// +optional
	BackupPathPrivateFiles string `json:"backupPathPrivateFiles,omitempty"`

	// Exclude specifies the DocTypes to not backup, separated by commas
	// +optional
	Exclude []string `json:"exclude,omitempty"`

	// Include specifies the DocTypes to backup, separated by commas
	// +optional
	Include []string `json:"include,omitempty"`

	// IgnoreBackupConf ignores excludes/includes set in config
	// +optional
	// +kubebuilder:default=false
	IgnoreBackupConf bool `json:"ignoreBackupConf,omitempty"`

	// Verbose adds verbosity to the backup process
	// +optional
	// +kubebuilder:default=false
	Verbose bool `json:"verbose,omitempty"`
}

// SiteBackupStatus defines the observed state of SiteBackup
type SiteBackupStatus struct {
	// Phase indicates the current phase of the backup
	// +optional
	Phase string `json:"phase,omitempty"`

	// LastBackup is the timestamp of the last successful backup
	// +optional
	LastBackup metav1.Time `json:"lastBackup,omitempty"`

	// LastBackupJob is the name of the last backup job or cronjob
	// +optional
	LastBackupJob string `json:"lastBackupJob,omitempty"`

	// Message provides additional information about the backup status
	// +optional
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SiteBackup is the Schema for the sitebackups API
type SiteBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteBackupSpec   `json:"spec,omitempty"`
	Status SiteBackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteBackupList contains a list of SiteBackup
type SiteBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteBackup{}, &SiteBackupList{})
}
