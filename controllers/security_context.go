package controllers

import (
	"os"
	"strconv"

	corev1 "k8s.io/api/core/v1"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

const (
	// Default UID/GID for Frappe containers (non-root user)
	defaultRunAsUserID  int64 = 1001
	defaultRunAsGroupID int64 = 1001
	defaultFSGroupID    int64 = 1001

	// Environment variable names for operator-level security context configuration
	envRunAsUserID  = "FRAPPE_RUN_AS_USER"
	envRunAsGroupID = "FRAPPE_RUN_AS_GROUP"
	envFSGroupID    = "FRAPPE_FS_GROUP"
)

// getConfiguredSecurityIDs returns the configured UID/GID values
// Priority: environment variables > defaults
func getConfiguredSecurityIDs() (userID, groupID, fsGroupID int64) {
	userID = defaultRunAsUserID
	groupID = defaultRunAsGroupID
	fsGroupID = defaultFSGroupID

	// Override from environment if set
	if envUser := os.Getenv(envRunAsUserID); envUser != "" {
		if parsed, err := strconv.ParseInt(envUser, 10, 64); err == nil {
			userID = parsed
		}
	}

	if envGroup := os.Getenv(envRunAsGroupID); envGroup != "" {
		if parsed, err := strconv.ParseInt(envGroup, 10, 64); err == nil {
			groupID = parsed
		}
	}

	if envFS := os.Getenv(envFSGroupID); envFS != "" {
		if parsed, err := strconv.ParseInt(envFS, 10, 64); err == nil {
			fsGroupID = parsed
		}
	}

	return
}

// applyBenchSecurityDefaults ensures that Pods created by the operator adhere to the
// minimum security posture required by hardened clusters while still allowing
// benches to override the defaults through spec.security.
func applyBenchSecurityDefaults(podSpec *corev1.PodSpec, bench *vyogotechv1alpha1.FrappeBench) {
	if podSpec == nil {
		return
	}

	defaultPodContext := getBenchPodSecurityContext(bench)
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = defaultPodContext
	} else {
		mergePodSecurityContext(podSpec.SecurityContext, defaultPodContext)
	}

	for i := range podSpec.InitContainers {
		ensureContainerSecurityContext(&podSpec.InitContainers[i], bench)
	}

	for i := range podSpec.Containers {
		ensureContainerSecurityContext(&podSpec.Containers[i], bench)
	}
}

func ensureContainerSecurityContext(container *corev1.Container, bench *vyogotechv1alpha1.FrappeBench) {
	if container == nil {
		return
	}

	defaultCtx := getBenchContainerSecurityContext(bench)
	if container.SecurityContext == nil {
		container.SecurityContext = defaultCtx
		return
	}

	mergeContainerSecurityContext(container.SecurityContext, defaultCtx)
}

func getBenchPodSecurityContext(bench *vyogotechv1alpha1.FrappeBench) *corev1.PodSecurityContext {
	if bench != nil && bench.Spec.Security != nil && bench.Spec.Security.PodSecurityContext != nil {
		return bench.Spec.Security.PodSecurityContext.DeepCopy()
	}

	userID, groupID, fsGroupID := getConfiguredSecurityIDs()

	return &corev1.PodSecurityContext{
		RunAsUser:  int64Ptr(userID),
		RunAsGroup: int64Ptr(groupID),
		FSGroup:    int64Ptr(fsGroupID),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func getBenchContainerSecurityContext(bench *vyogotechv1alpha1.FrappeBench) *corev1.SecurityContext {
	if bench != nil && bench.Spec.Security != nil && bench.Spec.Security.SecurityContext != nil {
		return bench.Spec.Security.SecurityContext.DeepCopy()
	}

	userID, groupID, _ := getConfiguredSecurityIDs()

	return &corev1.SecurityContext{
		RunAsUser:                int64Ptr(userID),
		RunAsGroup:               int64Ptr(groupID),
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		ReadOnlyRootFilesystem: boolPtr(false),
	}
}

func mergePodSecurityContext(target, defaults *corev1.PodSecurityContext) {
	if target == nil || defaults == nil {
		return
	}

	if target.RunAsUser == nil {
		target.RunAsUser = defaults.RunAsUser
	}

	if target.RunAsGroup == nil {
		target.RunAsGroup = defaults.RunAsGroup
	}

	if target.FSGroup == nil {
		target.FSGroup = defaults.FSGroup
	}

	if target.SeccompProfile == nil && defaults.SeccompProfile != nil {
		target.SeccompProfile = defaults.SeccompProfile.DeepCopy()
	}
}

func mergeContainerSecurityContext(target, defaults *corev1.SecurityContext) {
	if target == nil || defaults == nil {
		return
	}

	if target.RunAsUser == nil {
		target.RunAsUser = defaults.RunAsUser
	}

	if target.RunAsGroup == nil {
		target.RunAsGroup = defaults.RunAsGroup
	}

	if target.AllowPrivilegeEscalation == nil {
		target.AllowPrivilegeEscalation = defaults.AllowPrivilegeEscalation
	}

	if target.ReadOnlyRootFilesystem == nil {
		target.ReadOnlyRootFilesystem = defaults.ReadOnlyRootFilesystem
	}

	if target.SeccompProfile == nil && defaults.SeccompProfile != nil {
		target.SeccompProfile = defaults.SeccompProfile.DeepCopy()
	}

	if target.Capabilities == nil && defaults.Capabilities != nil {
		target.Capabilities = defaults.Capabilities.DeepCopy()
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
