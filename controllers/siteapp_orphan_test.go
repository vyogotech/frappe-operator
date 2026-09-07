package controllers

import (
	"context"
	"testing"
	"time"

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
