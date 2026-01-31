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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

var _ = Describe("FrappeSite Lifecycle", func() {
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
				DomainConfig: &vyogotechv1alpha1.DomainConfig{
					Suffix: ".example.com",
				},
			},
		}

		site = &vyogotechv1alpha1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-site",
				Namespace: namespace,
			},
			Spec: vyogotechv1alpha1.FrappeSiteSpec{
				SiteName: "mysite",
			},
		}

		scheme = runtime.NewScheme()
		_ = vyogotechv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = networkingv1.AddToScheme(scheme)
		_ = batchv1.AddToScheme(scheme)
		_ = routev1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		reconciler = &FrappeSiteReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: fakeRecorder,
		}
	})

	Describe("Domain Resolution", func() {
		It("should use explicit domain if provided", func() {
			site.Spec.Domain = "custom.domain.com"
			domain, source := reconciler.resolveDomain(ctx, site, bench)
			Expect(domain).To(Equal("custom.domain.com"))
			Expect(source).To(Equal("explicit"))
		})

		It("should use bench suffix when available", func() {
			domain, source := reconciler.resolveDomain(ctx, site, bench)
			Expect(domain).To(Equal("mysite.example.com"))
			Expect(source).To(Equal("bench-suffix"))
		})

		It("should fall back to site name", func() {
			bench.Spec.DomainConfig = nil
			domain, source := reconciler.resolveDomain(ctx, site, bench)
			Expect(domain).To(Equal("mysite"))
			Expect(source).To(Equal("sitename-default"))
		})
	})

	Describe("Security Contexts", func() {
		It("should provide correct security context for non-OpenShift", func() {
			// Use explicit Security overrides so the test does not depend on env (FRAPPE_DEFAULT_UID/GID)
			uid, gid := int64(1001), int64(0)
			bench.Spec.Security = &vyogotechv1alpha1.SecurityConfig{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsUser:    &uid,
					RunAsGroup:   &gid,
					RunAsNonRoot: boolPtr(true),
				},
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:    &uid,
					RunAsGroup:   &gid,
					RunAsNonRoot: boolPtr(true),
					Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
			}

			reconciler.IsOpenShift = false
			podSec := reconciler.getPodSecurityContext(ctx, bench)
			Expect(podSec.RunAsUser).NotTo(BeNil())
			Expect(*podSec.RunAsUser).To(Equal(int64(1001)))
			Expect(*podSec.RunAsNonRoot).To(BeTrue())

			contSec := reconciler.getContainerSecurityContext(ctx, bench)
			Expect(contSec.RunAsUser).NotTo(BeNil())
			Expect(*contSec.RunAsUser).To(Equal(int64(1001)))
			Expect(contSec.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
		})

		It("should provide correct security context for OpenShift", func() {
			reconciler.IsOpenShift = true
			podSec := reconciler.getPodSecurityContext(ctx, bench)
			Expect(podSec.RunAsUser).To(BeNil())

			contSec := reconciler.getContainerSecurityContext(ctx, bench)
			Expect(contSec.RunAsUser).To(BeNil())
		})
	})
})
