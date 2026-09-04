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

// SiteCronReconciler reconciles a SiteCron object
type SiteCronReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=sitecrons,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitecrons/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitecrons/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete

func (r *SiteCronReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	siteCron := &vyogotechv1.SiteCron{}
	if err := r.Get(ctx, req.NamespacedName, siteCron); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteCron.Spec.SiteRef == nil || siteCron.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteCron, "siteRef.name is required", "ValidationFailed")
	}
	if siteCron.Spec.Schedule == "" {
		return r.failReconciliation(ctx, siteCron, "schedule is required", "ValidationFailed")
	}
	if siteCron.Spec.Method == "" {
		return r.failReconciliation(ctx, siteCron, "method is required", "ValidationFailed")
	}

	siteNamespace := siteCron.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteCron.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteCron.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteCron.Status.Phase = "Pending"
		r.setCondition(siteCron, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteCron)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteCron.Status.Phase = "Pending"
		r.setCondition(siteCron, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteCron)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Fetch referenced FrappeBench for image
	bench := &vyogotechv1.FrappeBench{}
	benchKey := types.NamespacedName{Name: site.Spec.BenchRef.Name, Namespace: site.Namespace}
	if err := r.Get(ctx, benchKey, bench); err != nil {
		return r.failReconciliation(ctx, siteCron, fmt.Sprintf("Referenced FrappeBench %s not found: %v", benchKey.Name, err), "BenchNotFound")
	}

	cronJobName := fmt.Sprintf("%s-cron-%s", site.Name, siteCron.Name)
	pvcName := fmt.Sprintf("%s-sites", bench.Name)
	benchImage := "frappe/erpnext:latest"
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		benchImage = bench.Spec.ImageConfig.Repository
		if bench.Spec.ImageConfig.Tag != "" {
			benchImage = fmt.Sprintf("%s:%s", benchImage, bench.Spec.ImageConfig.Tag)
		}
	}

	timeoutSec := siteCron.Spec.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 3600
	}

	cmdStr := fmt.Sprintf("bench --site %s execute %s", site.Status.ResolvedDomain, siteCron.Spec.Method)

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJobName,
			Namespace: siteCron.Namespace,
			Labels:    map[string]string{"app": "frappe", "site": site.Name},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          siteCron.Spec.Schedule,
			Suspend:           &siteCron.Spec.Suspended,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					ActiveDeadlineSeconds: &timeoutSec,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "frappe", "site": site.Name},
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:            "cron-runner",
									Image:           benchImage,
									ImagePullPolicy: corev1.PullIfNotPresent,
									Command:         []string{"bash", "-c", cmdStr},
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
			},
		},
	}
	_ = controllerutil.SetControllerReference(siteCron, cronJob, r.Scheme)

	var existingCron batchv1.CronJob
	err := r.Get(ctx, types.NamespacedName{Name: cronJobName, Namespace: siteCron.Namespace}, &existingCron)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, cronJob); err != nil {
			return r.failReconciliation(ctx, siteCron, fmt.Sprintf("Failed to create CronJob: %v", err), "CronJobCreationFailed")
		}
	} else if err == nil {
		existingCron.Spec = cronJob.Spec
		if err := r.Update(ctx, &existingCron); err != nil {
			return r.failReconciliation(ctx, siteCron, fmt.Sprintf("Failed to update CronJob: %v", err), "CronJobUpdateFailed")
		}
	}

	phase := "Active"
	if siteCron.Spec.Suspended {
		phase = "Suspended"
	}

	siteCron.Status.Phase = phase
	siteCron.Status.CronJobName = cronJobName
	siteCron.Status.ObservedGeneration = siteCron.Generation
	r.setCondition(siteCron, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "CronScheduled",
		Message: fmt.Sprintf("CronJob %s active on schedule %s", cronJobName, siteCron.Spec.Schedule),
	})
	_ = r.updateStatus(ctx, siteCron)

	return ctrl.Result{}, nil
}

func (r *SiteCronReconciler) failReconciliation(ctx context.Context, siteCron *vyogotechv1.SiteCron, msg, reason string) (ctrl.Result, error) {
	siteCron.Status.Phase = "Failed"
	r.setCondition(siteCron, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteCron, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteCron)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteCronReconciler) setCondition(siteCron *vyogotechv1.SiteCron, condition metav1.Condition) {
	condition.ObservedGeneration = siteCron.Generation
	meta.SetStatusCondition(&siteCron.Status.Conditions, condition)
}

func (r *SiteCronReconciler) updateStatus(ctx context.Context, siteCron *vyogotechv1.SiteCron) error {
	latest := &vyogotechv1.SiteCron{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteCron.Name, Namespace: siteCron.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteCron.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteCronReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("sitecron-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteCron{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}
