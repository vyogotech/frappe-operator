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

// Contract tests for the objects the operator emits on OpenShift.
//
// Each spec here corresponds to a defect that reached a release: the operator
// rendered a Deployment, Job, Secret or Route that a real cluster then refused
// or mis-served. They assert on the emitted object rather than on cluster
// behaviour, so they run anywhere, but the inputs are the ones OpenShift really
// supplies - the namespace's SCC allocation annotations, and the cluster-scoped
// config.openshift.io/v1 Ingress that carries the router's wildcard domain.

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	"github.com/vyogotech/frappe-operator/pkg/resources"
	"github.com/vyogotech/frappe-operator/pkg/scripts"
)

const (
	// The range OpenShift stamps on a namespace. fsGroup must land inside it.
	testSCCGroupRange = "1000670000/10000"
	testSCCGroupStart = int64(1000670000)
	testClusterDomain = "apps.ci.example.com"
)

var openShiftIngressGVK = schema.GroupVersionKind{
	Group:   "config.openshift.io",
	Version: "v1",
	Kind:    "Ingress",
}

// newOpenShiftScheme registers the cluster-scoped config.openshift.io/v1 Ingress
// as an unstructured type, which is how the operator reads it.
func newOpenShiftScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	Expect(vyogotechv1.AddToScheme(s)).To(Succeed())
	Expect(corev1.AddToScheme(s)).To(Succeed())
	Expect(routev1.AddToScheme(s)).To(Succeed())

	s.AddKnownTypeWithName(openShiftIngressGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(openShiftIngressGVK.GroupVersion().WithKind("IngressList"), &unstructured.UnstructuredList{})
	return s
}

// openShiftNamespace is a namespace carrying the annotations OpenShift's SCC
// admission stamps on every project.
func openShiftNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"openshift.io/sa.scc.supplemental-groups": testSCCGroupRange,
				"openshift.io/sa.scc.uid-range":           testSCCGroupRange,
				"openshift.io/sa.scc.mcs":                 "s0:c26,c15",
			},
		},
	}
}

// clusterIngressConfig is the object that carries the router's wildcard domain.
func clusterIngressConfig(domain string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(openShiftIngressGVK)
	u.SetName("cluster")
	Expect(unstructured.SetNestedField(u.Object, domain, "spec", "domain")).To(Succeed())
	return u
}

