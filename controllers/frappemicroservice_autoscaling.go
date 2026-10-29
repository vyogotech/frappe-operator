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

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// kedaScaledObjectGVK is handled as unstructured so the operator needs neither
// the KEDA Go module nor the KEDA CRDs to be installed.
var kedaScaledObjectGVK = schema.GroupVersionKind{Group: "keda.sh", Version: "v1alpha1", Kind: "ScaledObject"}

// microserviceAutoscaling returns the autoscaling settings, or nil when the
// Deployment keeps a fixed replica count.
func microserviceAutoscaling(ms *vyogotechv1.FrappeMicroservice) *vyogotechv1.MicroserviceAutoscaling {
	if microserviceType(ms) != vyogotechv1.MicroserviceTypeDeployment || ms.Spec.Deployment == nil {
		return nil
	}
	return ms.Spec.Deployment.Autoscaling
}

func autoscalingMinReplicas(as *vyogotechv1.MicroserviceAutoscaling) int32 {
	if as.MinReplicas == nil {
		return 1
	}
	return *as.MinReplicas
}

// reconcileAutoscaling keeps the autoscaler the spec asks for, and no other:
// a HorizontalPodAutoscaler for CPU or memory targets, a KEDA ScaledObject
// for keda. Both are owned by the microservice.
func (r *FrappeMicroserviceReconciler) reconcileAutoscaling(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, dep *appsv1.Deployment) error {
	as := microserviceAutoscaling(ms)
	switch {
	case as == nil:
		if err := r.deleteOwned(ctx, ms, newHPA(ms)); err != nil {
			return err
		}
		// Only look for a ScaledObject when one may exist: on clusters without
		// KEDA the lookup is a failed API discovery.
		if condition := meta.FindStatusCondition(ms.Status.Conditions, vyogotechv1.MicroserviceConditionAutoscalingReady); condition != nil {
			if err := r.deleteOwned(ctx, ms, newScaledObject(ms)); err != nil {
				return err
			}
		}
		ms.Status.Autoscaling = nil
		meta.RemoveStatusCondition(&ms.Status.Conditions, vyogotechv1.MicroserviceConditionAutoscalingReady)
		return nil
	case as.Keda != nil:
		if err := r.deleteOwned(ctx, ms, newHPA(ms)); err != nil {
			return err
		}
		return r.reconcileScaledObject(ctx, ms, as, dep)
	default:
		if err := r.deleteOwned(ctx, ms, newScaledObject(ms)); err != nil {
			return err
		}
		return r.reconcileHPA(ctx, ms, as, dep)
	}
}

func newHPA(ms *vyogotechv1.FrappeMicroservice) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: ms.Name, Namespace: ms.Namespace}}
}

func newScaledObject(ms *vyogotechv1.FrappeMicroservice) *unstructured.Unstructured {
	so := &unstructured.Unstructured{}
	so.SetGroupVersionKind(kedaScaledObjectGVK)
	so.SetName(ms.Name)
	so.SetNamespace(ms.Namespace)
	return so
}

// deleteOwned deletes obj if it exists and this microservice controls it. An
// object of the same name that someone else created is left alone, and so is
// a ScaledObject when KEDA is not installed.
func (r *FrappeMicroserviceReconciler) deleteOwned(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, obj client.Object) error {
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(obj, ms) {
		return nil
	}
	return client.IgnoreNotFound(r.Delete(ctx, obj))
}

func (r *FrappeMicroserviceReconciler) setAutoscalingCondition(ms *vyogotechv1.FrappeMicroservice, status metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&ms.Status.Conditions, metav1.Condition{
		Type: vyogotechv1.MicroserviceConditionAutoscalingReady, Status: status, Reason: reason, Message: msg, ObservedGeneration: ms.Generation,
	})
}

func (r *FrappeMicroserviceReconciler) autoscalerNameConflict(ms *vyogotechv1.FrappeMicroservice, kind, name string) {
	msg := fmt.Sprintf("%s %s exists and is not owned by this FrappeMicroservice; it was left untouched", kind, name)
	r.setAutoscalingCondition(ms, metav1.ConditionFalse, "NameConflict", msg)
	r.Recorder.Event(ms, corev1.EventTypeWarning, "NameConflict", msg)
}

func (r *FrappeMicroserviceReconciler) reconcileHPA(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, as *vyogotechv1.MicroserviceAutoscaling, dep *appsv1.Deployment) error {
	hpa := newHPA(ms)
	conflict, err := r.createOrUpdateOwned(ctx, ms, hpa, func() error {
		hpa.Labels = mergeLabels(hpa.Labels, microserviceLabels(ms))
		hpa.Spec.ScaleTargetRef = autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: ms.Name}
		min := autoscalingMinReplicas(as)
		hpa.Spec.MinReplicas = &min
		hpa.Spec.MaxReplicas = as.MaxReplicas
		hpa.Spec.Metrics = nil
		if as.TargetCPUUtilizationPercentage != nil {
			hpa.Spec.Metrics = append(hpa.Spec.Metrics, utilizationMetric(corev1.ResourceCPU, *as.TargetCPUUtilizationPercentage))
		}
		if as.TargetMemoryUtilizationPercentage != nil {
			hpa.Spec.Metrics = append(hpa.Spec.Metrics, utilizationMetric(corev1.ResourceMemory, *as.TargetMemoryUtilizationPercentage))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if conflict {
		r.autoscalerNameConflict(ms, "HorizontalPodAutoscaler", hpa.Name)
		return nil
	}

	current, desired := hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas
	if current == 0 && desired == 0 {
		// The HPA has not reported yet.
		current, desired = dep.Status.Replicas, deploymentDesired(dep)
	}
	ms.Status.Autoscaling = &vyogotechv1.MicroserviceAutoscalingStatus{CurrentReplicas: current, DesiredReplicas: desired}

	for _, c := range hpa.Status.Conditions {
		if c.Type == autoscalingv2.ScalingActive && c.Status == corev1.ConditionFalse {
			r.setAutoscalingCondition(ms, metav1.ConditionFalse, c.Reason, c.Message)
			return nil
		}
	}
	r.setAutoscalingCondition(ms, metav1.ConditionTrue, "HPAConfigured", fmt.Sprintf("HorizontalPodAutoscaler %s scales %d-%d replicas", hpa.Name, *hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas))
	return nil
}

func utilizationMetric(name corev1.ResourceName, percent int32) autoscalingv2.MetricSpec {
	return autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   name,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: &percent},
		},
	}
}

