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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DefaultJobTTL is the default TTL for completed jobs (1 hour)
const DefaultJobTTL int32 = 3600

// JobBuilder provides a fluent interface for building Jobs
type JobBuilder struct {
	job    *batchv1.Job
	owner  metav1.Object
	scheme *runtime.Scheme
}

// NewJobBuilder creates a new JobBuilder
func NewJobBuilder(name, namespace string) *JobBuilder {
	ttl := DefaultJobTTL
	return &JobBuilder{
		job: &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    make(map[string]string),
			},
			Spec: batchv1.JobSpec{
				TTLSecondsAfterFinished: &ttl,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: make(map[string]string),
					},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers:    []corev1.Container{},
						Volumes:       []corev1.Volume{},
					},
				},
			},
		},
	}
}

// WithOwner sets the owner reference for garbage collection
func (b *JobBuilder) WithOwner(owner metav1.Object, scheme *runtime.Scheme) *JobBuilder {
	b.owner = owner
	b.scheme = scheme
	return b
}

// WithLabels sets labels on the job and pod template
func (b *JobBuilder) WithLabels(labels map[string]string) *JobBuilder {
	for k, v := range labels {
		b.job.Labels[k] = v
		b.job.Spec.Template.Labels[k] = v
	}
	return b
}

// WithAnnotations sets annotations on the job
func (b *JobBuilder) WithAnnotations(annotations map[string]string) *JobBuilder {
	if b.job.Annotations == nil {
		b.job.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.job.Annotations[k] = v
	}
	return b
}

// WithTTL sets the TTL after job completion
func (b *JobBuilder) WithTTL(seconds int32) *JobBuilder {
	b.job.Spec.TTLSecondsAfterFinished = &seconds
	return b
}

// WithBackoffLimit sets the backoff limit for retries
func (b *JobBuilder) WithBackoffLimit(limit int32) *JobBuilder {
	b.job.Spec.BackoffLimit = &limit
	return b
}

// WithActiveDeadline sets the active deadline for the job
func (b *JobBuilder) WithActiveDeadline(seconds int64) *JobBuilder {
	b.job.Spec.ActiveDeadlineSeconds = &seconds
	return b
}

// WithCompletions sets the number of successful completions required
func (b *JobBuilder) WithCompletions(completions int32) *JobBuilder {
	b.job.Spec.Completions = &completions
	return b
}

// WithParallelism sets the parallelism for the job
func (b *JobBuilder) WithParallelism(parallelism int32) *JobBuilder {
	b.job.Spec.Parallelism = &parallelism
	return b
}

// WithContainer adds a container to the job
func (b *JobBuilder) WithContainer(container corev1.Container) *JobBuilder {
	b.job.Spec.Template.Spec.Containers = append(
		b.job.Spec.Template.Spec.Containers,
		container,
	)
	return b
}

// WithInitContainer adds an init container
func (b *JobBuilder) WithInitContainer(container corev1.Container) *JobBuilder {
	b.job.Spec.Template.Spec.InitContainers = append(
		b.job.Spec.Template.Spec.InitContainers,
		container,
	)
	return b
}

// WithVolume adds a volume
func (b *JobBuilder) WithVolume(volume corev1.Volume) *JobBuilder {
	b.job.Spec.Template.Spec.Volumes = append(
		b.job.Spec.Template.Spec.Volumes,
		volume,
	)
	return b
}

// WithPVCVolume adds a PVC-backed volume
func (b *JobBuilder) WithPVCVolume(name, claimName string) *JobBuilder {
	return b.WithVolume(corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		},
	})
}

// WithSecretVolume adds a Secret-backed volume
func (b *JobBuilder) WithSecretVolume(name, secretName string, mode *int32) *JobBuilder {
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

// WithPodSecurityContext sets the pod security context
func (b *JobBuilder) WithPodSecurityContext(ctx *corev1.PodSecurityContext) *JobBuilder {
	b.job.Spec.Template.Spec.SecurityContext = ctx
	return b
}

// WithRestartPolicy sets the restart policy
func (b *JobBuilder) WithRestartPolicy(policy corev1.RestartPolicy) *JobBuilder {
	b.job.Spec.Template.Spec.RestartPolicy = policy
	return b
}

// WithServiceAccountName sets the service account
func (b *JobBuilder) WithServiceAccountName(name string) *JobBuilder {
	b.job.Spec.Template.Spec.ServiceAccountName = name
	return b
}

// WithImagePullSecrets sets image pull secrets
func (b *JobBuilder) WithImagePullSecrets(secrets []corev1.LocalObjectReference) *JobBuilder {
	b.job.Spec.Template.Spec.ImagePullSecrets = secrets
	return b
}

// WithNodeSelector sets the node selector
func (b *JobBuilder) WithNodeSelector(selector map[string]string) *JobBuilder {
	b.job.Spec.Template.Spec.NodeSelector = selector
	return b
}

// WithTolerations sets tolerations
func (b *JobBuilder) WithTolerations(tolerations []corev1.Toleration) *JobBuilder {
	b.job.Spec.Template.Spec.Tolerations = tolerations
	return b
}

// WithAffinity sets affinity rules
func (b *JobBuilder) WithAffinity(affinity *corev1.Affinity) *JobBuilder {
	b.job.Spec.Template.Spec.Affinity = affinity
	return b
}

// WithExtraPodLabels adds extra labels to the pod template
func (b *JobBuilder) WithExtraPodLabels(labels map[string]string) *JobBuilder {
	if b.job.Spec.Template.Labels == nil {
		b.job.Spec.Template.Labels = make(map[string]string)
	}
	for k, v := range labels {
		b.job.Spec.Template.Labels[k] = v
	}
	return b
}

// Build returns the constructed Job
func (b *JobBuilder) Build() (*batchv1.Job, error) {
	if b.owner != nil && b.scheme != nil {
		if err := controllerutil.SetControllerReference(b.owner, b.job, b.scheme); err != nil {
			return nil, err
		}
	}
	return b.job, nil
}

// MustBuild returns the Job or panics on error
func (b *JobBuilder) MustBuild() *batchv1.Job {
	j, err := b.Build()
	if err != nil {
		panic(err)
	}
	return j
}
