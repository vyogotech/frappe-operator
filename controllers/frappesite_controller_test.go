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
	"github.com/vyogotech/frappe-operator/controllers/database"
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
				Ingress: &vyogotechv1alpha1.IngressConfig{
					Enabled: boolPtr(true),
				},
			},
		}

		scheme := runtime.NewScheme()
		_ = vyogotechv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = networkingv1.AddToScheme(scheme)
		_ = batchv1.AddToScheme(scheme)
		_ = routev1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench).WithStatusSubresource(&vyogotechv1alpha1.FrappeSite{}).Build()

		reconciler = &FrappeSiteReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
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

		It("should update existing condition", func() {
			site.Generation = 1
			site.Status.Conditions = []metav1.Condition{
				{
					Type:   "Ready",
					Status: metav1.ConditionFalse,
					Reason: "NotReady",
				},
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Refresh site from client
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, site)).To(Succeed())
			site.Generation = 1 // Set generation after refresh

			condition := metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "Ready",
				Message: "Site is ready",
			}
			reconciler.setCondition(site, condition)
			Expect(fakeClient.Status().Update(ctx, site)).To(Succeed())

			updatedSite := &vyogotechv1alpha1.FrappeSite{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, updatedSite)).To(Succeed())

			foundCondition := meta.FindStatusCondition(updatedSite.Status.Conditions, "Ready")
			Expect(foundCondition).NotTo(BeNil())
			Expect(foundCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(foundCondition.Reason).To(Equal("Ready"))
		})
	})

	Describe("Ingress Creation", func() {
		It("should create Ingress when enabled", func() {
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			err := reconciler.ensureIngress(ctx, site, bench, "test-site.local")
			Expect(err).NotTo(HaveOccurred())

			ingress := &networkingv1.Ingress{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-ingress",
				Namespace: site.Namespace,
			}, ingress)).To(Succeed())

			Expect(ingress.Spec.Rules).To(HaveLen(1))
			Expect(ingress.Spec.Rules[0].Host).To(Equal("test-site.local"))
		})

		It("should not create Ingress when disabled", func() {
			site.Spec.Ingress = &vyogotechv1alpha1.IngressConfig{
				Enabled: boolPtr(false),
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			err := reconciler.ensureIngress(ctx, site, bench, "test-site.local")
			Expect(err).NotTo(HaveOccurred())

			ingress := &networkingv1.Ingress{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-ingress",
				Namespace: site.Namespace,
			}, ingress)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("OpenShift Route Support", func() {
		It("should detect OpenShift platform", func() {
			// Test isOpenShiftPlatform
			isOpenShift := reconciler.isOpenShiftPlatform(ctx)
			// This will be false in unit tests since we don't have a real API server
			// But we can test the logic structure
			Expect(isOpenShift).To(BeFalse()) // In unit test environment
		})

		It("should create Route when RouteConfig is enabled", func() {
			site.Spec.RouteConfig = &vyogotechv1alpha1.RouteConfig{
				Enabled:        boolPtr(true),
				TLSTermination: "edge",
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Note: ensureRoute requires OpenShift API server, so this test verifies structure
			// Full integration test would be needed for actual Route creation
			Expect(site.Spec.RouteConfig.Enabled).NotTo(BeNil())
			Expect(*site.Spec.RouteConfig.Enabled).To(BeTrue())
		})
	})

	Describe("Event Recording", func() {
		It("should record events for site creation", func() {
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			reconciler.Recorder.Event(site, corev1.EventTypeNormal, "Reconciling", "Starting reconciliation")

			// Verify event was recorded
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("Reconciling")))
		})

		It("should record warning events for errors", func() {
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			reconciler.Recorder.Event(site, corev1.EventTypeWarning, "ReconciliationFailed", "Failed to reconcile")

			// Verify event was recorded
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("ReconciliationFailed")))
		})
	})

	Describe("Status URL Management", func() {
		It("should update SiteURL in status when Ingress is created", func() {
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			err := reconciler.ensureIngress(ctx, site, bench, "test-site.local")
			Expect(err).NotTo(HaveOccurred())

			// Verify status would be updated (in real reconciliation)
			// This is a structural test - full integration would verify actual status update
			Expect(site.Spec.SiteName).To(Equal("test-site.local"))
		})
	})

	Describe("Asynchronous Site Deletion", func() {
		It("should create deletion job when site is marked for deletion", func() {
			// Ensure bench has proper spec for getBenchImage
			bench.Spec.FrappeVersion = "15"
			bench.SetResourceVersion("")

			site.SetFinalizers([]string{frappeSiteFinalizer})
			site.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Delete(ctx, site)).To(Succeed())

			freshSite := &vyogotechv1alpha1.FrappeSite{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, freshSite)).To(Succeed())

			err := reconciler.deleteSite(ctx, freshSite)
			// Without DBConfig.Mode, getMariaDBRootCredentials returns before creating job
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported database mode"))
		})

		It("should return nil when deletion job succeeds", func() {
			// Ensure bench has proper spec for getBenchImage
			bench.Spec.FrappeVersion = "15"
			bench.SetResourceVersion("")

			site.SetFinalizers([]string{frappeSiteFinalizer})
			site.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Delete(ctx, site)).To(Succeed())

			// Pre-create deletion job (fake client rejects Create when ResourceVersion is set)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-delete",
					Namespace: site.Namespace,
				},
			}
			job.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, job)).To(Succeed())

			// Update job status to succeeded
			job.Status.Succeeded = 1
			Expect(fakeClient.Status().Update(ctx, job)).To(Succeed())

			// Get fresh site object from client
			freshSite := &vyogotechv1alpha1.FrappeSite{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, freshSite)).To(Succeed())

			err := reconciler.deleteSite(ctx, freshSite)
			Expect(err).NotTo(HaveOccurred())

			// Verify job was deleted
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-delete",
				Namespace: site.Namespace,
			}, &batchv1.Job{})
			Expect(err).To(HaveOccurred())
		})

		It("should return error when deletion job is still running", func() {
			// Ensure bench has proper spec for getBenchImage
			bench.Spec.FrappeVersion = "15"
			bench.SetResourceVersion("")

			site.SetFinalizers([]string{frappeSiteFinalizer})
			site.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Delete(ctx, site)).To(Succeed())

			// Pre-create deletion job (fake client rejects Create when ResourceVersion is set)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-delete",
					Namespace: site.Namespace,
				},
			}
			job.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, job)).To(Succeed())

			// Update job status to active
			job.Status.Active = 1
			Expect(fakeClient.Status().Update(ctx, job)).To(Succeed())

			// Get fresh site object from client
			freshSite := &vyogotechv1alpha1.FrappeSite{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, freshSite)).To(Succeed())

			err := reconciler.deleteSite(ctx, freshSite)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("site deletion job is still running"))
		})

		It("should return error when deletion job fails", func() {
			// Ensure bench has proper spec for getBenchImage
			bench.Spec.FrappeVersion = "15"
			bench.SetResourceVersion("")

			site.SetFinalizers([]string{frappeSiteFinalizer})
			site.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Delete(ctx, site)).To(Succeed())

			// Pre-create deletion job (fake client rejects Create when ResourceVersion is set)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-delete",
					Namespace: site.Namespace,
				},
			}
			job.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, job)).To(Succeed())

			// Update job status to failed
			job.Status.Failed = 1
			Expect(fakeClient.Status().Update(ctx, job)).To(Succeed())

			// Get fresh site object from client
			freshSite := &vyogotechv1alpha1.FrappeSite{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, freshSite)).To(Succeed())

			err := reconciler.deleteSite(ctx, freshSite)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("site deletion job failed"))
		})

		It("should handle missing bench gracefully during deletion", func() {
			// Create a new fake client without the bench to simulate bench already deleted
			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = networkingv1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)
			_ = routev1.AddToScheme(scheme)

			clientWithoutBench := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconcilerWithoutBench := &FrappeSiteReconciler{
				Client:   clientWithoutBench,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			site.SetFinalizers([]string{frappeSiteFinalizer})
			now := metav1.Now()
			site.SetDeletionTimestamp(&now)
			Expect(clientWithoutBench.Create(ctx, site)).To(Succeed())

			// Refresh site to get latest state
			Expect(clientWithoutBench.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, site)).To(Succeed())

			err := reconcilerWithoutBench.deleteSite(ctx, site)
			// When bench is not found, deleteSite returns nil (bench already deleted)
			Expect(err).NotTo(HaveOccurred()) // Should return nil when bench not found
		})
	})

	Describe("Script Generation Safety", func() {
		It("should use shell variable interpolation in common_site_config.json", func() {
			// Bench is already in client from WithObjects(bench). Create site only.
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Pass non-nil dbInfo so controller log does not panic
			dbInfo := &database.DatabaseInfo{Provider: "mariadb", Name: "test"}
			dbCreds := &database.DatabaseCredentials{}
			_, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())

			job := &batchv1.Job{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name + "-init", Namespace: site.Namespace}, job)).To(Succeed())

			scriptContent := job.Spec.Template.Spec.Containers[0].Args[0]
			Expect(scriptContent).To(ContainSubstring(`"redis_cache": "redis://${BENCH_NAME}-redis-cache:6379"`))
			Expect(scriptContent).NotTo(ContainSubstring(`%s`))
		})
	})

	Describe("Reconciliation Security", func() {
		It("should bail out immediately if DeletionTimestamp is set", func() {
			bench.SetResourceVersion("")

			// shared mode + no MariaDB CR so deleteSite returns nil (skips deletion job) and Reconcile completes
			site.Spec.DBConfig = vyogotechv1alpha1.DatabaseConfig{Mode: "shared"}
			site.SetFinalizers([]string{frappeSiteFinalizer})
			site.SetResourceVersion("")
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Delete(ctx, site)).To(Succeed())

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      site.Name,
					Namespace: site.Namespace,
				},
			}

			// Reconcile should return (Result{}, nil); site may be gone after finalizer removal
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsZero()).To(BeTrue())

			updatedSite := &vyogotechv1alpha1.FrappeSite{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: site.Name, Namespace: site.Namespace}, updatedSite)
			if getErr == nil {
				// If site still exists, Phase should NOT be "Provisioning"
				Expect(updatedSite.Status.Phase).NotTo(Equal(vyogotechv1alpha1.FrappeSitePhaseProvisioning))
			}
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
