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
	"github.com/vyogotech/frappe-operator/controllers/database"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	"github.com/vyogotech/frappe-operator/pkg/scripts"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ensureSiteInitialized creates a Job to run bench new-site
func (r *FrappeSiteReconciler) ensureSiteInitialized(ctx context.Context, site *vyogotechv1alpha1.FrappeSite, bench *vyogotechv1alpha1.FrappeBench, domain string, dbInfo *database.DatabaseInfo, dbCreds *database.DatabaseCredentials) (bool, error) {
	logger := log.FromContext(ctx)

	jobName := fmt.Sprintf("%s-init", site.Name)
	job := &batchv1.Job{}

	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: site.Namespace}, job)
	if err == nil {
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
			logger.Error(nil, "Site initialization job failed", "job", jobName, "failedCount", job.Status.Failed)
			r.Recorder.Event(site, corev1.EventTypeWarning, "SiteInitializationFailed",
				fmt.Sprintf("Site initialization job failed after %d attempt(s)", job.Status.Failed))

			// Try to get pod logs for error details
			podList := &corev1.PodList{}
			listOpts := []client.ListOption{
				client.InNamespace(site.Namespace),
				client.MatchingLabels{"job-name": jobName},
			}
			if err := r.List(ctx, podList, listOpts...); err == nil && len(podList.Items) > 0 {
				// Check the most recent pod for error messages
				pod := podList.Items[len(podList.Items)-1]
				if pod.Status.Phase == corev1.PodFailed {
					logger.Error(nil, "Site initialization pod failed",
						"pod", pod.Name,
						"phase", pod.Status.Phase,
						"reason", pod.Status.Reason,
						"message", pod.Status.Message)

					// Update status with failure information
					if len(site.Spec.Apps) > 0 {
						site.Status.AppInstallationStatus = fmt.Sprintf("Failed to install apps: %s", pod.Status.Message)
						r.Recorder.Event(site, corev1.EventTypeWarning, "AppInstallationFailed",
							fmt.Sprintf("Failed to install apps. Check pod %s logs for details", pod.Name))
					}
				}
			}

			return false, fmt.Errorf("site initialization job failed")
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
	if err := r.ensureInitSecrets(ctx, site, bench, domain, dbInfo, dbCreds, adminPassword); err != nil {
		logger.Error(err, "Failed to create initialization secret")
		return false, fmt.Errorf("failed to create init secret: %w", err)
	}

	// Load site init script from pkg/scripts
	initScript, err := scripts.GetScript(scripts.SiteInit)
	if err != nil {
		return false, fmt.Errorf("failed to load site init script: %w", err)
	}

	// Get bench PVC name
	pvcName := fmt.Sprintf("%s-sites", bench.Name)

	// Build the container
	container := resources.NewContainerBuilder("site-init", r.getBenchImage(ctx, bench)).
		WithCommand("bash", "-c").
		WithArgs(initScript).
		WithVolumeMount("sites", "/home/frappe/frappe-bench/sites").
		WithVolumeMount("site-secrets", "/tmp/site-secrets").
		WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
		Build()

	// Build the job
	job = resources.NewJobBuilder(jobName, site.Namespace).
		WithLabels(map[string]string{
			"app":  "frappe",
			"site": site.Name,
		}).
		WithPodSecurityContext(r.getPodSecurityContext(ctx, bench)).
		WithContainer(container).
		WithPVCVolume("sites", pvcName).
		WithSecretVolume("site-secrets", fmt.Sprintf("%s-init-secrets", site.Name), resources.Int32Ptr(0444)).
		WithOwner(site, r.Scheme).
		MustBuild()

	if err := r.Create(ctx, job); err != nil {
		return false, err
	}

	logger.Info("Site initialization job created", "job", jobName)
	return false, nil // Not ready yet, job is running
}

// deleteSite implements the site deletion logic
func (r *FrappeSiteReconciler) deleteSite(ctx context.Context, site *vyogotechv1alpha1.FrappeSite) error {
	logger := log.FromContext(ctx)

	// Get the referenced bench
	bench := &vyogotechv1alpha1.FrappeBench{}
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

		// Get MariaDB root credentials for deletion
		rootUser, rootPassword, err := r.getMariaDBRootCredentials(ctx, site)
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

		// Build the container
		container := resources.NewContainerBuilder("site-delete", r.getBenchImage(ctx, bench)).
			WithCommand("bash", "-c").
			WithArgs(deleteScript).
			WithVolumeMount("sites", "/home/frappe/frappe-bench/sites").
			WithVolumeMountReadOnly("deletion-secret", "/tmp/secrets").
			WithSecurityContext(r.getContainerSecurityContext(ctx, bench)).
			Build()

		// Build the job
		job = resources.NewJobBuilder(jobName, site.Namespace).
			WithLabels(map[string]string{
				"app":  "frappe",
				"site": site.Name,
			}).
			WithPodSecurityContext(r.getPodSecurityContext(ctx, bench)).
			WithContainer(container).
			WithPVCVolume("sites", fmt.Sprintf("%s-sites", bench.Name)).
			WithSecretVolume("deletion-secret", deletionSecretName, resources.Int32Ptr(0400)).
			WithOwner(site, r.Scheme).
			MustBuild()

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
