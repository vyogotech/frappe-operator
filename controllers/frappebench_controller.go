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
	"strings"
	"time"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// FrappeBenchReconciler reconciles a FrappeBench object
type FrappeBenchReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	IsOpenShift bool
}

const frappeBenchFinalizer = "vyogo.tech/bench-finalizer"

//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keda.sh,resources=scaledobjects/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

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
	r.Recorder.Event(bench, corev1.EventTypeNormal, "Reconciling", "Starting FrappeBench reconciliation")

	// Handle finalizer for deletion
	if result, err := r.handleFinalizer(ctx, bench); err != nil {
		return result, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Set progressing condition at start
	r.setCondition(bench, metav1.Condition{
		Type:    "Progressing",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciling",
		Message: "Starting reconciliation",
	})
	if err := r.updateStatus(ctx, bench); err != nil {
		logger.Error(err, "Failed to update status")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "StatusUpdateFailed", fmt.Sprintf("Failed to update status: %v", err))
		return ctrl.Result{}, err
	}

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

	// Ensure storage
	if err := r.ensureBenchStorage(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure storage")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "StorageFailed", fmt.Sprintf("Failed to provision storage: %v", err))
		r.setCondition(bench, metav1.Condition{
			Type:    "StorageReady",
			Status:  metav1.ConditionFalse,
			Reason:  "StorageFailed",
			Message: fmt.Sprintf("Failed to provision storage: %v", err),
		})
		return ctrl.Result{}, err
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "StorageReady", "Storage provisioned successfully")

	// Ensure bench initialization
	ready, err := r.ensureBenchInitialized(ctx, bench, gitEnabled, fpmRepos)
	if err != nil {
		logger.Error(err, "Failed to ensure bench initialized")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "InitializationFailed", fmt.Sprintf("Failed to initialize bench: %v", err))
		r.setCondition(bench, metav1.Condition{
			Type:    "Initialized",
			Status:  metav1.ConditionFalse,
			Reason:  "InitializationFailed",
			Message: fmt.Sprintf("Failed to initialize bench: %v", err),
		})
		return ctrl.Result{}, err
	}
	if !ready {
		logger.Info("Bench initialization in progress, requeueing")
		r.Recorder.Event(bench, corev1.EventTypeNormal, "Initializing", "Bench initialization in progress")
		r.setCondition(bench, metav1.Condition{
			Type:    "Progressing",
			Status:  metav1.ConditionTrue,
			Reason:  "Initializing",
			Message: "Bench initialization is in progress",
		})
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "Initialized", "Bench initialization completed")

	// Ensure Redis
	if err := r.ensureRedis(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Redis")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "RedisFailed", fmt.Sprintf("Failed to ensure Redis: %v", err))
		return ctrl.Result{}, err
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "RedisReady", "Redis service created")

	// Ensure Gunicorn
	if err := r.ensureGunicorn(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Gunicorn")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "GunicornFailed", fmt.Sprintf("Failed to ensure Gunicorn: %v", err))
		return ctrl.Result{}, err
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "GunicornReady", "Gunicorn deployment created")

	// Ensure NGINX
	if err := r.ensureNginx(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure NGINX")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "NginxFailed", fmt.Sprintf("Failed to ensure NGINX: %v", err))
		return ctrl.Result{}, err
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "NginxReady", "NGINX deployment created")

	// Ensure Socket.IO
	if err := r.ensureSocketIO(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Socket.IO")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "SocketIOFailed", fmt.Sprintf("Failed to ensure Socket.IO: %v", err))
		return ctrl.Result{}, err
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "SocketIOReady", "Socket.IO deployment created")

	// Ensure Scheduler
	if err := r.ensureScheduler(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Scheduler")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "SchedulerFailed", fmt.Sprintf("Failed to ensure Scheduler: %v", err))
		return ctrl.Result{}, err
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "SchedulerReady", "Scheduler deployment created")

	// Ensure Workers
	if err := r.ensureWorkers(ctx, bench); err != nil {
		logger.Error(err, "Failed to ensure Workers")
		r.Recorder.Event(bench, corev1.EventTypeWarning, "WorkersFailed", fmt.Sprintf("Failed to ensure Workers: %v", err))
		return ctrl.Result{}, err
	}
	r.Recorder.Event(bench, corev1.EventTypeNormal, "WorkersReady", "Worker deployments created")

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

