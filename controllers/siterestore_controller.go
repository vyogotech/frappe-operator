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

// SiteRestoreReconciler reconciles a SiteRestore object
type SiteRestoreReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=siterestores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=siterestores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=siterestores/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *SiteRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	siteRestore := &vyogotechv1alpha1.SiteRestore{}
	if err := r.Get(ctx, req.NamespacedName, siteRestore); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle completion
	if siteRestore.Status.Phase == "Succeeded" || siteRestore.Status.Phase == "Failed" {
		return ctrl.Result{}, nil
	}

	// Get the bench
	bench := &vyogotechv1alpha1.FrappeBench{}
	if err := r.Get(ctx, client.ObjectKey{Name: siteRestore.Spec.BenchRef.Name, Namespace: siteRestore.Spec.BenchRef.Namespace}, bench); err != nil {
		return ctrl.Result{}, err
	}

	jobName := siteRestore.Name + "-restore"
	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: siteRestore.Namespace}, job)

	if errors.IsNotFound(err) {
		job = r.buildRestoreJob(siteRestore, bench)
		if err := r.Create(ctx, job); err != nil {
			logger.Error(err, "Failed to create restore job")
			return ctrl.Result{}, err
		}
		logger.Info("Created restore job", "job", job.Name)
		return ctrl.Result{}, r.updateStatus(ctx, siteRestore, "Running", "Restore job created", job.Name)
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	// Check job status
	if job.Status.Succeeded > 0 {
		return ctrl.Result{}, r.updateStatus(ctx, siteRestore, "Succeeded", "Restore completed successfully", job.Name)
	} else if job.Status.Failed > 0 {
		return ctrl.Result{}, r.updateStatus(ctx, siteRestore, "Failed", "Restore job failed", job.Name)
	}

	return ctrl.Result{}, nil
}

func (r *SiteRestoreReconciler) buildRestoreScript(siteRestore *vyogotechv1alpha1.SiteRestore) string {
	script := `#!/bin/bash
set -e

# Setup user for OpenShift compatibility
if ! whoami &>/dev/null; then
  export USER=frappe
  export LOGNAME=frappe
  if [ -w /etc/passwd ]; then
    echo "frappe:x:$(id -u):0:frappe user:/home/frappe:/sbin/nologin" >> /etc/passwd
  fi
fi

cd /home/frappe/frappe-bench
mkdir -p /tmp/restore
`

	// Helper for S3 download
	s3Download := func(source vyogotechv1alpha1.BackupSource, target string, envPrefix string) {
		if source.S3 != nil {
			script += fmt.Sprintf(`
echo "Downloading %s from S3..."
python3 << 'PYTHON_SCRIPT'
import os, boto3, sys
bucket = os.getenv("%s_S3_BUCKET")
region = os.getenv("%s_S3_REGION")
endpoint = os.getenv("%s_S3_ENDPOINT")
access_key = os.getenv("%s_AWS_ACCESS_KEY_ID")
secret_key = os.getenv("%s_AWS_SECRET_ACCESS_KEY")
key = os.getenv("%s_S3_KEY")

s3 = boto3.client('s3', 
    region_name=region if region else None,
    endpoint_url=endpoint if endpoint else None,
    aws_access_key_id=access_key,
    aws_secret_access_key=secret_key)

print(f"Downloading s3://{bucket}/{key} to %s...")
s3.download_file(bucket, key, "%s")
PYTHON_SCRIPT
`, target, envPrefix, envPrefix, envPrefix, envPrefix, envPrefix, envPrefix, target, target)
		} else if source.LocalPath != "" {
			script += fmt.Sprintf(`
echo "Using local backup path: %s"
cp "%s" "%s"
`, source.LocalPath, source.LocalPath, target)
		}
	}

	// Download DB Backup
	dbPath := "/tmp/restore/database.sql.gz"
	s3Download(siteRestore.Spec.DatabaseBackupSource, dbPath, "DB")

	// Base restore command
	restoreCmd := fmt.Sprintf("bench --site %s restore %s", siteRestore.Spec.Site, dbPath)

	if siteRestore.Spec.PublicFilesSource != nil {
		publicPath := "/tmp/restore/public.tar.gz"
		s3Download(*siteRestore.Spec.PublicFilesSource, publicPath, "PUBLIC")
		restoreCmd += fmt.Sprintf(" --with-public-files %s", publicPath)
	}

	if siteRestore.Spec.PrivateFilesSource != nil {
		privatePath := "/tmp/restore/private.tar.gz"
		s3Download(*siteRestore.Spec.PrivateFilesSource, privatePath, "PRIVATE")
		restoreCmd += fmt.Sprintf(" --with-private-files %s", privatePath)
	}

	if siteRestore.Spec.Force {
		restoreCmd += " --force"
	}

	script += fmt.Sprintf(`
echo "Executing restore command..."
# Handle admin password if provided via env
if [ ! -z "$ADMIN_PASSWORD" ]; then
  %s --admin-password "$ADMIN_PASSWORD"
else
  %s
fi

echo "Restore finished. Cleaning up..."
rm -rf /tmp/restore
`, restoreCmd, restoreCmd)

	return script
}

