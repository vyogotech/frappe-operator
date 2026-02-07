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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureGunicorn ensures the Gunicorn Deployment and Service exist
func (r *FrappeBenchReconciler) ensureGunicorn(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.ensureGunicornService(ctx, bench); err != nil {
		return err
	}
	return r.ensureGunicornDeployment(ctx, bench)
}

func (r *FrappeBenchReconciler) ensureGunicornService(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	svcName := fmt.Sprintf("%s-gunicorn", bench.Name)
	svc := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: bench.Namespace}, svc)
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Gunicorn Service", "service", svcName)

	// Apply Pod Config (Labels only for Service)
	_, _, _, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	svc, err = resources.NewServiceBuilder(svcName, bench.Namespace).
		WithLabels(extraLabels).
		WithSelector(r.componentLabels(bench, "gunicorn")).
		WithPort("http", 8000, 8000).
		WithOwner(bench, r.Scheme).
		Build()
	if err != nil {
		return err
	}

	return r.Create(ctx, svc)
}

func (r *FrappeBenchReconciler) ensureGunicornDeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-gunicorn", bench.Name)
	deploy := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)
	if err == nil {
		// Update existing deployment if image or resources have changed
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Gunicorn Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		resources := r.getGunicornResources(bench)
		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, resources) {
			logger.Info("Updating Gunicorn Deployment resources", "deployment", deployName)
			deploy.Spec.Template.Spec.Containers[0].Resources = resources
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

	logger.Info("Creating Gunicorn Deployment", "deployment", deployName)

	replicas := r.getGunicornReplicas(bench)
	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	container := resources.NewContainerBuilder("gunicorn", image).
		WithPort("http", 8000).
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(r.getGunicornResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	// Apply Pod Config
	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, "gunicorn")).
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

// ensureNginx ensures the NGINX Deployment and Service exist
func (r *FrappeBenchReconciler) ensureNginx(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	// Check KEDA availability once
	kedaAvailable := r.isKEDAAvailable(ctx)

	// Get autoscaling config for nginx
	config := r.getNginxAutoscalingConfig(bench)
	config = r.fillNginxAutoscalingDefaults(config)

	// Determine replica count based on scaling mode
	replicas := r.getNginxReplicaCount(config, kedaAvailable)

	if err := r.ensureNginxService(ctx, bench); err != nil {
		return err
	}

	if err := r.ensureNginxDeployment(ctx, bench, replicas, config, kedaAvailable); err != nil {
		return err
	}

	// Create/update ScaledObject if autoscaling is enabled
	if err := r.ensureNginxScaledObject(ctx, bench, config); err != nil {
		logger.Error(err, "Failed to ensure nginx ScaledObject")
		// Don't fail the reconciliation, just log the error
	}

	return nil
}

func (r *FrappeBenchReconciler) ensureNginxService(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	svcName := fmt.Sprintf("%s-nginx", bench.Name)
	svc := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: bench.Namespace}, svc)
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating NGINX Service", "service", svcName)

	// Apply Pod Config (Labels only for Service)
	_, _, _, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	svc, err = resources.NewServiceBuilder(svcName, bench.Namespace).
		WithLabels(extraLabels).
		WithSelector(r.componentLabels(bench, "nginx")).
		WithPort("http", 8080, 8080).
		WithOwner(bench, r.Scheme).
		Build()
	if err != nil {
		return err
	}

	return r.Create(ctx, svc)
}

func (r *FrappeBenchReconciler) ensureNginxDeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, replicas int32, config *vyogotechv1alpha1.NginxAutoscaling, kedaAvailable bool) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-nginx", bench.Name)
	deploy := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	// Determine if this nginx is managed by KEDA
	kedaManaged := kedaAvailable && config.Enabled != nil && *config.Enabled

	if err == nil {
		// Update existing deployment if image or resources have changed
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating NGINX Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		resources := r.getNginxResources(bench)
		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, resources) {
			logger.Info("Updating NGINX Deployment resources", "deployment", deployName)
			deploy.Spec.Template.Spec.Containers[0].Resources = resources
			changed = true
		}

		// Only update replicas if NOT managed by KEDA
		if !kedaManaged && deploy.Spec.Replicas != nil && *deploy.Spec.Replicas != replicas {
			logger.Info("Updating NGINX Deployment replicas", "deployment", deployName, "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
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

	logger.Info("Creating NGINX Deployment", "deployment", deployName, "replicas", replicas, "kedaManaged", kedaManaged)

	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	gunicornSvc := fmt.Sprintf("%s-gunicorn", bench.Name)

	container := resources.NewContainerBuilder("nginx", image).
		WithArgs("nginx-entrypoint.sh").
		WithPort("http", 8080).
		WithEnv("BACKEND", fmt.Sprintf("%s:8000", gunicornSvc)).
		WithEnv("SOCKETIO", fmt.Sprintf("%s-socketio:9000", bench.Name)).
		WithEnv("UPSTREAM_REAL_IP_ADDRESS", "127.0.0.1").
		WithEnv("UPSTREAM_REAL_IP_RECURSIVE", "off").
		WithEnv("UPSTREAM_REAL_IP_HEADER", "X-Forwarded-For").
		WithEnv("FRAPPE_SITE_NAME_HEADER", "$host").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(r.getNginxResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		Build()

	// Apply Pod Config
	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, "nginx")).
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

// ensureSocketIO ensures the Socket.IO Deployment and Service exist
func (r *FrappeBenchReconciler) ensureSocketIO(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.ensureSocketIOService(ctx, bench); err != nil {
		return err
	}
	return r.ensureSocketIODeployment(ctx, bench)
}

func (r *FrappeBenchReconciler) ensureSocketIOService(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	svcName := fmt.Sprintf("%s-socketio", bench.Name)
	svc := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: bench.Namespace}, svc)
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Socket.IO Service", "service", svcName)

	// Apply Pod Config (Labels only for Service)
	_, _, _, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	svc, err = resources.NewServiceBuilder(svcName, bench.Namespace).
		WithLabels(extraLabels).
		WithSelector(r.componentLabels(bench, "socketio")).
		WithPort("socketio", 9000, 9000).
		WithOwner(bench, r.Scheme).
		Build()
	if err != nil {
		return err
	}

	return r.Create(ctx, svc)
}

