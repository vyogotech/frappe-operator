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
	"fmt"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureWorkers ensures all Worker Deployments exist
func (r *FrappeBenchReconciler) ensureWorkers(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	// Check KEDA availability once
	kedaAvailable := r.isKEDAAvailable(ctx)
	if !kedaAvailable {
		logger.Info("KEDA not available, workers will use static replicas")
	}

	workers := []struct {
		name      string
		queue     string
		resources func(*vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements
	}{
		{"default", "default", r.getWorkerDefaultResources},
		{"long", "long", r.getWorkerLongResources},
		{"short", "short", r.getWorkerShortResources},
	}

	for _, worker := range workers {
		// Get autoscaling config for this worker
		config := r.getWorkerAutoscalingConfig(bench, worker.name)
		config = r.fillAutoscalingDefaults(config, worker.name)

		// Determine replica count based on scaling mode
		replicas := r.getWorkerReplicaCount(config, kedaAvailable)

		// Create/update worker deployment
		if err := r.ensureWorkerDeployment(ctx, bench, worker.name, worker.queue, replicas, worker.resources(bench), config, kedaAvailable); err != nil {
			return err
		}

		// Create/update ScaledObject if autoscaling is enabled
		if err := r.ensureScaledObject(ctx, bench, worker.name, config); err != nil {
			logger.Error(err, "Failed to ensure ScaledObject", "worker", worker.name)
			// Don't fail the reconciliation, just log the error
		}
	}

	return nil
}

func (r *FrappeBenchReconciler) ensureWorkerDeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, workerType, queue string, replicas int32, workerResources corev1.ResourceRequirements, config *vyogotechv1alpha1.WorkerAutoscaling, kedaAvailable bool) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)
	deploy := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	// Determine if this worker is managed by KEDA
	kedaManaged := kedaAvailable && config.Enabled != nil && *config.Enabled

	if err == nil {
		// Deployment exists, update it if needed
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating worker image", "worker", workerType, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		// Only update replicas if NOT managed by KEDA (KEDA controls replicas)
		if !kedaManaged && *deploy.Spec.Replicas != replicas {
			logger.Info("Updating worker replicas", "worker", workerType, "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
			deploy.Spec.Replicas = &replicas
			changed = true
		}

		if changed {
			return r.Update(ctx, deploy)
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Worker Deployment", "deployment", deployName, "queue", queue, "replicas", replicas, "kedaManaged", kedaManaged)

	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	// Add annotations to indicate scaling mode
	annotations := map[string]string{}
	if kedaManaged {
		annotations["frappe.io/scaling-mode"] = "autoscaled"
		annotations["keda.sh/managed-by"] = "keda"
	} else {
		annotations["frappe.io/scaling-mode"] = "static"
	}

	container := resources.NewContainerBuilder("worker", image).
		WithArgs("bench", "worker", "--queue", queue).
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithResources(workerResources).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	// Apply Pod Config
	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, fmt.Sprintf("worker-%s", workerType))).
		WithAnnotations(annotations).
		WithReplicas(replicas).
		WithNodeSelector(nodeSelector).
		WithAffinity(affinity).
		WithTolerations(tolerations).
		WithPodSecurityContext(r.getPodSecurityContext(ctx, bench)).
		WithContainer(container).
		WithPVCVolume("sites", pvcName).
		WithOwner(bench, r.Scheme).
		Build()
	if err != nil {
		return err
	}

	return r.Create(ctx, deploy)
}

// isKEDAAvailable checks if KEDA CRDs are installed
func (r *FrappeBenchReconciler) isKEDAAvailable(ctx context.Context) bool {
	// Create a minimal unstructured list to check if the resource exists
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})

	// Attempt to list - if this succeeds, KEDA is available
	err := r.Client.List(ctx, list, client.Limit(1))

	// NoMatchError means the CRD doesn't exist
	if errors.IsNotFound(err) {
		return false
	}

	// Any other error or success means KEDA is likely available
	// We don't care about permission errors - just whether the CRD exists
	return true
}

// ensureScaledObject creates or updates a KEDA ScaledObject for a worker
func (r *FrappeBenchReconciler) ensureScaledObject(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, workerType string, config *vyogotechv1alpha1.WorkerAutoscaling) error {
	logger := log.FromContext(ctx)

	// Skip if KEDA is not enabled for this worker
	if config.Enabled == nil || !*config.Enabled {
		// Clean up any existing ScaledObject
		return r.deleteScaledObjectIfExists(ctx, bench, workerType)
	}

	// Check if KEDA is available
	if !r.isKEDAAvailable(ctx) {
		logger.Info("KEDA not available, skipping ScaledObject creation", "worker", workerType)
		return nil
	}

	scaledObjectName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)
	deploymentName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)
	queueName := fmt.Sprintf("rq:queue:%s", workerType)

	// Build the ScaledObject using unstructured
	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})
	scaledObject.SetName(scaledObjectName)
	scaledObject.SetNamespace(bench.Namespace)
	scaledObject.SetLabels(r.componentLabels(bench, fmt.Sprintf("worker-%s", workerType)))

	// Build spec
	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       deploymentName,
		},
		"minReplicaCount": int64(*config.MinReplicas),
		"maxReplicaCount": int64(*config.MaxReplicas),
		"cooldownPeriod":  int64(*config.CooldownPeriod),
		"pollingInterval": int64(*config.PollingInterval),
		"triggers": []interface{}{
			map[string]interface{}{
				"type": "redis",
				"metadata": map[string]interface{}{
					"address":              r.getRedisAddress(bench),
					"listName":             queueName,
					"listLength":           fmt.Sprintf("%d", *config.QueueLength),
					"enableTLS":            "false",
					"databaseIndex":        "0",
					"activationListLength": "1",
				},
			},
		},
	}

	if err := unstructured.SetNestedField(scaledObject.Object, spec, "spec"); err != nil {
		return fmt.Errorf("failed to set ScaledObject spec: %w", err)
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(bench, scaledObject, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(scaledObject.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Creating ScaledObject", "worker", workerType, "name", scaledObjectName)
			return r.Create(ctx, scaledObject)
		}
		return err
	}

	// Update existing
	scaledObject.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating ScaledObject", "worker", workerType, "name", scaledObjectName)
	return r.Update(ctx, scaledObject)
}

// deleteScaledObjectIfExists deletes a ScaledObject if it exists
func (r *FrappeBenchReconciler) deleteScaledObjectIfExists(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, workerType string) error {
	logger := log.FromContext(ctx)

	scaledObjectName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)

	scaledObject := &unstructured.Unstructured{}
	scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	})

	err := r.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, scaledObject)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // Already deleted
		}
		return err
	}

	logger.Info("Deleting ScaledObject", "worker", workerType, "name", scaledObjectName)
	return r.Delete(ctx, scaledObject)
}
