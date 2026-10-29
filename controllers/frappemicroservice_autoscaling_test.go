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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func withHPA(min, max, cpu int32) func(*vyogotechv1.FrappeMicroservice) {
	return func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.Resources = &vyogotechv1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		}}
		ms.Spec.Deployment.Replicas = int32Ptr(3)
		ms.Spec.Deployment.Autoscaling = &vyogotechv1.MicroserviceAutoscaling{
			MinReplicas: int32Ptr(min), MaxReplicas: max, TargetCPUUtilizationPercentage: int32Ptr(cpu),
		}
	}
}

func withKEDA(min, max int32) func(*vyogotechv1.FrappeMicroservice) {
	return func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.Deployment.Autoscaling = &vyogotechv1.MicroserviceAutoscaling{
			MinReplicas: int32Ptr(min), MaxReplicas: max,
			Keda: &vyogotechv1.MicroserviceKEDA{
				PollingInterval: int32Ptr(15), CooldownPeriod: int32Ptr(120),
				Triggers: []vyogotechv1.MicroserviceKEDATrigger{{
					Type:     "redis",
					Metadata: map[string]string{"address": "redis-queue:6379", "listName": "rq:queue:default", "listLength": "50"},
				}},
			},
		}
	}
}

var scaledObjectGVK = schema.GroupVersionKind{Group: "keda.sh", Version: "v1alpha1", Kind: "ScaledObject"}

func (f *msFixture) scaledObject() (*unstructured.Unstructured, bool) {
	f.t.Helper()
	so := &unstructured.Unstructured{}
	so.SetGroupVersionKind(scaledObjectGVK)
	return so, f.get(msName, so)
}

func (f *msFixture) scaleDeployment(replicas int32) {
	f.t.Helper()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	dep.Spec.Replicas = &replicas
	if err := f.client.Update(f.ctx, dep); err != nil {
		f.t.Fatalf("scale deployment: %v", err)
	}
}

func TestMicroservice_HPA_CreatedAndOwned(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70)})
	f.reconcile()

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	f.mustGet(msName, hpa)
	if !metav1.IsControlledBy(hpa, f.ms()) {
		t.Errorf("HPA is not owned by the microservice")
	}
	ref := hpa.Spec.ScaleTargetRef
	if ref.Kind != "Deployment" || ref.Name != msName || ref.APIVersion != "apps/v1" {
		t.Errorf("scaleTargetRef = %+v", ref)
	}
	if hpa.Spec.MinReplicas == nil || *hpa.Spec.MinReplicas != 2 || hpa.Spec.MaxReplicas != 10 {
		t.Errorf("bounds = %v..%d", hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas)
	}
	if len(hpa.Spec.Metrics) != 1 {
		t.Fatalf("metrics = %+v", hpa.Spec.Metrics)
	}
	m := hpa.Spec.Metrics[0]
	if m.Type != autoscalingv2.ResourceMetricSourceType || m.Resource.Name != corev1.ResourceCPU ||
		m.Resource.Target.Type != autoscalingv2.UtilizationMetricType || *m.Resource.Target.AverageUtilization != 70 {
		t.Errorf("metric = %+v", m)
	}
}

func TestMicroservice_HPA_CPUAndMemoryTargets(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) {
		withHPA(1, 5, 70)(ms)
		ms.Spec.Deployment.Autoscaling.TargetMemoryUtilizationPercentage = int32Ptr(80)
	}})
	f.reconcile()
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	f.mustGet(msName, hpa)
	if len(hpa.Spec.Metrics) != 2 || hpa.Spec.Metrics[1].Resource.Name != corev1.ResourceMemory ||
		*hpa.Spec.Metrics[1].Resource.Target.AverageUtilization != 80 {
		t.Errorf("metrics = %+v", hpa.Spec.Metrics)
	}
}

