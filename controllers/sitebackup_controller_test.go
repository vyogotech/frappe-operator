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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

var _ = Describe("SiteBackup Controller", func() {
	var (
		ctx        context.Context
		reconciler *SiteBackupReconciler
		siteBackup *vyogotechv1alpha1.SiteBackup
		site       *vyogotechv1alpha1.FrappeSite
		bench      *vyogotechv1alpha1.FrappeBench
	)

	BeforeEach(func() {
		ctx = context.Background()

		bench = &vyogotechv1alpha1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bench",
				Namespace: "default",
			},
			Spec: vyogotechv1alpha1.FrappeBenchSpec{
				FrappeVersion: "15",
			},
		}

		site = &vyogotechv1alpha1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-site",
				Namespace: "default",
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
				Namespace: "default",
			},
			Spec: vyogotechv1alpha1.SiteBackupSpec{
				Site: "test-site.local",
			},
		}

		reconciler = &SiteBackupReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		// Create bench and site
		Expect(k8sClient.Create(ctx, bench)).To(Succeed())
		Expect(k8sClient.Create(ctx, site)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up
		k8sClient.Delete(ctx, siteBackup)
		k8sClient.Delete(ctx, site)
		k8sClient.Delete(ctx, bench)
	})

	Context("One-time backup", func() {
		BeforeEach(func() {
			siteBackup.Spec.Schedule = ""
		})

		It("should create a Job for one-time backup", func() {
			Expect(k8sClient.Create(ctx, siteBackup)).To(Succeed())

			req := ctrl.Request{}
			req.Namespace = siteBackup.Namespace
			req.Name = siteBackup.Name
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{}
			jobKey := ctrl.Request{}
			jobKey.Namespace = siteBackup.Namespace
			jobKey.Name = siteBackup.Name + "-backup"
			Eventually(func() error {
				return k8sClient.Get(ctx, jobKey.NamespacedName, job)
			}, "10s", "1s").Should(Succeed())

			Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"bench"}))
			Expect(job.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--site", "test-site.local", "backup"))
		})
	})
})
