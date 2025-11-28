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
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

// ensureBenchStorage ensures the PVC for the bench exists
func (r *FrappeBenchReconciler) ensureBenchStorage(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	pvc := &corev1.PersistentVolumeClaim{}

	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: bench.Namespace}, pvc)
	if err == nil {
		logger.V(1).Info("PVC already exists", "pvc", pvcName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	sc, err := r.chooseStorageClass(ctx, bench)
	if err != nil {
		return err
	}

	accessMode, err := r.determineAccessMode(ctx, bench, sc)
	if err != nil {
		return err
	}

	return r.createBenchPVC(ctx, bench, accessMode, sc)
}

func (r *FrappeBenchReconciler) createBenchPVC(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, accessMode corev1.PersistentVolumeAccessMode, sc *storagev1.StorageClass) error {
	logger := log.FromContext(ctx)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	storageSize := resource.MustParse("10Gi")

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
			Annotations: map[string]string{
				"frappe.tech/requested-access": string(accessMode),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				accessMode,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
		},
	}

	// Set storage class name if available
	if sc != nil {
		pvc.Spec.StorageClassName = &sc.Name
		pvc.Annotations["frappe.tech/storage-class"] = sc.Name
		pvc.Annotations["frappe.tech/provisioner"] = sc.Provisioner
	}

	if accessMode == corev1.ReadWriteOnce {
		pvc.Annotations["frappe.tech/fallback"] = "true"
	}

	if err := controllerutil.SetControllerReference(bench, pvc, r.Scheme); err != nil {
		return err
	}

	logger.Info("Creating PVC for bench", "pvc", pvcName, "accessMode", accessMode)
	return r.Create(ctx, pvc)
}

func (r *FrappeBenchReconciler) chooseStorageClass(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) (*storagev1.StorageClass, error) {
	logger := log.FromContext(ctx)

	if bench.Spec.StorageClassName != "" {
		sc := &storagev1.StorageClass{}
		if err := r.Get(ctx, types.NamespacedName{Name: bench.Spec.StorageClassName}, sc); err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("specified storage class '%s' not found in cluster. Available storage classes can be listed with 'kubectl get storageclass'", bench.Spec.StorageClassName)
			}
			return nil, fmt.Errorf("failed to get storage class '%s': %w", bench.Spec.StorageClassName, err)
		}

		// Validate that the storage class is ready for use
		if sc.Provisioner == "" {
			return nil, fmt.Errorf("storage class '%s' has no provisioner configured", bench.Spec.StorageClassName)
		}

		logger.Info("Using specified storage class", "storageClass", sc.Name, "provisioner", sc.Provisioner)
		return sc, nil
	}

	// Get all storage classes for selection
	list := &storagev1.StorageClassList{}
	if err := r.List(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to list storage classes: %w", err)
	}

	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no storage classes available in cluster. Please create a storage class or specify storageClassName in bench spec")
	}

	// Try to find default storage class
	for _, sc := range list.Items {
		if isDefaultStorageClass(&sc) {
			logger.Info("Using default storage class", "storageClass", sc.Name, "provisioner", sc.Provisioner)
			return &sc, nil
		}
	}

	// No default found, use first available with warning
	sc := &list.Items[0]
	logger.Info("No default storage class found, using first available",
		"storageClass", sc.Name,
		"provisioner", sc.Provisioner,
		"recommendation", "Set a default storage class or specify storageClassName in bench spec")
	return sc, nil
}

func (r *FrappeBenchReconciler) determineAccessMode(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, sc *storagev1.StorageClass) (corev1.PersistentVolumeAccessMode, error) {
	logger := log.FromContext(ctx)

	if bench.Annotations != nil {
		if modeStr, ok := bench.Annotations["frappe.tech/storage-access-mode"]; ok {
			logger.V(1).Info("Using existing storage access mode from annotations", "mode", modeStr)
			return corev1.PersistentVolumeAccessMode(modeStr), nil
		}
	}

	mode := corev1.ReadWriteOnce
	if storageClassSupportsRWX(sc) {
		mode = corev1.ReadWriteMany
	}

	// Use patch instead of update to avoid race conditions
	patch := client.MergeFrom(bench.DeepCopy())
	if bench.Annotations == nil {
		bench.Annotations = make(map[string]string)
	}
	bench.Annotations["frappe.tech/storage-access-mode"] = string(mode)

	logger.Info("Setting storage access mode annotation", "mode", mode, "storageClass", sc.Name)
	if err := r.Patch(ctx, bench, patch); err != nil {
		logger.Error(err, "Failed to patch bench with storage access mode", "mode", mode)
		return corev1.ReadWriteOnce, err
	}
	return mode, nil
}