func TestMicroservice_HPA_MinReplicasDefaultsToOne(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) {
		withHPA(1, 5, 70)(ms)
		ms.Spec.Deployment.Autoscaling.MinReplicas = nil
	}})
	f.reconcile()
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	f.mustGet(msName, hpa)
	if hpa.Spec.MinReplicas == nil || *hpa.Spec.MinReplicas != 1 {
		t.Errorf("minReplicas = %v, want 1", hpa.Spec.MinReplicas)
	}
}

func TestMicroservice_Autoscaling_ReplicasSetOnlyOnCreate(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70)})
	f.reconcile()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if *dep.Spec.Replicas != 2 {
		t.Fatalf("created with %d replicas, want minReplicas 2 (not deployment.replicas 3)", *dep.Spec.Replicas)
	}

	// The autoscaler scales up; a later reconcile must leave it alone.
	f.scaleDeployment(7)
	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Image.Tag = "1.0.1" })
	f.reconcile()
	f.mustGet(msName, dep)
	if *dep.Spec.Replicas != 7 {
		t.Errorf("operator reset replicas to %d, the autoscaler owns the count", *dep.Spec.Replicas)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != "ghcr.io/acme/orders:1.0.1" {
		t.Errorf("other spec changes must still apply, image = %s", dep.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestMicroservice_Autoscaling_StatusAndAvailableFollowAutoscaler(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70)})
	f.reconcile()
	f.scaleDeployment(5)
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	dep.Status.Replicas, dep.Status.ReadyReplicas = 5, 5
	if err := f.client.Status().Update(f.ctx, dep); err != nil {
		t.Fatal(err)
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	f.mustGet(msName, hpa)
	hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas = 5, 6
	hpa.Status.Conditions = []autoscalingv2.HorizontalPodAutoscalerCondition{{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionTrue, Reason: "ValidMetricFound"}}
	if err := f.client.Status().Update(f.ctx, hpa); err != nil {
		t.Fatal(err)
	}

	f.reconcile()
	ms := f.ms()
	if a := ms.Status.Autoscaling; a == nil || a.CurrentReplicas != 5 || a.DesiredReplicas != 6 {
		t.Errorf("status.autoscaling = %+v, want current 5 desired 6", a)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionAutoscalingReady, metav1.ConditionTrue, "")
	// 5 of the 5 the autoscaler asked for are ready, not 5 of spec.deployment.replicas (3).
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionAvailable, metav1.ConditionTrue, "ReplicasReady")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseReady {
		t.Errorf("phase = %s", ms.Status.Phase)
	}
}

func TestMicroservice_HPA_InactiveMetricsReportNotReady(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70)})
	f.reconcile()
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	f.mustGet(msName, hpa)
	hpa.Status.Conditions = []autoscalingv2.HorizontalPodAutoscalerCondition{{
		Type: autoscalingv2.ScalingActive, Status: corev1.ConditionFalse, Reason: "FailedGetResourceMetric", Message: "metrics API not available",
	}}
	if err := f.client.Status().Update(f.ctx, hpa); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionAutoscalingReady, metav1.ConditionFalse, "FailedGetResourceMetric")
}

func TestMicroservice_HPA_DriftIsCorrected(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70)})
	f.reconcile()
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	f.mustGet(msName, hpa)
	hpa.Spec.MaxReplicas = 99
	if err := f.client.Update(f.ctx, hpa); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	f.mustGet(msName, hpa)
	if hpa.Spec.MaxReplicas != 10 {
		t.Errorf("maxReplicas = %d, want 10", hpa.Spec.MaxReplicas)
	}
}

