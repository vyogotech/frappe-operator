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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ContainerBuilder provides a fluent interface for building containers
type ContainerBuilder struct {
	container corev1.Container
}

// NewContainerBuilder creates a new ContainerBuilder
func NewContainerBuilder(name, image string) *ContainerBuilder {
	return &ContainerBuilder{
		container: corev1.Container{
			Name:         name,
			Image:        image,
			Env:          []corev1.EnvVar{},
			VolumeMounts: []corev1.VolumeMount{},
			Ports:        []corev1.ContainerPort{},
		},
	}
}

// WithCommand sets the command
func (b *ContainerBuilder) WithCommand(command ...string) *ContainerBuilder {
	b.container.Command = command
	return b
}

// WithArgs sets the args
func (b *ContainerBuilder) WithArgs(args ...string) *ContainerBuilder {
	b.container.Args = args
	return b
}

// WithEnv adds an environment variable
func (b *ContainerBuilder) WithEnv(name, value string) *ContainerBuilder {
	b.container.Env = append(b.container.Env, corev1.EnvVar{
		Name:  name,
		Value: value,
	})
	return b
}

// WithEnvFrom adds an environment variable from a source
func (b *ContainerBuilder) WithEnvFrom(envVar corev1.EnvVar) *ContainerBuilder {
	b.container.Env = append(b.container.Env, envVar)
	return b
}

// WithEnvFromSecret adds an env from a secret key
func (b *ContainerBuilder) WithEnvFromSecret(envName, secretName, key string) *ContainerBuilder {
	return b.WithEnvFrom(corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  key,
			},
		},
	})
}

// WithEnvFromConfigMap adds an env from a configmap key
func (b *ContainerBuilder) WithEnvFromConfigMap(envName, configMapName, key string) *ContainerBuilder {
	return b.WithEnvFrom(corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
				Key:                  key,
			},
		},
	})
}

// WithEnvFromFieldRef adds an env from a field selector (e.g., metadata.name)
func (b *ContainerBuilder) WithEnvFromFieldRef(envName, fieldPath string) *ContainerBuilder {
	return b.WithEnvFrom(corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: fieldPath,
			},
		},
	})
}

// WithVolumeMount adds a volume mount
func (b *ContainerBuilder) WithVolumeMount(name, mountPath string) *ContainerBuilder {
	b.container.VolumeMounts = append(b.container.VolumeMounts, corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
	})
	return b
}

// WithVolumeMountSubPath adds a volume mount with subPath
func (b *ContainerBuilder) WithVolumeMountSubPath(name, mountPath, subPath string) *ContainerBuilder {
	b.container.VolumeMounts = append(b.container.VolumeMounts, corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		SubPath:   subPath,
	})
	return b
}

// WithVolumeMountReadOnly adds a read-only volume mount
func (b *ContainerBuilder) WithVolumeMountReadOnly(name, mountPath string) *ContainerBuilder {
	b.container.VolumeMounts = append(b.container.VolumeMounts, corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		ReadOnly:  true,
	})
	return b
}

// WithPort adds a container port
func (b *ContainerBuilder) WithPort(name string, port int32) *ContainerBuilder {
	return b.WithPortProtocol(name, port, corev1.ProtocolTCP)
}

// WithPortProtocol adds a container port with protocol
func (b *ContainerBuilder) WithPortProtocol(name string, port int32, protocol corev1.Protocol) *ContainerBuilder {
	b.container.Ports = append(b.container.Ports, corev1.ContainerPort{
		Name:          name,
		ContainerPort: port,
		Protocol:      protocol,
	})
	return b
}

// WithResources sets resource requirements
func (b *ContainerBuilder) WithResources(resources corev1.ResourceRequirements) *ContainerBuilder {
	b.container.Resources = resources
	return b
}

// WithResourceLimits sets resource limits
func (b *ContainerBuilder) WithResourceLimits(limits corev1.ResourceList) *ContainerBuilder {
	b.container.Resources.Limits = limits
	return b
}

// WithResourceRequests sets resource requests
func (b *ContainerBuilder) WithResourceRequests(requests corev1.ResourceList) *ContainerBuilder {
	b.container.Resources.Requests = requests
	return b
}

// WithSecurityContext sets the container security context
func (b *ContainerBuilder) WithSecurityContext(ctx *corev1.SecurityContext) *ContainerBuilder {
	b.container.SecurityContext = ctx
	return b
}

// WithReadinessProbe sets the readiness probe
func (b *ContainerBuilder) WithReadinessProbe(probe *corev1.Probe) *ContainerBuilder {
	b.container.ReadinessProbe = probe
	return b
}

// WithLivenessProbe sets the liveness probe
func (b *ContainerBuilder) WithLivenessProbe(probe *corev1.Probe) *ContainerBuilder {
	b.container.LivenessProbe = probe
	return b
}

// WithStartupProbe sets the startup probe
func (b *ContainerBuilder) WithStartupProbe(probe *corev1.Probe) *ContainerBuilder {
	b.container.StartupProbe = probe
	return b
}

// WithHTTPReadinessProbe adds an HTTP readiness probe
func (b *ContainerBuilder) WithHTTPReadinessProbe(path string, port int, initialDelay, period int32) *ContainerBuilder {
	return b.WithReadinessProbe(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt(port),
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
	})
}

// WithHTTPLivenessProbe adds an HTTP liveness probe
func (b *ContainerBuilder) WithHTTPLivenessProbe(path string, port int, initialDelay, period int32) *ContainerBuilder {
	return b.WithLivenessProbe(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt(port),
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
	})
}

// WithTCPReadinessProbe adds a TCP readiness probe
func (b *ContainerBuilder) WithTCPReadinessProbe(port int, initialDelay, period int32) *ContainerBuilder {
	return b.WithReadinessProbe(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(port),
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
	})
}

// WithExecReadinessProbe adds an exec readiness probe
func (b *ContainerBuilder) WithExecReadinessProbe(command []string, initialDelay, period int32) *ContainerBuilder {
	return b.WithReadinessProbe(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: command,
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
	})
}

// WithImagePullPolicy sets the image pull policy
func (b *ContainerBuilder) WithImagePullPolicy(policy corev1.PullPolicy) *ContainerBuilder {
	b.container.ImagePullPolicy = policy
	return b
}

// WithWorkingDir sets the working directory
func (b *ContainerBuilder) WithWorkingDir(dir string) *ContainerBuilder {
	b.container.WorkingDir = dir
	return b
}

// WithLifecycle sets the container lifecycle
func (b *ContainerBuilder) WithLifecycle(lifecycle *corev1.Lifecycle) *ContainerBuilder {
	b.container.Lifecycle = lifecycle
	return b
}

// Build returns the constructed Container
func (b *ContainerBuilder) Build() corev1.Container {
	return b.container
}