func (r *FrappeBenchReconciler) ensureSocketIODeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-socketio", bench.Name)
	deploy := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)
	if err == nil {
		// Update existing deployment if image or resources have changed
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Socket.IO Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		resources := r.getSocketIOResources(bench)
		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, resources) {
			logger.Info("Updating Socket.IO Deployment resources", "deployment", deployName)
			deploy.Spec.Template.Spec.Containers[0].Resources = resources
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

	logger.Info("Creating Socket.IO Deployment", "deployment", deployName)

	replicas := r.getSocketIOReplicas(bench)
	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	container := resources.NewContainerBuilder("socketio", image).
		WithArgs("node", "/home/frappe/frappe-bench/apps/frappe/socketio.js").
		WithPort("socketio", 9000).
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(r.getSocketIOResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	// Apply Pod Config
	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, "socketio")).
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

// ensureScheduler ensures the Scheduler Deployment exists
func (r *FrappeBenchReconciler) ensureScheduler(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-scheduler", bench.Name)
	deploy := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)
	if err == nil {
		// Update existing deployment if image or resources have changed
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Scheduler Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		resources := r.getSchedulerResources(bench)
		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, resources) {
			logger.Info("Updating Scheduler Deployment resources", "deployment", deployName)
			deploy.Spec.Template.Spec.Containers[0].Resources = resources
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

	logger.Info("Creating Scheduler Deployment", "deployment", deployName)

	replicas := int32(1) // Scheduler should only have 1 replica
	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	container := resources.NewContainerBuilder("scheduler", image).
		WithArgs("bench", "schedule").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(r.getSchedulerResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	// Apply Pod Config
	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, "scheduler")).
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

// ensureNginxScaledObject ensures the KEDA ScaledObject for nginx exists
func (r *FrappeBenchReconciler) ensureNginxScaledObject(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, config *vyogotechv1alpha1.NginxAutoscaling) error {
logger := log.FromContext(ctx)

// Skip if KEDA is not enabled for nginx
if config.Enabled == nil || !*config.Enabled {
// Clean up any existing ScaledObject
return r.deleteNginxScaledObjectIfExists(ctx, bench)
}

// Check if KEDA is available
if !r.isKEDAAvailable(ctx) {
logger.Info("KEDA not available, skipping nginx ScaledObject creation")
return nil
}

scaledObjectName := fmt.Sprintf("%s-nginx", bench.Name)
deploymentName := fmt.Sprintf("%s-nginx", bench.Name)

// Build the ScaledObject using unstructured
scaledObject := &unstructured.Unstructured{}
scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
Group:   "keda.sh",
Version: "v1alpha1",
Kind:    "ScaledObject",
})
scaledObject.SetName(scaledObjectName)
scaledObject.SetNamespace(bench.Namespace)
scaledObject.SetLabels(r.componentLabels(bench, "nginx"))

// Build trigger based on metric type
var trigger map[string]interface{}
if config.MetricType == "memory" {
trigger = map[string]interface{}{
"type": "memory",
"metricType": "Utilization",
"metadata": map[string]interface{}{
"value": config.TargetAverageValue,
},
}
} else {
// Default to CPU
trigger = map[string]interface{}{
"type": "cpu",
"metricType": "Utilization",
"metadata": map[string]interface{}{
"value": config.TargetAverageValue,
},
}
}

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
trigger,
},
}

if err := unstructured.SetNestedField(scaledObject.Object, spec, "spec"); err != nil {
return fmt.Errorf("failed to set nginx ScaledObject spec: %w", err)
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
logger.Info("Creating nginx ScaledObject", "name", scaledObjectName)
return r.Create(ctx, scaledObject)
}
return err
}

// Update existing
scaledObject.SetResourceVersion(existing.GetResourceVersion())
logger.Info("Updating nginx ScaledObject", "name", scaledObjectName)
return r.Update(ctx, scaledObject)
}

// deleteNginxScaledObjectIfExists deletes nginx ScaledObject if it exists
func (r *FrappeBenchReconciler) deleteNginxScaledObjectIfExists(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
logger := log.FromContext(ctx)

scaledObjectName := fmt.Sprintf("%s-nginx", bench.Name)

scaledObject := &unstructured.Unstructured{}
scaledObject.SetGroupVersionKind(schema.GroupVersionKind{
Group:   "keda.sh",
Version: "v1alpha1",
Kind:    "ScaledObject",
})

err := r.Get(ctx, types.NamespacedName{Name: scaledObjectName, Namespace: bench.Namespace}, scaledObject)
if err != nil {
if meta.IsNoMatchError(err) || errors.IsNotFound(err) {
return nil // Already deleted or CRD doesn't exist
}
return err
}

logger.Info("Deleting nginx ScaledObject", "name", scaledObjectName)
return r.Delete(ctx, scaledObject)
}
