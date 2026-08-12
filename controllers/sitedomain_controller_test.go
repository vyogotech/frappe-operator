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
	"testing"
	"time"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSiteDomainReconciler_Reconcile_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	siteDomain := &vyogotechv1.SiteDomain{
		ObjectMeta: metav1.ObjectMeta{Name: "acme-domain", Namespace: "default"},
		Spec: vyogotechv1.SiteDomainSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
			Domain:  "erp.acmecorp.com",
			TLS: &vyogotechv1.SiteDomainTLSSpec{
				Enabled:    true,
				SecretName: "acme-tls",
			},
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"},
		},
		Status: vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteDomain, site).WithStatusSubresource(siteDomain).Build()
	r := &SiteDomainReconciler{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		DNSLookupFunc: func(host string) ([]string, error) {
			return []string{"203.0.113.50"}, nil
		},
	}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "acme-domain", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	updatedDomain := &vyogotechv1.SiteDomain{}
	err = client.Get(ctx, types.NamespacedName{Name: "acme-domain", Namespace: "default"}, updatedDomain)
	if err != nil {
		t.Fatalf("failed to fetch updated site domain: %v", err)
	}

	if updatedDomain.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updatedDomain.Status.Phase)
	}
	if !updatedDomain.Status.DNSConfigured {
		t.Error("expected DNSConfigured to be true")
	}

	// Verify Ingress was created
	ingress := &networkingv1.Ingress{}
	err = client.Get(ctx, types.NamespacedName{Name: updatedDomain.Status.IngressName, Namespace: "default"}, ingress)
	if err != nil {
		t.Fatalf("failed to fetch created Ingress: %v", err)
	}
	if ingress.Spec.Rules[0].Host != "erp.acmecorp.com" {
		t.Errorf("expected host erp.acmecorp.com, got %s", ingress.Spec.Rules[0].Host)
	}
}

func TestSiteDomainReconciler_Reconcile_ValidationAndNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing Domain", func(t *testing.T) {
		siteDomain := &vyogotechv1.SiteDomain{
			ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "default"},
			Spec:       vyogotechv1.SiteDomainSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteDomain).WithStatusSubresource(siteDomain).Build()
		r := &SiteDomainReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "d1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing Domain")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteDomain := &vyogotechv1.SiteDomain{
			ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "default"},
			Spec: vyogotechv1.SiteDomainSpec{
				SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
				Domain:  "erp.acme.com",
			},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteDomain, site).WithStatusSubresource(siteDomain).Build()
		r := &SiteDomainReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "d1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}
