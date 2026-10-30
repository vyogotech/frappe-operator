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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// maxJobNameLength keeps Job names within the 63 characters Kubernetes allows
// for the job-name label it puts on the Job's pods.
const maxJobNameLength = 63

// microserviceJobName names the Job for the current spec: one Job per spec,
// so editing the image, command, args, env, resources or run settings starts a
// new run, and an unchanged spec never runs twice. The binding is deliberately
// not part of it: a one-time task must not re-run because credentials rotated.
func microserviceJobName(ms *vyogotechv1.FrappeMicroservice) string {
	key, _ := json.Marshal(struct {
		Name      string                            `json:"name"`
		Image     vyogotechv1.ImageConfig           `json:"image"`
		Command   []string                          `json:"command,omitempty"`
		Args      []string                          `json:"args,omitempty"`
		Env       []corev1.EnvVar                   `json:"env,omitempty"`
		Resources *vyogotechv1.ResourceRequirements `json:"resources,omitempty"`
		Job       *vyogotechv1.MicroserviceJobSpec  `json:"job,omitempty"`
	}{ms.Name, ms.Spec.Image, ms.Spec.Command, ms.Spec.Args, ms.Spec.Env, ms.Spec.Resources, ms.Spec.Job})
	sum := sha256.Sum256(key)
	suffix := hex.EncodeToString(sum[:])[:8]

	prefix := ms.Name
	if max := maxJobNameLength - len(suffix) - 1; len(prefix) > max {
		prefix = strings.TrimRight(prefix[:max], "-.")
	}
	return prefix + "-" + suffix
}

// microserviceJobSpec builds the Job spec for a run of type Job, and the job
// template of type CronJob.
func (r *FrappeMicroserviceReconciler) microserviceJobSpec(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, podAnnotations map[string]string) batchv1.JobSpec {
	spec := batchv1.JobSpec{}
	if j := ms.Spec.Job; j != nil {
		spec.BackoffLimit = j.BackoffLimit
		spec.ActiveDeadlineSeconds = j.ActiveDeadlineSeconds
		spec.TTLSecondsAfterFinished = j.TTLSecondsAfterFinished
	}
	spec.Template.Labels = microserviceLabels(ms)
	spec.Template.Annotations = podAnnotations
	r.mutatePodSpec(ctx, ms, &spec.Template.Spec, false)
	return spec
}

// reconcileJob handles type Job. It creates the Job for the current spec if it
// has not run yet, prunes Jobs of earlier specs, and reports the run.
func (r *FrappeMicroserviceReconciler) reconcileJob(ctx context.Context, ms *vyogotechv1.FrappeMicroservice) (ctrl.Result, error) {
	name := microserviceJobName(ms)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ms.Namespace}, job)
	switch {
	case apierrors.IsNotFound(err):
		if microserviceJobFinished(ms, name) {
			// The run finished and Kubernetes removed it (ttlSecondsAfterFinished).
			// Keep the recorded result; never start the same spec again.
			return ctrl.Result{}, r.pruneJobs(ctx, ms, name)
		}
		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ms.Namespace, Labels: microserviceLabels(ms)},
			Spec:       r.microserviceJobSpec(ctx, ms, nil),
		}
		if err := controllerutil.SetControllerReference(ms, job, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
	case err != nil:
		return ctrl.Result{}, err
	case !metav1.IsControlledBy(job, ms):
		msg := fmt.Sprintf("Job %s exists and is not owned by this FrappeMicroservice; it was left untouched", name)
		meta.SetStatusCondition(&ms.Status.Conditions, metav1.Condition{
			Type: vyogotechv1.MicroserviceConditionComplete, Status: metav1.ConditionFalse, Reason: "NameConflict", Message: msg, ObservedGeneration: ms.Generation,
		})
		r.Recorder.Event(ms, corev1.EventTypeWarning, "NameConflict", msg)
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}

	if ms.Status.JobName != name {
		// A new run: forget the previous run's result.
		meta.RemoveStatusCondition(&ms.Status.Conditions, vyogotechv1.MicroserviceConditionComplete)
	}
	ms.Status.JobName = name
	if err := r.pruneJobs(ctx, ms, name); err != nil {
		return ctrl.Result{}, err
	}

	cond := metav1.Condition{Type: vyogotechv1.MicroserviceConditionComplete, ObservedGeneration: ms.Generation}
	running := false
	switch failed, reason := jobFailure(job); {
	case jobSucceeded(job):
		cond.Status, cond.Reason, cond.Message = metav1.ConditionTrue, "Succeeded", fmt.Sprintf("Job %s succeeded", name)
	case failed:
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, "Failed", fmt.Sprintf("Job %s failed: %s", name, reason)
	default:
		running = true
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, "Running", fmt.Sprintf("Job %s is running", name)
	}
	meta.SetStatusCondition(&ms.Status.Conditions, cond)
	if running {
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	return ctrl.Result{}, nil
}

