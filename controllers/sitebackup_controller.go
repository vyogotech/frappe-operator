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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

// SiteBackupReconciler reconciles a SiteBackup object
type SiteBackupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SiteBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get the SiteBackup
	siteBackup := &vyogotechv1alpha1.SiteBackup{}
	err := r.Get(ctx, req.NamespacedName, siteBackup)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Find the associated FrappeSite
	siteList := &vyogotechv1alpha1.FrappeSiteList{}
	err = r.List(ctx, siteList, client.InNamespace(req.Namespace))
	if err != nil {
		return ctrl.Result{}, err
	}

	var benchRef *vyogotechv1alpha1.NamespacedName
	for _, site := range siteList.Items {
		if site.Spec.SiteName == siteBackup.Spec.Site {
			benchRef = site.Spec.BenchRef
			break
		}
	}

	if benchRef == nil {
		logger.Error(fmt.Errorf("no FrappeSite found for site %s", siteBackup.Spec.Site), "cannot proceed with backup")
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Failed", fmt.Sprintf("No FrappeSite found for site %s", siteBackup.Spec.Site), "")
	}

	// Get the bench
	bench := &vyogotechv1alpha1.FrappeBench{}
	err = r.Get(ctx, client.ObjectKey{
		Name:      benchRef.Name,
		Namespace: benchRef.Namespace,
	}, bench)
	if err != nil {
		return ctrl.Result{}, err
	}

	if siteBackup.Spec.Schedule == "" {
		// One-time backup
		return r.reconcileOneTimeBackup(ctx, siteBackup, bench)
	} else {
		// Scheduled backup
		return r.reconcileScheduledBackup(ctx, siteBackup, bench)
	}
}

// reconcileOneTimeBackup handles one-time backup creation and status updates
func (r *SiteBackupReconciler) reconcileOneTimeBackup(ctx context.Context, siteBackup *vyogotechv1alpha1.SiteBackup, bench *vyogotechv1alpha1.FrappeBench) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	jobName := siteBackup.Name + "-backup"

	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      jobName,
		Namespace: siteBackup.Namespace,
	}, job)

	if errors.IsNotFound(err) {
		// Job doesn't exist, create it
		job = r.buildBackupJob(siteBackup, bench)
		if err := r.Create(ctx, job); err != nil {
			logger.Error(err, "Failed to create backup job")
			return ctrl.Result{}, err
		}
		logger.Info("Created backup job", "job", job.Name)

		// Update status
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Running", "Backup job created", job.Name)
	}

	if err != nil {
		logger.Error(err, "Failed to get backup job")
		return ctrl.Result{}, err
	}

	// Job exists, check its status
	if job.Status.Succeeded > 0 {
		// Job completed successfully
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Succeeded", "Backup completed successfully", job.Name)
	}

	if job.Status.Failed > 0 {
		// Job failed
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Failed", "Backup job failed", job.Name)
	}

	// Job is still running
	return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Running", "Backup job running", job.Name)
}

// reconcileScheduledBackup handles scheduled backup creation
func (r *SiteBackupReconciler) reconcileScheduledBackup(ctx context.Context, siteBackup *vyogotechv1alpha1.SiteBackup, bench *vyogotechv1alpha1.FrappeBench) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cronJobName := siteBackup.Name + "-backup"

	cronJob := &batchv1.CronJob{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      cronJobName,
		Namespace: siteBackup.Namespace,
	}, cronJob)

	if errors.IsNotFound(err) {
		// CronJob doesn't exist, create it
		cronJob = r.buildBackupCronJob(siteBackup, bench)
		if err := r.Create(ctx, cronJob); err != nil {
			logger.Error(err, "Failed to create backup cronjob")
			return ctrl.Result{}, err
		}
		logger.Info("Created backup cronjob", "cronjob", cronJob.Name)

		// Update status
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Scheduled", "Scheduled backup created", cronJob.Name)
	}

	if err != nil {
		logger.Error(err, "Failed to get backup cronjob")
		return ctrl.Result{}, err
	}

	// CronJob exists, ensure it's up to date
	return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Scheduled", "Scheduled backup active", cronJob.Name)
}