func TestMicroservice_Autoscaling_RemovalDeletesAutoscalerAndRestoresReplicas(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70)})
	f.reconcile()
	f.scaleDeployment(7)

	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Deployment.Autoscaling = nil })
	f.reconcile()

	if f.get(msName, &autoscalingv2.HorizontalPodAutoscaler{}) {
		t.Errorf("HPA must be deleted when autoscaling is removed")
	}
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if *dep.Spec.Replicas != 3 {
		t.Errorf("replicas = %d, want deployment.replicas 3", *dep.Spec.Replicas)
	}
	ms := f.ms()
	if ms.Status.Autoscaling != nil || condition(ms, vyogotechv1.MicroserviceConditionAutoscalingReady) != nil {
		t.Errorf("autoscaling status must be cleared: %+v %+v", ms.Status.Autoscaling, ms.Status.Conditions)
	}
}

func TestMicroservice_NoAutoscaling_DoesNotTouchForeignHPA(t *testing.T) {
	foreign := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: msName, Namespace: msNS},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: msName},
			MaxReplicas:    4,
		},
	}
	f := newMSFixture(t, msFixtureOpts{extra: []client.Object{foreign}})
	f.reconcile()
	if !f.get(msName, &autoscalingv2.HorizontalPodAutoscaler{}) {
		t.Errorf("a user's own HPA must not be deleted")
	}
}

func TestMicroservice_HPA_NameConflictIsNotTakenOver(t *testing.T) {
	foreign := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: msName, Namespace: msNS},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: msName},
			MaxReplicas:    4,
		},
	}
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70), extra: []client.Object{foreign}})
	f.reconcile()
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	f.mustGet(msName, hpa)
	if hpa.Spec.MaxReplicas != 4 {
		t.Errorf("foreign HPA was modified")
	}
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionAutoscalingReady, metav1.ConditionFalse, "NameConflict")
}

// --- KEDA ------------------------------------------------------------------

func TestMicroservice_KEDA_ScaledObjectCreatedAndOwned(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withKEDA(1, 20)})
	f.reconcile()

	if f.get(msName, &autoscalingv2.HorizontalPodAutoscaler{}) {
		t.Errorf("no operator HPA expected next to a ScaledObject")
	}
	so, ok := f.scaledObject()
	if !ok {
		t.Fatalf("ScaledObject not created")
	}
	if !metav1.IsControlledBy(so, f.ms()) {
		t.Errorf("ScaledObject is not owned by the microservice")
	}
	spec, _, _ := unstructured.NestedMap(so.Object, "spec")
	if got, _, _ := unstructured.NestedString(spec, "scaleTargetRef", "name"); got != msName {
		t.Errorf("scaleTargetRef.name = %q", got)
	}
	if got, _, _ := unstructured.NestedString(spec, "scaleTargetRef", "kind"); got != "Deployment" {
		t.Errorf("scaleTargetRef.kind = %q", got)
	}
	for field, want := range map[string]int64{"minReplicaCount": 1, "maxReplicaCount": 20, "pollingInterval": 15, "cooldownPeriod": 120} {
		if got, _, _ := unstructured.NestedInt64(spec, field); got != want {
			t.Errorf("%s = %d, want %d", field, got, want)
		}
	}
	triggers, _, _ := unstructured.NestedSlice(spec, "triggers")
	if len(triggers) != 1 {
		t.Fatalf("triggers = %v", triggers)
	}
	trig := triggers[0].(map[string]interface{})
	md := trig["metadata"].(map[string]interface{})
	if trig["type"] != "redis" || md["listName"] != "rq:queue:default" || md["listLength"] != "50" || md["address"] != "redis-queue:6379" {
		t.Errorf("trigger = %v", trig)
	}
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionAutoscalingReady, metav1.ConditionTrue, "")
}

func TestMicroservice_KEDA_CanScaleToZero(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withKEDA(0, 20)})
	f.reconcile()
	so, _ := f.scaledObject()
	if got, found, _ := unstructured.NestedInt64(so.Object, "spec", "minReplicaCount"); !found || got != 0 {
		t.Errorf("minReplicaCount = %d (found %v), want 0", got, found)
	}
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if *dep.Spec.Replicas != 0 {
		t.Errorf("created with %d replicas, want 0", *dep.Spec.Replicas)
	}
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionAvailable, metav1.ConditionTrue, "ScaledToZero")
}

