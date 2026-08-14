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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func TestEnsureNamespacePolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = vyogotechv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "b1", Namespace: "tenant-x"},
		Spec: vyogotechv1.FrappeBenchSpec{
			FrappeVersion: "15",
			NamespacePolicy: &vyogotechv1.NamespacePolicy{
				ResourceQuota:   corev1.ResourceList{corev1.ResourceLimitsCPU: resource.MustParse("4")},
				DefaultRequests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
				DefaultLimits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
				MaxLimits:       corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench).Build()
	r := &FrappeBenchReconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
	ctx := context.Background()

	// Policy set -> ResourceQuota + LimitRange are created with the requested values.
	if err := r.ensureNamespacePolicy(ctx, bench); err != nil {
		t.Fatalf("ensureNamespacePolicy: %v", err)
	}

	rq := &corev1.ResourceQuota{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "b1-quota", Namespace: "tenant-x"}, rq); err != nil {
		t.Fatalf("expected ResourceQuota b1-quota: %v", err)
	}
	if got := rq.Spec.Hard[corev1.ResourceLimitsCPU]; got.String() != "4" {
		t.Errorf("quota limits.cpu = %s, want 4", got.String())
	}
	if len(rq.OwnerReferences) == 0 {
		t.Errorf("expected owner reference on ResourceQuota")
	}

	lr := &corev1.LimitRange{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "b1-limits", Namespace: "tenant-x"}, lr); err != nil {
		t.Fatalf("expected LimitRange b1-limits: %v", err)
	}
	if len(lr.Spec.Limits) != 1 || lr.Spec.Limits[0].Type != corev1.LimitTypeContainer {
		t.Fatalf("unexpected LimitRange spec: %+v", lr.Spec)
	}
	if got := lr.Spec.Limits[0].Default[corev1.ResourceMemory]; got.String() != "512Mi" {
		t.Errorf("limitrange default memory = %s, want 512Mi", got.String())
	}
	if got := lr.Spec.Limits[0].Max[corev1.ResourceCPU]; got.String() != "2" {
		t.Errorf("limitrange max cpu = %s, want 2", got.String())
	}

	// Policy cleared -> the operator-managed objects are removed (level-based converge).
	bench.Spec.NamespacePolicy = nil
	if err := r.ensureNamespacePolicy(ctx, bench); err != nil {
		t.Fatalf("ensureNamespacePolicy (clear): %v", err)
	}
	if err := cl.Get(ctx, types.NamespacedName{Name: "b1-quota", Namespace: "tenant-x"}, &corev1.ResourceQuota{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected ResourceQuota deleted, got err=%v", err)
	}
	if err := cl.Get(ctx, types.NamespacedName{Name: "b1-limits", Namespace: "tenant-x"}, &corev1.LimitRange{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected LimitRange deleted, got err=%v", err)
	}
}
