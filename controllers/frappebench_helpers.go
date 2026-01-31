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
	"k8s.io/apimachinery/pkg/api/resource"
)

// benchLabels returns standard labels for bench resources
func (r *FrappeBenchReconciler) benchLabels(bench *vyogotechv1alpha1.FrappeBench) map[string]string {
	return map[string]string{
		"app":   "frappe",
		"bench": bench.Name,
	}
}

// componentLabels returns labels for a specific component
func (r *FrappeBenchReconciler) componentLabels(bench *vyogotechv1alpha1.FrappeBench, component string) map[string]string {
	labels := r.benchLabels(bench)
	labels["component"] = component
	return labels
}

// Image getters

func (r *FrappeBenchReconciler) getRedisImage(bench *vyogotechv1alpha1.FrappeBench) string {
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.Image != "" {
		return bench.Spec.RedisConfig.Image
	}
	return "redis:7-alpine"
}

// Replica getters

func (r *FrappeBenchReconciler) getGunicornReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.Gunicorn
	}
	return 1
}

func (r *FrappeBenchReconciler) getNginxReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.Nginx
	}
	return 1
}

func (r *FrappeBenchReconciler) getSocketIOReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.Socketio
	}
	return 1
}

func (r *FrappeBenchReconciler) getWorkerDefaultReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.WorkerDefault
	}
	return 1
}

func (r *FrappeBenchReconciler) getWorkerLongReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.WorkerLong
	}
	return 1
}

func (r *FrappeBenchReconciler) getWorkerShortReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.WorkerShort
	}
	return 1
}

// Resource getters

func (r *FrappeBenchReconciler) getRedisResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.Resources != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.RedisConfig.Resources.Requests,
			Limits:   bench.Spec.RedisConfig.Resources.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getGunicornResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Gunicorn != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Gunicorn.Requests,
			Limits:   bench.Spec.ComponentResources.Gunicorn.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getNginxResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Nginx != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Nginx.Requests,
			Limits:   bench.Spec.ComponentResources.Nginx.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
}

func (r *FrappeBenchReconciler) getSocketIOResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Socketio != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Socketio.Requests,
			Limits:   bench.Spec.ComponentResources.Socketio.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getSchedulerResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Scheduler != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Scheduler.Requests,
			Limits:   bench.Spec.ComponentResources.Scheduler.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerDefaultResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerDefault != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerDefault.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerDefault.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerLongResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerLong != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerLong.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerLong.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerShortResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerShort != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerShort.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerShort.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

// Autoscaling configuration helpers

// getWorkerAutoscalingConfig returns the autoscaling config for a specific worker type
// Falls back to legacy ComponentReplicas if WorkerAutoscaling not configured
func (r *FrappeBenchReconciler) getWorkerAutoscalingConfig(bench *vyogotechv1alpha1.FrappeBench, workerType string) *vyogotechv1alpha1.WorkerAutoscaling {
	// Return configured autoscaling if available
	if bench.Spec.WorkerAutoscaling != nil {
		switch workerType {
		case "short":
			if bench.Spec.WorkerAutoscaling.Short != nil {
				return bench.Spec.WorkerAutoscaling.Short
			}
		case "long":
			if bench.Spec.WorkerAutoscaling.Long != nil {
				return bench.Spec.WorkerAutoscaling.Long
			}
		case "default":
			if bench.Spec.WorkerAutoscaling.Default != nil {
				return bench.Spec.WorkerAutoscaling.Default
			}
		}
	}

	// Fall back to legacy ComponentReplicas
	if bench.Spec.ComponentReplicas != nil {
		config := &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled: boolPtr(false), // Legacy is always static
		}
		switch workerType {
		case "short":
			if bench.Spec.ComponentReplicas.WorkerShort > 0 {
				config.StaticReplicas = int32Ptr(bench.Spec.ComponentReplicas.WorkerShort)
			} else {
				config.StaticReplicas = int32Ptr(2)
			}
		case "long":
			if bench.Spec.ComponentReplicas.WorkerLong > 0 {
				config.StaticReplicas = int32Ptr(bench.Spec.ComponentReplicas.WorkerLong)
			} else {
				config.StaticReplicas = int32Ptr(1)
			}
		case "default":
			if bench.Spec.ComponentReplicas.WorkerDefault > 0 {
				config.StaticReplicas = int32Ptr(bench.Spec.ComponentReplicas.WorkerDefault)
			} else {
				config.StaticReplicas = int32Ptr(1)
			}
		}
		return config
	}

	// Return nil to use defaults
	return nil
}

