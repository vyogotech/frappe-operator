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

package resources

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// intOrStringFromInt creates an IntOrString from an int
func intOrStringFromInt(val int) *intstr.IntOrString {
	v := intstr.FromInt(val)
	return &v
}

// Int32Ptr returns a pointer to the passed int32 value
func Int32Ptr(i int32) *int32 {
	return &i
}

// Int64Ptr returns a pointer to the passed int64 value
func Int64Ptr(i int64) *int64 {
	return &i
}

// BoolPtr returns a pointer to the passed bool value
func BoolPtr(b bool) *bool {
	return &b
}

// StringPtr returns a pointer to the passed string value
func StringPtr(s string) *string {
	return &s
}

// DefaultSecurityContext returns a secure container security context
func DefaultSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             BoolPtr(true),
		AllowPrivilegeEscalation: BoolPtr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		ReadOnlyRootFilesystem: BoolPtr(false),
	}
}

// DefaultPodSecurityContext returns a secure pod security context
func DefaultPodSecurityContext(uid, gid int64) *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsNonRoot: BoolPtr(true),
		RunAsUser:    Int64Ptr(uid),
		RunAsGroup:   Int64Ptr(gid),
		FSGroup:      Int64Ptr(gid),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// StandardLabels creates standard Kubernetes labels
func StandardLabels(app, component, instance string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       app,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/instance":   instance,
		"app.kubernetes.io/managed-by": "frappe-operator",
	}
}

// MergeLabels merges multiple label maps, later maps take precedence
func MergeLabels(labelMaps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range labelMaps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// ResourceList creates a resource list from CPU and memory strings
func ResourceList(cpu, memory string) corev1.ResourceList {
	list := corev1.ResourceList{}
	if cpu != "" {
		list[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		list[corev1.ResourceMemory] = resource.MustParse(memory)
	}
	return list
}

// ResourceRequirements creates resource requirements from request/limit strings
func ResourceRequirements(cpuRequest, memoryRequest, cpuLimit, memoryLimit string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: ResourceList(cpuRequest, memoryRequest),
		Limits:   ResourceList(cpuLimit, memoryLimit),
	}
}
