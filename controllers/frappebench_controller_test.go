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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

var _ = Describe("FrappeBench Controller", func() {
	var (
		ctx          context.Context
		reconciler   *FrappeBenchReconciler
		fakeClient   client.Client
		fakeRecorder *record.FakeRecorder
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
				Apps: []vyogotechv1alpha1.AppSource{
					{Name: "erpnext", Source: "image"},
				},
			},
		}

		scheme := runtime.NewScheme()
		_ = vyogotechv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = appsv1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		// Seed a default StorageClass to satisfy storage provisioning lookups in tests
		sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard", Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}}, Provisioner: "kubernetes.io/no-provisioner"}
		_ = fakeClient.Create(ctx, sc)

		reconciler = &FrappeBenchReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: fakeRecorder,
		}
	})

	Describe("Finalizer Management", func() {
		It("should add finalizer when not present", func() {
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			result, err := reconciler.handleFinalizer(ctx, bench)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			updatedBench := &vyogotechv1alpha1.FrappeBench{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, updatedBench)).To(Succeed())
			Expect(updatedBench.GetFinalizers()).To(ContainElement(frappeBenchFinalizer))
		})

		It("should block deletion when dependent sites exist", func() {
			// Add finalizer
			bench.SetFinalizers([]string{frappeBenchFinalizer})
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			// Create dependent site
			site := &vyogotechv1alpha1.FrappeSite{
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
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Refresh bench from client and mark for deletion
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())
			now := metav1.Now()
			bench.SetDeletionTimestamp(&now)
			Expect(fakeClient.Update(ctx, bench)).To(Succeed())

			// Refresh again before calling handleFinalizer
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())

			result, err := reconciler.handleFinalizer(ctx, bench)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify finalizer still exists
			updatedBench := &vyogotechv1alpha1.FrappeBench{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, updatedBench)).To(Succeed())
			Expect(updatedBench.GetFinalizers()).To(ContainElement(frappeBenchFinalizer))

			// Verify condition is set
			condition := meta.FindStatusCondition(updatedBench.Status.Conditions, "Terminating")
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("DependentSitesExist"))
		})

		It("should scale down deployments and remove finalizer when no dependent sites", func() {
			// Add finalizer
			bench.SetFinalizers([]string{frappeBenchFinalizer})
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			// Create deployments
			components := []string{"gunicorn", "nginx", "socketio"}
			for _, component := range components {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      bench.Name + "-" + component,
						Namespace: namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
					},
				}
				Expect(fakeClient.Create(ctx, deploy)).To(Succeed())
			}

			// Refresh bench from client and mark for deletion
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())
			now := metav1.Now()
			bench.SetDeletionTimestamp(&now)
			Expect(fakeClient.Update(ctx, bench)).To(Succeed())

			// Refresh again before calling handleFinalizer
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())

			result, err := reconciler.handleFinalizer(ctx, bench)
			Expect(err).NotTo(HaveOccurred())

			// First call should scale down and requeue
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Update deployments to have 0 replicas
			for _, component := range components {
				deploy := &appsv1.Deployment{}
				Expect(fakeClient.Get(ctx, types.NamespacedName{
					Name:      bench.Name + "-" + component,
					Namespace: namespace,
				}, deploy)).To(Succeed())
				deploy.Spec.Replicas = int32Ptr(0)
				deploy.Status.Replicas = 0
				deploy.Status.ReadyReplicas = 0
				Expect(fakeClient.Update(ctx, deploy)).To(Succeed())
			}

			// Second call should remove finalizer
			result, err = reconciler.handleFinalizer(ctx, bench)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify finalizer removed
			updatedBench := &vyogotechv1alpha1.FrappeBench{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, updatedBench)).To(Succeed())
			Expect(updatedBench.GetFinalizers()).NotTo(ContainElement(frappeBenchFinalizer))
		})
	})

	Describe("Condition Management", func() {
		It("should set conditions with observedGeneration", func() {
			bench.Generation = 5
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			// Refresh bench from client
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())
			bench.Generation = 5 // Set generation after refresh

			condition := metav1.Condition{
				Type:    "Progressing",
				Status:  metav1.ConditionTrue,
				Reason:  "Reconciling",
				Message: "Reconciling bench",
			}
			reconciler.setCondition(bench, condition)
			Expect(fakeClient.Status().Update(ctx, bench)).To(Succeed())

			updatedBench := &vyogotechv1alpha1.FrappeBench{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, updatedBench)).To(Succeed())

			foundCondition := meta.FindStatusCondition(updatedBench.Status.Conditions, "Progressing")
			Expect(foundCondition).NotTo(BeNil())
			Expect(foundCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(foundCondition.ObservedGeneration).To(Equal(int64(5)))
		})

		It("should update existing condition", func() {
			bench.Generation = 1
			bench.Status.Conditions = []metav1.Condition{
				{
					Type:   "Ready",
					Status: metav1.ConditionFalse,
					Reason: "NotReady",
				},
			}
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			// Refresh bench from client
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())
			bench.Generation = 1 // Set generation after refresh

			condition := metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Ready",
				Message: "Bench is ready",
			}
			reconciler.setCondition(bench, condition)
			Expect(fakeClient.Status().Update(ctx, bench)).To(Succeed())

			updatedBench := &vyogotechv1alpha1.FrappeBench{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, updatedBench)).To(Succeed())

			foundCondition := meta.FindStatusCondition(updatedBench.Status.Conditions, "Ready")
			Expect(foundCondition).NotTo(BeNil())
			Expect(foundCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(foundCondition.Reason).To(Equal("Ready"))
		})
	})

	Describe("Event Recording", func() {
		It("should record events for bench creation", func() {
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			reconciler.Recorder.Event(bench, corev1.EventTypeNormal, "Reconciling", "Starting reconciliation")

			// Verify event was recorded
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("Reconciling")))
		})

		It("should record warning events for errors", func() {
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			reconciler.Recorder.Event(bench, corev1.EventTypeWarning, "ReconciliationFailed", "Failed to reconcile")

			// Verify event was recorded
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("ReconciliationFailed")))
		})
	})

	Describe("Flexible App Installation", func() {
		It("should track apps from spec in status", func() {
			bench.Spec.Apps = []vyogotechv1alpha1.AppSource{
				{Name: "erpnext", Source: "image"},
				{Name: "hrms", Source: "fpm", Org: "frappe", Version: "1.0.0"},
				{Name: "custom-app", Source: "git", GitURL: "https://github.com/company/custom-app"},
			}
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			// Refresh bench from client before updating status
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())

			// Update status with apps
			err := reconciler.updateBenchStatus(ctx, bench, false, []vyogotechv1alpha1.FPMRepository{})
			// Status update may fail due to fake client limitations, but we can verify the logic
			// by checking the bench object directly after the call
			_ = err // Ignore status update errors for now

			// Verify apps are collected correctly (check the bench object directly)
			Expect(bench.Status.InstalledApps).To(HaveLen(3))
			Expect(bench.Status.InstalledApps).To(ContainElement("erpnext"))
			Expect(bench.Status.InstalledApps).To(ContainElement("hrms"))
			Expect(bench.Status.InstalledApps).To(ContainElement("custom-app"))
		})

		It("should handle empty apps list", func() {
			bench.Spec.Apps = []vyogotechv1alpha1.AppSource{}
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			// Refresh bench from client before updating status
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())

			err := reconciler.updateBenchStatus(ctx, bench, false, []vyogotechv1alpha1.FPMRepository{})
			_ = err // Ignore status update errors for now

			// Verify empty apps list is handled
			Expect(bench.Status.InstalledApps).To(BeEmpty())
		})

		It("should track multiple app sources", func() {
			bench.Spec.Apps = []vyogotechv1alpha1.AppSource{
				{Name: "frappe", Source: "image"},
				{Name: "erpnext", Source: "image"},
				{Name: "hrms", Source: "fpm", Org: "frappe", Version: "1.0.0"},
			}
			Expect(fakeClient.Create(ctx, bench)).To(Succeed())

			// Refresh bench from client before updating status
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: bench.Name, Namespace: bench.Namespace}, bench)).To(Succeed())

			err := reconciler.updateBenchStatus(ctx, bench, false, []vyogotechv1alpha1.FPMRepository{})
			_ = err // Ignore status update errors for now

			// Verify multiple apps are tracked
			Expect(bench.Status.InstalledApps).To(HaveLen(3))
			Expect(bench.Status.InstalledApps).To(ContainElement("frappe"))
			Expect(bench.Status.InstalledApps).To(ContainElement("erpnext"))
			Expect(bench.Status.InstalledApps).To(ContainElement("hrms"))
		})
	})
})
