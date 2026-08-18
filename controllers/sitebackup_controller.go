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
	"strings"
	"time"

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

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

const siteBackupFinalizer = "vyogo.tech/finalizer"

// SiteBackupReconciler reconciles a SiteBackup object
type SiteBackupReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	IsOpenShift bool
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteconfigs,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SiteBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	siteBackup := &vyogotechv1.SiteBackup{}
	if err := r.Get(ctx, req.NamespacedName, siteBackup); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		ReconciliationErrors.WithLabelValues("sitebackup", "fetch_error").Inc()
		return ctrl.Result{}, err
	}

	// Handle finalizer
	if siteBackup.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(siteBackup, siteBackupFinalizer) {
			controllerutil.AddFinalizer(siteBackup, siteBackupFinalizer)
			if err := r.Update(ctx, siteBackup); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(siteBackup, siteBackupFinalizer) {
			if err := r.handleFinalizer(ctx, siteBackup); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(siteBackup, siteBackupFinalizer)
			if err := r.Update(ctx, siteBackup); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Find the associated FrappeSite
	siteList := &vyogotechv1.FrappeSiteList{}
	if err := r.List(ctx, siteList, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	var benchRef *vyogotechv1.NamespacedName
	var targetSite *vyogotechv1.FrappeSite
	for i := range siteList.Items {
		if siteList.Items[i].Spec.SiteName == siteBackup.Spec.Site {
			benchRef = siteList.Items[i].Spec.BenchRef
			targetSite = &siteList.Items[i]
			break
		}
	}

	if benchRef == nil {
		err := fmt.Errorf("no FrappeSite found for site %s", siteBackup.Spec.Site)
		logger.Error(err, "cannot proceed with backup")
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Failed", err.Error(), "")
	}

	// Hold backup processing until the Site is fully Ready
	if targetSite != nil && targetSite.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		msg := fmt.Sprintf("Waiting for FrappeSite '%s' to be Ready (currently %s)", targetSite.Name, targetSite.Status.Phase)
		logger.Info(msg)

		if siteBackup.Status.Phase != "Pending" {
			_ = r.updateSiteBackupStatus(ctx, siteBackup, "Pending", msg, "")
		}
		// Requeue to check again later
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	benchNamespace := benchRef.Namespace
	if benchNamespace == "" {
		benchNamespace = siteBackup.Namespace
	}

	// Get the bench
	bench := &vyogotechv1.FrappeBench{}
	if err := r.Get(ctx, client.ObjectKey{Name: benchRef.Name, Namespace: benchNamespace}, bench); err != nil {
		return ctrl.Result{}, err
	}

	if siteBackup.Spec.Schedule == "" {
		result, err := r.reconcileOneTimeBackup(ctx, siteBackup, bench)
		if err != nil {
			ReconciliationErrors.WithLabelValues("sitebackup", "backup_error").Inc()
			ReconciliationDuration.WithLabelValues("sitebackup", "error").Observe(time.Since(startTime).Seconds())
		} else {
			ReconciliationDuration.WithLabelValues("sitebackup", "success").Observe(time.Since(startTime).Seconds())
		}
		return result, err
	} else {
		result, err := r.reconcileScheduledBackup(ctx, siteBackup, bench)
		if err != nil {
			ReconciliationErrors.WithLabelValues("sitebackup", "schedule_error").Inc()
			ReconciliationDuration.WithLabelValues("sitebackup", "error").Observe(time.Since(startTime).Seconds())
		} else {
			ReconciliationDuration.WithLabelValues("sitebackup", "success").Observe(time.Since(startTime).Seconds())
		}
		return result, err
	}
}

func (r *SiteBackupReconciler) handleFinalizer(ctx context.Context, siteBackup *vyogotechv1.SiteBackup) error {
	logger := log.FromContext(ctx)
	jobName := siteBackup.Name + "-backup"

	if siteBackup.Spec.Schedule == "" {
		// One-time backup: delete Job
		job := &batchv1.Job{}
		err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: siteBackup.Namespace}, job)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			logger.Info("Deleting associated Job", "Job", job.Name)
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				return err
			}
		}
	} else {
		// Scheduled backup: delete CronJob
		cronJob := &batchv1.CronJob{}
		err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: siteBackup.Namespace}, cronJob)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			logger.Info("Deleting associated CronJob", "CronJob", cronJob.Name)
			if err := r.Delete(ctx, cronJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				return err
			}
		}
	}
	return nil
}

