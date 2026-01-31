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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/controllers/database"
)

var _ = Describe("FrappeSite Jobs", func() {
	var (
		ctx          context.Context
		reconciler   *FrappeSiteReconciler
		fakeClient   client.Client
		fakeRecorder *record.FakeRecorder
		site         *vyogotechv1alpha1.FrappeSite
		bench        *vyogotechv1alpha1.FrappeBench
		namespace    string
		scheme       *runtime.Scheme
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
			},
		}

		scheme = runtime.NewScheme()
		_ = vyogotechv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = batchv1.AddToScheme(scheme)
		_ = routev1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench).WithStatusSubresource(&vyogotechv1alpha1.FrappeSite{}).Build()

		reconciler = &FrappeSiteReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: fakeRecorder,
		}
	})

	Describe("Asynchronous Site Deletion", func() {
		It("should create deletion job when site is marked for deletion", func() {
			site.SetFinalizers([]string{frappeSiteFinalizer})
			site.Spec.DBConfig = vyogotechv1alpha1.DatabaseConfig{Mode: "shared"}
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
})
