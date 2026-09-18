package controllers

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// Deleting a FrappeSite before its SiteApps left every SiteApp in Terminating
// forever: the finalizer insisted on an uninstall Job, which cannot run against
// a site that no longer exists. With the site gone there is nothing to
// uninstall from, so the finalizer must be released.
func TestSiteAppReconciler_ReleasesFinalizerWhenSiteIsGone(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	now := metav1.NewTime(time.Now())
	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "app-orphan",
			Namespace:         "bench-v16",
			DeletionTimestamp: &now,
			Finalizers:        []string{siteAppFinalizer},
		},
		Spec: vyogotechv1.SiteAppSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site-that-was-deleted"},
			AppName: "hrms",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteApp).WithStatusSubresource(siteApp).Build()
	r := &SiteAppReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

	key := types.NamespacedName{Name: "app-orphan", Namespace: "bench-v16"}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("an orphan must not be requeued forever, got RequeueAfter=%s", res.RequeueAfter)
	}
	// With its last finalizer removed the fake client deletes the object outright.
	got := &vyogotechv1.SiteApp{}
	if err := client.Get(context.Background(), key, got); err == nil {
		t.Fatalf("SiteApp still exists with finalizers %v", got.Finalizers)
	} else if !errors.IsNotFound(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newOrphanTestReconciler(objs ...runtime.Object) *SiteAppReconciler {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(&vyogotechv1.SiteApp{}).Build()
	return &SiteAppReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
}

// reconcileUntilGone reconciles the SiteApp until it no longer exists: the
// first pass deletes it, the second (the delete event, in a real cluster)
// releases the finalizer.
func reconcileUntilGone(t *testing.T, r *SiteAppReconciler, key types.NamespacedName) {
	t.Helper()
	for i := 0; i < 3; i++ {
		res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := &vyogotechv1.SiteApp{}
		if err := r.Get(context.Background(), key, got); errors.IsNotFound(err) {
			return
		} else if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 0 {
			t.Fatalf("SiteApp of a deleted site is requeued (RequeueAfter=%s) instead of going away", res.RequeueAfter)
		}
	}
	t.Fatalf("SiteApp %s still exists", key.Name)
}

func assertNoJobs(t *testing.T, r *SiteAppReconciler) {
	t.Helper()
	jobs := &batchv1.JobList{}
	if err := r.List(context.Background(), jobs); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("no uninstall Job may run against a site that is terminating or gone, got %s", jobs.Items[0].Name)
	}
}

// The hub incident: a site was deleted and nobody deleted its SiteApp, which
// sat Pending on SiteNotFound for three days. A SiteApp that carries the
// finalizer has been reconciled against its site before, so a missing site
// means a deleted site, and the SiteApp goes with it.
func TestSiteAppReconciler_DeletesItselfWhenItsSiteWasDeleted(t *testing.T) {
	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Name: "app-left-behind", Namespace: "bench-v16", Finalizers: []string{siteAppFinalizer}},
		Spec: vyogotechv1.SiteAppSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site-that-was-deleted"},
			AppName: "drive",
		},
	}
	r := newOrphanTestReconciler(siteApp)
	reconcileUntilGone(t, r, types.NamespacedName{Name: "app-left-behind", Namespace: "bench-v16"})
	assertNoJobs(t, r)
}

// A SiteApp applied ahead of its FrappeSite (GitOps applies both at once) has
// never seen the site: it has no finalizer yet and must wait, not delete itself.
func TestSiteAppReconciler_WaitsForASiteThatDoesNotExistYet(t *testing.T) {
	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Name: "app-early", Namespace: "bench-v16"},
		Spec: vyogotechv1.SiteAppSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site-not-created-yet"},
			AppName: "drive",
		},
	}
	r := newOrphanTestReconciler(siteApp)
	key := types.NamespacedName{Name: "app-early", Namespace: "bench-v16"}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatal("a SiteApp waiting for its site must be requeued")
	}
	got := &vyogotechv1.SiteApp{}
	if err := r.Get(context.Background(), key, got); err != nil {
		t.Fatalf("SiteApp must still exist: %v", err)
	}
	if got.DeletionTimestamp != nil || got.Status.Phase != "Pending" {
		t.Fatalf("want a live Pending SiteApp, got phase=%q deletionTimestamp=%v", got.Status.Phase, got.DeletionTimestamp)
	}
}

// While the site is terminating its SiteApps are deleted without an uninstall
// Job: the database is about to be dropped (or retained untouched), and the Job
// would race `bench drop-site`.
func TestSiteAppReconciler_SkipsUninstallWhenSiteIsTerminating(t *testing.T) {
	now := metav1.NewTime(time.Now())
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{
			Name: "site-going", Namespace: "bench-v16",
			DeletionTimestamp: &now, Finalizers: []string{frappeSiteFinalizer},
		},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "site-going.example.com",
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench"},
		},
		Status: vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	bench := &vyogotechv1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "bench-v16"}}

	for name, deleting := range map[string]bool{"app-live": false, "app-terminating": true} {
		siteApp := &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "bench-v16", Finalizers: []string{siteAppFinalizer}},
			Spec: vyogotechv1.SiteAppSpec{
				SiteRef: &vyogotechv1.NamespacedName{Name: "site-going"},
				AppName: "drive",
			},
		}
		if deleting {
			siteApp.DeletionTimestamp = &now
		}
		r := newOrphanTestReconciler(site.DeepCopy(), bench.DeepCopy(), siteApp)
		reconcileUntilGone(t, r, types.NamespacedName{Name: name, Namespace: "bench-v16"})
		assertNoJobs(t, r)
	}
}
