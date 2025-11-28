/*
Copyright 2024 Vyogo Technologies.

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
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

// FrappeBenchReconciler reconciles a FrappeBench object
type FrappeBenchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop
func (r *FrappeBenchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the FrappeBench instance
	bench := &vyogotechv1alpha1.FrappeBench{}
	if err := r.Get(ctx, req.NamespacedName, bench); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get FrappeBench")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling FrappeBench", "name", bench.Name, "namespace", bench.Namespace)

	// Get operator configuration
	operatorConfig, err := r.getOperatorConfig(ctx, bench.Namespace)
	if err != nil {
		logger.Error(err, "Failed to get operator config")
		// Continue with defaults
	}

	// Determine Git enabled status
	gitEnabled := r.isGitEnabled(operatorConfig, bench)
	logger.Info("Git configuration", "enabled", gitEnabled)

	// Merge FPM repositories
	fpmRepos, err := r.mergeFPMRepositories(operatorConfig, bench)
	if err != nil {
		logger.Error(err, "Failed to merge FPM repositories")
	}
	logger.Info("FPM repositories configured", "count", len(fpmRepos))

	// Ensure bench initialization
	if err := r.ensureBenchInitialized(ctx, bench, gitEnabled, fpmRepos); err != nil {
		logger.Error(err, "Failed to ensure bench initialized")
		return ctrl.Result{}, err
	}

	// Ensure storage
	if err := r.ensureBenchStorage(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure storage")
		return ctrl.Result{}, err
	}

	// Ensure Redis
	if err := r.ensureRedis(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Redis")
		return ctrl.Result{}, err
	}

	// Ensure Gunicorn
	if err := r.ensureGunicorn(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Gunicorn")
		return ctrl.Result{}, err
	}

	// Ensure NGINX
	if err := r.ensureNginx(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure NGINX")
		return ctrl.Result{}, err
	}

	// Ensure Socket.IO
	if err := r.ensureSocketIO(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Socket.IO")
		return ctrl.Result{}, err
	}

	// Ensure Scheduler
	if err := r.ensureScheduler(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Scheduler")
		return ctrl.Result{}, err
	}

	// Ensure Workers
	if err := r.ensureWorkers(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Workers")
		return ctrl.Result{}, err
	}

	// Update worker scaling status
	if err := r.updateWorkerScalingStatus(ctx, bench); err != nil {
		logger.Error(err, "Failed to update worker scaling status")
		// Don't fail the reconciliation, just log the error
	}

	// Update status
	if err := r.updateBenchStatus(ctx, bench, gitEnabled, fpmRepos); err != nil {
		logger.Error(err, "Failed to update bench status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// getOperatorConfig retrieves the operator-level configuration
func (r *FrappeBenchReconciler) getOperatorConfig(ctx context.Context, namespace string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      "frappe-operator-config",
		Namespace: "frappe-operator-system", // Operator namespace
	}, configMap)
	return configMap, err
}

// isGitEnabled determines if Git is enabled based on operator and bench config
func (r *FrappeBenchReconciler) isGitEnabled(operatorConfig *corev1.ConfigMap, bench *vyogotechv1alpha1.FrappeBench) bool {
	// Priority 1: Bench-level override
	if bench.Spec.GitConfig != nil && bench.Spec.GitConfig.Enabled != nil {
		return *bench.Spec.GitConfig.Enabled
	}

	// Priority 2: Operator-level default
	if operatorConfig != nil {
		if gitEnabledStr, ok := operatorConfig.Data["gitEnabled"]; ok {
			return gitEnabledStr == "true"
		}
	}

	// Default: false (enterprise mode - no Git)
	return false
}

// mergeFPMRepositories merges operator-level and bench-level FPM repositories
func (r *FrappeBenchReconciler) mergeFPMRepositories(operatorConfig *corev1.ConfigMap, bench *vyogotechv1alpha1.FrappeBench) ([]vyogotechv1alpha1.FPMRepository, error) {
	var repos []vyogotechv1alpha1.FPMRepository

	// Add operator-level repositories
	if operatorConfig != nil {
		if fpmReposJSON, ok := operatorConfig.Data["fpmRepositories"]; ok {
			var operatorRepos []vyogotechv1alpha1.FPMRepository
			if err := json.Unmarshal([]byte(fpmReposJSON), &operatorRepos); err == nil {
				repos = append(repos, operatorRepos...)
			}
		}
	}

	// Add bench-level repositories
	if bench.Spec.FPMConfig != nil {
		repos = append(repos, bench.Spec.FPMConfig.Repositories...)
	}

	return repos, nil
}

// ensureBenchInitialized creates a job to initialize the Frappe bench
func (r *FrappeBenchReconciler) ensureBenchInitialized(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, gitEnabled bool, fpmRepos []vyogotechv1alpha1.FPMRepository) error {
	logger := log.FromContext(ctx)

	jobName := fmt.Sprintf("%s-init", bench.Name)
	job := &batchv1.Job{}

	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: bench.Namespace}, job)
	if err == nil {
		// Job already exists
		logger.Info("Bench init job already exists", "job", jobName)
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	// Create init job
	logger.Info("Creating bench init job", "job", jobName)

	// Simplified init script - configure and build assets
	// The frappe/erpnext image already has apps installed
	initScript := fmt.Sprintf(`#!/bin/bash
set -e

cd /home/frappe/frappe-bench

echo "Configuring Frappe bench..."

# Create apps.txt from existing apps
ls -1 apps > sites/apps.txt

# Create or update common_site_config.json
cat > sites/common_site_config.json <<EOF
{
  "redis_cache": "redis://%s-redis-cache:6379",
  "redis_queue": "redis://%s-redis-queue:6379",
  "socketio_port": 9000
}
EOF

echo "Building assets for production..."
bench build --production

echo "Bench configuration complete"
`, bench.Name, bench.Name)

	// Create the job
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: bench.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "bench-init",
							Image:   r.getBenchImage(bench),
							Command: []string{"bash", "-c"},
							Args:    []string{initScript},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
								},
							},
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

	if err := controllerutil.SetControllerReference(bench, job, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, job)
}

// getBenchImage returns the image to use for the bench
func (r *FrappeBenchReconciler) getBenchImage(bench *vyogotechv1alpha1.FrappeBench) string {
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		image := bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			image = fmt.Sprintf("%s:%s", image, bench.Spec.ImageConfig.Tag)
		}
		return image
	}
	return "frappe/erpnext:latest"
}

// parseAppsJSON converts legacy appsJSON to AppSource array
func (r *FrappeBenchReconciler) parseAppsJSON(appsJSON string) []vyogotechv1alpha1.AppSource {
	var appNames []string
	if err := json.Unmarshal([]byte(appsJSON), &appNames); err != nil {
		return nil
	}

	apps := make([]vyogotechv1alpha1.AppSource, 0, len(appNames))
	for _, name := range appNames {
		// Assume image source for legacy format
		apps = append(apps, vyogotechv1alpha1.AppSource{
			Name:   name,
			Source: "image",
		})
	}
	return apps
}

// updateBenchStatus updates the FrappeBench status
// updateWorkerScalingStatus updates the status with current worker scaling information
func (r *FrappeBenchReconciler) updateWorkerScalingStatus(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	logger := log.FromContext(ctx)

	if bench.Status.WorkerScaling == nil {
		bench.Status.WorkerScaling = make(map[string]vyogotechv1alpha1.WorkerScalingStatus)
	}

	kedaAvailable := r.isKEDAAvailable(ctx)
	workerTypes := []string{"default", "long", "short"}

	for _, workerType := range workerTypes {
		// Get the deployment
		deployName := fmt.Sprintf("%s-worker-%s", bench.Name, workerType)
		deploy := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy)
		if err != nil {
			if errors.IsNotFound(err) {
				continue // Worker not created yet
			}
			logger.Error(err, "Failed to get worker deployment", "worker", workerType)
			continue
		}

		// Get autoscaling config
		config := r.getWorkerAutoscalingConfig(bench, workerType)
		config = r.fillAutoscalingDefaults(config, workerType)

		// Determine mode and replicas
		mode := "static"
		kedaManaged := false
		if kedaAvailable && config.Enabled != nil && *config.Enabled {
			mode = "autoscaled"
			kedaManaged = true
		}

		currentReplicas := int32(0)
		if deploy.Status.Replicas > 0 {
			currentReplicas = deploy.Status.Replicas
		}

		desiredReplicas := int32(0)
		if deploy.Spec.Replicas != nil {
			desiredReplicas = *deploy.Spec.Replicas
		}

		// Update status
		bench.Status.WorkerScaling[workerType] = vyogotechv1alpha1.WorkerScalingStatus{
			Mode:            mode,
			CurrentReplicas: currentReplicas,
			DesiredReplicas: desiredReplicas,
			KEDAManaged:     kedaManaged,
		}
	}

	return nil // Status will be updated in updateBenchStatus
}

func (r *FrappeBenchReconciler) updateBenchStatus(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, gitEnabled bool, fpmRepos []vyogotechv1alpha1.FPMRepository) error {
	// Collect installed app names
	installedApps := make([]string, 0, len(bench.Spec.Apps))
	for _, app := range bench.Spec.Apps {
		installedApps = append(installedApps, app.Name)
	}

	// Collect FPM repository names
	repoNames := make([]string, 0, len(fpmRepos))
	for _, repo := range fpmRepos {
		repoNames = append(repoNames, repo.Name)
	}

	bench.Status.Phase = "Ready"
	bench.Status.GitEnabled = gitEnabled
	bench.Status.InstalledApps = installedApps
	bench.Status.FPMRepositories = repoNames
	bench.Status.ObservedGeneration = bench.Generation

	return r.Status().Update(ctx, bench)
}

// SetupWithManager sets up the controller with the Manager
func (r *FrappeBenchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1alpha1.FrappeBench{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
