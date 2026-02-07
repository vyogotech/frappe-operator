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
	"reflect"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureWorkers ensures all Worker Deployments exist
func (r *FrappeBenchReconciler) ensureWorkers(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	workerTypes := []string{"default", "long", "short"}
	
	for _, typeName := range workerTypes {
		componentName := fmt.Sprintf("worker-%s", typeName)
		queue := typeName
		
		// Get autoscaling config
		config := r.getComponentAutoscaling(bench, componentName)
		config = r.fillComponentDefaults(config, componentName)

		// Determine which provider to use
		var provider ScalingProvider
		if config.Enabled != nil && *config.Enabled && config.Provider != "" {
			provider = resolveProvider(config.Provider, r.Client, r.Scheme)
		}

		providerAvailable := false
		if provider != nil {
			providerAvailable = provider.IsAvailable(ctx)
		}

		// Determine replica count
		replicas := r.getComponentReplicaCount(config, providerAvailable)

		// Create/update deployment
		deployName := fmt.Sprintf("%s-%s", bench.Name, componentName)
		if err := r.ensureWorkerDeployment(ctx, bench, componentName, queue, replicas, config, providerAvailable); err != nil {
			return err
		}

		// Handle scaling provider
		if providerAvailable {
			if err := provider.Ensure(ctx, bench, componentName, deployName, config); err != nil {
				logger.Error(err, "Failed to ensure scaling for component", "component", componentName, "provider", provider.Name())
			}
		} else {
			// Clean up both possible providers if autoscaling is disabled or provider not available
			_ = resolveProvider("keda", r.Client, r.Scheme).Delete(ctx, bench, componentName)
			_ = resolveProvider("hpa", r.Client, r.Scheme).Delete(ctx, bench, componentName)
		}
	}

	return nil
}

func (r *FrappeBenchReconciler) ensureWorkerDeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, componentName, queue string, replicas int32, config *vyogotechv1alpha1.ComponentAutoscaling, providerManaged bool) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-%s", bench.Name, componentName)
	deploy := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	managedByProvider := providerManaged && config.Enabled != nil && *config.Enabled
	workerResources := r.getWorkerResources(bench, componentName)

	if err == nil {
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating worker image", "component", componentName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		// Only update replicas if NOT managed by an external provider
		if !managedByProvider && *deploy.Spec.Replicas != replicas {
			logger.Info("Updating worker replicas", "component", componentName, "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
			deploy.Spec.Replicas = &replicas
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, workerResources) {
			logger.Info("Updating worker resources", "component", componentName)
			deploy.Spec.Template.Spec.Containers[0].Resources = workerResources
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

	logger.Info("Creating Worker Deployment", "deployment", deployName, "queue", queue, "replicas", replicas, "managedByProvider", managedByProvider)

	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	annotations := map[string]string{}
	if managedByProvider {
		annotations["frappe.io/managed-by-provider"] = config.Provider
	}

	container := resources.NewContainerBuilder("worker", image).
		WithArgs("bench", "worker", "--queue", queue).
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(workerResources).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, componentName)).
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

func (r *FrappeBenchReconciler) getWorkerResources(bench *vyogotechv1alpha1.FrappeBench, componentName string) corev1.ResourceRequirements {
	switch componentName {
	case "worker-long":
		return r.getWorkerLongResources(bench)
	case "worker-short":
		return r.getWorkerShortResources(bench)
	default:
		return r.getWorkerDefaultResources(bench)
	}
}
