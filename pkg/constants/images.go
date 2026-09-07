package constants

// Image constants for Frappe Operator
// All images use fully qualified names for production deployments
// Supports multiple registries for air-gapped and enterprise environments

import (
	corev1 "k8s.io/api/core/v1"
)

// Default Images - Docker Hub
const (
	// Core Frappe/ERPNext images
	DefaultFrappeImage = "docker.io/frappe/erpnext:latest"
	DefaultBenchImage  = "docker.io/frappe/erpnext:latest"

	// Database images
	DefaultMariaDBImage  = "docker.io/library/mariadb:10.6"
	DefaultPostgresImage = "docker.io/library/postgres:15-alpine"
	DefaultRedisImage    = "docker.io/library/redis:7-alpine"

	// Infrastructure images
	DefaultNginxImage   = "docker.io/library/nginx:1.25-alpine"
	DefaultAlpineImage  = "docker.io/library/alpine:latest"
	DefaultBusyboxImage = "docker.io/library/busybox:latest"
)

// KEDA Images for autoscaling
const (
	KEDAImage                 = "docker.io/kedacore/keda:2.13.0"
	KEDAMetricsAPIServerImage = "docker.io/kedacore/keda-metrics-apiserver:2.13.0"
	KEDAWebhooksImage         = "docker.io/kedacore/keda-admission-webhooks:2.13.0"
)

// MariaDB Operator Images
const (
	MariaDBOperatorImage        = "docker.io/mariadb/mariadb-operator:v0.0.25"
	MariaDBOperatorWebhookImage = "docker.io/mariadb/mariadb-operator-webhook:v0.0.25"
)

// OpenShift Registry Images (for OpenShift deployments)
const (
	OpenShiftMariaDBImage  = "registry.redhat.io/rhel8/mariadb-103:latest"
	OpenShiftPostgresImage = "registry.redhat.io/rhel8/postgresql-15:latest"
	OpenShiftRedisImage    = "registry.redhat.io/rhel8/redis-6:latest"
	OpenShiftNginxImage    = "registry.redhat.io/rhel8/nginx-120:latest"
)

// Google Container Registry (alternative)
const (
	GCRMariaDBImage  = "gcr.io/cloud-marketplace/google/mariadb:latest"
	GCRPostgresImage = "gcr.io/cloud-sql-connectors/cloud-sql-proxy:latest"
	GCRRedisImage    = "gcr.io/google_containers/redis:e2e"
)

// Quay.io Registry (alternative for Red Hat ecosystems)
const (
	QuayMariaDBImage  = "quay.io/sclorg/mariadb-103-c8s:latest"
	QuayPostgresImage = "quay.io/sclorg/postgresql-15-c9s:latest"
	QuayRedisImage    = "quay.io/fedora/redis-7:latest"
)

// Image Pull Policies
const (
	DefaultImagePullPolicy = string(corev1.PullIfNotPresent)
	AlwaysPullPolicy       = string(corev1.PullAlways)
	NeverPullPolicy        = string(corev1.PullNever)
)

// Registry Configuration
const (
	DefaultRegistry = "docker.io"
	RedHatRegistry  = "registry.redhat.io"
	GoogleRegistry  = "gcr.io"
	QuayRegistry    = "quay.io"
)

// Security Contexts
const (
	DefaultRunAsUser  = 1001
	DefaultRunAsGroup = 1001
)

// GetImageWithRegistry returns the appropriate image based on registry preference
func GetImageWithRegistry(baseImage, registry string) string {
	switch registry {
	case RedHatRegistry:
		switch baseImage {
		case DefaultMariaDBImage:
			return OpenShiftMariaDBImage
		case DefaultPostgresImage:
			return OpenShiftPostgresImage
		case DefaultRedisImage:
			return OpenShiftRedisImage
		case DefaultNginxImage:
			return OpenShiftNginxImage
		}
	case GoogleRegistry:
		switch baseImage {
		case DefaultMariaDBImage:
			return GCRMariaDBImage
		case DefaultPostgresImage:
			return GCRPostgresImage
		case DefaultRedisImage:
			return GCRRedisImage
		}
	case QuayRegistry:
		switch baseImage {
		case DefaultMariaDBImage:
			return QuayMariaDBImage
		case DefaultPostgresImage:
			return QuayPostgresImage
		case DefaultRedisImage:
			return QuayRedisImage
		}
	}
	return baseImage
}

// GetFrappeImage returns the Frappe image with version
func GetFrappeImage(version string) string {
	if version == "" || version == "latest" {
		return DefaultFrappeImage
	}
	return "docker.io/frappe/erpnext:" + version
}

// GetBenchImage returns the bench image with version
func GetBenchImage(version string) string {
	if version == "" || version == "latest" {
		return DefaultBenchImage
	}
	return "docker.io/frappe/erpnext:" + version
}
