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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// These specs run against the envtest API server with the generated CRDs, so
// they exercise the OpenAPI schema and CEL rules exactly as a real cluster
// would. The operator itself never sees an object that fails them.
var _ = Describe("FrappeMicroservice CRD validation", func() {
	var (
		ctx context.Context
		n   int
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	newMS := func(mutate func(ms *vyogotechv1.FrappeMicroservice)) *vyogotechv1.FrappeMicroservice {
		n++
		ms := &vyogotechv1.FrappeMicroservice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("ms-validation-%d", n),
				Namespace: "default",
			},
			Spec: vyogotechv1.FrappeMicroserviceSpec{
				SiteRef: vyogotechv1.LocalSiteReference{Name: "acme-site"},
				Image:   vyogotechv1.ImageConfig{Repository: "ghcr.io/acme/orders", Tag: "1.0.0"},
			},
		}
		if mutate != nil {
			mutate(ms)
		}
		return ms
	}

	withPaths := func(paths ...string) func(ms *vyogotechv1.FrappeMicroservice) {
		return func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Routing = &vyogotechv1.MicroserviceRouting{Paths: paths}
		}
	}

	expectRejected := func(ms *vyogotechv1.FrappeMicroservice, msgFragment string) {
		err := k8sClient.Create(ctx, ms)
		Expect(err).To(HaveOccurred(), "expected the API server to reject %+v", ms.Spec)
		Expect(err.Error()).To(ContainSubstring(msgFragment))
	}

	It("accepts a minimal spec and applies defaults", func() {
		ms := newMS(nil)
		Expect(k8sClient.Create(ctx, ms)).To(Succeed())

		got := &vyogotechv1.FrappeMicroservice{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ms.Name, Namespace: ms.Namespace}, got)).To(Succeed())
		Expect(got.Spec.Port).To(Equal(int32(8000)))
		Expect(got.Spec.Replicas).NotTo(BeNil())
		Expect(*got.Spec.Replicas).To(Equal(int32(1)))
		Expect(got.Spec.Routing).To(BeNil())
	})

	It("accepts valid routing paths", func() {
		ms := newMS(withPaths("/orders", "/api/resource/Sales%20Order", "/api/v1/orders"))
		Expect(k8sClient.Create(ctx, ms)).To(Succeed())
	})

	It("requires siteRef.name", func() {
		expectRejected(newMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.SiteRef.Name = "" }), "siteRef.name")
	})

	It("requires image.repository", func() {
		expectRejected(newMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Image.Repository = "" }), "image.repository is required")
	})

	It("rejects an out-of-range port", func() {
		expectRejected(newMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Port = 70000 }), "spec.port")
	})

	It("rejects negative replicas", func() {
		r := int32(-1)
		expectRejected(newMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Replicas = &r }), "spec.replicas")
	})

	It("accepts zero replicas", func() {
		r := int32(0)
		Expect(k8sClient.Create(ctx, newMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Replicas = &r }))).To(Succeed())
	})

	DescribeTable("rejects malformed paths",
		func(path string) {
			expectRejected(newMS(withPaths(path)), "must be an absolute path")
		},
		Entry("relative path", "orders"),
		Entry("root", "/"),
		Entry("trailing slash", "/orders/"),
		Entry("double slash", "/orders//x"),
		Entry("whitespace", "/sales order"),
		Entry("query string", "/orders?x=1"),
	)

	DescribeTable("rejects dot segments",
		func(path string) {
			expectRejected(newMS(withPaths(path)), "dot segments")
		},
		Entry("dot-dot", "/orders/../app"),
		Entry("dot", "/orders/./x"),
		Entry("trailing dot-dot", "/orders/.."),
	)

	DescribeTable("rejects reserved monolith prefixes",
		func(path string) {
			expectRejected(newMS(withPaths(path)), "reserved")
		},
		Entry("login method", "/api/method/login"),
		Entry("below login method", "/api/method/login/x"),
		Entry("parent of login method", "/api/method"),
		Entry("grandparent of login method", "/api"),
		Entry("desk", "/app"),
		Entry("below desk", "/app/sales-order"),
		Entry("login page", "/login"),
		Entry("assets", "/assets"),
		Entry("files", "/files"),
		Entry("private files", "/private"),
		Entry("socket.io", "/socket.io"),
	)

	It("does not treat a reserved name as a reserved prefix of a longer segment", func() {
		Expect(k8sClient.Create(ctx, newMS(withPaths("/application", "/login-history", "/api/methods")))).To(Succeed())
	})

	It("rejects duplicate paths within one CR", func() {
		expectRejected(newMS(withPaths("/orders", "/orders")), "Duplicate")
	})

	It("rejects an empty paths list when routing is set", func() {
		expectRejected(newMS(withPaths()), "spec.routing.paths")
	})

	DescribeTable("rejects env vars that would override binding keys",
		func(name string) {
			expectRejected(newMS(func(ms *vyogotechv1.FrappeMicroservice) {
				ms.Spec.Env = []corev1.EnvVar{{Name: name, Value: "x"}}
			}), "binding")
		},
		Entry("FRAPPE_SITE", "FRAPPE_SITE"),
		Entry("CENTRAL_SITE_URL", "CENTRAL_SITE_URL"),
		Entry("PORT", "PORT"),
		Entry("DB_PASSWORD", "DB_PASSWORD"),
		Entry("any DB_ key", "DB_SOMETHING"),
	)

	It("accepts unrelated env vars", func() {
		Expect(k8sClient.Create(ctx, newMS(func(ms *vyogotechv1.FrappeMicroservice) {
			ms.Spec.Env = []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "INFO"}, {Name: "REDIS_CACHE_HOST", Value: "redis"}}
		}))).To(Succeed())
	})

	It("forbids changing siteRef after creation", func() {
		ms := newMS(nil)
		Expect(k8sClient.Create(ctx, ms)).To(Succeed())
		ms.Spec.SiteRef.Name = "other-site"
		err := k8sClient.Update(ctx, ms)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("siteRef is immutable"))
	})
})