// warnIfObjectStorage emits a warning when a --with-files backup is requested for a site
// that offloads its Files to S3 object storage. Those files live in the bucket, not on the
// sites PVC, so `bench backup --with-files` cannot capture them — the file source of truth
// is the bucket (enable versioning). Detection is read-only: it looks for a SiteConfig in
// the namespace that references this site with objectStorage set.
func (r *SiteBackupReconciler) warnIfObjectStorage(ctx context.Context, sb *vyogotechv1.SiteBackup) {
	if !sb.Spec.WithFiles {
		return
	}
	var list vyogotechv1.SiteConfigList
	if err := r.List(ctx, &list, client.InNamespace(sb.Namespace)); err != nil {
		return
	}
	for i := range list.Items {
		sc := &list.Items[i]
		if sc.Spec.ObjectStorage != nil && sc.Spec.SiteRef != nil && sc.Spec.SiteRef.Name == sb.Spec.Site {
			r.Recorder.Event(sb, corev1.EventTypeWarning, "FilesInObjectStorage",
				"Site offloads Files to S3 object storage; --with-files will not capture them. Ensure bucket versioning for file durability.")
			return
		}
	}
}

// reconcileOneTimeBackup handles one-time backup creation and status updates
func (r *SiteBackupReconciler) reconcileOneTimeBackup(ctx context.Context, siteBackup *vyogotechv1.SiteBackup, bench *vyogotechv1.FrappeBench) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	jobName := siteBackup.Name + "-backup"

	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: siteBackup.Namespace}, job)

	if errors.IsNotFound(err) {
		if siteBackup.Status.Phase == "Succeeded" || siteBackup.Status.Phase == "Failed" {
			// Job is finished, do not recreate
			return ctrl.Result{}, nil
		}
		r.warnIfObjectStorage(ctx, siteBackup)
		job = r.buildBackupJob(ctx, siteBackup, bench)
		if err := r.Create(ctx, job); err != nil {
			logger.Error(err, "Failed to create backup job")
			return ctrl.Result{}, err
		}
		logger.Info("Created backup job", "job", job.Name)
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Running", "Backup job created", job.Name)
	}

	if err != nil {
		logger.Error(err, "Failed to get backup job")
		return ctrl.Result{}, err
	}

	if job.Status.Succeeded > 0 {
		if siteBackup.Status.Phase != "Succeeded" {
			return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Succeeded", "Backup completed successfully", job.Name)
		}
	} else if job.Status.Failed > 0 {
		if siteBackup.Status.Phase != "Failed" {
			return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Failed", "Backup job failed", job.Name)
		}
	} else {
		if siteBackup.Status.Phase != "Running" {
			return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Running", "Backup job running", job.Name)
		}
	}

	return ctrl.Result{}, nil
}

// reconcileScheduledBackup handles scheduled backup creation
func (r *SiteBackupReconciler) reconcileScheduledBackup(ctx context.Context, siteBackup *vyogotechv1.SiteBackup, bench *vyogotechv1.FrappeBench) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cronJobName := siteBackup.Name + "-backup"

	r.warnIfObjectStorage(ctx, siteBackup)
	desiredCronJob := r.buildBackupCronJob(ctx, siteBackup, bench)
	currentCronJob := &batchv1.CronJob{}
	err := r.Get(ctx, client.ObjectKey{Name: cronJobName, Namespace: siteBackup.Namespace}, currentCronJob)

	if errors.IsNotFound(err) {
		if err := r.Create(ctx, desiredCronJob); err != nil {
			logger.Error(err, "Failed to create backup cronjob")
			return ctrl.Result{}, err
		}
		logger.Info("Created backup cronjob", "cronjob", desiredCronJob.Name)
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Scheduled", "Scheduled backup created", desiredCronJob.Name)
	}

	if err != nil {
		logger.Error(err, "Failed to get backup cronjob")
		return ctrl.Result{}, err
	}

	// CronJob exists, check if it needs updating
	if !reflect.DeepEqual(desiredCronJob.Spec, currentCronJob.Spec) {
		currentCronJob.Spec = desiredCronJob.Spec
		if err := r.Update(ctx, currentCronJob); err != nil {
			logger.Error(err, "Failed to update backup cronjob")
			return ctrl.Result{}, err
		}
		logger.Info("Updated backup cronjob", "cronjob", currentCronJob.Name)
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Scheduled", "Scheduled backup updated", currentCronJob.Name)
	}

	if siteBackup.Status.Phase != "Scheduled" {
		return ctrl.Result{}, r.updateSiteBackupStatus(ctx, siteBackup, "Scheduled", "Scheduled backup active", currentCronJob.Name)
	}

	return ctrl.Result{}, nil
}

