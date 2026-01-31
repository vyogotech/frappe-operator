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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/pkg/resources"
)

var _ = Describe("FrappeSite Ingress", func() {
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
				Ingress: &vyogotechv1alpha1.IngressConfig{
					Enabled: resources.BoolPtr(true),
				},
			},
		}

		scheme = runtime.NewScheme()
		_ = vyogotechv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = networkingv1.AddToScheme(scheme)
		_ = routev1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(bench).WithStatusSubresource(&vyogotechv1alpha1.FrappeSite{}).Build()

		reconciler = &FrappeSiteReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: fakeRecorder,
		}
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

			Expect(ingress.Spec.Rules[0].Host).To(Equal("test-site.local"))
		})

		It("should not create Ingress when disabled", func() {
			site.Spec.Ingress.Enabled = resources.BoolPtr(false)
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
		It("should create Route on OpenShift platforms", func() {
			reconciler.IsOpenShift = true
			site.Spec.RouteConfig = &vyogotechv1alpha1.RouteConfig{
				Enabled: resources.BoolPtr(true),
			}
			Expect(fakeClient.Create(ctx, site)).To(Succeed())

			err := reconciler.ensureRoute(ctx, site, bench, "test-site.local")
			Expect(err).NotTo(HaveOccurred())

			route := &routev1.Route{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-route",
				Namespace: site.Namespace,
			}, route)).To(Succeed())

			Expect(route.Spec.Host).To(Equal("test-site.local"))
		})
	})
})
