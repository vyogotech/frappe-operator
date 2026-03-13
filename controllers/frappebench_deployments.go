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

const (
	// MetricTypeCPU is the CPU metric type for autoscaling
	MetricTypeCPU = "cpu"
	// MetricTypeMemory is the memory metric type for autoscaling
	MetricTypeMemory = "memory"
)

// ensureSocketIO ensures the Socket.IO Deployment and Service exist
func (r *FrappeBenchReconciler) ensureSocketIO(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.ensureSocketIOService(ctx, bench); err != nil {
		return err
	}
	return r.ensureSocketIODeployment(ctx, bench)
}

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

func (r *FrappeBenchReconciler) ensureGunicornDeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)
	componentName := "gunicorn"
	deployName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	config := r.getComponentAutoscaling(bench, componentName)
	config = r.fillComponentDefaults(config, componentName)

	var provider ScalingProvider
	if config.Enabled != nil && *config.Enabled && config.Provider != "" {
		provider = resolveProvider(config.Provider, r.Client, r.Scheme)
	}

	providerAvailable := false
	if provider != nil {
		providerAvailable = provider.IsAvailable(ctx)
	}

	replicas := r.getComponentReplicaCount(config, providerAvailable)

	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	managedByProvider := providerAvailable && config.Enabled != nil && *config.Enabled
	gunicornResources := r.getGunicornResources(bench)

	if err == nil {
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Gunicorn image", "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		if !managedByProvider && *deploy.Spec.Replicas != replicas {
			logger.Info("Updating Gunicorn replicas", "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
			deploy.Spec.Replicas = &replicas
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, gunicornResources) {
			logger.Info("Updating Gunicorn resources")
			deploy.Spec.Template.Spec.Containers[0].Resources = gunicornResources
			changed = true
		}

		if changed {
			return r.Update(ctx, deploy)
		}

		// Handle scaling provider
		if managedByProvider {
			r.cleanupOtherProviders(ctx, bench, componentName, config.Provider)
			return provider.Ensure(ctx, bench, componentName, deployName, config)
		} else {
			r.cleanupOtherProviders(ctx, bench, componentName, "")
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Gunicorn Deployment", "deployment", deployName, "replicas", replicas)

	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	annotations := map[string]string{}
	if managedByProvider {
		annotations["frappe.io/managed-by-provider"] = config.Provider
	}

	container := resources.NewContainerBuilder("gunicorn", image).
		WithImagePullPolicy(r.getImagePullPolicy(bench)).
		WithPort("http", 8000).
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(gunicornResources).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, componentName)).
		WithAnnotations(annotations).
		WithImagePullSecrets(r.getImagePullSecrets(bench)).
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

	if err := r.Create(ctx, deploy); err != nil {
		return err
	}

	if managedByProvider {
		return provider.Ensure(ctx, bench, componentName, deployName, config)
	}

	return nil
}