var _ = Describe("OpenShift emitted-object contracts", func() {
	const namespace = "frappe-openshift"

	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		fakeClient client.Client
		reconciler *FrappeSiteReconciler
		recorder   *record.FakeRecorder
		bench      *vyogotechv1.FrappeBench
		site       *vyogotechv1.FrappeSite
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = newOpenShiftScheme()
		recorder = record.NewFakeRecorder(20)

		bench = &vyogotechv1.FrappeBench{
			ObjectMeta: metav1.ObjectMeta{Name: "ocp-bench", Namespace: namespace},
			Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
		}

		site = &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "ocp-site", Namespace: namespace},
			Spec: vyogotechv1.FrappeSiteSpec{
				// Deliberately unroutable: a reserved TLD per RFC 6761.
				SiteName: "dev.localhost",
				BenchRef: &vyogotechv1.NamespacedName{Name: bench.Name},
			},
		}

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(openShiftNamespace(namespace), clusterIngressConfig(testClusterDomain), bench, site).
			WithStatusSubresource(&vyogotechv1.FrappeSite{}).
			Build()

		reconciler = &FrappeSiteReconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			Recorder:    recorder,
			IsOpenShift: true,
		}
	})

	// Regression: FSGroup was left nil on OpenShift on the assumption that SCC
	// admission would inject one. Under a permissive SCC nothing is injected, the
	// CephFS volume stays root:root 0755, and bench-init dies with
	// "sites directory is NOT writable".
	Describe("pod security context", func() {
		It("sets an fsGroup inside the namespace's allocated range", func() {
			psc := PodSecurityContextForBench(ctx, fakeClient, true, namespace, nil)

			Expect(psc.FSGroup).NotTo(BeNil(),
				"fsGroup must be explicit; relying on SCC admission to inject one fails under permissive SCCs")
			Expect(*psc.FSGroup).To(Equal(testSCCGroupStart))
		})

		It("still honours an explicit fsGroup from the spec", func() {
			override := int64(2002)
			psc := PodSecurityContextForBench(ctx, fakeClient, true, namespace, &vyogotechv1.SecurityConfig{
				PodSecurityContext: &corev1.PodSecurityContext{FSGroup: &override},
			})

			Expect(psc.FSGroup).NotTo(BeNil())
			Expect(*psc.FSGroup).To(Equal(override))
		})

		It("leaves fsGroup unset when the namespace carries no SCC allocation", func() {
			bare := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "bare"}}).Build()

			psc := PodSecurityContextForBench(ctx, bare, true, "bare", nil)
			Expect(psc.FSGroup).To(BeNil())
		})
	})

	// Regression: domain detection only probed nginx/traefik Services, so on
	// OpenShift it always fell back to the raw siteName and produced a Route on a
	// host that resolves nowhere.
	Describe("domain resolution", func() {
		It("detects the cluster domain and corrects an unroutable siteName", func() {
			domain, source := reconciler.resolveDomain(ctx, site, bench)

			Expect(domain).To(Equal("dev." + testClusterDomain))
			Expect(source).To(Equal("auto-corrected"))
			Expect(recorder.Events).To(Receive(ContainSubstring("DomainAutoCorrected")))
		})

		It("leaves a real external domain alone", func() {
			site.Spec.SiteName = "erp.acme.com"

			domain, _ := reconciler.resolveDomain(ctx, site, bench)
			Expect(domain).To(Equal("erp.acme.com"))
		})

		It("pins the resolved domain once the site is Ready", func() {
			site.Status.Phase = vyogotechv1.FrappeSitePhaseReady
			site.Status.ResolvedDomain = "already." + testClusterDomain
			site.Status.DomainSource = "auto-corrected"

			domain, _ := reconciler.resolveDomain(ctx, site, bench)
			Expect(domain).To(Equal("already."+testClusterDomain),
				"the site directory and database name derive from this, so it must not move")
		})
	})

	// Regression: Frappe resolves a request by matching the Host header to a
	// directory under sites/, so a Route host that differs from the site name
	// is answered 404. Renaming the site to the resolved domain fixed that but
	// broke every controller that addresses the site by spec.SiteName (backup,
	// restore, migration, app install, deletion). The site therefore keeps its
	// own name, and the init script aliases sites/<domain> -> <siteName>.
	Describe("site initialization secret", func() {
		It("keeps the site under spec.SiteName and aliases the Route host to it", func() {
			domain := "dev." + testClusterDomain
			Expect(domain).NotTo(Equal(site.Spec.SiteName), "test must exercise the diverging case")

			Expect(reconciler.ensureInitSecrets(ctx, site, bench, domain,
				&database.DatabaseInfo{Host: "db", Port: "3306", Name: "sitedb", Provider: "mariadb"},
				&database.DatabaseCredentials{Username: "u", Password: "p"},
				"admin-pw", "redis-cache:6379", "redis-queue:6379",
			)).To(Succeed())

			secret := &corev1.Secret{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-init-secrets",
				Namespace: namespace,
			}, secret)).To(Succeed())

			Expect(string(secret.Data["site_name"])).To(Equal(site.Spec.SiteName),
				"bench --site, backups, restores and migrations all address the site by spec.SiteName")
			Expect(string(secret.Data["domain"])).To(Equal(domain),
				"the Route host and site_config host_name come from the resolved domain")

			initScript := scripts.MustGetScript(scripts.SiteInit)
			Expect(initScript).To(ContainSubstring(`ln -sfn "$SITE_NAME" "/home/frappe/frappe-bench/sites/$DOMAIN"`),
				"without the sites/<domain> alias, a request on the Route host is answered 404")
		})
	})

	// Regression: routeConfig.host was documented in the CRD as overriding the
	// generated hostname but was never read.
	Describe("route", func() {
		It("honours routeConfig.host", func() {
			site.Spec.RouteConfig = &vyogotechv1.RouteConfig{
				Enabled: resources.BoolPtr(true),
				Host:    "custom." + testClusterDomain,
			}

			Expect(reconciler.ensureRoute(ctx, site, bench, "dev."+testClusterDomain)).To(Succeed())

			route := &routev1.Route{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-route",
				Namespace: namespace,
			}, route)).To(Succeed())

			Expect(route.Spec.Host).To(Equal("custom." + testClusterDomain))
		})

		It("falls back to the resolved domain when no override is given", func() {
			site.Spec.RouteConfig = &vyogotechv1.RouteConfig{Enabled: resources.BoolPtr(true)}

			Expect(reconciler.ensureRoute(ctx, site, bench, "dev."+testClusterDomain)).To(Succeed())

			route := &routev1.Route{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      site.Name + "-route",
				Namespace: namespace,
			}, route)).To(Succeed())

			Expect(route.Spec.Host).To(Equal("dev." + testClusterDomain))
		})
	})
})