// microserviceJobFinished reports whether the run named name already finished,
// according to the status the operator recorded.
func microserviceJobFinished(ms *vyogotechv1.FrappeMicroservice, name string) bool {
	if ms.Status.JobName != name {
		return false
	}
	c := meta.FindStatusCondition(ms.Status.Conditions, vyogotechv1.MicroserviceConditionComplete)
	return c != nil && (c.Status == metav1.ConditionTrue || c.Reason == "Failed")
}

func jobSucceeded(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func jobFailure(job *batchv1.Job) (bool, string) {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			reason := c.Reason
			if c.Message != "" {
				reason = fmt.Sprintf("%s (%s)", c.Reason, c.Message)
			}
			return true, reason
		}
	}
	return false, ""
}

// pruneJobs deletes Jobs of earlier specs. A run that is still going is
// stopped: the new spec supersedes it.
func (r *FrappeMicroserviceReconciler) pruneJobs(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, keep string) error {
	list := &batchv1.JobList{}
	if err := r.List(ctx, list, client.InNamespace(ms.Namespace), client.MatchingLabels{microserviceLabel: ms.Name}); err != nil {
		return err
	}
	for i := range list.Items {
		job := &list.Items[i]
		if job.Name == keep || !metav1.IsControlledBy(job, ms) {
			continue
		}
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

// reconcileCronJob handles type CronJob.
func (r *FrappeMicroserviceReconciler) reconcileCronJob(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, version string) (ctrl.Result, error) {
	spec := ms.Spec.CronJob
	if spec == nil {
		// Rejected by CEL; guard against objects created before validation.
		spec = &vyogotechv1.MicroserviceCronJobSpec{}
	}
	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: ms.Name, Namespace: ms.Namespace}}
	conflict, err := r.createOrUpdateOwned(ctx, ms, cj, func() error {
		cj.Labels = mergeLabels(cj.Labels, microserviceLabels(ms))
		cj.Spec.Schedule = spec.Schedule
		cj.Spec.TimeZone = spec.TimeZone
		cj.Spec.ConcurrencyPolicy = spec.ConcurrencyPolicy
		if cj.Spec.ConcurrencyPolicy == "" {
			cj.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
		}
		suspend := spec.Suspend != nil && *spec.Suspend
		cj.Spec.Suspend = &suspend
		cj.Spec.StartingDeadlineSeconds = spec.StartingDeadlineSeconds
		cj.Spec.SuccessfulJobsHistoryLimit = spec.SuccessfulJobsHistoryLimit
		cj.Spec.FailedJobsHistoryLimit = spec.FailedJobsHistoryLimit
		cj.Spec.JobTemplate.Labels = microserviceLabels(ms)
		// The binding version lives on the job template, so every run after a
		// credential rotation is started from the updated template.
		cj.Spec.JobTemplate.Spec = r.microserviceJobSpec(ctx, ms, map[string]string{bindingVersionAnnotation: version})
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if conflict {
		msg := fmt.Sprintf("CronJob %s exists and is not owned by this FrappeMicroservice; it was left untouched", cj.Name)
		meta.SetStatusCondition(&ms.Status.Conditions, metav1.Condition{
			Type: vyogotechv1.MicroserviceConditionScheduled, Status: metav1.ConditionFalse, Reason: "NameConflict", Message: msg, ObservedGeneration: ms.Generation,
		})
		r.Recorder.Event(ms, corev1.EventTypeWarning, "NameConflict", msg)
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}

	ms.Status.LastScheduleTime = cj.Status.LastScheduleTime
	ms.Status.LastSuccessfulTime = cj.Status.LastSuccessfulTime
	ms.Status.ActiveJobs = int32(len(cj.Status.Active))

	cond := metav1.Condition{Type: vyogotechv1.MicroserviceConditionScheduled, Status: metav1.ConditionTrue, ObservedGeneration: ms.Generation}
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		cond.Reason, cond.Message = "Suspended", fmt.Sprintf("CronJob %s is suspended", cj.Name)
	} else {
		cond.Reason, cond.Message = "Scheduled", fmt.Sprintf("CronJob %s runs on %q", cj.Name, cj.Spec.Schedule)
	}
	meta.SetStatusCondition(&ms.Status.Conditions, cond)
	return ctrl.Result{RequeueAfter: microserviceResync}, nil
}