func (r *SiteRestoreReconciler) buildRestoreJob(siteRestore *vyogotechv1alpha1.SiteRestore, bench *vyogotechv1alpha1.FrappeBench) *batchv1.Job {
	env := []corev1.EnvVar{}

	// Helper for adding S3 env vars
	addS3Env := func(source vyogotechv1alpha1.BackupSource, prefix string) {
		if source.S3 != nil {
			env = append(env, corev1.EnvVar{Name: prefix + "_S3_BUCKET", Value: source.S3.Bucket})
			env = append(env, corev1.EnvVar{Name: prefix + "_S3_KEY", Value: source.S3.Key})
			if source.S3.Region != "" {
				env = append(env, corev1.EnvVar{Name: prefix + "_S3_REGION", Value: source.S3.Region})
			}
			if source.S3.Endpoint != "" {
				env = append(env, corev1.EnvVar{Name: prefix + "_S3_ENDPOINT", Value: source.S3.Endpoint})
			}
			env = append(env, corev1.EnvVar{
				Name: prefix + "_AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &source.S3.AccessKeySecret,
				},
			})
			env = append(env, corev1.EnvVar{
				Name: prefix + "_AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &source.S3.SecretKeySecret,
				},
			})
		}
	}

	addS3Env(siteRestore.Spec.DatabaseBackupSource, "DB")
	if siteRestore.Spec.PublicFilesSource != nil {
		addS3Env(*siteRestore.Spec.PublicFilesSource, "PUBLIC")
	}
	if siteRestore.Spec.PrivateFilesSource != nil {
		addS3Env(*siteRestore.Spec.PrivateFilesSource, "PRIVATE")
	}

	if siteRestore.Spec.AdminPasswordSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name: "ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: siteRestore.Spec.AdminPasswordSecretRef,
			},
		})
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      siteRestore.Name + "-restore",
			Namespace: siteRestore.Namespace,
			Labels: map[string]string{
				"app":     "frappe",
				"site":    siteRestore.Spec.Site,
				"restore": "true",
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					// Reusing logic from SiteBackup for now
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "restore",
							Image:   r.getBenchImage(bench),
							Command: []string{"bash", "-c"},
							Args:    []string{r.buildRestoreScript(siteRestore)},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
									SubPath:   "frappe-sites",
								},
							},
							Env: env,
							// Reusing logic from SiteBackup for now
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             boolPtr(true),
								AllowPrivilegeEscalation: boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("%s-sites", bench.Name),
								},
							},
						},
					},
				},
			},
		},
	}

	controllerutil.SetControllerReference(siteRestore, job, r.Scheme)
	return job
}

func (r *SiteRestoreReconciler) getBenchImage(bench *vyogotechv1alpha1.FrappeBench) string {
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		image := bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			return fmt.Sprintf("%s:%s", image, bench.Spec.ImageConfig.Tag)
		}
		return fmt.Sprintf("%s:%s", image, bench.Spec.FrappeVersion)
	}
	return fmt.Sprintf("frappe/erpnext:%s", bench.Spec.FrappeVersion)
}

func (r *SiteRestoreReconciler) updateStatus(ctx context.Context, siteRestore *vyogotechv1alpha1.SiteRestore, phase, message, jobName string) error {
	latest := &vyogotechv1alpha1.SiteRestore{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(siteRestore), latest); err != nil {
		return err
	}

	latest.Status.Phase = phase
	latest.Status.Message = message
	latest.Status.RestoreJob = jobName
	if phase == "Succeeded" || phase == "Failed" {
		now := metav1.Now()
		latest.Status.CompletionTime = &now
	}

	return r.Status().Update(ctx, latest)
}

func (r *SiteRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1alpha1.SiteRestore{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