func storageClassSupportsRWX(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	provisioner := strings.ToLower(sc.Provisioner)
	rwxProviders := []string{"nfs", "ceph", "gluster", "netapp", "azurefile", "filestore", "portworx"}
	for _, provider := range rwxProviders {
		if strings.Contains(provisioner, provider) {
			return true
		}
	}
	return false
}

func isDefaultStorageClass(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
		return true
	}
	if sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
		return true
	}
	return false
}

func (r *FrappeBenchReconciler) getBenchStorageAccessMode(bench *vyogotechv1alpha1.FrappeBench) corev1.PersistentVolumeAccessMode {
	if bench.Annotations != nil && bench.Annotations["frappe.tech/storage-fallback"] == "true" {
		return corev1.ReadWriteOnce
	}
	return corev1.ReadWriteMany
}

func (r *FrappeBenchReconciler) markStorageFallback(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	// Use patch instead of update to avoid race conditions
	patch := client.MergeFrom(bench.DeepCopy())
	if bench.Annotations == nil {
		bench.Annotations = make(map[string]string)
	}
	bench.Annotations["frappe.tech/storage-fallback"] = "true"

	logger.Info("Marking bench for storage fallback", "bench", bench.Name)
	return r.Patch(ctx, bench, patch)
}

func shouldFallbackStorage(pvc *corev1.PersistentVolumeClaim, bench *vyogotechv1alpha1.FrappeBench) bool {
	if pvc.Status.Phase != corev1.ClaimPending {
		return false
	}
	if pvc.Annotations["frappe.tech/requested-access"] != string(corev1.ReadWriteMany) {
		return false
	}
	if bench.Annotations != nil && bench.Annotations["frappe.tech/storage-fallback"] == "true" {
		return false
	}
	return true
}

// ensureRedis ensures the Redis StatefulSet and Service exist
func (r *FrappeBenchReconciler) ensureRedis(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	// Create redis-cache and redis-queue services (socketio not needed for v15+)
	if err := r.ensureRedisService(ctx, bench, "redis-cache"); err != nil {
		return err
	}
	if err := r.ensureRedisService(ctx, bench, "redis-queue"); err != nil {
		return err
	}
	if err := r.ensureRedisStatefulSet(ctx, bench, "redis-cache"); err != nil {
		return err
	}
	return r.ensureRedisStatefulSet(ctx, bench, "redis-queue")
}

func (r *FrappeBenchReconciler) ensureRedisService(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, serviceType string) error {
	logger := log.FromContext(ctx)

	svcName := fmt.Sprintf("%s-%s", bench.Name, serviceType)
	svc := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: bench.Namespace}, svc)
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Redis Service", "service", svcName, "type", serviceType)

	svc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP, // Regular ClusterIP service for DNS resolution
			Selector: r.componentLabels(bench, fmt.Sprintf("redis-%s", serviceType)),
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, svc, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, svc)
}