// getDefaultAutoscalingConfig returns opinionated defaults for each worker type
func (r *FrappeBenchReconciler) getDefaultAutoscalingConfig(workerType string) *vyogotechv1alpha1.WorkerAutoscaling {
	switch workerType {
	case "short":
		// Short jobs: scale-to-zero with aggressive scaling
		return &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled:         boolPtr(true),
			MinReplicas:     int32Ptr(0),
			MaxReplicas:     int32Ptr(10),
			QueueLength:     int32Ptr(5),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(15),
		}
	case "long":
		// Long jobs: scale-to-zero with conservative scaling
		return &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled:         boolPtr(true),
			MinReplicas:     int32Ptr(0),
			MaxReplicas:     int32Ptr(5),
			QueueLength:     int32Ptr(2),
			CooldownPeriod:  int32Ptr(300),
			PollingInterval: int32Ptr(30),
		}
	case "default":
		// Default/scheduler: always one replica (scheduler must run)
		return &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled:        boolPtr(false),
			StaticReplicas: int32Ptr(1),
		}
	}
	return nil
}

// fillAutoscalingDefaults fills in missing fields with defaults
func (r *FrappeBenchReconciler) fillAutoscalingDefaults(config *vyogotechv1alpha1.WorkerAutoscaling, workerType string) *vyogotechv1alpha1.WorkerAutoscaling {
	if config == nil {
		return r.getDefaultAutoscalingConfig(workerType)
	}

	result := &vyogotechv1alpha1.WorkerAutoscaling{}
	*result = *config

	// Fill in defaults
	defaults := r.getDefaultAutoscalingConfig(workerType)
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
	if result.QueueLength == nil {
		result.QueueLength = defaults.QueueLength
	}
	if result.CooldownPeriod == nil {
		result.CooldownPeriod = defaults.CooldownPeriod
	}
	if result.PollingInterval == nil {
		result.PollingInterval = defaults.PollingInterval
	}

	return result
}

// getWorkerReplicaCount determines the replica count based on scaling mode
func (r *FrappeBenchReconciler) getWorkerReplicaCount(config *vyogotechv1alpha1.WorkerAutoscaling, kedaAvailable bool) int32 {
	// If KEDA autoscaling enabled and available, use MinReplicas
	if config.Enabled != nil && *config.Enabled && kedaAvailable {
		if config.MinReplicas != nil {
			return *config.MinReplicas
		}
		return 0 // Default to scale-to-zero
	}

	// Otherwise use static replicas
	if config.StaticReplicas != nil {
		return *config.StaticReplicas
	}

	// Fallback
	return 1
}

// Security context helpers (shared logic in security_context.go; Redis uses fixed UID 999)

func (r *FrappeBenchReconciler) getPodSecurityContext(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) *corev1.PodSecurityContext {
	return PodSecurityContextForBench(ctx, r.Client, r.IsOpenShift, bench.Namespace, bench.Spec.Security)
}

func (r *FrappeBenchReconciler) getContainerSecurityContext(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) *corev1.SecurityContext {
	return ContainerSecurityContextForBench(r.IsOpenShift, bench.Spec.Security)
}

func (r *FrappeBenchReconciler) getRedisPodSecurityContext(bench *vyogotechv1alpha1.FrappeBench) *corev1.PodSecurityContext {
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
	}

	return secCtx
}

func (r *FrappeBenchReconciler) getRedisContainerSecurityContext(bench *vyogotechv1alpha1.FrappeBench) *corev1.SecurityContext {
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
	}

	return secCtx
}