func TestMicroservice_KEDA_AuthenticationRefAndTriggerName(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) {
		withKEDA(1, 5)(ms)
		tr := &ms.Spec.Deployment.Autoscaling.Keda.Triggers[0]
		tr.Name, tr.AuthenticationRef = "queue", "redis-auth"
	}})
	f.reconcile()
	so, _ := f.scaledObject()
	triggers, _, _ := unstructured.NestedSlice(so.Object, "spec", "triggers")
	trig := triggers[0].(map[string]interface{})
	ref, _ := trig["authenticationRef"].(map[string]interface{})
	if trig["name"] != "queue" || ref["name"] != "redis-auth" {
		t.Errorf("trigger = %v", trig)
	}
}

func TestMicroservice_KEDA_NotInstalledReportsConditionWithoutFailing(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withKEDA(1, 20)})
	f.r.Client = interceptor.NewClient(f.client.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if u, ok := obj.(*unstructured.Unstructured); ok && u.GroupVersionKind().Group == "keda.sh" {
				return &meta.NoKindMatchError{GroupKind: scaledObjectGVK.GroupKind(), SearchedVersions: []string{"v1alpha1"}}
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if u, ok := obj.(*unstructured.Unstructured); ok && u.GroupVersionKind().Group == "keda.sh" {
				return &meta.NoKindMatchError{GroupKind: scaledObjectGVK.GroupKind(), SearchedVersions: []string{"v1alpha1"}}
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	f.reconcile() // must not return an error

	ms := f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionAutoscalingReady, metav1.ConditionFalse, "KEDANotInstalled")
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if *dep.Spec.Replicas != 1 {
		t.Errorf("deployment still runs at minReplicas, got %d", *dep.Spec.Replicas)
	}
}

func TestMicroservice_KEDA_SwitchFromHPADeletesHPA(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withHPA(2, 10, 70)})
	f.reconcile()
	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { withKEDA(1, 20)(ms) })
	f.reconcile()
	if f.get(msName, &autoscalingv2.HorizontalPodAutoscaler{}) {
		t.Errorf("HPA must be removed when switching to KEDA")
	}
	if _, ok := f.scaledObject(); !ok {
		t.Errorf("ScaledObject missing after switch")
	}
}

func TestMicroservice_KEDA_SwitchToHPADeletesScaledObject(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withKEDA(1, 20)})
	f.reconcile()
	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { withHPA(2, 10, 70)(ms) })
	f.reconcile()
	if _, ok := f.scaledObject(); ok {
		t.Errorf("ScaledObject must be removed when switching to HPA")
	}
	f.mustGet(msName, &autoscalingv2.HorizontalPodAutoscaler{})
}

func TestMicroservice_KEDA_RemovalDeletesScaledObject(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withKEDA(1, 20)})
	f.reconcile()
	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Deployment.Autoscaling = nil })
	f.reconcile()
	if _, ok := f.scaledObject(); ok {
		t.Errorf("ScaledObject must be deleted when autoscaling is removed")
	}
}

func TestMicroservice_NoAutoscaling_WithoutKEDAInstalledStillReconciles(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.r.Client = interceptor.NewClient(f.client.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if u, ok := obj.(*unstructured.Unstructured); ok && u.GroupVersionKind().Group == "keda.sh" {
				return &meta.NoKindMatchError{GroupKind: scaledObjectGVK.GroupKind(), SearchedVersions: []string{"v1alpha1"}}
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	f.reconcile()
	f.mustGet(msName, &appsv1.Deployment{})
	if condition(f.ms(), vyogotechv1.MicroserviceConditionAutoscalingReady) != nil {
		t.Errorf("no autoscaling condition expected without spec.deployment.autoscaling")
	}
}
