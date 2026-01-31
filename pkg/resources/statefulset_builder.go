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

// StatefulSetBuilder provides a fluent interface for building StatefulSets
type StatefulSetBuilder struct {
	sts    *appsv1.StatefulSet
	owner  metav1.Object
	scheme *runtime.Scheme
}

// NewStatefulSetBuilder creates a new StatefulSetBuilder
func NewStatefulSetBuilder(name, namespace string) *StatefulSetBuilder {
	return &StatefulSetBuilder{
		sts: &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    make(map[string]string),
			},
			Spec: appsv1.StatefulSetSpec{
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
func (b *StatefulSetBuilder) WithOwner(owner metav1.Object, scheme *runtime.Scheme) *StatefulSetBuilder {
	b.owner = owner
	b.scheme = scheme
	return b
}

// WithLabels sets labels on the statefulset and pod template
func (b *StatefulSetBuilder) WithLabels(labels map[string]string) *StatefulSetBuilder {
	for k, v := range labels {
		b.sts.Labels[k] = v
		b.sts.Spec.Template.Labels[k] = v
	}
	return b
}

// WithSelector sets the selector labels
func (b *StatefulSetBuilder) WithSelector(selector map[string]string) *StatefulSetBuilder {
	for k, v := range selector {
		b.sts.Spec.Selector.MatchLabels[k] = v
		b.sts.Spec.Template.Labels[k] = v
	}
	return b
}

// WithServiceName sets the service name for the statefulset
func (b *StatefulSetBuilder) WithServiceName(name string) *StatefulSetBuilder {
	b.sts.Spec.ServiceName = name
	return b
}

// WithReplicas sets the replica count
func (b *StatefulSetBuilder) WithReplicas(replicas int32) *StatefulSetBuilder {
	b.sts.Spec.Replicas = &replicas
	return b
}

// WithContainer adds a container to the statefulset
func (b *StatefulSetBuilder) WithContainer(container corev1.Container) *StatefulSetBuilder {
	b.sts.Spec.Template.Spec.Containers = append(
		b.sts.Spec.Template.Spec.Containers,
		container,
	)
	return b
}

// WithPodSecurityContext sets the pod security context
func (b *StatefulSetBuilder) WithPodSecurityContext(ctx *corev1.PodSecurityContext) *StatefulSetBuilder {
	b.sts.Spec.Template.Spec.SecurityContext = ctx
	return b
}

// Build returns the constructed StatefulSet
func (b *StatefulSetBuilder) Build() (*appsv1.StatefulSet, error) {
	if b.owner != nil && b.scheme != nil {
		if err := controllerutil.SetControllerReference(b.owner, b.sts, b.scheme); err != nil {
			return nil, err
		}
	}
	return b.sts, nil
}

// MustBuild returns the StatefulSet or panics on error
func (b *StatefulSetBuilder) MustBuild() *appsv1.StatefulSet {
	s, err := b.Build()
	if err != nil {
		panic(err)
	}
	return s
}