func deploymentDesired(dep *appsv1.Deployment) int32 {
	if dep.Spec.Replicas == nil {
		return 1
	}
	return *dep.Spec.Replicas
}

func (r *FrappeMicroserviceReconciler) reconcileScaledObject(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, as *vyogotechv1.MicroserviceAutoscaling, dep *appsv1.Deployment) error {
	so := newScaledObject(ms)
	conflict, err := r.createOrUpdateOwned(ctx, ms, so, func() error {
		so.SetGroupVersionKind(kedaScaledObjectGVK)
		so.SetLabels(mergeLabels(so.GetLabels(), microserviceLabels(ms)))
		// Set the fields one by one so settings KEDA or a user added to the
		// spec, such as advanced.*, survive.
		fields := map[string]interface{}{
			"scaleTargetRef":  map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "name": ms.Name},
			"minReplicaCount": int64(autoscalingMinReplicas(as)),
			"maxReplicaCount": int64(as.MaxReplicas),
			"triggers":        kedaTriggers(as.Keda.Triggers),
		}
		for field, v := range fields {
			if err := unstructured.SetNestedField(so.Object, v, "spec", field); err != nil {
				return err
			}
		}
		for field, v := range map[string]*int32{"pollingInterval": as.Keda.PollingInterval, "cooldownPeriod": as.Keda.CooldownPeriod} {
			if v == nil {
				unstructured.RemoveNestedField(so.Object, "spec", field)
			} else if err := unstructured.SetNestedField(so.Object, int64(*v), "spec", field); err != nil {
				return err
			}
		}
		return nil
	})
	if meta.IsNoMatchError(err) {
		msg := "keda is set but the KEDA CRDs (keda.sh/v1alpha1 ScaledObject) are not installed in the cluster; the Deployment stays at its current replica count"
		r.setAutoscalingCondition(ms, metav1.ConditionFalse, "KEDANotInstalled", msg)
		ms.Status.Autoscaling = &vyogotechv1.MicroserviceAutoscalingStatus{CurrentReplicas: dep.Status.Replicas, DesiredReplicas: deploymentDesired(dep)}
		return nil
	}
	if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsBadRequest(err) {
		// KEDA's admission webhook (or RBAC) refused the ScaledObject. Retrying
		// at once cannot help; tell the user why and look again at the next resync.
		r.setAutoscalingCondition(ms, metav1.ConditionFalse, "ScaledObjectRejected", err.Error())
		r.Recorder.Event(ms, corev1.EventTypeWarning, "ScaledObjectRejected", err.Error())
		ms.Status.Autoscaling = &vyogotechv1.MicroserviceAutoscalingStatus{CurrentReplicas: dep.Status.Replicas, DesiredReplicas: deploymentDesired(dep)}
		return nil
	}
	if err != nil {
		return err
	}
	if conflict {
		r.autoscalerNameConflict(ms, "ScaledObject", so.GetName())
		return nil
	}

	ms.Status.Autoscaling = &vyogotechv1.MicroserviceAutoscalingStatus{CurrentReplicas: dep.Status.Replicas, DesiredReplicas: deploymentDesired(dep)}
	conditions, _, _ := unstructured.NestedSlice(so.Object, "status", "conditions")
	for _, c := range conditions {
		cond, _ := c.(map[string]interface{})
		if cond["type"] == "Ready" && cond["status"] == string(metav1.ConditionFalse) {
			reason, _ := cond["reason"].(string)
			message, _ := cond["message"].(string)
			if reason == "" {
				reason = "ScaledObjectNotReady"
			}
			r.setAutoscalingCondition(ms, metav1.ConditionFalse, reason, message)
			return nil
		}
	}
	r.setAutoscalingCondition(ms, metav1.ConditionTrue, "ScaledObjectConfigured", fmt.Sprintf("ScaledObject %s scales %d-%d replicas", so.GetName(), autoscalingMinReplicas(as), as.MaxReplicas))
	return nil
}

func kedaTriggers(triggers []vyogotechv1.MicroserviceKEDATrigger) []interface{} {
	out := make([]interface{}, 0, len(triggers))
	for _, t := range triggers {
		trigger := map[string]interface{}{"type": t.Type}
		if t.Name != "" {
			trigger["name"] = t.Name
		}
		if t.MetricType != "" {
			trigger["metricType"] = t.MetricType
		}
		md := make(map[string]interface{}, len(t.Metadata))
		for k, v := range t.Metadata {
			md[k] = v
		}
		trigger["metadata"] = md
		if t.AuthenticationRef != "" {
			trigger["authenticationRef"] = map[string]interface{}{"name": t.AuthenticationRef}
		}
		out = append(out, trigger)
	}
	return out
}
