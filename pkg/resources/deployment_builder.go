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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeploymentBuilder provides a fluent interface for building Deployments
type DeploymentBuilder struct {
	deployment *appsv1.Deployment
	owner      metav1.Object
	scheme     *runtime.Scheme
}

// NewDeploymentBuilder creates a new DeploymentBuilder
func NewDeploymentBuilder(name, namespace string) *DeploymentBuilder {
	return &DeploymentBuilder{
		deployment: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    make(map[string]string),
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: make(map[string]string),
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: make(map[string]string),
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{},
						Volumes:    []corev1.Volume{},
					},
				},
			},
		},
	}
}

// WithOwner sets the owner reference for garbage collection
func (b *DeploymentBuilder) WithOwner(owner metav1.Object, scheme *runtime.Scheme) *DeploymentBuilder {
	b.owner = owner
	b.scheme = scheme
	return b
}

// WithLabels sets labels on the deployment and pod template
func (b *DeploymentBuilder) WithLabels(labels map[string]string) *DeploymentBuilder {
	for k, v := range labels {
		b.deployment.Labels[k] = v
		b.deployment.Spec.Template.Labels[k] = v
	}
	return b
}

// WithSelector sets the selector labels
func (b *DeploymentBuilder) WithSelector(selector map[string]string) *DeploymentBuilder {
	for k, v := range selector {
		b.deployment.Spec.Selector.MatchLabels[k] = v
		b.deployment.Spec.Template.Labels[k] = v
	}
	return b
}

// WithAnnotations sets annotations on the deployment
func (b *DeploymentBuilder) WithAnnotations(annotations map[string]string) *DeploymentBuilder {
	if b.deployment.Annotations == nil {
		b.deployment.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.deployment.Annotations[k] = v
	}
	return b
}

// WithReplicas sets the replica count
func (b *DeploymentBuilder) WithReplicas(replicas int32) *DeploymentBuilder {
	b.deployment.Spec.Replicas = &replicas
	return b
}

// WithContainer adds a container to the deployment
func (b *DeploymentBuilder) WithContainer(container corev1.Container) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.Containers = append(
		b.deployment.Spec.Template.Spec.Containers,
		container,
	)
	return b
}

// WithInitContainer adds an init container
func (b *DeploymentBuilder) WithInitContainer(container corev1.Container) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.InitContainers = append(
		b.deployment.Spec.Template.Spec.InitContainers,
		container,
	)
	return b
}

// WithVolume adds a volume
func (b *DeploymentBuilder) WithVolume(volume corev1.Volume) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.Volumes = append(
		b.deployment.Spec.Template.Spec.Volumes,
		volume,
	)
	return b
}

// WithPVCVolume adds a PVC-backed volume
func (b *DeploymentBuilder) WithPVCVolume(name, claimName string) *DeploymentBuilder {
	return b.WithVolume(corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		},
	})
}

// WithConfigMapVolume adds a ConfigMap-backed volume
func (b *DeploymentBuilder) WithConfigMapVolume(name, configMapName string) *DeploymentBuilder {
	return b.WithVolume(corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	})
}

// WithSecretVolume adds a Secret-backed volume
func (b *DeploymentBuilder) WithSecretVolume(name, secretName string, mode *int32) *DeploymentBuilder {
	return b.WithVolume(corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: mode,
			},
		},
	})
}

// WithEmptyDirVolume adds an EmptyDir volume
func (b *DeploymentBuilder) WithEmptyDirVolume(name string) *DeploymentBuilder {
	return b.WithVolume(corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
}

// WithPodSecurityContext sets the pod security context
func (b *DeploymentBuilder) WithPodSecurityContext(ctx *corev1.PodSecurityContext) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.SecurityContext = ctx
	return b
}

// WithServiceAccountName sets the service account
func (b *DeploymentBuilder) WithServiceAccountName(name string) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.ServiceAccountName = name
	return b
}

// WithImagePullSecrets sets image pull secrets
func (b *DeploymentBuilder) WithImagePullSecrets(secrets []corev1.LocalObjectReference) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.ImagePullSecrets = secrets
	return b
}

// WithNodeSelector sets the node selector
func (b *DeploymentBuilder) WithNodeSelector(selector map[string]string) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.NodeSelector = selector
	return b
}

// WithTolerations sets tolerations
func (b *DeploymentBuilder) WithTolerations(tolerations []corev1.Toleration) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.Tolerations = tolerations
	return b
}

// WithAffinity sets affinity rules
func (b *DeploymentBuilder) WithAffinity(affinity *corev1.Affinity) *DeploymentBuilder {
	b.deployment.Spec.Template.Spec.Affinity = affinity
	return b
}

// WithStrategy sets the deployment strategy
func (b *DeploymentBuilder) WithStrategy(strategy appsv1.DeploymentStrategy) *DeploymentBuilder {
	b.deployment.Spec.Strategy = strategy
	return b
}

// WithRollingUpdateStrategy sets a rolling update strategy with max surge and unavailable
func (b *DeploymentBuilder) WithRollingUpdateStrategy(maxSurge, maxUnavailable int) *DeploymentBuilder {
	return b.WithStrategy(appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxSurge:       intOrStringFromInt(maxSurge),
			MaxUnavailable: intOrStringFromInt(maxUnavailable),
		},
	})
}

// Build returns the constructed Deployment
func (b *DeploymentBuilder) Build() (*appsv1.Deployment, error) {
	if b.owner != nil && b.scheme != nil {
		if err := controllerutil.SetControllerReference(b.owner, b.deployment, b.scheme); err != nil {
			return nil, err
		}
	}
	return b.deployment, nil
}

// MustBuild returns the Deployment or panics on error
func (b *DeploymentBuilder) MustBuild() *appsv1.Deployment {
	d, err := b.Build()
	if err != nil {
		panic(err)
	}
	return d
}
