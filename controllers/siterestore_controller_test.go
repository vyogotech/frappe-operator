package controllers

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func restoreCR(withNamespace bool) *vyogotechv1.SiteRestore {
	ref := vyogotechv1.NamespacedName{Name: "bench1"}
	if withNamespace {
		ref.Namespace = "t1"
	}
	return &vyogotechv1.SiteRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "t1"},
		Spec: vyogotechv1.SiteRestoreSpec{
			Site:                 "site1.example.com",
			BenchRef:             ref,
			DatabaseBackupSource: vyogotechv1.BackupSource{LocalPath: "sites/site1.example.com/private/backups/x/database.sql.gz"},
		},
	}
}

// benchRef.namespace defaults to the SiteRestore's namespace; before, an empty
// namespace made the bench lookup fail forever with no status on the CR.
func TestSiteRestoreDefaultsBenchNamespaceAndCreatesJob(t *testing.T) {
	scheme := testScheme(t)
	bench := &vyogotechv1.FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "t1"}}
	sr := restoreCR(false)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(bench, sr).WithStatusSubresource(sr).Build()
	r := &SiteRestoreReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "r1", Namespace: "t1"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	job := &batchv1.Job{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "r1-restore", Namespace: "t1"}, job); err != nil {
		t.Fatalf("restore Job not created: %v", err)
	}
	got := &vyogotechv1.SiteRestore{}
	_ = c.Get(context.Background(), types.NamespacedName{Name: "r1", Namespace: "t1"}, got)
	if got.Status.Phase != "Running" {
		t.Fatalf("phase = %q, want Running", got.Status.Phase)
	}
}

func TestSiteRestoreReportsMissingBench(t *testing.T) {
	scheme := testScheme(t)
	sr := restoreCR(true)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(sr).WithStatusSubresource(sr).Build()
	r := &SiteRestoreReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "r1", Namespace: "t1"}})
	if err != nil || res.RequeueAfter == 0 {
		t.Fatalf("want a clean requeue while the bench is missing, got res=%v err=%v", res, err)
	}
	got := &vyogotechv1.SiteRestore{}
	_ = c.Get(context.Background(), types.NamespacedName{Name: "r1", Namespace: "t1"}, got)
	if got.Status.Phase != "Pending" || got.Status.Message == "" {
		t.Fatalf("missing bench must be visible on the CR, got phase=%q message=%q", got.Status.Phase, got.Status.Message)
	}
}