// handleFinalizer manages the finalizer for FrappeBench deletion
func (r *FrappeBenchReconciler) handleFinalizer(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if bench.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(bench, frappeBenchFinalizer) {
			logger.Info("Deleting FrappeBench", "bench", bench.Name)
			r.Recorder.Event(bench, corev1.EventTypeNormal, "Deleting", "FrappeBench deletion started")

			// Set deletion condition
			r.setCondition(bench, metav1.Condition{
				Type:    "Terminating",
				Status:  metav1.ConditionTrue,
				Reason:  "Deleting",
				Message: "FrappeBench is being deleted",
			})
			if err := r.updateStatus(ctx, bench); err != nil {
				return ctrl.Result{}, err
			}

			// 1. Check for dependent sites
			siteList := &vyogotechv1alpha1.FrappeSiteList{}
			if err := r.List(ctx, siteList, client.InNamespace(bench.Namespace)); err != nil {
				logger.Error(err, "Failed to list dependent sites")
				r.Recorder.Event(bench, corev1.EventTypeWarning, "DeletionFailed", fmt.Sprintf("Failed to check dependent sites: %v", err))
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}

			dependentSites := []string{}
			for _, site := range siteList.Items {
				if site.Spec.BenchRef != nil && site.Spec.BenchRef.Name == bench.Name {
					dependentSites = append(dependentSites, site.Name)
				}
			}

			if len(dependentSites) > 0 {
				logger.Info("FrappeBench has dependent sites, blocking deletion", "sites", dependentSites)
				r.Recorder.Event(bench, corev1.EventTypeWarning, "DeletionBlocked", fmt.Sprintf("Cannot delete bench with dependent sites: %v", dependentSites))
				r.setCondition(bench, metav1.Condition{
					Type:    "Terminating",
					Status:  metav1.ConditionFalse,
					Reason:  "DependentSitesExist",
					Message: fmt.Sprintf("Cannot delete bench with dependent sites: %v", dependentSites),
				})
				if err := r.updateStatus(ctx, bench); err != nil {
					return ctrl.Result{}, err
				}
				// Requeue to retry after sites are deleted
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}

			// 2. Scale down all deployments and statefulsets to 0
			deploymentComponents := []string{"gunicorn", "nginx", "socketio", "scheduler", "worker-default", "worker-long", "worker-short"}
			for _, component := range deploymentComponents {
				deployName := fmt.Sprintf("%s-%s", bench.Name, component)
				deploy := &appsv1.Deployment{}
				if err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy); err == nil {
					if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas > 0 {
						logger.Info("Scaling down deployment", "deployment", deployName)
						zero := int32(0)
						deploy.Spec.Replicas = &zero
						if err := r.Update(ctx, deploy); err != nil {
							logger.Error(err, "Failed to scale down deployment", "deployment", deployName)
							r.Recorder.Event(bench, corev1.EventTypeWarning, "ScaleDownFailed", fmt.Sprintf("Failed to scale down %s: %v", deployName, err))
						} else {
							r.Recorder.Event(bench, corev1.EventTypeNormal, "ScaledDown", fmt.Sprintf("Scaled down deployment %s", deployName))
						}
					}
				}
			}

			redisComponents := []string{"redis-cache", "redis-queue"}
			for _, component := range redisComponents {
				stsName := fmt.Sprintf("%s-%s", bench.Name, component)
				sts := &appsv1.StatefulSet{}
				if err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: bench.Namespace}, sts); err == nil {
					if sts.Spec.Replicas != nil && *sts.Spec.Replicas > 0 {
						logger.Info("Scaling down statefulset", "statefulset", stsName)
						zero := int32(0)
						sts.Spec.Replicas = &zero
						if err := r.Update(ctx, sts); err != nil {
							logger.Error(err, "Failed to scale down statefulset", "statefulset", stsName)
							r.Recorder.Event(bench, corev1.EventTypeWarning, "ScaleDownFailed", fmt.Sprintf("Failed to scale down %s: %v", stsName, err))
						} else {
							r.Recorder.Event(bench, corev1.EventTypeNormal, "ScaledDown", fmt.Sprintf("Scaled down statefulset %s", stsName))
						}
					}
				}
			}

			// 3. Wait for pods to terminate (check if any pods are still running)
			allTerminated := true
			for _, component := range deploymentComponents {
				deployName := fmt.Sprintf("%s-%s", bench.Name, component)
				deploy := &appsv1.Deployment{}
				if err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: bench.Namespace}, deploy); err == nil {
					if deploy.Status.Replicas > 0 || deploy.Status.ReadyReplicas > 0 {
						allTerminated = false
						logger.Info("Waiting for pods to terminate", "deployment", deployName, "replicas", deploy.Status.Replicas)
					}
				}
			}
			for _, component := range redisComponents {
				stsName := fmt.Sprintf("%s-%s", bench.Name, component)
				sts := &appsv1.StatefulSet{}
				if err := r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: bench.Namespace}, sts); err == nil {
					if sts.Status.Replicas > 0 || sts.Status.ReadyReplicas > 0 {
						allTerminated = false
						logger.Info("Waiting for pods to terminate", "statefulset", stsName, "replicas", sts.Status.Replicas)
					}
				}
			}

			if !allTerminated {
				logger.Info("Pods still terminating, requeuing")
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}

			// 4. Clean up PVC
			pvcName := fmt.Sprintf("%s-sites", bench.Name)
			pvc := &corev1.PersistentVolumeClaim{}
			if err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: bench.Namespace}, pvc); err == nil {
				logger.Info("Deleting bench PVC", "pvc", pvcName)
				if err := r.Delete(ctx, pvc); err != nil {
					logger.Error(err, "Failed to delete bench PVC", "pvc", pvcName)
					r.Recorder.Event(bench, corev1.EventTypeWarning, "PVCDeletionFailed", fmt.Sprintf("Failed to delete PVC %s: %v", pvcName, err))
				} else {
					r.Recorder.Event(bench, corev1.EventTypeNormal, "PVCDeleted", fmt.Sprintf("Deleted PVC %s", pvcName))
				}
			}

			// 5. Cleanup is complete - remove finalizer
			logger.Info("FrappeBench cleanup complete, removing finalizer")
			r.Recorder.Event(bench, corev1.EventTypeNormal, "Deleted", "FrappeBench cleanup completed")
			controllerutil.RemoveFinalizer(bench, frappeBenchFinalizer)
			if err := r.Update(ctx, bench); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(bench, frappeBenchFinalizer) {
		controllerutil.AddFinalizer(bench, frappeBenchFinalizer)
		if err := r.Update(ctx, bench); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(bench, corev1.EventTypeNormal, "FinalizerAdded", "Finalizer added to FrappeBench")
	}

	return ctrl.Result{}, nil
}

