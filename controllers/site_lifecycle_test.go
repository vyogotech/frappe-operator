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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

var _ = Describe("FrappeSite Lifecycle", func() {
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
				DomainConfig: &vyogotechv1.DomainConfig{
					Suffix: ".example.com",
				},
			},
		}

		site = &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-site",
				Namespace: namespace,
			},
			Spec: vyogotechv1.FrappeSiteSpec{
				SiteName: "mysite",
			},
		}

		scheme = runtime.NewScheme()
		_ = vyogotechv1.AddToScheme(scheme)
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
			bench.Spec.Security = &vyogotechv1.SecurityConfig{
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

	Describe("Object Storage Credentials Injection", func() {
		It("should inject S3 config and credentials into initialization secret", func() {
			site.Spec.ObjectStorage = &vyogotechv1.S3Config{
				Endpoint: "http://minio:9000",
				Bucket:   "mybucket",
				Region:   "us-east-1",
				UseSSL:   false,
				AccessKeySecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "s3-credentials"},
					Key:                  "access-key",
				},
				SecretKeySecret: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "s3-credentials"},
					Key:                  "secret-key",
				},
			}

			// Pre-create the credentials secret in the fake client
			s3Secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-credentials",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"access-key": []byte("myaccesskey"),
					"secret-key": []byte("mysecretkey"),
				},
			}
			Expect(fakeClient.Create(ctx, s3Secret)).To(Succeed())

			// Pre-create the encryption key secret (as ensureInitSecrets requires it or generates one)
			encSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-site-encryption-key",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"encryption_key": []byte("myencryptionkey"),
				},
			}
			Expect(fakeClient.Create(ctx, encSecret)).To(Succeed())

			// Set the Spec encryption key ref to point to it
			site.Spec.EncryptionKeySecretRef = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "test-site-encryption-key"},
				Key:                  "encryption_key",
			}

			// Call ensureInitSecrets
			err := reconciler.ensureInitSecrets(ctx, site, bench, "test-site.example.com", nil, nil, "admin123", "redis://cache", "redis://queue")
			Expect(err).NotTo(HaveOccurred())

			// Get the generated initialization secret
			initSecretName := fmt.Sprintf("%s-init-secrets", site.Name)
			initSecret := &corev1.Secret{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: initSecretName, Namespace: namespace}, initSecret)).To(Succeed())

			// Verify ObjectStorage keys exist in secret
			Expect(string(initSecret.Data["s3_endpoint"])).To(Equal("http://minio:9000"))
			Expect(string(initSecret.Data["s3_bucket"])).To(Equal("mybucket"))
			Expect(string(initSecret.Data["s3_region"])).To(Equal("us-east-1"))
			Expect(string(initSecret.Data["s3_use_ssl"])).To(Equal("false"))
			Expect(string(initSecret.Data["s3_key"])).To(Equal("myaccesskey"))
			Expect(string(initSecret.Data["s3_secret"])).To(Equal("mysecretkey"))
		})
	})
})
