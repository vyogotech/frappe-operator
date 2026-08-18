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
	"time"

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

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// SiteMigrationReconciler reconciles a SiteMigration object
type SiteMigrationReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	IsOpenShift bool
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=sitemigrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitemigrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitemigrations/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups,verbs=get;list;watch;create
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *SiteMigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	siteMigration := &vyogotechv1.SiteMigration{}
	if err := r.Get(ctx, req.NamespacedName, siteMigration); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteMigration.Spec.SiteRef == nil || siteMigration.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteMigration, "siteRef.name is required", "ValidationFailed")
	}

	siteNamespace := siteMigration.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteMigration.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteMigration.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteMigration.Status.Phase = "Pending"
		r.setCondition(siteMigration, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteMigration)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteMigration.Status.Phase = "Pending"
		r.setCondition(siteMigration, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteMigration)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Fetch referenced FrappeBench for image
	bench := &vyogotechv1.FrappeBench{}
	benchKey := types.NamespacedName{Name: site.Spec.BenchRef.Name, Namespace: site.Namespace}
	if err := r.Get(ctx, benchKey, bench); err != nil {
		return r.failReconciliation(ctx, siteMigration, fmt.Sprintf("Referenced FrappeBench %s not found: %v", benchKey.Name, err), "BenchNotFound")
	}

	jobName := fmt.Sprintf("%s-migrate-%s", site.Name, siteMigration.Name)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	benchImage := "frappe/erpnext:latest"
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		benchImage = bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			benchImage = fmt.Sprintf("%s:%s", benchImage, bench.Spec.ImageConfig.Tag)
		}
	}

	cmdStr := fmt.Sprintf("bench --site %s migrate", site.Status.ResolvedDomain)
	if siteMigration.Spec.SkipFixtures {
		cmdStr += " --skip-fixtures"
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: siteMigration.Namespace,
			Labels:    map[string]string{"app": "frappe", "site": site.Name},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "frappe", "site": site.Name},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:   corev1.RestartPolicyOnFailure,
					SecurityContext: PodSecurityContextForBench(ctx, r.Client, r.IsOpenShift, bench.Namespace, bench.Spec.Security),
					Containers: []corev1.Container{
						{
							Name:            "migrate-runner",
							Image:           benchImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"bash", "-c", cmdStr},
							Env:             benchJobEnv(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
									SubPath:   "sites",
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
	_ = controllerutil.SetControllerReference(siteMigration, job, r.Scheme)

	var existingJob batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: siteMigration.Namespace}, &existingJob)
	if err != nil && errors.IsNotFound(err) {
		// Rollback safety: take a full backup and wait for it to succeed before
		// running `bench migrate`. The migrate job is not created until the
		// preflight backup is done, so a corrupt migration can always be reverted.
		if siteMigration.Spec.BackupBeforeMigrate {
			backupName := fmt.Sprintf("%s-pre-g%d", siteMigration.Name, siteMigration.Generation)
			done, berr := ensurePreflightBackup(ctx, r.Client, siteMigration.Namespace, site.Spec.SiteName, backupName)
			if berr != nil {
				return r.failReconciliation(ctx, siteMigration, fmt.Sprintf("Pre-migration backup failed: %v", berr), "PreBackupFailed")
			}
			if !done {
				siteMigration.Status.Phase = "BackingUp"
				siteMigration.Status.PreBackupRef = backupName
				r.setCondition(siteMigration, metav1.Condition{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "BackingUp",
					Message: fmt.Sprintf("Taking pre-migration backup %s before migrating %s", backupName, site.Name),
				})
				siteMigration.Status.ObservedGeneration = siteMigration.Generation
				_ = r.updateStatus(ctx, siteMigration)
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			siteMigration.Status.PreBackupRef = backupName
		}

		if err := r.Create(ctx, job); err != nil {
			return r.failReconciliation(ctx, siteMigration, fmt.Sprintf("Failed to create Migration Job: %v", err), "JobCreationFailed")
		}
		now := metav1.Now()
		siteMigration.Status.Phase = "Migrating"
		siteMigration.Status.JobName = jobName
		siteMigration.Status.StartTime = &now
	} else if err == nil {
		if existingJob.Status.Succeeded > 0 {
			now := metav1.Now()
			siteMigration.Status.Phase = "Succeeded"
			siteMigration.Status.CompletionTime = &now
			r.setCondition(siteMigration, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "MigrationSucceeded",
				Message: fmt.Sprintf("bench migrate completed successfully for %s", site.Name),
			})
		} else if existingJob.Status.Failed > 0 {
			siteMigration.Status.Phase = "Failed"
			r.setCondition(siteMigration, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "MigrationFailed",
				Message: fmt.Sprintf("bench migrate Job %s failed", jobName),
			})
		}
	}

	siteMigration.Status.ObservedGeneration = siteMigration.Generation
	_ = r.updateStatus(ctx, siteMigration)

	return ctrl.Result{}, nil
}

func (r *SiteMigrationReconciler) failReconciliation(ctx context.Context, siteMigration *vyogotechv1.SiteMigration, msg, reason string) (ctrl.Result, error) {
	siteMigration.Status.Phase = "Failed"
	r.setCondition(siteMigration, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteMigration, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteMigration)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteMigrationReconciler) setCondition(siteMigration *vyogotechv1.SiteMigration, condition metav1.Condition) {
	condition.ObservedGeneration = siteMigration.Generation
	meta.SetStatusCondition(&siteMigration.Status.Conditions, condition)
}

func (r *SiteMigrationReconciler) updateStatus(ctx context.Context, siteMigration *vyogotechv1.SiteMigration) error {
	latest := &vyogotechv1.SiteMigration{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteMigration.Name, Namespace: siteMigration.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteMigration.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteMigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("sitemigration-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteMigration{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
