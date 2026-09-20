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

// Same orphan rule as SiteApp: a SiteDomain whose site has been deleted has
// nothing left to clean up and must not sit in Terminating behind its finalizer.
func TestSiteDomainReconciler_ReleasesFinalizerWhenSiteIsGone(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	now := metav1.NewTime(time.Now())
	dom := &vyogotechv1.SiteDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "dom-orphan",
			Namespace:         "bench-v16",
			DeletionTimestamp: &now,
			Finalizers:        []string{siteDomainFinalizer},
		},
		Spec: vyogotechv1.SiteDomainSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site-that-was-deleted"},
			Domain:  "erp.example.com",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(dom).WithStatusSubresource(dom).Build()
	r := &SiteDomainReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

	key := types.NamespacedName{Name: "dom-orphan", Namespace: "bench-v16"}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("an orphan must not be requeued forever, got RequeueAfter=%s", res.RequeueAfter)
	}
	if err := client.Get(context.Background(), key, &vyogotechv1.SiteDomain{}); !errors.IsNotFound(err) {
		t.Fatalf("SiteDomain should be gone once its finalizer is released, got err=%v", err)
	}
}
