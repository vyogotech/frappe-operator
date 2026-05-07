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
	"io"
	"strings"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	"github.com/vyogotech/frappe-operator/pkg/scripts"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureSiteInitialized creates a Job to run bench new-site
func (r *FrappeSiteReconciler) ensureSiteInitialized(ctx context.Context, site *vyogotechv1.FrappeSite, bench *vyogotechv1.FrappeBench, domain string, dbInfo *database.DatabaseInfo, dbCreds *database.DatabaseCredentials) (bool, error) {
	logger := log.FromContext(ctx)

	jobName := fmt.Sprintf("%s-init", site.Name)
	job := &batchv1.Job{}

	// Check for site version annotation
	versionAnnotation := "frappe.io/site-version"
	siteVersionVal := site.Annotations[versionAnnotation]
	if siteVersionVal == "" {
		siteVersionVal = "default"
	}
	logger.Info("Checking site version annotation", "siteVersion", siteVersionVal)

	// If already initialized with this exact version, return success immediately
	if site.Status.ObservedSiteVersion == siteVersionVal && site.Status.Phase == vyogotechv1.FrappeSitePhaseReady {
		return true, nil
	}

	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err == nil {
		// Job exists
		jobVersionVal := job.Annotations[versionAnnotation]
		if jobVersionVal == "" {
			jobVersionVal = "default"
		}
		if siteVersionVal == "" {
			siteVersionVal = "default"
		}

		currentAppsList := strings.Join(site.Spec.Apps, ",")
		jobAppsList := job.Annotations["frappe.io/apps-list"]

		logger.Info("Job exists, checking annotations", "jobName", jobName, "jobVersion", jobVersionVal, "siteVersion", siteVersionVal, "jobApps", jobAppsList, "siteApps", currentAppsList)

		// If the existing job has an older version or different apps, delete it to restart
		if siteVersionVal != jobVersionVal || currentAppsList != jobAppsList {
			logger.Info("Init job outdated (version or apps changed), deleting to restart", "oldVersion", jobVersionVal, "newVersion", siteVersionVal, "oldApps", jobAppsList, "newApps", currentAppsList)
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				return false, fmt.Errorf("failed to delete init job for update: %w", err)
			}
			// Requeue to create new job
			return false, nil
		}

		// Job exists, check if it completed
		if job.Status.Succeeded > 0 {
			logger.Info("Site initialization job completed successfully", "job", jobName)

			// Update status with requested apps
			if len(site.Spec.Apps) > 0 {
				site.Status.InstalledApps = site.Spec.Apps
				site.Status.AppInstallationStatus = fmt.Sprintf("Completed app installation for %d requested app(s) - check logs for any skipped apps", len(site.Spec.Apps))
				logger.Info("App installation process completed", "requestedApps", site.Spec.Apps)
				r.Recorder.Event(site, corev1.EventTypeNormal, "AppsProcessed",
					fmt.Sprintf("Processed app installation for: %v - check job logs for any skipped apps", site.Spec.Apps))
			} else {
				site.Status.AppInstallationStatus = "No apps specified - only frappe framework installed"
				logger.Info("Site initialized with frappe framework only")
			}

			return true, nil
		}

		if job.Status.Failed > 0 {
			// Check whether the job has permanently failed (backoff limit exhausted)
			// or is still retrying pods. A permanently failed job has a "Failed"
			// condition with status True set by the Job controller.
			jobPermanentlyFailed := false
			for _, cond := range job.Status.Conditions {
				if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
					jobPermanentlyFailed = true
					break
				}
			}

			if !jobPermanentlyFailed {
				// Transient failure — job is still within backoffLimit, let it retry.
				logger.Info("Site initialization job has pod failures but is still retrying",
					"job", jobName, "failedCount", job.Status.Failed)
				return false, nil
			}

			// Permanent failure — backoff limit exhausted.
			logger.Error(nil, "Site initialization job permanently failed", "job", jobName, "failedCount", job.Status.Failed)
			r.Recorder.Event(site, corev1.EventTypeWarning, "SiteInitializationFailed",
				fmt.Sprintf("Site initialization job permanently failed after %d attempt(s)", job.Status.Failed))

			// Collect pod logs for debugging
			podList := &corev1.PodList{}
			listOpts := []client.ListOption{
				client.InNamespace(site.Namespace),
				client.MatchingLabels{"job-name": jobName},
			}
			if err := r.List(ctx, podList, listOpts...); err == nil && len(podList.Items) > 0 {
				pod := podList.Items[len(podList.Items)-1]
				if pod.Status.Phase == corev1.PodFailed {
					logger.Error(nil, "Site initialization pod failed",
						"pod", pod.Name,
						"phase", pod.Status.Phase,
						"reason", pod.Status.Reason,
						"message", pod.Status.Message)

					if logs, err := r.GetPodLogs(ctx, pod.Namespace, pod.Name); err == nil {
						fmt.Printf("--- Logs for failed pod %s ---\n%s\n---------------------------\n", pod.Name, logs)
					}

					if len(site.Spec.Apps) > 0 {
						site.Status.AppInstallationStatus = fmt.Sprintf("Failed to install apps: %s", pod.Status.Message)
						r.Recorder.Event(site, corev1.EventTypeWarning, "AppInstallationFailed",
							fmt.Sprintf("Failed to install apps. Check pod %s logs for details", pod.Name))
					}
				}
			}

			return false, fmt.Errorf("site initialization job permanently failed after %d attempt(s): backoff limit exhausted", job.Status.Failed)
		}
		// Job is still running
		logger.Info("Site initialization job in progress", "job", jobName)
		if len(site.Spec.Apps) > 0 {
			site.Status.AppInstallationStatus = fmt.Sprintf("Installing %d app(s)...", len(site.Spec.Apps))
		}
		return false, nil
	}

	if !errors.IsNotFound(err) {
		return false, err
	}

	// Create the initialization job
	logger.Info("Creating site initialization job",
		"job", jobName,
		"domain", domain,
		"dbProvider", dbInfo.Provider,
		"dbName", dbInfo.Name,
		"apps", site.Spec.Apps,
		"appsCount", len(site.Spec.Apps))

	if len(site.Spec.Apps) > 0 {
		r.Recorder.Event(site, corev1.EventTypeNormal, "CreatingInitJob",
			fmt.Sprintf("Creating initialization job to install %d app(s): %v", len(site.Spec.Apps), site.Spec.Apps))
	} else {
		r.Recorder.Event(site, corev1.EventTypeNormal, "CreatingInitJob",
			"Creating initialization job (frappe framework only)")
	}

	// Get or generate admin password
	adminPassword, err := r.ensureAdminPassword(ctx, site)
	if err != nil {
		return false, err
	}

	// Ensure initialization secret exists with all credentials
	if err := r.ensureInitSecrets(ctx, site, bench, domain, dbInfo, dbCreds, adminPassword, r.resolveRedisCacheURL(ctx, bench), r.resolveRedisQueueURL(ctx, bench)); err != nil {
		logger.Error(err, "Failed to create initialization secret")
		return false, fmt.Errorf("failed to create init secret: %w", err)
	}

	// Load site init script from pkg/scripts
	initScript, err := scripts.GetScript(scripts.SiteInit)
	if err != nil {
		return false, fmt.Errorf("failed to load site init script: %w", err)
	}

	// Apply Pod Config from Site Spec (init jobs use site config)
	nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(site.Spec.PodConfig, map[string]string{
		"app":  "frappe",
		"site": site.Name,
	})

	// Get bench PVC name
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	// Build the container
	container := resources.NewContainerBuilder("site-init", r.getBenchImage(ctx, bench)).
		WithImagePullPolicy(r.getImagePullPolicy(bench)).
		WithCommand("bash", "-c").
		WithArgs(initScript).
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
		WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
		WithResources(r.getSiteInitResources(bench)).
		WithVolumeMount("site-secrets", "/tmp/site-secrets").
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		Build()

	// Prepare job annotations
	jobAnnotations := map[string]string{}
	if siteVersionVal != "" {
		jobAnnotations[versionAnnotation] = siteVersionVal
	}
	jobAnnotations["frappe.io/apps-list"] = strings.Join(site.Spec.Apps, ",")

	// Build the job
	job = resources.NewJobBuilder(jobName, site.Namespace).
		WithLabels(extraLabels).
		WithExtraPodLabels(extraLabels).
		WithAnnotations(jobAnnotations).
		WithImagePullSecrets(r.getImagePullSecrets(bench)).
		WithNodeSelector(nodeSelector).
		WithAffinity(affinity).
		WithTolerations(tolerations).
		WithPodSecurityContext(r.getPodSecurityContext(ctx, bench)).
		WithContainer(container).
		WithPVCVolume("sites", pvcName).
		WithSecretVolume("site-secrets", fmt.Sprintf("%s-init-secrets", site.Name), resources.Int32Ptr(0444)).
		WithOwner(site, r.Scheme).
		MustBuild()

	job.Spec.BackoffLimit = int32Ptr(0)

	if err := r.Create(ctx, job); err != nil {
		return false, err
	}

	logger.Info("Site initialization job created", "job", jobName)
	return false, nil // Not ready yet, job is running
}