// setCondition sets a condition on the FrappeBench using meta.SetStatusCondition
func (r *FrappeBenchReconciler) setCondition(bench *vyogotechv1alpha1.FrappeBench, condition metav1.Condition) {
	condition.ObservedGeneration = bench.Generation
	meta.SetStatusCondition(&bench.Status.Conditions, condition)
}

// updateStatus updates the FrappeBench status with proper error handling
func (r *FrappeBenchReconciler) updateStatus(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) error {
	if err := r.Status().Update(ctx, bench); err != nil {
		if errors.IsConflict(err) {
			// Requeue on conflict
			return fmt.Errorf("status update conflict, will requeue: %w", err)
		}
		return fmt.Errorf("failed to update status: %w", err)
	}
	return nil
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
func (r *FrappeBenchReconciler) ensureBenchInitialized(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench, gitEnabled bool, fpmRepos []vyogotechv1alpha1.FPMRepository) (bool, error) {
	logger := log.FromContext(ctx)

	jobName := fmt.Sprintf("%s-init", bench.Name)
	job := &batchv1.Job{}

	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: bench.Namespace}, job)
	if err == nil {
		// Job exists, check status
		if job.Status.Succeeded > 0 {
			return true, nil
		}
		return false, nil
	}
	if !errors.IsNotFound(err) {
		return false, err
	}

	// Create init job
	logger.Info("Creating bench init job", "job", jobName)

	// Simplified init script - configure and build assets
	// The frappe/erpnext image already has apps installed
	initScript := fmt.Sprintf(`#!/bin/bash
set -e

# Setup user for OpenShift compatibility (fixes getpwuid() error)
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  # Try to add user to /etc/passwd if writable
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench

echo "Checking directory permissions..."
ls -ld sites || true
id

echo "Configuring Frappe bench..."

# Create sites directory if it doesn't exist
mkdir -p sites

# Test write access
if ! touch sites/.permission_test; then
    echo "ERROR: sites directory is NOT writable by $(whoami) (UID $(id -u), GID $(id -g))."
    ls -ld sites
    exit 1
fi
rm sites/.permission_test

# Create apps.txt from existing apps
if [ -d "apps" ]; then
    echo "Creating apps.txt..."
    # Write to sites/apps.txt since that is the shared volume
    ls -1 apps > sites/apps.txt || { echo "ERROR: Failed to write to sites/apps.txt"; exit 1; }
fi

# Create or update common_site_config.json
echo "Creating common_site_config.json..."
cat > sites/common_site_config.json <<EOF
{
  "redis_cache": "redis://%s-redis-cache:6379",
  "redis_queue": "redis://%s-redis-queue:6379",
  "socketio_port": 9000
}
EOF

# Sync assets from the image cache to the Persistent Volume
if [ -d "/home/frappe/assets_cache" ]; then
    echo "Syncing pre-built assets from image to PVC..."
    mkdir -p sites/assets
    # Use -n to not overwrite existing files, preserving permissions where possible
    cp -rn /home/frappe/assets_cache/* sites/assets/ || true
fi

echo "Bench configuration complete"
`, bench.Name, bench.Name)

	// detect if we should skip bench build via annotation
	skipBuild := "0"
	if bench.Annotations != nil && bench.Annotations["frappe.tech/skip-bench-build"] == "1" {
		skipBuild = "1"
	}

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
					RestartPolicy:   corev1.RestartPolicyNever,
					SecurityContext: r.getPodSecurityContext(ctx, bench),
					Containers: []corev1.Container{
						{
							Name:    "bench-init",
							Image:   r.getBenchImage(ctx, bench),
							Command: []string{"bash", "-c"},
							Args:    []string{initScript},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
									SubPath:   "frappe-sites",
								},
							},
							SecurityContext: r.getContainerSecurityContext(ctx, bench),
							Env: []corev1.EnvVar{
								{
									Name:  "SKIP_BENCH_BUILD",
									Value: skipBuild,
								},
								{
									Name:  "USER",
									Value: "frappe",
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
		return false, err
	}

	return false, r.Create(ctx, job)
}

