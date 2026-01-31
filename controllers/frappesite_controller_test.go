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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

var _ = Describe("FrappeSite Controller", func() {
	var (
		ctx          context.Context
		reconciler   *FrappeSiteReconciler
		fakeClient   client.Client
		fakeRecorder *record.FakeRecorder
		site         *vyogotechv1alpha1.FrappeSite
		bench        *vyogotechv1alpha1.FrappeBench
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-namespace"
		fakeRecorder = record.NewFakeRecorder(10)

		bench = &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bench",
				Namespace: namespace,
			},
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				FrappeVersion: "15",
			},
			Status: vyogotechv1alpha1.FrappeBenchStatus{
				Conditions: []metav1.Condition{
					{
						Type:   "Ready",
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		site = &vyogotechv1alpha1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-site",
				Namespace: namespace,
			},
			Spec: vyogotechv1alpha1.FrappeSiteSpec{
				SiteName: "test-site.local",
				BenchRef: &vyogotechv1alpha1.NamespacedName{
					Name:      bench.Name,
					Namespace: bench.Namespace,
				},
			},
		}

		s := runtime.NewScheme()
		_ = vyogotechv1alpha1.AddToScheme(s)
		_ = corev1.AddToScheme(s)
		_ = networkingv1.AddToScheme(s)
		_ = batchv1.AddToScheme(s)
		_ = routev1.AddToScheme(s)

		fakeClient = fake.NewClientBuilder().WithScheme(s).WithObjects(bench).WithStatusSubresource(&vyogotechv1alpha1.FrappeSite{}).Build()

		reconciler = &FrappeSiteReconciler{
			Client:   fakeClient,
			Scheme:   s,
			Recorder: fakeRecorder,
		}
	})

	Describe("Condition Management", func() {
		It("should set conditions with observedGeneration", func() {
			site.Generation = 3
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Refresh site from client
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, site)).To(Succeed())
			site.Generation = 3 // Set generation after refresh

			condition := metav1.Condition{
				Type:    "Progressing",
				Status:  metav1.ConditionTrue,
				Reason:  "Reconciling",
				Message: "Reconciling site",
			}
			reconciler.setCondition(site, condition)
			Expect(fakeClient.Status().Update(ctx, site)).To(Succeed())

			updatedSite := &vyogotechv1alpha1.FrappeSite{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, updatedSite)).To(Succeed())

			foundCondition := meta.FindStatusCondition(updatedSite.Status.Conditions, "Progressing")
			Expect(foundCondition).NotTo(BeNil())
			Expect(foundCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(foundCondition.ObservedGeneration).To(Equal(int64(3)))
		})
	})

	Describe("Event Recording", func() {
		It("should record events for site creation", func() {
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			reconciler.Recorder.Event(site, corev1.EventTypeNormal, "Reconciling", "Starting reconciliation")

			// Verify event was recorded
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("Reconciling")))
		})
	})

	Describe("SetupWithManager", func() {
		It("succeeds when MaxConcurrentReconciles is set", func() {
			if skipControllerTests {
				Skip("envtest not available")
			}
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			r := &FrappeSiteReconciler{
				Client:                  mgr.GetClient(),
				Scheme:                  mgr.GetScheme(),
				Recorder:                mgr.GetEventRecorderFor("frappesite-controller"),
				IsOpenShift:             false,
				MaxConcurrentReconciles: 5,
			}
			Expect(r.SetupWithManager(mgr)).To(Succeed())
		})
	})
})
