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

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// benchLabels returns standard labels for bench resources
func (r *FrappeBenchReconciler) benchLabels(bench *vyogotechv1.FrappeBench) map[string]string {
	return map[string]string{
		"app":   "frappe",
		"bench": bench.Name,
	}
}

// componentLabels returns labels for a specific component
func (r *FrappeBenchReconciler) componentLabels(bench *vyogotechv1.FrappeBench, component string) map[string]string {
	labels := r.benchLabels(bench)
	labels["component"] = component
	return labels
}

// Image getters

func (r *FrappeBenchReconciler) getImagePullSecrets(bench *vyogotechv1.FrappeBench) []corev1.LocalObjectReference {
	if bench.Spec.ImageConfig != nil && len(bench.Spec.ImageConfig.PullSecrets) > 0 {
		secrets := make([]corev1.LocalObjectReference, len(bench.Spec.ImageConfig.PullSecrets))
		for i, s := range bench.Spec.ImageConfig.PullSecrets {
			secrets[i] = corev1.LocalObjectReference{Name: s.Name}
		}
		return secrets
	}
	return nil
}

func (r *FrappeBenchReconciler) getImagePullPolicy(bench *vyogotechv1.FrappeBench) corev1.PullPolicy {
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.PullPolicy != "" {
		return bench.Spec.ImageConfig.PullPolicy
	}
	return corev1.PullPolicy("") // Leave empty so Kubernetes defaults apply
}

func (r *FrappeBenchReconciler) getRedisImage(bench *vyogotechv1.FrappeBench) string {
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.Image != "" {
		return bench.Spec.RedisConfig.Image
	}
	return "redis:7-alpine"
}

// Replica getters

// Resource getters

func (r *FrappeBenchReconciler) getRedisResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.Resources != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.RedisConfig.Resources.Requests,
			Limits:   bench.Spec.RedisConfig.Resources.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getGunicornResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Gunicorn != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Gunicorn.Requests,
			Limits:   bench.Spec.ComponentResources.Gunicorn.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getNginxResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Nginx != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Nginx.Requests,
			Limits:   bench.Spec.ComponentResources.Nginx.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
}

