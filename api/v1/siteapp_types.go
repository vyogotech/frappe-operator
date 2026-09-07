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

// SiteAppSpec defines the desired state of SiteApp
type SiteAppSpec struct {
	// SiteRef specifies the target FrappeSite custom resource.
	// +kubebuilder:validation:Required
	SiteRef *NamespacedName `json:"siteRef"`

	// AppName specifies the name of the Frappe Python module/app (e.g. "erpnext_australian_localisation").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	AppName string `json:"appName"`

	// GitRepo specifies an optional Git repository URL to dynamically fetch the app source code if not already in the bench.
	// +optional
	GitRepo string `json:"gitRepo,omitempty"`

	// GitBranch specifies the Git branch or tag to clone (defaults to "main" or "master").
	// +optional
	GitBranch string `json:"gitBranch,omitempty"`

	// FPMPackage, when set, installs the app from a prebuilt FPM package
	// ("<org>/<app>==<version>", e.g. "frappe/wiki==3.0.0") instead of a runtime
	// git clone. The package ships compiled assets + vendored wheels, so no
	// yarn/pip build runs on the bench. This is the preferred path; GitRepo is the
	// fallback for apps that have no published package.
	// +optional
	FPMPackage string `json:"fpmPackage,omitempty"`

	// FPMRepo is the FPM registry the package is resolved from — either an HTTP
	// registry ("https://fpm.vyogo.tech") or an OCI registry
	// ("ghcr.io/vyogotech/fpm", where the built packages live). Required
	// alongside FPMPackage.
	// +optional
	FPMRepo string `json:"fpmRepo,omitempty"`

	// FPMRepoType is the registry backend: "oci" or "http" (default "http").
	// OCI registries (ghcr) hold the prod-built packages and are usually private,
	// so credentials come from the "fpm-registry-auth" Secret (keys: username,
	// token) in the site's namespace.
	// +optional
	// +kubebuilder:validation:Enum=http;oci
	FPMRepoType string `json:"fpmRepoType,omitempty"`

	// AutoMigrate determines whether to trigger database migration after app installation. Defaults to true.
	// +optional
	// +kubebuilder:default=true
	AutoMigrate bool `json:"autoMigrate,omitempty"`

	// BackupBeforeInstall determines whether the operator takes a full backup of
	// the site and waits for it to succeed before installing/upgrading the app.
	// This creates a rollback point in case the install corrupts the site.
	// Defaults to true; set false to skip (e.g. for throwaway sites).
	// +optional
	// +kubebuilder:default=true
	BackupBeforeInstall bool `json:"backupBeforeInstall,omitempty"`
}

// SiteAppStatus defines the observed state of SiteApp
type SiteAppStatus struct {
	// Phase represents the current state of the SiteApp (Pending, Installing, Ready, Failed, Uninstalling).
	Phase string `json:"phase,omitempty"`

	// InstalledVersion stores the version or commit of the installed app.
	InstalledVersion string `json:"installedVersion,omitempty"`

	// PreBackupRef is the name of the SiteBackup taken before this install as a
	// rollback point (set when backupBeforeInstall is enabled).
	// +optional
	PreBackupRef string `json:"preBackupRef,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the SiteApp's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="App",type="string",JSONPath=".spec.appName"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SiteApp is the Schema for the siteapps API
type SiteApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteAppSpec   `json:"spec,omitempty"`
	Status SiteAppStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SiteAppList contains a list of SiteApp
type SiteAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SiteApp{}, &SiteAppList{})
}
