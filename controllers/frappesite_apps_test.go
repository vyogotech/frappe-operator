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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/controllers/database"
)

var _ = Describe("FrappeSite App Installation", func() {
	var (
		ctx          context.Context
		reconciler   *FrappeSiteReconciler
		fakeRecorder *record.FakeRecorder
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-namespace"
		fakeRecorder = record.NewFakeRecorder(100)
	})

	Describe("App Name Validation", func() {
		It("should accept valid app names", func() {
			validNames := []string{
				"erpnext",
				"hrms",
				"custom_app",
				"app-with-hyphens",
				"app123",
				"APP_NAME",
			}

			for _, name := range validNames {
				isValid := true
				for _, char := range name {
					if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
						(char >= '0' && char <= '9') || char == '_' || char == '-') {
						isValid = false
						break
					}
				}
				Expect(isValid).To(BeTrue(), "App name '%s' should be valid", name)
			}
		})

		It("should reject invalid app names", func() {
			invalidNames := []string{
				"app@domain",
				"app$special",
				"app.with.dots",
				"app with spaces",
				"app/path",
				"app;cmd",
			}

			for _, name := range invalidNames {
				isValid := true
				for _, char := range name {
					if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
						(char >= '0' && char <= '9') || char == '_' || char == '-') {
						isValid = false
						break
					}
				}
				Expect(isValid).To(BeFalse(), "App name '%s' should be invalid", name)
			}
		})
	})

	Describe("ensureInitSecrets with Apps", func() {
		var (
			site   *vyogotechv1alpha1.FrappeSite
			bench  *vyogotechv1alpha1.FrappeBench
			dbInfo *database.DatabaseInfo
			dbCreds *database.DatabaseCredentials
		)

		BeforeEach(func() {
			bench = &vyogotechv1alpha1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bench",
					Namespace: namespace,
				},
				Spec: vyogotechv1alpha1.FrappeBenchSpec{
					FrappeVersion: "15",
					Apps: []vyogotechv1alpha1.AppSource{
						{Name: "erpnext", Source: "fpm"},
						{Name: "hrms", Source: "fpm"},
					},
				},
				Status: vyogotechv1alpha1.FrappeBenchStatus{
					Phase: "Ready",
					InstalledApps: []string{"frappe", "erpnext", "hrms"},
				},
			}

			dbInfo = &database.DatabaseInfo{
				Provider: "mariadb",
				Host:     "mariadb",
				Port:     "3306",
				Name:     "testdb",
			}

			dbCreds = &database.DatabaseCredentials{
				Username: "testuser",
				Password: "testpass",
			}
		})

		It("should create secret with apps_to_install when apps are specified", func() {
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
					Apps: []string{"erpnext", "hrms"},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			err := reconciler.ensureInitSecrets(ctx, site, bench, "test-site.local", dbInfo, dbCreds, "adminpass")
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-init-secrets",
				Namespace: site.Namespace,
			}, secret)
			Expect(err).NotTo(HaveOccurred())

			// Verify apps_to_install field
			appsToInstall := string(secret.Data["apps_to_install"])
			Expect(appsToInstall).To(ContainSubstring("erpnext"))
			Expect(appsToInstall).To(ContainSubstring("hrms"))
		})

		It("should create secret with empty apps_to_install when no apps specified", func() {
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
					Apps: []string{},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			err := reconciler.ensureInitSecrets(ctx, site, bench, "test-site.local", dbInfo, dbCreds, "adminpass")
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-init-secrets",
				Namespace: site.Namespace,
			}, secret)
			Expect(err).NotTo(HaveOccurred())

			// Verify apps_to_install is empty
			appsToInstall := string(secret.Data["apps_to_install"])
			Expect(appsToInstall).To(BeEmpty())
		})

		It("should skip apps with invalid characters", func() {
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
					Apps: []string{"erpnext", "invalid@app", "hrms"},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			err := reconciler.ensureInitSecrets(ctx, site, bench, "test-site.local", dbInfo, dbCreds, "adminpass")
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created with only valid apps
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-init-secrets",
				Namespace: site.Namespace,
			}, secret)
			Expect(err).NotTo(HaveOccurred())

			appsToInstall := string(secret.Data["apps_to_install"])
			Expect(appsToInstall).To(ContainSubstring("erpnext"))
			Expect(appsToInstall).To(ContainSubstring("hrms"))
			Expect(appsToInstall).NotTo(ContainSubstring("invalid@app"))

			// Verify warning event was emitted
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("InvalidAppName")))
		})

		It("should emit event when apps are requested", func() {
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
					Apps: []string{"erpnext"},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			err := reconciler.ensureInitSecrets(ctx, site, bench, "test-site.local", dbInfo, dbCreds, "adminpass")
			Expect(err).NotTo(HaveOccurred())

			// Verify event was emitted
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("AppsRequested")))
		})
	})

	Describe("Job Script Generation with Apps", func() {
		var (
			site   *vyogotechv1alpha1.FrappeSite
			bench  *vyogotechv1alpha1.FrappeBench
			dbInfo *database.DatabaseInfo
		)

		BeforeEach(func() {
			bench = &vyogotechv1alpha1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bench",
					Namespace: namespace,
				},
				Spec: vyogotechv1alpha1.FrappeBenchSpec{
					FrappeVersion: "15",
					Apps: []vyogotechv1alpha1.AppSource{
						{Name: "erpnext", Source: "fpm"},
					},
				},
				Status: vyogotechv1alpha1.FrappeBenchStatus{
					Phase: "Ready",
				},
			}

			dbInfo = &database.DatabaseInfo{
				Provider: "mariadb",
				Host:     "mariadb",
				Port:     "3306",
				Name:     "testdb",
			}
		})

		It("should include app installation script when apps specified", func() {
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
					Apps: []string{"erpnext", "hrms"},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			dbCreds := &database.DatabaseCredentials{Username: "user", Password: "pass"}
			_, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())

			// Verify job was created
			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-init",
				Namespace: site.Namespace,
			}, job)
			Expect(err).NotTo(HaveOccurred())

			// Verify script contains app installation logic
			scriptContent := job.Spec.Template.Spec.Containers[0].Args[0]
			Expect(scriptContent).To(ContainSubstring("APPS_TO_INSTALL"))
			Expect(scriptContent).To(ContainSubstring("--install-app"))
			Expect(scriptContent).To(ContainSubstring("App Installation Configuration"))
		})

		It("should include graceful skipping logic in script", func() {
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
					Apps: []string{"erpnext"},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			dbCreds := &database.DatabaseCredentials{Username: "user", Password: "pass"}
			_, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())

			// Verify job was created
			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-init",
				Namespace: site.Namespace,
			}, job)
			Expect(err).NotTo(HaveOccurred())

			// Verify script contains graceful skipping logic
			scriptContent := job.Spec.Template.Spec.Containers[0].Args[0]
			Expect(scriptContent).To(ContainSubstring("WARNING: App"))
			Expect(scriptContent).To(ContainSubstring("skipping"))
			Expect(scriptContent).To(ContainSubstring("SKIPPED_APPS"))
		})

		It("should check apps directory in script", func() {
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
					Apps: []string{"erpnext"},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			dbCreds := &database.DatabaseCredentials{Username: "user", Password: "pass"}
			_, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())

			// Verify job was created
			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-init",
				Namespace: site.Namespace,
			}, job)
			Expect(err).NotTo(HaveOccurred())

			// Verify script checks apps directory
			scriptContent := job.Spec.Template.Spec.Containers[0].Args[0]
			Expect(scriptContent).To(ContainSubstring("apps/$app"))
			Expect(scriptContent).To(ContainSubstring("Available apps in bench"))
		})
	})

	Describe("Status Updates for App Installation", func() {
		var (
			site   *vyogotechv1alpha1.FrappeSite
			bench  *vyogotechv1alpha1.FrappeBench
			job    *batchv1.Job
			dbInfo *database.DatabaseInfo
		)

		BeforeEach(func() {
			bench = &vyogotechv1alpha1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bench",
					Namespace: namespace,
				},
				Spec: vyogotechv1alpha1.FrappeBenchSpec{
					FrappeVersion: "15",
				},
				Status: vyogotechv1alpha1.FrappeBenchStatus{
					Phase: "Ready",
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
					Apps: []string{"erpnext", "hrms"},
				},
			}

			dbInfo = &database.DatabaseInfo{
				Provider: "mariadb",
				Host:     "mariadb",
				Port:     "3306",
				Name:     "testdb",
			}
		})

		It("should update status with requested apps on success", func() {
			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-init",
					Namespace: site.Namespace,
				},
				Status: batchv1.JobStatus{
					Succeeded: 1,
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bench, site, job).
				WithStatusSubresource(&vyogotechv1alpha1.FrappeSite{}).
				Build()

			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			dbCreds := &database.DatabaseCredentials{Username: "user", Password: "pass"}
			ready, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())

			// Verify status was updated
			Expect(site.Status.InstalledApps).To(Equal([]string{"erpnext", "hrms"}))
			Expect(site.Status.AppInstallationStatus).To(ContainSubstring("Completed app installation"))
		})

		It("should set status message when no apps specified", func() {
			site.Spec.Apps = []string{}

			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-init",
					Namespace: site.Namespace,
				},
				Status: batchv1.JobStatus{
					Succeeded: 1,
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bench, site, job).
				WithStatusSubresource(&vyogotechv1alpha1.FrappeSite{}).
				Build()

			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			dbCreds := &database.DatabaseCredentials{Username: "user", Password: "pass"}
			ready, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())

			// Verify status message
			Expect(site.Status.AppInstallationStatus).To(ContainSubstring("No apps specified"))
		})

		It("should update status when job is in progress", func() {
			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-init",
					Namespace: site.Namespace,
				},
				Status: batchv1.JobStatus{
					Active: 1,
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bench, site, job).
				WithStatusSubresource(&vyogotechv1alpha1.FrappeSite{}).
				Build()

			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			dbCreds := &database.DatabaseCredentials{Username: "user", Password: "pass"}
			ready, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())

			// Verify status reflects in-progress state
			Expect(site.Status.AppInstallationStatus).To(ContainSubstring("Installing"))
		})
	})

	Describe("Event Recording for Apps", func() {
		It("should emit AppsRequested event with app list", func() {
			bench := &vyogotechv1alpha1.FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bench",
					Namespace: namespace,
				},
				Spec: vyogotechv1alpha1.FrappeBenchSpec{
					FrappeVersion: "15",
				},
				Status: vyogotechv1alpha1.FrappeBenchStatus{
					Phase: "Ready",
				},
			}

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
					Apps: []string{"erpnext", "hrms"},
				},
			}

			scheme := runtime.NewScheme()
			_ = vyogotechv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = batchv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()
			reconciler = &FrappeSiteReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			dbInfo := &database.DatabaseInfo{Provider: "mariadb", Host: "host", Port: "3306", Name: "db"}
			dbCreds := &database.DatabaseCredentials{Username: "user", Password: "pass"}

			err := reconciler.ensureInitSecrets(ctx, site, bench, "test-site.local", dbInfo, dbCreds, "adminpass")
			Expect(err).NotTo(HaveOccurred())

			// Verify event contains app information
			var eventMsg string
			Eventually(fakeRecorder.Events).Should(Receive(&eventMsg))
			Expect(eventMsg).To(ContainSubstring("AppsRequested"))
			Expect(strings.Contains(eventMsg, "erpnext") || strings.Contains(eventMsg, "container")).To(BeTrue())
		})
	})
})
