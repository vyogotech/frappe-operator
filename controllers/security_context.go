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

package controllers

import (
	"context"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PodSecurityContextForBench returns the pod-level security context for bench/site pods.
// Used by both FrappeBenchReconciler and FrappeSiteReconciler to avoid duplication.
func PodSecurityContextForBench(ctx context.Context, c client.Client, isOpenShift bool, namespace string, security *vyogotechv1alpha1.SecurityConfig) *corev1.PodSecurityContext {
	defaultFSGroup := getDefaultFSGroup()
	fsGroupChangePolicy := corev1.FSGroupChangeAlways

	secCtx := &corev1.PodSecurityContext{
		RunAsNonRoot:        boolPtr(true),
		FSGroup:             defaultFSGroup,
		FSGroupChangePolicy: &fsGroupChangePolicy,
	}

	logger := log.FromContext(ctx)
	if isOpenShift {
		logger.Info("OpenShift platform detected for Bench security context")
		mcsLabel := getNamespaceMCSLabel(ctx, c, namespace)
		if mcsLabel != "" {
			logger.Info("Applying Namespace MCS label to PodSecurityContext", "mcsLabel", mcsLabel)
			secCtx.SELinuxOptions = &corev1.SELinuxOptions{Level: mcsLabel}
		} else {
			logger.Info("Namespace MCS label is empty, skipping SELinuxOptions")
		}
		secCtx.FSGroup = nil
		secCtx.SupplementalGroups = nil
		logger.Info("Using OpenShift defaults (no explicit FSGroup/SupplementalGroups due to SCC restricted-v2)")
	} else {
		logger.V(1).Info("Not on OpenShift platform, skipping MCS label matching")
	}

	if !isOpenShift {
		secCtx.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
		secCtx.RunAsUser = getDefaultUID()
		secCtx.RunAsGroup = getDefaultGID()
	}

	if security != nil && security.PodSecurityContext != nil {
		userCtx := security.PodSecurityContext
		if userCtx.RunAsNonRoot != nil {
			secCtx.RunAsNonRoot = userCtx.RunAsNonRoot
		}
		if userCtx.RunAsUser != nil {
			secCtx.RunAsUser = userCtx.RunAsUser
		}
		if userCtx.RunAsGroup != nil {
			secCtx.RunAsGroup = userCtx.RunAsGroup
		}
		if userCtx.FSGroup != nil {
			secCtx.FSGroup = userCtx.FSGroup
		}
		if userCtx.FSGroupChangePolicy != nil {
			secCtx.FSGroupChangePolicy = userCtx.FSGroupChangePolicy
		}
		if userCtx.SupplementalGroups != nil {
			secCtx.SupplementalGroups = userCtx.SupplementalGroups
		}
		if userCtx.SELinuxOptions != nil {
			secCtx.SELinuxOptions = userCtx.SELinuxOptions
		}
		if userCtx.WindowsOptions != nil {
			secCtx.WindowsOptions = userCtx.WindowsOptions
		}
		if userCtx.Sysctls != nil {
			secCtx.Sysctls = userCtx.Sysctls
		}
		if userCtx.SeccompProfile != nil {
			secCtx.SeccompProfile = userCtx.SeccompProfile
		}
	}

	return secCtx
}

// ContainerSecurityContextForBench returns the container-level security context for bench/site containers.
// Used by both FrappeBenchReconciler and FrappeSiteReconciler to avoid duplication.
func ContainerSecurityContextForBench(isOpenShift bool, security *vyogotechv1alpha1.SecurityConfig) *corev1.SecurityContext {
	defaultUID := getDefaultUID()
	defaultGID := getDefaultGID()

	secCtx := &corev1.SecurityContext{
		RunAsNonRoot:             boolPtr(true),
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		ReadOnlyRootFilesystem:   boolPtr(false),
	}

	if !isOpenShift {
		secCtx.RunAsUser = defaultUID
		secCtx.RunAsGroup = defaultGID
	}

	if security != nil && security.SecurityContext != nil {
		userCtx := security.SecurityContext
		if userCtx.RunAsNonRoot != nil {
			secCtx.RunAsNonRoot = userCtx.RunAsNonRoot
		}
		if userCtx.RunAsUser != nil {
			secCtx.RunAsUser = userCtx.RunAsUser
		}
		if userCtx.RunAsGroup != nil {
			secCtx.RunAsGroup = userCtx.RunAsGroup
		}
		if userCtx.Privileged != nil {
			secCtx.Privileged = userCtx.Privileged
		}
		if userCtx.AllowPrivilegeEscalation != nil {
			secCtx.AllowPrivilegeEscalation = userCtx.AllowPrivilegeEscalation
		}
		if userCtx.Capabilities != nil {
			secCtx.Capabilities = userCtx.Capabilities
		}
		if userCtx.ReadOnlyRootFilesystem != nil {
			secCtx.ReadOnlyRootFilesystem = userCtx.ReadOnlyRootFilesystem
		}
		if userCtx.SELinuxOptions != nil {
			secCtx.SELinuxOptions = userCtx.SELinuxOptions
		}
		if userCtx.WindowsOptions != nil {
			secCtx.WindowsOptions = userCtx.WindowsOptions
		}
		if userCtx.ProcMount != nil {
			secCtx.ProcMount = userCtx.ProcMount
		}
		if userCtx.SeccompProfile != nil {
			secCtx.SeccompProfile = userCtx.SeccompProfile
		}
	}

	return secCtx
}
