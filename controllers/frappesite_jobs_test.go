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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/controllers/database"
)

var _ = Describe("FrappeSite Jobs", func() {
	var (
		ctx          context.Context
		reconciler   *FrappeSiteReconciler
		fakeClient   client.Client
		fakeRecorder *record.FakeRecorder
		site         *vyogotechv1.FrappeSite
		bench        *vyogotechv1.FrappeBench
		namespace    string
		scheme       *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-namespace"
		fakeRecorder = record.NewFakeRecorder(10)

		bench = &vyogotechv1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bench",
				Namespace: namespace,
			},
			Spec: vyogotechv1.FrappeBenchSpec{
				FrappeVersion: "15",
			},
			Status: vyogotechv1.FrappeBenchStatus{
				Phase: "Ready",
			},
		}

		site = &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-site",
				Namespace: namespace,
			},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: "test-site.local",
				BenchRef: &vyogotechv1.NamespacedName{
					Name:      bench.Name,
					Namespace: bench.Namespace,
				},
			},
		}

		scheme = runtime.NewScheme()
		_ = vyogotechv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = batchv1.AddToScheme(scheme)
		_ = routev1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench).WithStatusSubresource(&vyogotechv1.FrappeSite{}, &vyogotechv1.SiteRestore{}, &vyogotechv1.SiteBackup{}).Build()

		reconciler = &FrappeSiteReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: fakeRecorder,
		}
	})

	Describe("Asynchronous Site Deletion", func() {
		It("should create deletion job when site is marked for deletion", func() {
			site.SetFinalizers([]string{frappeSiteFinalizer})
			site.Spec.DBConfig = vyogotechv1.DatabaseConfig{Mode: "shared"}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Add MariaDB root secret for shared mode
			rootSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "frappe-mariadb-root", Namespace: namespace},
				Data:       map[string][]byte{"password": []byte("rootpass")},
			}
			Expect(fakeClient.Create(ctx, rootSecret)).To(Succeed())

			// Mock MariaDB CR
			mariadbGVK := schema.GroupVersionKind{Group: "k8s.mariadb.com", Version: "v1alpha1", Kind: "MariaDB"}
			mariadbObj := &unstructured.Unstructured{}
			mariadbObj.SetGroupVersionKind(mariadbGVK)
			mariadbObj.SetName("frappe-mariadb")
			mariadbObj.SetNamespace(namespace)
			_ = unstructured.SetNestedMap(mariadbObj.Object, map[string]interface{}{
				"rootPasswordSecretKeyRef": map[string]interface{}{
					"name": "frappe-mariadb-root",
					"key":  "password",
				},
			}, "spec")
			Expect(fakeClient.Create(ctx, mariadbObj)).To(Succeed())

			err := reconciler.deleteSite(ctx, site)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("site deletion job created"))

			job := &batchv1.Job{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name + "-delete", Namespace: site.Namespace}, job)).To(Succeed())
		})
	})

	Describe("Site Initialization Job", func() {
		It("should create initialization job with correct labels and volumes", func() {
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			dbInfo := &database.DatabaseInfo{Provider: "mariadb", Name: "test"}
			dbCreds := &database.DatabaseCredentials{Username: "test", Password: "test"}

			_, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())

			job := &batchv1.Job{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: site.Name + "-init", Namespace: site.Namespace}, job)).To(Succeed())
			Expect(job.Labels["site"]).To(Equal(site.Name))
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(2)) // sites PVC + secrets secret
		})
	})

	Describe("Init Job Failure Detection", func() {
		var (
			dbInfo  *database.DatabaseInfo
			dbCreds *database.DatabaseCredentials
		)

		BeforeEach(func() {
			dbInfo = &database.DatabaseInfo{Provider: "mariadb", Name: "test"}
			dbCreds = &database.DatabaseCredentials{Username: "test", Password: "test"}
		})

		It("should return (false, nil) for transient failure — job still retrying, no JobFailed condition", func() {
			// Job has pod failures but backoffLimit not yet exhausted:
			// no batchv1.JobFailed condition is present.
			transientJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-init",
					Namespace: namespace,
					Labels:    map[string]string{"site": site.Name},
				},
				Status: batchv1.JobStatus{
					Failed: 2,
					// Deliberately no JobFailed condition — still within backoffLimit.
				},
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Create(ctx, transientJob)).To(Succeed())

			done, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred(), "transient failure should not surface as an error")
			Expect(done).To(BeFalse())
		})

		It("should return (false, error) for permanent failure — JobFailed condition True", func() {
			// Job has exceeded backoffLimit: Job controller sets JobFailed condition.
			failedJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-init",
					Namespace: namespace,
					Labels:    map[string]string{"site": site.Name},
				},
				Status: batchv1.JobStatus{
					Failed: 4,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached the specified backoff limit",
						},
					},
				},
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Create(ctx, failedJob)).To(Succeed())

			done, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).To(HaveOccurred(), "permanent failure must be returned as an error")
			Expect(err.Error()).To(ContainSubstring("permanently failed"))
			Expect(done).To(BeFalse())
		})

		It("should return (true, nil) when job succeeded", func() {
			succeededJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-init",
					Namespace: namespace,
					Labels:    map[string]string{"site": site.Name},
				},
				Status: batchv1.JobStatus{
					Succeeded: 1,
				},
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Create(ctx, succeededJob)).To(Succeed())

			done, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())
			Expect(done).To(BeTrue())
		})

		It("should return (false, nil) and emit a Warning event on permanent failure", func() {
			// Permanent failure event should be recorded via the Recorder.
			failedJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      site.Name + "-init",
					Namespace: namespace,
					Labels:    map[string]string{"site": site.Name},
				},
				Status: batchv1.JobStatus{
					Failed: 3,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
							Reason: "BackoffLimitExceeded",
						},
					},
				},
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())
			Expect(fakeClient.Create(ctx, failedJob)).To(Succeed())

			_, _ = reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("SiteInitializationFailed")))
		})
	})

	Describe("Canary Upgrade Strategy", func() {
		var (
			dbInfo  *database.DatabaseInfo
			dbCreds *database.DatabaseCredentials
		)

		BeforeEach(func() {
			dbInfo = &database.DatabaseInfo{Provider: "mariadb", Name: "test"}
			dbCreds = &database.DatabaseCredentials{Username: "test", Password: "pwd"}

			// Set site status phase to Ready and observed version to version-15 to simulate a running site
			site.Status.Phase = vyogotechv1.FrappeSitePhaseReady
			site.Status.ObservedSiteVersion = "version-15"
			site.Annotations = map[string]string{
				"frappe.io/site-version": "version-15",
			}
			site.Spec.UpgradeStrategy = "Canary"
		})

		It("should trigger a backup before starting the migration on image tag update", func() {
			// Trigger upgrade by changing annotation version tag
			site.Annotations["frappe.io/site-version"] = "version-16"

			// Create the site in fake client
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Run reconcile loop (ensureSiteInitialized)
			done, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())
			Expect(done).To(BeFalse()) // Should not be complete since we are waiting for backup

			// Verify that the pre-upgrade SiteBackup resource was created
			backupName := fmt.Sprintf("%s-pre-upgrade", site.Name)
			backup := &vyogotechv1.SiteBackup{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: backupName, Namespace: site.Namespace}, backup)
			Expect(err).NotTo(HaveOccurred())
			Expect(backup.Spec.Site).To(Equal(site.Name))
		})

		It("should initiate automatic database rollback if the migration job fails permanently", func() {
			// Trigger upgrade by changing annotation version tag
			site.Annotations["frappe.io/site-version"] = "version-16"

			// Create pre-upgrade backup in Succeeded state
			backupName := fmt.Sprintf("%s-pre-upgrade", site.Name)
			backup := &vyogotechv1.SiteBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupName,
					Namespace: site.Namespace,
				},
				Spec: vyogotechv1.SiteBackupSpec{
					Site: site.Name,
				},
				Status: vyogotechv1.SiteBackupStatus{
					Phase: "Succeeded",
				},
			}
			Expect(fakeClient.Create(ctx, backup)).To(Succeed())

			// Create migration initialization job in failed state
			failedJobName := fmt.Sprintf("%s-init", site.Name)
			failedJob := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      failedJobName,
					Namespace: site.Namespace,
					Annotations: map[string]string{
						"frappe.io/site-version": "version-16",
					},
				},
				Status: batchv1.JobStatus{
					Failed: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(fakeClient.Create(ctx, failedJob)).To(Succeed())
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			// Run reconcile - should not error yet, but initiate rollback and return false, nil
			done, err := reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).NotTo(HaveOccurred())
			Expect(done).To(BeFalse())

			// Verify that rollback SiteRestore resource was created
			restoreName := fmt.Sprintf("%s-rollback", site.Name)
			restore := &vyogotechv1.SiteRestore{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: restoreName, Namespace: site.Namespace}, restore)
			Expect(err).NotTo(HaveOccurred())
			Expect(restore.Spec.Site).To(Equal(site.Name))
			Expect(restore.Spec.DatabaseBackupSource.LocalPath).To(Equal(fmt.Sprintf("sites/%s/private/backups/pre-upgrade.sql", site.Name)))
			Expect(restore.Spec.Force).To(BeTrue())

			// Update the restore status to Succeeded to simulate a completed rollback
			restore.Status.Phase = "Succeeded"
			Expect(fakeClient.Status().Update(ctx, restore)).To(Succeed())

			// Run reconcile again - now it should return an error indicating migration failed but rollback succeeded
			done, err = reconciler.ensureSiteInitialized(ctx, site, bench, "test-site.local", dbInfo, dbCreds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("migration failed and database was successfully rolled back"))
			Expect(done).To(BeFalse())
		})
	})
})