// sitesRelativeBackupPath normalizes a backup path for `bench backup`.
//
// Callers (the console/agent) express backup paths bench-root-relative, e.g.
// "sites/<site>/private/backups/<name>/database.sql.gz" — the same convention
// the restore path uses (the restore job `cp`s from the bench root). But
// `bench backup --backup-path*` resolves its arguments relative to the *sites*
// directory (frappe-bench/sites/), so passing "sites/..." lands the artifacts
// at "frappe-bench/sites/sites/..." (a doubled "sites/"). The restore then
// reads "frappe-bench/sites/..." (single) and never finds them.
//
// Stripping a single leading "sites/" makes `bench backup` write to the same
// single-"sites/" location the restore reads, so the round-trip matches. Paths
// that don't start with "sites/" (absolute paths, already-relative paths) are
// left untouched.
func sitesRelativeBackupPath(p string) string {
	return strings.TrimPrefix(p, "sites/")
}

// normalizeBackupJobPath prepares a console-supplied backup path for the
// `bench backup` job: it relocates the toxic "private/backups/" segment to the
// Frappe-safe "private/vyogo-backups/" dir (see relocateNamedBackupPath) and then
// strips the leading "sites/" so the sites-relative path `bench backup` expects
// lands at the same single-"sites/" location the restore job reads.
func normalizeBackupJobPath(p string) string {
	return sitesRelativeBackupPath(relocateNamedBackupPath(p))
}