// buildBackupJob creates a Job for one-time backup
func (r *SiteBackupReconciler) buildBackupJob(siteBackup *vyogotechv1alpha1.SiteBackup, bench *vyogotechv1alpha1.FrappeBench) *batchv1.Job {
	jobName := siteBackup.Name + "-backup"

	args := []string{"bench", "--site", siteBackup.Spec.Site, "backup"}
	if siteBackup.Spec.WithFiles {
		args = append(args, "--with-files")
	}
	if siteBackup.Spec.Compress {
		args = append(args, "--compress")
	}
	if siteBackup.Spec.BackupPath != "" {
		args = append(args, "--backup-path", siteBackup.Spec.BackupPath)
	}
	if siteBackup.Spec.BackupPathDB != "" {
		args = append(args, "--backup-path-db", siteBackup.Spec.BackupPathDB)
	}
	if siteBackup.Spec.BackupPathConf != "" {
		args = append(args, "--backup-path-conf", siteBackup.Spec.BackupPathConf)
	}
	if siteBackup.Spec.BackupPathFiles != "" {
		args = append(args, "--backup-path-files", siteBackup.Spec.BackupPathFiles)
	}
	if siteBackup.Spec.BackupPathPrivateFiles != "" {
		args = append(args, "--backup-path-private-files", siteBackup.Spec.BackupPathPrivateFiles)
	}
	if len(siteBackup.Spec.Exclude) > 0 {
		args = append(args, "--exclude", strings.Join(siteBackup.Spec.Exclude, ","))
	}
	if len(siteBackup.Spec.Include) > 0 {
		args = append(args, "--include", strings.Join(siteBackup.Spec.Include, ","))
	}
	if siteBackup.Spec.IgnoreBackupConf {
		args = append(args, "--ignore-backup-conf")
	}
	if siteBackup.Spec.Verbose {
		args = append(args, "--verbose")
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: siteBackup.Namespace,
			Labels: map[string]string{
				"app":        "frappe",
				"site":       siteBackup.Spec.Site,
				"backup":     "true",
				"backupType": "one-time",
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "backup",
							Image:   r.getBenchImage(bench),
							Command: []string{"sh", "-c"},
							Args:    []string{strings.Join(args, " ")},
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
									ClaimName: r.getSitesPVCName(bench),
								},
							},
						},
					},
				},
			},
		},
	}

	controllerutil.SetControllerReference(siteBackup, job, r.Scheme)
	return job
}

// buildBackupCronJob creates a CronJob for scheduled backup
func (r *SiteBackupReconciler) buildBackupCronJob(siteBackup *vyogotechv1alpha1.SiteBackup, bench *vyogotechv1alpha1.FrappeBench) *batchv1.CronJob {
	cronJobName := siteBackup.Name + "-backup"

	args := []string{"bench", "--site", siteBackup.Spec.Site, "backup"}
	if siteBackup.Spec.WithFiles {
		args = append(args, "--with-files")
	}
	if siteBackup.Spec.Compress {
		args = append(args, "--compress")
	}
	if siteBackup.Spec.BackupPath != "" {
		args = append(args, "--backup-path", siteBackup.Spec.BackupPath)
	}
	if siteBackup.Spec.BackupPathDB != "" {
		args = append(args, "--backup-path-db", siteBackup.Spec.BackupPathDB)
	}
	if siteBackup.Spec.BackupPathConf != "" {
		args = append(args, "--backup-path-conf", siteBackup.Spec.BackupPathConf)
	}
	if siteBackup.Spec.BackupPathFiles != "" {
		args = append(args, "--backup-path-files", siteBackup.Spec.BackupPathFiles)
	}
	if siteBackup.Spec.BackupPathPrivateFiles != "" {
		args = append(args, "--backup-path-private-files", siteBackup.Spec.BackupPathPrivateFiles)
	}
	if len(siteBackup.Spec.Exclude) > 0 {
		args = append(args, "--exclude", strings.Join(siteBackup.Spec.Exclude, ","))
	}
	if len(siteBackup.Spec.Include) > 0 {
		args = append(args, "--include", strings.Join(siteBackup.Spec.Include, ","))
	}
	if siteBackup.Spec.IgnoreBackupConf {
		args = append(args, "--ignore-backup-conf")
	}
	if siteBackup.Spec.Verbose {
		args = append(args, "--verbose")
	}

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName,
			Namespace: siteBackup.Namespace,
			Labels: map[string]string{
				"app":        "frappe",
				"site":       siteBackup.Spec.Site,
				"backup":     "true",
				"backupType": "scheduled",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: siteBackup.Spec.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:    "backup",
									Image:   r.getBenchImage(bench),
									Command: []string{"sh", "-c"},
									Args:    []string{strings.Join(args, " ")},
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
											ClaimName: r.getSitesPVCName(bench),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	controllerutil.SetControllerReference(siteBackup, cronJob, r.Scheme)
	return cronJob
}

// getBenchImage returns the image to use for the bench
func (r *SiteBackupReconciler) getBenchImage(bench *vyogotechv1alpha1.FrappeBench) string {
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		image := bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			image = fmt.Sprintf("%s:%s", image, bench.Spec.ImageConfig.Tag)
		} else if bench.Spec.FrappeVersion != "" {
			image = fmt.Sprintf("%s:%s", image, bench.Spec.FrappeVersion)
		}
		return image
	}
	// Default image
	return fmt.Sprintf("frappe/erpnext:%s", bench.Spec.FrappeVersion)
}

// getSitesPVCName returns the PVC name for sites volume
func (r *SiteBackupReconciler) getSitesPVCName(bench *vyogotechv1alpha1.FrappeBench) string {
	return fmt.Sprintf("%s-sites", bench.Name)
}

// updateSiteBackupStatus updates the status of a SiteBackup resource
func (r *SiteBackupReconciler) updateSiteBackupStatus(ctx context.Context, siteBackup *vyogotechv1alpha1.SiteBackup, phase, message, jobName string) error {
	// Re-get the latest version to avoid conflicts
	latest := &vyogotechv1alpha1.SiteBackup{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(siteBackup), latest); err != nil {
		return err
	}

	latest.Status.Phase = phase
	latest.Status.Message = message
	latest.Status.LastBackupJob = jobName

	if phase == "Succeeded" {
		latest.Status.LastBackup = metav1.Now()
	}

	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1alpha1.SiteBackup{}).
		Complete(r)
}