func (r *FrappeBenchReconciler) ensureRedisStatefulSet(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, role string) error {
	logger := log.FromContext(ctx)

	stsName := fmt.Sprintf("%s-%s", bench.Name, role)
	sts := &appsv1.StatefulSet{}

	err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: bench.Namespace}, sts)
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Redis StatefulSet", "statefulset", stsName)

	replicas := int32(1)
	redisImage := r.getRedisImage(bench)

	sts = &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stsName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: stsName,
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.componentLabels(bench, fmt.Sprintf("redis-%s", role)),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.componentLabels(bench, fmt.Sprintf("redis-%s", role)),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: redisImage,
							Args:  []string{"redis-server"},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
							},
							Resources: r.getRedisResources(bench),
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, sts, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, sts)
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

	svc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: corev1.ServiceSpec{
			Selector: r.componentLabels(bench, "gunicorn"),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8000,
					TargetPort: intstr.FromInt(8000),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, svc, r.Scheme); err != nil {
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
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Gunicorn Deployment", "deployment", deployName)

	replicas := r.getGunicornReplicas(bench)
	image := r.getBenchImage(bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	deploy = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.componentLabels(bench, "gunicorn"),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.componentLabels(bench, "gunicorn"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "gunicorn",
							Image: image,
							// No command/args - uses image default
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8000,
									Name:          "http",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
							Resources: r.getGunicornResources(bench),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, deploy, r.Scheme); err != nil {
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

	svc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: corev1.ServiceSpec{
			Selector: r.componentLabels(bench, "nginx"),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, svc, r.Scheme); err != nil {
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
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating NGINX Deployment", "deployment", deployName)

	replicas := r.getNginxReplicas(bench)
	image := r.getBenchImage(bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	gunicornSvc := fmt.Sprintf("%s-gunicorn", bench.Name)

	deploy = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.componentLabels(bench, "nginx"),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.componentLabels(bench, "nginx"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: image,
							Args: []string{
								"nginx-entrypoint.sh",
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Name:          "http",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "BACKEND",
									Value: fmt.Sprintf("%s:8000", gunicornSvc),
								},
								{
									Name:  "SOCKETIO",
									Value: fmt.Sprintf("%s-socketio:9000", bench.Name),
								},
								{
									Name:  "UPSTREAM_REAL_IP_ADDRESS",
									Value: "127.0.0.1",
								},
								{
									Name:  "UPSTREAM_REAL_IP_RECURSIVE",
									Value: "off",
								},
								{
									Name:  "UPSTREAM_REAL_IP_HEADER",
									Value: "X-Forwarded-For",
								},
								{
									Name:  "FRAPPE_SITE_NAME_HEADER",
									Value: "$host",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
							Resources: r.getNginxResources(bench),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, deploy, r.Scheme); err != nil {
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

	svc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: corev1.ServiceSpec{
			Selector: r.componentLabels(bench, "socketio"),
			Ports: []corev1.ServicePort{
				{
					Name:       "socketio",
					Port:       9000,
					TargetPort: intstr.FromInt(9000),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, svc, r.Scheme); err != nil {
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
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Socket.IO Deployment", "deployment", deployName)

	replicas := r.getSocketIOReplicas(bench)
	image := r.getBenchImage(bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	deploy = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.componentLabels(bench, "socketio"),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.componentLabels(bench, "socketio"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "socketio",
							Image: image,
							Args: []string{
								"node",
								"/home/frappe/frappe-bench/apps/frappe/socketio.js",
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 9000,
									Name:          "socketio",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
							Resources: r.getSocketIOResources(bench),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, deploy, r.Scheme); err != nil {
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
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Scheduler Deployment", "deployment", deployName)

	replicas := int32(1) // Scheduler should only have 1 replica
	image := r.getBenchImage(bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	deploy = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: bench.Namespace,
			Labels:    r.benchLabels(bench),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.componentLabels(bench, "scheduler"),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.componentLabels(bench, "scheduler"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "scheduler",
							Image: image,
							Args: []string{
								"bench",
								"schedule",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
							Resources: r.getSchedulerResources(bench),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, deploy, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, deploy)
}

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

func (r *FrappeBenchReconciler) ensureWorkerDeployment(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, workerType, queue string, replicas int32, resources corev1.ResourceRequirements, config *vyogotechv1alpha1.WorkerAutoscaling, kedaAvailable bool) error {
	logger := log.FromContext(ctx)

	deployName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)
	deploy := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)

	// Determine if this worker is managed by KEDA
	kedaManaged := kedaAvailable && config.Enabled != nil && *config.Enabled

	if err == nil {
		// Deployment exists, update it if needed
		// Only update replicas if NOT managed by KEDA (KEDA controls replicas)
		if !kedaManaged && *deploy.Spec.Replicas != replicas {
			logger.Info("Updating worker replicas", "worker", workerType, "oldReplicas", *deploy.Spec.Replicas, "newReplicas", replicas)
			deploy.Spec.Replicas = &replicas
			return r.Update(ctx, deploy)
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	logger.Info("Creating Worker Deployment", "deployment", deployName, "queue", queue, "replicas", replicas, "kedaManaged", kedaManaged)

	image := r.getBenchImage(bench)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	// Add annotations to indicate scaling mode
	annotations := map[string]string{}
	if kedaManaged {
		annotations["frappe.io/scaling-mode"] = "autoscaled"
		annotations["keda.sh/managed-by"] = "keda"
	} else {
		annotations["frappe.io/scaling-mode"] = "static"
	}

	deploy = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        deployName,
			Namespace:   bench.Namespace,
			Labels:      r.benchLabels(bench),
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.componentLabels(bench, fmt.Sprintf("worker-%s", workerType)),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.componentLabels(bench, fmt.Sprintf("worker-%s", workerType)),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "worker",
							Image: image,
							Args: []string{
								"bench",
								"worker",
								"--queue",
								queue,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
							Resources: resources,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(bench, deploy, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, deploy)
}

// Helper functions for getting configuration values

func (r *FrappeBenchReconciler) getRedisImage(bench *vyogotechv1alpha1.FrappeBench) string {
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.Image != "" {
		return bench.Spec.RedisConfig.Image
	}
	return "redis:7-alpine"
}

func (r *FrappeBenchReconciler) getGunicornReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.Gunicorn
	}
	return 1
}

func (r *FrappeBenchReconciler) getNginxReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.Nginx
	}
	return 1
}

func (r *FrappeBenchReconciler) getSocketIOReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.Socketio
	}
	return 1
}

func (r *FrappeBenchReconciler) getWorkerDefaultReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.WorkerDefault
	}
	return 1
}

func (r *FrappeBenchReconciler) getWorkerLongReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.WorkerLong
	}
	return 1
}

func (r *FrappeBenchReconciler) getWorkerShortReplicas(bench *vyogotechv1alpha1.FrappeBench) int32 {
	if bench.Spec.ComponentReplicas != nil {
		return bench.Spec.ComponentReplicas.WorkerShort
	}
	return 1
}

func (r *FrappeBenchReconciler) getRedisResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.Resources != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.RedisConfig.Resources.Requests,
			Limits:   bench.Spec.RedisConfig.Resources.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getGunicornResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Gunicorn != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Gunicorn.Requests,
			Limits:   bench.Spec.ComponentResources.Gunicorn.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getNginxResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Nginx != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Nginx.Requests,
			Limits:   bench.Spec.ComponentResources.Nginx.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
}

func (r *FrappeBenchReconciler) getSocketIOResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Socketio != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Socketio.Requests,
			Limits:   bench.Spec.ComponentResources.Socketio.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getSchedulerResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.Scheduler != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.Scheduler.Requests,
			Limits:   bench.Spec.ComponentResources.Scheduler.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerDefaultResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerDefault != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerDefault.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerDefault.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerLongResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerLong != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerLong.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerLong.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) getWorkerShortResources(bench *vyogotechv1alpha1.FrappeBench) corev1.ResourceRequirements {
	if bench.Spec.ComponentResources != nil && bench.Spec.ComponentResources.WorkerShort != nil {
		return corev1.ResourceRequirements{
			Requests: bench.Spec.ComponentResources.WorkerShort.Requests,
			Limits:   bench.Spec.ComponentResources.WorkerShort.Limits,
		}
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func (r *FrappeBenchReconciler) benchLabels(bench *vyogotechv1alpha1.FrappeBench) map[string]string {
	return map[string]string{
		"app":   "frappe",
		"bench": bench.Name,
	}
}

func (r *FrappeBenchReconciler) componentLabels(bench *vyogotechv1alpha1.FrappeBench, component string) map[string]string {
	labels := r.benchLabels(bench)
	labels["component"] = component
	return labels
}

// getWorkerAutoscalingConfig returns the autoscaling config for a specific worker type
// Falls back to legacy ComponentReplicas if WorkerAutoscaling not configured
func (r *FrappeBenchReconciler) getWorkerAutoscalingConfig(bench *vyogotechv1alpha1.FrappeBench, workerType string) *vyogotechv1alpha1.WorkerAutoscaling {
	// Return configured autoscaling if available
	if bench.Spec.WorkerAutoscaling != nil {
		switch workerType {
		case "short":
			if bench.Spec.WorkerAutoscaling.Short != nil {
				return bench.Spec.WorkerAutoscaling.Short
			}
		case "long":
			if bench.Spec.WorkerAutoscaling.Long != nil {
				return bench.Spec.WorkerAutoscaling.Long
			}
		case "default":
			if bench.Spec.WorkerAutoscaling.Default != nil {
				return bench.Spec.WorkerAutoscaling.Default
			}
		}
	}

	// Fall back to legacy ComponentReplicas
	if bench.Spec.ComponentReplicas != nil {
		config := &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled: boolPtr(false), // Legacy is always static
		}
		switch workerType {
		case "short":
			if bench.Spec.ComponentReplicas.WorkerShort > 0 {
				config.StaticReplicas = int32Ptr(bench.Spec.ComponentReplicas.WorkerShort)
			} else {
				config.StaticReplicas = int32Ptr(2)
			}
		case "long":
			if bench.Spec.ComponentReplicas.WorkerLong > 0 {
				config.StaticReplicas = int32Ptr(bench.Spec.ComponentReplicas.WorkerLong)
			} else {
				config.StaticReplicas = int32Ptr(1)
			}
		case "default":
			if bench.Spec.ComponentReplicas.WorkerDefault > 0 {
				config.StaticReplicas = int32Ptr(bench.Spec.ComponentReplicas.WorkerDefault)
			} else {
				config.StaticReplicas = int32Ptr(1)
			}
		}
		return config
	}

	// Return nil to use defaults
	return nil
}

// getDefaultAutoscalingConfig returns opinionated defaults for each worker type
func (r *FrappeBenchReconciler) getDefaultAutoscalingConfig(workerType string) *vyogotechv1alpha1.WorkerAutoscaling {
	switch workerType {
	case "short":
		// Short jobs: scale-to-zero with aggressive scaling
		return &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled:         boolPtr(true),
			MinReplicas:     int32Ptr(0),
			MaxReplicas:     int32Ptr(10),
			QueueLength:     int32Ptr(5),
			CooldownPeriod:  int32Ptr(60),
			PollingInterval: int32Ptr(15),
		}
	case "long":
		// Long jobs: scale-to-zero with conservative scaling
		return &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled:         boolPtr(true),
			MinReplicas:     int32Ptr(0),
			MaxReplicas:     int32Ptr(5),
			QueueLength:     int32Ptr(2),
			CooldownPeriod:  int32Ptr(300),
			PollingInterval: int32Ptr(30),
		}
	case "default":
		// Default/scheduler: always one replica (scheduler must run)
		return &vyogotechv1alpha1.WorkerAutoscaling{
			Enabled:        boolPtr(false),
			StaticReplicas: int32Ptr(1),
		}
	}
	return nil
}

// fillAutoscalingDefaults fills in missing fields with defaults
func (r *FrappeBenchReconciler) fillAutoscalingDefaults(config *vyogotechv1alpha1.WorkerAutoscaling, workerType string) *vyogotechv1alpha1.WorkerAutoscaling {
	if config == nil {
		return r.getDefaultAutoscalingConfig(workerType)
	}

	result := &vyogotechv1alpha1.WorkerAutoscaling{}
	*result = *config

	// Fill in defaults
	defaults := r.getDefaultAutoscalingConfig(workerType)
	if result.Enabled == nil {
		result.Enabled = defaults.Enabled
	}
	if result.MinReplicas == nil {
		result.MinReplicas = defaults.MinReplicas
	}
	if result.MaxReplicas == nil {
		result.MaxReplicas = defaults.MaxReplicas
	}
	if result.StaticReplicas == nil {
		result.StaticReplicas = defaults.StaticReplicas
	}
	if result.QueueLength == nil {
		result.QueueLength = defaults.QueueLength
	}
	if result.CooldownPeriod == nil {
		result.CooldownPeriod = defaults.CooldownPeriod
	}
	if result.PollingInterval == nil {
		result.PollingInterval = defaults.PollingInterval
	}

	return result
}

// getWorkerReplicaCount determines the replica count based on scaling mode
func (r *FrappeBenchReconciler) getWorkerReplicaCount(config *vyogotechv1alpha1.WorkerAutoscaling, kedaAvailable bool) int32 {
	// If KEDA autoscaling enabled and available, use MinReplicas
	if config.Enabled != nil && *config.Enabled && kedaAvailable {
		if config.MinReplicas != nil {
			return *config.MinReplicas
		}
		return 0 // Default to scale-to-zero
	}

	// Otherwise use static replicas
	if config.StaticReplicas != nil {
		return *config.StaticReplicas
	}

	// Fallback
	return 1
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

// getRedisAddress returns the Redis address for the bench
func (r *FrappeBenchReconciler) getRedisAddress(bench *vyogotechv1alpha1.FrappeBench) string {
	// TODO: If ConnectionSecretRef is set, read the secret to get the host
	// For now, default to in-cluster Redis service with fully qualified domain name
	// KEDA needs the full service name since it runs in a different namespace
	return fmt.Sprintf("%s-redis-queue.%s.svc.cluster.local:6379", bench.Name, bench.Namespace)
}

// Helper functions for pointer types
func boolPtr(b bool) *bool {
	return &b
}

func int32Ptr(i int32) *int32 {
	return &i
}