// buildBackupArgs creates the command arguments for the backup job
func (r *SiteBackupReconciler) buildBackupArgs(siteBackup *vyogotechv1.SiteBackup) []string {
	args := []string{"--site", siteBackup.Spec.Site, "backup"}
	if siteBackup.Spec.WithFiles {
		args = append(args, "--with-files")
	}
	if siteBackup.Spec.Compress {
		args = append(args, "--compress")
	}
	if siteBackup.Spec.BackupPath != "" {
		args = append(args, "--backup-path", normalizeBackupJobPath(siteBackup.Spec.BackupPath))
	}
	if siteBackup.Spec.BackupPathDB != "" {
		args = append(args, "--backup-path-db", normalizeBackupJobPath(siteBackup.Spec.BackupPathDB))
	}
	if siteBackup.Spec.BackupPathConf != "" {
		args = append(args, "--backup-path-conf", normalizeBackupJobPath(siteBackup.Spec.BackupPathConf))
	}
	if siteBackup.Spec.BackupPathFiles != "" {
		args = append(args, "--backup-path-files", normalizeBackupJobPath(siteBackup.Spec.BackupPathFiles))
	}
	if siteBackup.Spec.BackupPathPrivateFiles != "" {
		args = append(args, "--backup-path-private-files", normalizeBackupJobPath(siteBackup.Spec.BackupPathPrivateFiles))
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
	return args
}

// s3Configured reports whether this backup should also be uploaded to S3.
func s3Configured(sb *vyogotechv1.SiteBackup) bool {
	return sb.Spec.Storage != nil && sb.Spec.Storage.Type == "s3" && sb.Spec.Storage.S3 != nil
}

// benchRootPath turns a bench-root-relative backup path (as the console sends,
// "sites/<site>/private/backups/<name>/file") into the absolute on-disk path the
// artifacts actually live at, applying the same private/backups → vyogo-backups
// relocation the backup job uses.
func benchRootPath(p string) string {
	return "/home/frappe/frappe-bench/" + relocateNamedBackupPath(p)
}

// backupS3Env returns the env vars the S3 upload step needs, pulling the access
// and secret keys from the referenced Secrets.
func backupS3Env(s3 *vyogotechv1.S3Config) []corev1.EnvVar {
	useSSL := "true"
	if !s3.UseSSL {
		useSSL = "false"
	}
	return []corev1.EnvVar{
		{Name: "S3_ENDPOINT", Value: s3.Endpoint},
		{Name: "S3_BUCKET", Value: s3.Bucket},
		{Name: "S3_REGION", Value: s3.Region},
		{Name: "S3_USE_SSL", Value: useSSL},
		{Name: "S3_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &s3.AccessKeySecret}},
		{Name: "S3_SECRET_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &s3.SecretKeySecret}},
	}
}

// backupCommandAndArgs returns the container command/args for the backup job.
// For a plain PVC backup it runs `bench ... backup ...` directly. When S3 storage
// is configured it wraps the same bench invocation in a shell script that, on
// success, uploads the produced artifacts to s3://<bucket>/<site>/<name>/ via
// boto3 (already present in the bench image, as the restore job relies on it).
// The S3 upload is additive: artifacts are always written to the PVC first, so a
// failed upload never loses the local backup.
func (r *SiteBackupReconciler) backupCommandAndArgs(sb *vyogotechv1.SiteBackup) (command []string, args []string) {
	benchArgs := r.buildBackupArgs(sb)
	if !s3Configured(sb) {
		return []string{"bench"}, benchArgs
	}

	// Map each configured artifact path to (localAbsolutePath, s3Key).
	prefix := fmt.Sprintf("%s/%s", sb.Spec.Site, sb.Name)
	type artifact struct{ local, key string }
	var artifacts []artifact
	add := func(p, name string) {
		if p != "" {
			artifacts = append(artifacts, artifact{local: benchRootPath(p), key: prefix + "/" + name})
		}
	}
	add(sb.Spec.BackupPathDB, "database.sql.gz")
	add(sb.Spec.BackupPathConf, "site_config_backup.json")
	add(sb.Spec.BackupPathFiles, "files.tar")
	add(sb.Spec.BackupPathPrivateFiles, "private-files.tar")

	var uploads strings.Builder
	for _, a := range artifacts {
		uploads.WriteString(fmt.Sprintf("  (%q, %q),\n", a.local, a.key))
	}

	// bench is invoked via `bench <args...>` inside the script; quote the args.
	quoted := make([]string, len(benchArgs))
	for i, a := range benchArgs {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	script := fmt.Sprintf(`set -euo pipefail
bench %s
echo "Backup complete, uploading artifacts to S3 bucket $S3_BUCKET ..."
# boto3 is used for the upload. Prefer the image's copy; fall back to installing
# it into PYTHONPATH (/tmp/pip) at runtime so this works on bench images that
# don't bundle it. For production, bake boto3 into the bench image to avoid the
# network install.
if ! python3 -c "import boto3" 2>/dev/null; then
  echo "boto3 not found in image, installing to /tmp/pip ..."
  mkdir -p /tmp/pip
  python3 -m pip install --quiet --target /tmp/pip boto3
fi
python3 - <<'PYEOF'
import os, sys, boto3
from botocore.client import Config
endpoint = os.environ["S3_ENDPOINT"]
use_ssl = os.environ.get("S3_USE_SSL", "true").lower() == "true"
s3 = boto3.client(
    "s3",
    endpoint_url=endpoint,
    region_name=os.environ.get("S3_REGION") or None,
    aws_access_key_id=os.environ["S3_ACCESS_KEY"],
    aws_secret_access_key=os.environ["S3_SECRET_KEY"],
    use_ssl=use_ssl,
    config=Config(signature_version="s3v4"),
)
bucket = os.environ["S3_BUCKET"]
artifacts = [
%s]
for local, key in artifacts:
    if not os.path.exists(local):
        print(f"  skip (missing): {local}")
        continue
    print(f"  upload {local} -> s3://{bucket}/{key}")
    s3.upload_file(local, bucket, key)
print("S3 upload complete")
PYEOF
`, strings.Join(quoted, " "), uploads.String())

	return []string{"bash", "-c"}, []string{script}
}

// buildBackupJob creates a Job for one-time backup
func (r *SiteBackupReconciler) buildBackupJob(ctx context.Context, siteBackup *vyogotechv1.SiteBackup, bench *vyogotechv1.FrappeBench) *batchv1.Job {
	jobName := siteBackup.Name + "-backup"
	command, args := r.backupCommandAndArgs(siteBackup)
	env := benchJobEnv()
	if s3Configured(siteBackup) {
		env = append(env, backupS3Env(siteBackup.Spec.Storage.S3)...)
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
			BackoffLimit: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:   corev1.RestartPolicyNever,
					SecurityContext: PodSecurityContextForBench(ctx, r.Client, r.IsOpenShift, bench.Namespace, bench.Spec.Security),
					ImagePullSecrets: func() []corev1.LocalObjectReference {
						if bench.Spec.ImageConfig != nil && len(bench.Spec.ImageConfig.PullSecrets) > 0 {
							secrets := make([]corev1.LocalObjectReference, len(bench.Spec.ImageConfig.PullSecrets))
							for i, s := range bench.Spec.ImageConfig.PullSecrets {
								secrets[i] = corev1.LocalObjectReference{Name: s.Name}
							}
							return secrets
						}
						return nil
					}(),
					Containers: []corev1.Container{
						{
							Name:  "backup",
							Image: r.getBenchImage(bench),
							ImagePullPolicy: func() corev1.PullPolicy {
								if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.PullPolicy != "" {
									return bench.Spec.ImageConfig.PullPolicy
								}
								return corev1.PullPolicy("")
							}(),
							Command: command,
							Args:    args,
							Env:     env,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
									SubPath:   "frappe-sites",
								},
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites/assets",
									SubPath:   "frappe-sites/assets",
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
	applyDefaultJobTTL(&job.Spec)

	_ = controllerutil.SetControllerReference(siteBackup, job, r.Scheme)
	return job
}

// buildBackupCronJob creates a CronJob for scheduled backup
func (r *SiteBackupReconciler) buildBackupCronJob(ctx context.Context, siteBackup *vyogotechv1.SiteBackup, bench *vyogotechv1.FrappeBench) *batchv1.CronJob {
	cronJobName := siteBackup.Name + "-backup"
	command, args := r.backupCommandAndArgs(siteBackup)
	env := benchJobEnv()
	if s3Configured(siteBackup) {
		env = append(env, backupS3Env(siteBackup.Spec.Storage.S3)...)
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
			Schedule:          siteBackup.Spec.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					BackoffLimit: int32Ptr(1),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:   corev1.RestartPolicyNever,
							SecurityContext: PodSecurityContextForBench(ctx, r.Client, r.IsOpenShift, bench.Namespace, bench.Spec.Security),
							ImagePullSecrets: func() []corev1.LocalObjectReference {
								if bench.Spec.ImageConfig != nil && len(bench.Spec.ImageConfig.PullSecrets) > 0 {
									secrets := make([]corev1.LocalObjectReference, len(bench.Spec.ImageConfig.PullSecrets))
									for i, s := range bench.Spec.ImageConfig.PullSecrets {
										secrets[i] = corev1.LocalObjectReference{Name: s.Name}
									}
									return secrets
								}
								return nil
							}(),
							Containers: []corev1.Container{
								{
									Name:  "backup",
									Image: r.getBenchImage(bench),
									ImagePullPolicy: func() corev1.PullPolicy {
										if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.PullPolicy != "" {
											return bench.Spec.ImageConfig.PullPolicy
										}
										return corev1.PullPolicy("")
									}(),
									Command: command,
									Args:    args,
									Env:     env,
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "sites",
											MountPath: "/home/frappe/frappe-bench/sites",
											SubPath:   "frappe-sites",
										},
										{
											Name:      "sites",
											MountPath: "/home/frappe/frappe-bench/sites/assets",
											SubPath:   "frappe-sites/assets",
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

	_ = controllerutil.SetControllerReference(siteBackup, cronJob, r.Scheme)
	applyDefaultJobTTL(&cronJob.Spec.JobTemplate.Spec)

	return cronJob
}

// getBenchImage returns the image to use for the bench
func (r *SiteBackupReconciler) getBenchImage(bench *vyogotechv1.FrappeBench) string {
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
func (r *SiteBackupReconciler) getSitesPVCName(bench *vyogotechv1.FrappeBench) string {
	return fmt.Sprintf("%s-sites", bench.Name)
}

// updateSiteBackupStatus updates the status of a SiteBackup resource
func (r *SiteBackupReconciler) updateSiteBackupStatus(ctx context.Context, siteBackup *vyogotechv1.SiteBackup, phase, message, jobName string) error {
	// Re-get the latest version to avoid conflicts
	latest := &vyogotechv1.SiteBackup{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(siteBackup), latest); err != nil {
		return err
	}

	latest.Status.Phase = phase
	latest.Status.Message = message
	latest.Status.LastBackupJob = jobName

	if phase == "Succeeded" {
		latest.Status.LastBackup = metav1.Now()
		if s3Configured(latest) {
			// Record where the artifacts were uploaded so the control plane can
			// generate presigned download URLs (see backupCommandAndArgs S3 prefix).
			latest.Status.StorageLocation = fmt.Sprintf("s3://%s/%s/%s", latest.Spec.Storage.S3.Bucket, latest.Spec.Site, latest.Name)
		}
	}

	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteBackup{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}