// deleteSite implements the site deletion logic
func (r *FrappeSiteReconciler) deleteSite(ctx context.Context, site *vyogotechv1.FrappeSite) error {
	logger := log.FromContext(ctx)

	// Get the referenced bench
	bench := &vyogotechv1.FrappeBench{}
	benchKey := types.NamespacedName{
		Name:      site.Spec.BenchRef.Name,
		Namespace: site.Spec.BenchRef.Namespace,
	}
	if benchKey.Namespace == "" {
		benchKey.Namespace = site.Namespace
	}

	if err := r.Get(ctx, benchKey, bench); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Referenced bench not found, assuming it's already deleted")
			return nil
		}
		return fmt.Errorf("failed to get referenced bench for deletion: %w", err)
	}

	// Create deletion job to run bench drop-site
	jobName := fmt.Sprintf("%s-delete", site.Name)
	job := &batchv1.Job{}

	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get deletion job: %w", err)
		}

		// Job doesn't exist, create it
		logger.Info("Creating site deletion job", "job", jobName)

		// Resolve DB Config
		dbConfig := r.resolveDBConfig(site, bench)

		// Get MariaDB root credentials for deletion
		rootUser, rootPassword, err := r.getMariaDBRootCredentials(ctx, site, dbConfig)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("MariaDB instance not found, skipping site deletion job")
				return nil
			}
			return fmt.Errorf("failed to get MariaDB root credentials: %w", err)
		}

		// Create deletion secret with root credentials
		deletionSecretName := fmt.Sprintf("%s-deletion-secret", site.Name)
		deletionSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deletionSecretName,
				Namespace: site.Namespace,
				Labels: map[string]string{
					"app":  "frappe",
					"site": site.Name,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"db_root_user":     []byte(rootUser),
				"db_root_password": []byte(rootPassword),
				"site_name":        []byte(site.Spec.SiteName),
			},
		}

		if err := controllerutil.SetControllerReference(site, deletionSecret, r.Scheme); err != nil {
			return err
		}

		if err := r.Create(ctx, deletionSecret); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create deletion secret: %w", err)
			}
			var existing corev1.Secret
			if err := r.Get(ctx, types.NamespacedName{Name: deletionSecretName, Namespace: site.Namespace}, &existing); err != nil {
				return fmt.Errorf("failed to get existing deletion secret: %w", err)
			}
			existing.Data = deletionSecret.Data
			if err := r.Update(ctx, &existing); err != nil {
				return fmt.Errorf("failed to update deletion secret: %w", err)
			}
		}

		// Load site delete script from pkg/scripts
		deleteScript, err := scripts.GetScript(scripts.SiteDelete)
		if err != nil {
			return fmt.Errorf("failed to load site delete script: %w", err)
		}

		// Apply Pod Config from Site Spec
		nodeSelector, affinity, tolerations, extraLabels := applyPodConfig(site.Spec.PodConfig, map[string]string{
			"app":  "frappe",
			"site": site.Name,
		})

		// Build the container
		container := resources.NewContainerBuilder("site-delete", r.getBenchImage(ctx, bench)).
			WithImagePullPolicy(r.getImagePullPolicy(bench)).
			WithCommand("bash", "-c").
			WithArgs(deleteScript).
			WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites", "frappe-sites").
			WithVolumeMountSubPath("sites", "/home/frappe/frappe-bench/sites/assets", "frappe-sites/assets").
			WithResources(r.getSiteDeleteResources(bench)).
			WithVolumeMountReadOnly("deletion-secret", "/tmp/secrets").
			WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
			Build()

		// Build the job
		job = resources.NewJobBuilder(jobName, site.Namespace).
			WithLabels(extraLabels).
			WithExtraPodLabels(extraLabels).
			WithImagePullSecrets(r.getImagePullSecrets(bench)).
			WithNodeSelector(nodeSelector).
			WithAffinity(affinity).
			WithTolerations(tolerations).
			WithPodSecurityContext(r.getPodSecurityContext(ctx, bench)).
			WithContainer(container).
			WithPVCVolume("sites", fmt.Sprintf("%s-sites", bench.Name)).
			WithSecretVolume("deletion-secret", deletionSecretName, resources.Int32Ptr(0400)).
			WithOwner(site, r.Scheme).
			MustBuild()

		job.Spec.BackoffLimit = int32Ptr(1)

		if err := r.Create(ctx, job); err != nil {
			return fmt.Errorf("failed to create site deletion job: %w", err)
		}

		return fmt.Errorf("site deletion job created, waiting for completion")
	}

	// Job exists, check its status
	if job.Status.Succeeded > 0 {
		logger.Info("Site deletion job completed successfully")
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			return fmt.Errorf("failed to delete completed deletion job: %w", err)
		}
		return nil
	}

	if job.Status.Failed > 0 {
		return fmt.Errorf("site deletion job failed")
	}

	return fmt.Errorf("site deletion job is still running")
}

