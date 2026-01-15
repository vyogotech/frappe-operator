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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

var _ = Describe("SiteBackup Controller", func() {
	var (
		ctx          context.Context
		reconciler   *SiteBackupReconciler
		fakeClient   client.Client
		fakeRecorder *record.FakeRecorder
		siteBackup   *vyogotechv1alpha1.SiteBackup
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

		siteBackup = &vyogotechv1alpha1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-backup",
				Namespace: namespace,
			},
			Spec: vyogotechv1alpha1.SiteBackupSpec{
				Site: "test-site.local",
			},
		}

		scheme := runtime.NewScheme()
		_ = vyogotechv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = batchv1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench, site).Build()

		reconciler = &SiteBackupReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: fakeRecorder,
		}
	})

	Context("One-time backup", func() {
		BeforeEach(func() {
			siteBackup.Spec.Schedule = ""
		})

		It("should create a Job for one-time backup", func() {
			err := fakeClient.Create(ctx, siteBackup)
			Expect(err).ToNot(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name:      siteBackup.Name + "-backup",
				Namespace: namespace,
			}, job)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Spec.Template.Spec.Containers[0].Command).To(ContainElement("bench"))
			Expect(job.Spec.Template.Spec.Containers[0].Args[0]).To(ContainSubstring("backup"))
		})

		It("should update status when Job completes successfully", func() {
			// Create the SiteBackup
			err := fakeClient.Create(ctx, siteBackup)
			Expect(err).ToNot(HaveOccurred())

			// First reconcile creates the Job
			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			// Simulate Job completion
			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name:      siteBackup.Name + "-backup",
				Namespace: namespace,
			}, job)
			Expect(err).ToNot(HaveOccurred())

			job.Status.Conditions = []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			}
			err = fakeClient.Status().Update(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			// Second reconcile updates status
			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			updatedBackup := &vyogotechv1alpha1.SiteBackup{}
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(siteBackup), updatedBackup)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedBackup.Status.Phase).To(Equal("Succeeded"))
			Expect(updatedBackup.Status.LastBackupJob).To(Equal(job.Name))
		})
	})

	Context("Scheduled backup", func() {
		BeforeEach(func() {
			siteBackup.Spec.Schedule = "0 2 * * *"
		})

		It("should create a CronJob for scheduled backup", func() {
			err := fakeClient.Create(ctx, siteBackup)
			Expect(err).ToNot(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			cronJob := &batchv1.CronJob{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name:      siteBackup.Name + "-backup",
				Namespace: namespace,
			}, cronJob)
			Expect(err).ToNot(HaveOccurred())
			Expect(cronJob.Spec.Schedule).To(Equal("0 2 * * *"))
			Expect(cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command).To(ContainElement("bench"))
		})
	})

	Context("Backup options", func() {
		It("should include --with-files when WithFiles is true", func() {
			siteBackup.Spec.WithFiles = true
			err := fakeClient.Create(ctx, siteBackup)
			Expect(err).ToNot(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name:      siteBackup.Name + "-backup",
				Namespace: namespace,
			}, job)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Spec.Template.Spec.Containers[0].Args[0]).To(ContainSubstring("--with-files"))
		})

		It("should include --compress when Compress is true", func() {
			siteBackup.Spec.Compress = true
			err := fakeClient.Create(ctx, siteBackup)
			Expect(err).ToNot(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name:      siteBackup.Name + "-backup",
				Namespace: namespace,
			}, job)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Spec.Template.Spec.Containers[0].Args[0]).To(ContainSubstring("--compress"))
		})

		It("should include --exclude when Exclude is set", func() {
			siteBackup.Spec.Exclude = []string{"User", "Role"}
			err := fakeClient.Create(ctx, siteBackup)
			Expect(err).ToNot(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name:      siteBackup.Name + "-backup",
				Namespace: namespace,
			}, job)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Spec.Template.Spec.Containers[0].Args[0]).To(ContainSubstring("--exclude User,Role"))
		})

		It("should include --include when Include is set", func() {
			siteBackup.Spec.Include = []string{"DocType", "Module Def"}
			err := fakeClient.Create(ctx, siteBackup)
			Expect(err).ToNot(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, client.ObjectKeyFromObject(siteBackup))
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{}
			err = fakeClient.Get(ctx, client.ObjectKey{
				Name:      siteBackup.Name + "-backup",
				Namespace: namespace,
			}, job)
			Expect(err).ToNot(HaveOccurred())
			Expect(job.Spec.Template.Spec.Containers[0].Args[0]).To(ContainSubstring("--include DocType,Module Def"))
		})
	})
})