// ensureNginx ensures the NGINX Deployment and Service exist
func (r *FrappeBenchReconciler) ensureNginx(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.ensureNginxService(ctx, bench); err != nil {
		return err
	}

	logger := log.FromContext(ctx)
	componentName := "nginx"
	deployName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	config := r.getComponentAutoscaling(bench, componentName)
	config = r.fillComponentDefaults(config, componentName)

	var provider ScalingProvider
	if config.Enabled != nil && *config.Enabled && config.Provider != "" {
		provider = resolveProvider(config.Provider, r.Client, r.Scheme)
	}

	providerAvailable := false
	if provider != nil {
		providerAvailable = provider.IsAvailable(ctx)
	}

	replicas := r.getComponentReplicaCount(config, providerAvailable)

	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	managedByProvider := providerAvailable && config.Enabled != nil && *config.Enabled
	nginxResources := r.getNginxResources(bench)

	if err == nil {
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating NGINX image", "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		if !managedByProvider && *deploy.Spec.Replicas != replicas {
			logger.Info("Updating NGINX replicas", "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
			deploy.Spec.Replicas = &replicas
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, nginxResources) {
			logger.Info("Updating NGINX resources")
			deploy.Spec.Template.Spec.Containers[0].Resources = nginxResources
			changed = true
		}

		if changed {
			if err := r.Update(ctx, deploy); err != nil {
				return err
			}
		}

		if managedByProvider {
			r.cleanupOtherProviders(ctx, bench, componentName, config.Provider)
			return provider.Ensure(ctx, bench, componentName, deployName, config)
		} else {
			r.cleanupOtherProviders(ctx, bench, componentName, "")
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	gunicornSvc := fmt.Sprintf("%s-gunicorn", bench.Name)

	annotations := map[string]string{}
	if managedByProvider {
		annotations["frappe.io/managed-by-provider"] = config.Provider
	}

	container := resources.NewContainerBuilder("nginx", image).
		WithImagePullPolicy(r.getImagePullPolicy(bench)).
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
		WithResources(nginxResources).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		Build()

	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, componentName)).
		WithAnnotations(annotations).
		WithImagePullSecrets(r.getImagePullSecrets(bench)).
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

	if err := r.Create(ctx, deploy); err != nil {
		return err
	}

	if managedByProvider {
		return provider.Ensure(ctx, bench, componentName, deployName, config)
	}

	return nil
}

func (r *FrappeBenchReconciler) ensureSocketIODeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)
	componentName := "socketio"
	deployName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	config := r.getComponentAutoscaling(bench, componentName)
	config = r.fillComponentDefaults(config, componentName)

	var provider ScalingProvider
	if config.Enabled != nil && *config.Enabled && config.Provider != "" {
		provider = resolveProvider(config.Provider, r.Client, r.Scheme)
	}

	providerAvailable := false
	if provider != nil {
		providerAvailable = provider.IsAvailable(ctx)
	}

	replicas := r.getComponentReplicaCount(config, providerAvailable)

	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	managedByProvider := providerAvailable && config.Enabled != nil && *config.Enabled
	socketIOResources := r.getSocketIOResources(bench)

	if err == nil {
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Socket.IO image", "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		if !managedByProvider && *deploy.Spec.Replicas != replicas {
			logger.Info("Updating Socket.IO replicas", "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
			deploy.Spec.Replicas = &replicas
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, socketIOResources) {
			logger.Info("Updating Socket.IO resources")
			deploy.Spec.Template.Spec.Containers[0].Resources = socketIOResources
			changed = true
		}

		if changed {
			if err := r.Update(ctx, deploy); err != nil {
				return err
			}
		}

		if managedByProvider {
			r.cleanupOtherProviders(ctx, bench, componentName, config.Provider)
			return provider.Ensure(ctx, bench, componentName, deployName, config)
		} else {
			r.cleanupOtherProviders(ctx, bench, componentName, "")
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	annotations := map[string]string{}
	if managedByProvider {
		annotations["frappe.io/managed-by-provider"] = config.Provider
	}

	container := resources.NewContainerBuilder("socketio", image).
		WithImagePullPolicy(r.getImagePullPolicy(bench)).
		WithArgs("node", "/home/frappe/frappe-bench/apps/frappe/socketio.js").
		WithPort("socketio", 9000).
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(socketIOResources).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, componentName)).
		WithAnnotations(annotations).
		WithImagePullSecrets(r.getImagePullSecrets(bench)).
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

	if err := r.Create(ctx, deploy); err != nil {
		return err
	}

	if managedByProvider {
		return provider.Ensure(ctx, bench, componentName, deployName, config)
	}

	return nil
}

// ensureScheduler ensures the Scheduler Deployment exists
func (r *FrappeBenchReconciler) ensureScheduler(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)
	componentName := "scheduler"
	deployName := fmt.Sprintf("%s-%s", bench.Name, componentName)

	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	schedulerResources := r.getSchedulerResources(bench)
	replicas := int32(1)

	if err == nil {
		changed := false
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Scheduler image", "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
			changed = true
		}

		if *deploy.Spec.Replicas != replicas {
			logger.Info("Updating Scheduler replicas", "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
			deploy.Spec.Replicas = &replicas
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, schedulerResources) {
			logger.Info("Updating Scheduler resources")
			deploy.Spec.Template.Spec.Containers[0].Resources = schedulerResources
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

	image := r.getBenchImage(ctx, bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	container := resources.NewContainerBuilder("scheduler", image).
		WithImagePullPolicy(r.getImagePullPolicy(bench)).
		WithArgs("bench", "schedule").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(schedulerResources).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(bench.Spec.PodConfig, r.benchLabels(bench))

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithSelector(r.componentLabels(bench, componentName)).
		WithImagePullSecrets(r.getImagePullSecrets(bench)).
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