// resolveRedisCacheURL returns the Redis cache connection URL for the bench
func (r *FrappeSiteReconciler) resolveRedisCacheURL(ctx context.Context, bench *vyogotechv1.FrappeBench) string {
	// Default internal Redis URL
	host := fmt.Sprintf("%s-redis-cache", bench.Name)
	port := int32(6379)
	password := ""

	// If external Redis is configured, resolve host, port, and password
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.External {
		if bench.Spec.RedisConfig.Host != "" {
			host = bench.Spec.RedisConfig.Host
		}
		if bench.Spec.RedisConfig.Port > 0 {
			port = bench.Spec.RedisConfig.Port
		}

		// Resolve password from secret if provided
		if bench.Spec.RedisConfig.ConnectionSecretRef != nil {
			secret := &corev1.Secret{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      bench.Spec.RedisConfig.ConnectionSecretRef.Name,
				Namespace: bench.Namespace,
			}, secret)
			if err == nil {
				if p, ok := secret.Data["password"]; ok {
					password = string(p)
				}
			}
		}
	}

	if password != "" {
		return fmt.Sprintf(":%s@%s:%d", password, host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// resolveRedisQueueURL returns the Redis queue connection URL for the bench
func (r *FrappeSiteReconciler) resolveRedisQueueURL(ctx context.Context, bench *vyogotechv1.FrappeBench) string {
	// Default internal Redis URL
	host := fmt.Sprintf("%s-redis-queue", bench.Name)
	port := int32(6379)
	password := ""

	// If external Redis is configured, resolve host, port, and password
	// (usually external configurations share the same endpoint for cache and queue)
	if bench.Spec.RedisConfig != nil && bench.Spec.RedisConfig.External {
		if bench.Spec.RedisConfig.Host != "" {
			host = bench.Spec.RedisConfig.Host
		}
		if bench.Spec.RedisConfig.Port > 0 {
			port = bench.Spec.RedisConfig.Port
		}

		// Resolve password from secret if provided
		if bench.Spec.RedisConfig.ConnectionSecretRef != nil {
			secret := &corev1.Secret{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      bench.Spec.RedisConfig.ConnectionSecretRef.Name,
				Namespace: bench.Namespace,
			}, secret)
			if err == nil {
				if p, ok := secret.Data["password"]; ok {
					password = string(p)
				}
			}
		}
	}

	if password != "" {
		return fmt.Sprintf(":%s@%s:%d", password, host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// GetPodLogs retrieves logs from a pod
func (r *FrappeSiteReconciler) GetPodLogs(ctx context.Context, namespace, podName string) (string, error) {
	clientset, err := kubernetes.NewForConfig(r.Config)
	if err != nil {
		return "", err
	}

	podLogOpts := &corev1.PodLogOptions{}
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