// getBenchImage returns the image to use for the bench
// Priority: 1. bench.spec.imageConfig, 2. operator ConfigMap defaults, 3. hardcoded constants
func (r *FrappeBenchReconciler) getBenchImage(ctx context.Context, bench *vyogotechv1alpha1.FrappeBench) string {
	// Priority 1: Check bench-level ImageConfig override
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		image := bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			image = fmt.Sprintf("%s:%s", image, bench.Spec.ImageConfig.Tag)
		} else if bench.Spec.FrappeVersion != "" {
			// If tag not specified but version is, use version as tag
			image = fmt.Sprintf("%s:%s", image, bench.Spec.FrappeVersion)
		}
		return image
	}

	// Priority 2: Check operator ConfigMap defaults
	operatorConfig, err := r.getOperatorConfig(ctx, bench.Namespace)
	if err == nil && operatorConfig != nil {
		if defaultImage, ok := operatorConfig.Data["defaultFrappeImage"]; ok && defaultImage != "" {
			// If version is specified, replace tag in default image
			if bench.Spec.FrappeVersion != "" && bench.Spec.FrappeVersion != "latest" {
				// Extract repository from default image and append version tag
				parts := strings.Split(defaultImage, ":")
				if len(parts) == 2 {
					return fmt.Sprintf("%s:%s", parts[0], bench.Spec.FrappeVersion)
				}
			}
			return defaultImage
		}
	}

	// Priority 3: Fall back to constants with version
	if bench.Spec.FrappeVersion != "" && bench.Spec.FrappeVersion != "latest" {
		return fmt.Sprintf("docker.io/frappe/erpnext:%s", bench.Spec.FrappeVersion)
	}
	return constants.DefaultFrappeImage
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
	logger := log.FromContext(ctx)

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

	// Determine phase and conditions
	isReady := false
	if bench.Status.Phase == "" || (bench.Status.Phase != "Provisioning" && bench.Status.Phase != "Ready") {
		bench.Status.Phase = "Provisioning"
		r.setCondition(bench, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "Provisioning",
			Message: "FrappeBench is being provisioned",
		})
	}

	// Check if init job is succeeded
	jobName := fmt.Sprintf("%s-init", bench.Name)
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: bench.Namespace}, job); err == nil {
		if job.Status.Succeeded > 0 {
			bench.Status.Phase = "Ready"
			isReady = true
			r.setCondition(bench, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Initialized",
				Message: "FrappeBench is ready and initialized",
			})
			r.setCondition(bench, metav1.Condition{
				Type:    "Initialized",
				Status:  metav1.ConditionTrue,
				Reason:  "JobCompleted",
				Message: "Initialization job completed successfully",
			})
		} else if job.Status.Failed > 0 {
			bench.Status.Phase = "Failed"
			r.setCondition(bench, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "InitializationFailed",
				Message: "Initialization job failed",
			})
			r.setCondition(bench, metav1.Condition{
				Type:    "Degraded",
				Status:  metav1.ConditionTrue,
				Reason:  "JobFailed",
				Message: "Initialization job failed",
			})
		}
	}

	// Update status fields
	bench.Status.GitEnabled = gitEnabled
	bench.Status.InstalledApps = installedApps
	bench.Status.FPMRepositories = repoNames
	bench.Status.ObservedGeneration = bench.Generation

	// Update status with proper error handling
	if err := r.updateStatus(ctx, bench); err != nil {
		logger.Error(err, "Failed to update bench status")
		return err
	}

	if isReady {
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *FrappeBenchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up event recorder
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("frappebench-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1alpha1.FrappeBench{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
