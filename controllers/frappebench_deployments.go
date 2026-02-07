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

// ensureGunicorn ensures the Gunicorn Deployment and Service exist
func (r *FrappeBenchReconciler) ensureGunicorn(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.ensureGunicornService(ctx, bench); err != nil {
		return err
	}

	// Ensure Autoscaler (HPA or KEDA)
	deployName := fmt.Sprintf("%s-gunicorn", bench.Name)
	if err := r.ensureAutoscaler(ctx, bench, deployName, bench.Spec.GunicornAutoscaling); err != nil {
		log.FromContext(ctx).Error(err, "Failed to ensure Autoscaler for Gunicorn")
		// Don't fail reconciliation for autoscaling errors
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

	// Determine if HPA is enabled for Gunicorn
	hpaEnabled := bench.Spec.GunicornAutoscaling != nil && bench.Spec.GunicornAutoscaling.Enabled

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

		// Only update replicas if HPA is NOT enabled
		// If HPA is enabled, we let HPA manage the replicas
		if !hpaEnabled {
			replicas := r.getGunicornReplicas(bench)
			if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas != replicas {
				logger.Info("Updating Gunicorn Deployment replicas", "deployment", deployName, "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
				deploy.Spec.Replicas = &replicas
				changed = true
			}
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

	// Add annotation to indicate scaling mode
	if hpaEnabled {
		if deploy.Annotations == nil {
			deploy.Annotations = make(map[string]string)
		}
		deploy.Annotations["frappe.io/scaling-mode"] = "hpa"
	}

	return r.Create(ctx, deploy)
}

// ensureNginx ensures the NGINX Deployment and Service exist
func (r *FrappeBenchReconciler) ensureNginx(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.ensureNginxService(ctx, bench); err != nil {
		return err
	}

	// Ensure Autoscaler (HPA or KEDA)
	deployName := fmt.Sprintf("%s-nginx", bench.Name)
	if err := r.ensureAutoscaler(ctx, bench, deployName, bench.Spec.NginxAutoscaling); err != nil {
		log.FromContext(ctx).Error(err, "Failed to ensure Autoscaler for NGINX")
		// Don't fail reconciliation for autoscaling errors
	}

	return r.ensureNginxDeployment(ctx, bench)
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

func (r *FrappeBenchReconciler) ensureNginxDeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-nginx", bench.Name)
	deploy := &appsv1.Deployment{}

	// Determine if HPA is enabled for NGINX
	hpaEnabled := bench.Spec.NginxAutoscaling != nil && bench.Spec.NginxAutoscaling.Enabled

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)
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

		// Only update replicas if HPA is NOT enabled
		if !hpaEnabled {
			replicas := r.getNginxReplicas(bench)
			if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas != replicas {
				logger.Info("Updating NGINX Deployment replicas", "deployment", deployName, "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
				deploy.Spec.Replicas = &replicas
				changed = true
			}
		}

		if changed {
			return r.Update(ctx, deploy)
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating NGINX Deployment", "deployment", deployName)

	replicas := r.getNginxReplicas(bench)
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

	// Add annotation to indicate scaling mode
	if hpaEnabled {
		if deploy.Annotations == nil {
			deploy.Annotations = make(map[string]string)
		}
		deploy.Annotations["frappe.io/scaling-mode"] = "hpa"
	}

	return r.Create(ctx, deploy)
}

// ensureSocketIO ensures the Socket.IO Deployment and Service exist
func (r *FrappeBenchReconciler) ensureSocketIO(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.ensureSocketIOService(ctx, bench); err != nil {
		return err
	}

	// Ensure Autoscaler (HPA or KEDA)
	deployName := fmt.Sprintf("%s-socketio", bench.Name)
	if err := r.ensureAutoscaler(ctx, bench, deployName, bench.Spec.SocketIOAutoscaling); err != nil {
		log.FromContext(ctx).Error(err, "Failed to ensure Autoscaler for SocketIO")
		// Don't fail reconciliation for autoscaling errors
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

	// Determine if HPA is enabled for SocketIO
	hpaEnabled := bench.Spec.SocketIOAutoscaling != nil && bench.Spec.SocketIOAutoscaling.Enabled

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

		// Only update replicas if HPA is NOT enabled
		if !hpaEnabled {
			replicas := r.getSocketIOReplicas(bench)
			if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas != replicas {
				logger.Info("Updating Socket.IO Deployment replicas", "deployment", deployName, "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
				deploy.Spec.Replicas = &replicas
				changed = true
			}
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

	// Add annotation to indicate scaling mode
	if hpaEnabled {
		if deploy.Annotations == nil {
			deploy.Annotations = make(map[string]string)
		}
		deploy.Annotations["frappe.io/scaling-mode"] = "hpa"
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