func (r *FrappeBenchReconciler) getSocketIOResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Socketio != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Socketio.Requests,
			Limits:   bench.Spec.ComponentResources.Socketio.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getSchedulerResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Scheduler != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Scheduler.Requests,
			Limits:   bench.Spec.ComponentResources.Scheduler.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerDefaultResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerDefault != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerDefault.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerDefault.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerLongResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerLong != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerLong.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerLong.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerShortResources(bench *vyogotechv1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerShort != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerShort.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerShort.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

// Autoscaling configuration helpers

// Autoscaling configuration helpers

// getComponentAutoscaling gets the autoscaling config for a component
func (r *FrappeBenchReconciler) getComponentAutoscaling(bench *vyogotechv1.FrappeBench, componentName string) *vyogotechv1.ComponentAutoscaling {
	if bench.Spec.ComponentAutoscaling != nil {
		if config, exists := bench.Spec.ComponentAutoscaling[componentName]; exists && config != nil {
			return config
		}
	}
	return nil
}

// fillComponentDefaults fills in default values for a component's autoscaling config
func (r *FrappeBenchReconciler) fillComponentDefaults(config *vyogotechv1.ComponentAutoscaling, componentName string) *vyogotechv1.ComponentAutoscaling {
	if config == nil {
		return r.getComponentDefaults(componentName)
	}

	result := &vyogotechv1.ComponentAutoscaling{}
	*result = *config

	defaults := r.getComponentDefaults(componentName)

	// Fill in missing fields with defaults
	if result.Enabled == nil {
		result.Enabled = defaults.Enabled
	}
	if result.MinReplicas == nil {
		result.MinReplicas = defaults.MinReplicas
	}
	if result.MaxReplicas == nil {
		result.MaxReplicas = defaults.MaxReplicas
	}
	if result.StaticReplicas == nil {
		result.StaticReplicas = defaults.StaticReplicas
	}
	if result.CooldownPeriod == nil {
		result.CooldownPeriod = defaults.CooldownPeriod
	}
	if result.PollingInterval == nil {
		result.PollingInterval = defaults.PollingInterval
	}
	if result.Provider == "" {
		result.Provider = defaults.Provider
	}
	if result.KEDA == nil && defaults.KEDA != nil {
		result.KEDA = defaults.KEDA
	}
	if result.HPA == nil && defaults.HPA != nil {
		result.HPA = defaults.HPA
	}

	return result
}

// cleanupOtherProviders removes scaling resources from providers other than the current one
func (r *FrappeBenchReconciler) cleanupOtherProviders(ctx context.Context, bench *vyogotechv1.FrappeBench, componentName string, currentProvider string) {
	providers := []string{"keda", "hpa"}
	for _, p := range providers {
		if p != currentProvider {
			_ = resolveProvider(p, r.Client, r.Scheme).Delete(ctx, bench, componentName)
		}
	}
}

// getComponentDefaults returns opinionated defaults per component
func (r *FrappeBenchReconciler) getComponentDefaults(componentName string) *vyogotechv1.ComponentAutoscaling {
	defaults := map[string]*vyogotechv1.ComponentAutoscaling{
		"nginx": {
			Enabled:         boolPtr(false),
			StaticReplicas:  int32Ptr(1),
			MinReplicas:     int32Ptr(1),
			MaxReplicas:     int32Ptr(10),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(30),
			Provider:        "hpa",
			HPA: &vyogotechv1.HPAScalingConfig{
				Metric:                 "cpu",
				TargetUtilization:      int32Ptr(70),
				ScaleUpStabilization:   int32Ptr(0),
				ScaleDownStabilization: int32Ptr(300),
			},
		},
		"gunicorn": {
			Enabled:         boolPtr(false),
			StaticReplicas:  int32Ptr(1),
			MinReplicas:     int32Ptr(2),
			MaxReplicas:     int32Ptr(10),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(30),
			Provider:        "hpa",
			HPA: &vyogotechv1.HPAScalingConfig{
				Metric:                 "cpu",
				TargetUtilization:      int32Ptr(70),
				ScaleUpStabilization:   int32Ptr(0),
				ScaleDownStabilization: int32Ptr(300),
			},
		},
		"socketio": {
			Enabled:         boolPtr(false),
			StaticReplicas:  int32Ptr(1),
			MinReplicas:     int32Ptr(1),
			MaxReplicas:     int32Ptr(5),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(30),
			Provider:        "hpa",
			HPA: &vyogotechv1.HPAScalingConfig{
				Metric:                 "cpu",
				TargetUtilization:      int32Ptr(70),
				ScaleUpStabilization:   int32Ptr(0),
				ScaleDownStabilization: int32Ptr(300),
			},
		},
		"scheduler": {
			Enabled:         boolPtr(false),
			StaticReplicas:  int32Ptr(1),
			MinReplicas:     int32Ptr(1),
			MaxReplicas:     int32Ptr(1), // Always 1
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(30),
			// No provider for scheduler
		},
		"worker-short": {
			Enabled:         boolPtr(false),
			StaticReplicas:  int32Ptr(1),
			MinReplicas:     int32Ptr(0),
			MaxReplicas:     int32Ptr(10),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(30),
			Provider:        "keda",
			KEDA: &vyogotechv1.KEDAScalingConfig{
				Trigger:     "redis",
				TargetValue: "5",
			},
		},
		"worker-long": {
			Enabled:         boolPtr(false),
			StaticReplicas:  int32Ptr(1),
			MinReplicas:     int32Ptr(0),
			MaxReplicas:     int32Ptr(5),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(30),
			Provider:        "keda",
			KEDA: &vyogotechv1.KEDAScalingConfig{
				Trigger:     "redis",
				TargetValue: "2",
			},
		},
		"worker-default": {
			Enabled:         boolPtr(false),
			StaticReplicas:  int32Ptr(1),
			MinReplicas:     int32Ptr(0),
			MaxReplicas:     int32Ptr(5),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(30),
			Provider:        "keda",
			KEDA: &vyogotechv1.KEDAScalingConfig{
				Trigger:     "redis",
				TargetValue: "5",
			},
		},
	}

	if def, ok := defaults[componentName]; ok {
		return def
	}

	// Safe fallback for unknown components
	return &vyogotechv1.ComponentAutoscaling{
		Enabled:         boolPtr(false),
		StaticReplicas:  int32Ptr(1),
		MinReplicas:     int32Ptr(1),
		MaxReplicas:     int32Ptr(10),
		CooldownPeriod:  int32Ptr(60),
		PollingInterval: int32Ptr(30),
	}
}

// getComponentReplicaCount determines the replica count based on scaling mode
func (r *FrappeBenchReconciler) getComponentReplicaCount(config *vyogotechv1.ComponentAutoscaling, providerManaged bool) int32 {
	if config == nil {
		return 1
	}

	// If provider is managing (autoscaling enabled and available), use MinReplicas
	if providerManaged && config.Enabled != nil && *config.Enabled {
		if config.MinReplicas != nil {
			return *config.MinReplicas
		}
		return 1
	}

	// Otherwise use StaticReplicas
	if config.StaticReplicas != nil {
		return *config.StaticReplicas
	}
	return 1
}

// Security context helpers (shared logic in security_context.go; Redis uses fixed UID 999)

func (r *FrappeBenchReconciler) getPodSecurityContext(ctx context.Context, bench *vyogotechv1.FrappeBench) *corev1.PodSecurityContext {
	return PodSecurityContextForBench(ctx, r.Client, r.IsOpenShift, bench.Namespace, bench.Spec.Security)
}

func (r *FrappeBenchReconciler) getContainerSecurityContext(ctx context.Context, bench *vyogotechv1.FrappeBench) *corev1.SecurityContext {
	return ContainerSecurityContextForBench(r.IsOpenShift, bench.Spec.Security)
}

func (r *FrappeBenchReconciler) getRedisPodSecurityContext(bench *vyogotechv1.FrappeBench) *corev1.PodSecurityContext {
	// If user provided custom security context, use it
	if bench.Spec.Security != nil && bench.Spec.Security.PodSecurityContext != nil {
		return bench.Spec.Security.PodSecurityContext
	}

	secCtx := &corev1.PodSecurityContext{
		RunAsNonRoot: boolPtr(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	// Only set fixed UIDs if not on OpenShift
	if !r.IsOpenShift {
		// Redis alpine images use UID/GID 999
		redisUID := int64(999)
		secCtx.RunAsUser = &redisUID
		secCtx.RunAsGroup = &redisUID
		secCtx.FSGroup = &redisUID
	} else {
		// On OpenShift, if we don't provide a numeric UID, we must omit RunAsNonRoot
		// to avoid "image will run as root" validation error for named users.
		// OpenShift's SCC will still enforce non-root execution.
		secCtx.RunAsNonRoot = nil
	}

	return secCtx
}

func (r *FrappeBenchReconciler) getRedisContainerSecurityContext(bench *vyogotechv1.FrappeBench) *corev1.SecurityContext {
	// If user provided custom security context, use it
	if bench.Spec.Security != nil && bench.Spec.Security.SecurityContext != nil {
		return bench.Spec.Security.SecurityContext
	}

	secCtx := &corev1.SecurityContext{
		RunAsNonRoot:             boolPtr(true),
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		ReadOnlyRootFilesystem: boolPtr(false),
	}

	// Only set fixed UIDs if not on OpenShift
	if !r.IsOpenShift {
		// Redis alpine images use UID/GID 999
		redisUID := int64(999)
		secCtx.RunAsUser = &redisUID
		secCtx.RunAsGroup = &redisUID
	} else {
		// On OpenShift, if we don't provide a numeric UID, we must omit RunAsNonRoot
		// to avoid "image will run as root" validation error for named users.
		// OpenShift's SCC will still enforce non-root execution.
		secCtx.RunAsNonRoot = nil
	}

	return secCtx
}
