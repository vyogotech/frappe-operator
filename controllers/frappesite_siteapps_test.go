package controllers

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// Deleting a FrappeSite deletes the SiteApps that reference it - and only
// those: same-named sites in other namespaces and other sites' apps stay.
func TestFrappeSiteReconciler_DeleteSiteAppsCascades(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	siteApp := func(name, namespace, site, siteNamespace string) *vyogotechv1.SiteApp {
		return &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: vyogotechv1.SiteAppSpec{
				SiteRef: &vyogotechv1.NamespacedName{Name: site, Namespace: siteNamespace},
				AppName: "drive",
			},
		}
	}
	site := &vyogotechv1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "tenant", Namespace: "bench-v16"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		site,
		siteApp("same-ns", "bench-v16", "tenant", ""),
		siteApp("same-ns-explicit", "bench-v16", "tenant", "bench-v16"),
		siteApp("cross-ns", "apps", "tenant", "bench-v16"),
		siteApp("other-site", "bench-v16", "neighbour", ""),
		siteApp("same-name-other-ns", "bench-v15", "tenant", ""),
	).Build()
	r := &FrappeSiteReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

	if err := r.deleteSiteApps(context.Background(), site); err != nil {
		t.Fatalf("deleteSiteApps: %v", err)
	}

	left := &vyogotechv1.SiteAppList{}
	if err := c.List(context.Background(), left); err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]bool{}
	for _, sa := range left.Items {
		got[sa.Name] = true
	}
	for _, gone := range []string{"same-ns", "same-ns-explicit", "cross-ns"} {
		if got[gone] {
			t.Errorf("SiteApp %s references the deleted site and must be deleted", gone)
		}
	}
	for _, kept := range []string{"other-site", "same-name-other-ns"} {
		if !got[kept] {
			t.Errorf("SiteApp %s does not reference the deleted site and must be kept", kept)
		}
	}
}

// The whole deletion path, both controllers: deleting the site leaves neither
// the site nor its SiteApp behind, and no uninstall Job is created on the way.
func TestFrappeSiteDeletion_TakesItsSiteAppsWithIt(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	now := metav1.NewTime(time.Now())
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tenant", Namespace: "bench-v16",
			DeletionTimestamp: &now, Finalizers: []string{frappeSiteFinalizer},
		},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "tenant.example.com",
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench"},
		},
		Status: vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-drive", Namespace: "bench-v16", Finalizers: []string{siteAppFinalizer}},
		Spec: vyogotechv1.SiteAppSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "tenant"},
			AppName: "drive",
		},
		Status: vyogotechv1.SiteAppStatus{Phase: "Ready"},
	}
	bench := &vyogotechv1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "bench-v16"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(site, siteApp, bench).
		WithStatusSubresource(&vyogotechv1.FrappeSite{}, &vyogotechv1.SiteApp{}).Build()
	siteReconciler := &FrappeSiteReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(20)}
	appReconciler := &SiteAppReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(20)}

	ctx := context.Background()
	siteKey := client.ObjectKeyFromObject(site)
	appKey := client.ObjectKeyFromObject(siteApp)

	// The site finalizer deletes the SiteApp (deletionPolicy Retain: no drop-site Job) ...
	if _, err := siteReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: siteKey}); err != nil {
		t.Fatalf("FrappeSite Reconcile: %v", err)
	}
	// ... and the SiteApp's delete event releases its finalizer.
	if _, err := appReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: appKey}); err != nil {
		t.Fatalf("SiteApp Reconcile: %v", err)
	}

	if err := c.Get(ctx, siteKey, &vyogotechv1.FrappeSite{}); !errors.IsNotFound(err) {
		t.Errorf("FrappeSite must be gone, got err=%v", err)
	}
	if err := c.Get(ctx, appKey, &vyogotechv1.SiteApp{}); !errors.IsNotFound(err) {
		t.Errorf("SiteApp must be gone with its site, got err=%v", err)
	}
	assertNoJobs(t, appReconciler)
}
