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
	"k8s.io/apimachinery/pkg/types"
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

	svc, err = resources.NewServiceBuilder(svcName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
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
		// Update existing deployment if image has changed
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Gunicorn Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
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
		WithResources(r.getGunicornResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
		WithSelector(r.componentLabels(bench, "gunicorn")).
		WithReplicas(replicas).
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
	if err := r.ensureNginxService(ctx, bench); err != nil {
		return err
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

	svc, err = resources.NewServiceBuilder(svcName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
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

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)
	if err == nil {
		// Update existing deployment if image has changed
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating NGINX Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
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
		WithResources(r.getNginxResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		Build()

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
		WithSelector(r.componentLabels(bench, "nginx")).
		WithReplicas(replicas).
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

	svc, err = resources.NewServiceBuilder(svcName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
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
		// Update existing deployment if image has changed
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Socket.IO Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
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
		WithResources(r.getSocketIOResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
		WithSelector(r.componentLabels(bench, "socketio")).
		WithReplicas(replicas).
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
		// Update existing deployment if image has changed
		image := r.getBenchImage(ctx, bench)
		if deploy.Spec.Template.Spec.Containers[0].Image != image {
			logger.Info("Updating Scheduler Deployment image", "deployment", deployName, "oldImage", deploy.Spec.Template.Spec.Containers[0].Image, "newImage", image)
			deploy.Spec.Template.Spec.Containers[0].Image = image
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
		WithResources(r.getSchedulerResources(bench)).
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		WithEnv("USER", "frappe").
		Build()

	deploy, err = resources.NewDeploymentBuilder(deployName, bench.Namespace).
		WithLabels(r.benchLabels(bench)).
		WithSelector(r.componentLabels(bench, "scheduler")).
		WithReplicas(replicas).
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
