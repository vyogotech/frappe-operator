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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestSiteAppReconciler_Reconcile_SuccessAndUninstall(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	appInstalled := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/login" {
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/method/frappe.installer.install_app" {
			appInstalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "Installed"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
		Spec: vyogotechv1.SiteAppSpec{
			SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
			AppName: "erpnext_australian_localisation",
		},
	}
	// The site references a bench: the uninstall path builds a Job that mounts the
	// bench's <bench>-sites PVC, so the bench must exist for the delete path.
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"},
			SiteName: "site1.example.com",
		},
		Status: vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "15.0.0"},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "site1-admin", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("adminpass")},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteApp, site, bench, adminSecret).WithStatusSubresource(siteApp).Build()
	r := &SiteAppReconciler{
		Client:       client,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(10),
		FrappeClient: NewFrappeClient(ts.URL, "Administrator", "adminpass"),
	}

	ctx := context.Background()

	// 1. First Reconcile adds finalizer and installs app
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if !appInstalled {
		t.Error("expected app to be installed via REST API")
	}

	updatedApp := &vyogotechv1.SiteApp{}
	err = client.Get(ctx, types.NamespacedName{Name: "app1", Namespace: "default"}, updatedApp)
	if err != nil {
		t.Fatalf("failed to fetch updated site app: %v", err)
	}

	if updatedApp.Status.Phase != "Ready" {
		t.Errorf("expected phase Ready, got %s", updatedApp.Status.Phase)
	}

	// 2. Test Deletion Finalizer: deleting the SiteApp must run a bench
	// uninstall-app Job (not merely drop the card), and the finalizer must be
	// held until that Job completes.
	err = client.Delete(ctx, updatedApp)
	if err != nil {
		t.Fatalf("failed to delete site app: %v", err)
	}

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile deletion failed: %v", err)
	}

	uninstallJob := &batchv1.Job{}
	err = client.Get(ctx, types.NamespacedName{Name: "app1-app-uninstall", Namespace: "default"}, uninstallJob)
	if err != nil {
		t.Fatalf("expected uninstall Job to be created on deletion: %v", err)
	}

	cmd := uninstallJob.Spec.Template.Spec.Containers[0].Command
	joined := strings.Join(cmd, "\n")
	if !strings.Contains(joined, "uninstall-app") {
		t.Errorf("expected uninstall Job command to contain 'uninstall-app', got: %s", joined)
	}

	// The uninstall Job must mount the bench's <bench>-sites PVC (bench1-sites).
	foundPVC := false
	for _, v := range uninstallJob.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == "bench1-sites" {
			foundPVC = true
		}
	}
	if !foundPVC {
		t.Error("expected uninstall Job to mount PVC 'bench1-sites'")
	}

	// The finalizer must still be present (uninstall Job has not completed yet), so
	// the CR is not prematurely removed while the app is still on the site.
	pendingApp := &vyogotechv1.SiteApp{}
	if err := client.Get(ctx, types.NamespacedName{Name: "app1", Namespace: "default"}, pendingApp); err != nil {
		t.Fatalf("failed to re-fetch site app during deletion: %v", err)
	}
	if !controllerutil.ContainsFinalizer(pendingApp, siteAppFinalizer) {
		t.Error("expected finalizer to be retained while uninstall Job is running")
	}

	// 3. Simulate the uninstall Job succeeding; the next reconcile must drop the
	// finalizer so the CR is finally removed.
	uninstallJob.Status.Succeeded = 1
	if err := client.Status().Update(ctx, uninstallJob); err != nil {
		t.Fatalf("failed to mark uninstall job succeeded: %v", err)
	}

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile after uninstall success failed: %v", err)
	}

	goneApp := &vyogotechv1.SiteApp{}
	err = client.Get(ctx, types.NamespacedName{Name: "app1", Namespace: "default"}, goneApp)
	if err == nil && controllerutil.ContainsFinalizer(goneApp, siteAppFinalizer) {
		t.Error("expected finalizer to be removed after uninstall Job succeeded")
	}
}

func TestSiteAppReconciler_Reconcile_ValidationAndNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("Missing AppName", func(t *testing.T) {
		siteApp := &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
			Spec:       vyogotechv1.SiteAppSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteApp).WithStatusSubresource(siteApp).Build()
		r := &SiteAppReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
		if err == nil {
			t.Error("expected error for missing AppName")
		}
	})

	t.Run("Site Not Ready", func(t *testing.T) {
		siteApp := &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
			Spec: vyogotechv1.SiteAppSpec{
				SiteRef: &vyogotechv1.NamespacedName{Name: "site1"},
				AppName: "payments",
			},
		}
		site := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
			Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseProvisioning},
		}
		adminSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "site1-admin", Namespace: "default"},
			Data:       map[string][]byte{"password": []byte("adminpass")},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(siteApp, site, adminSecret).WithStatusSubresource(siteApp).Build()
		r := &SiteAppReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RequeueAfter != 15*time.Second {
			t.Errorf("expected RequeueAfter 15s, got %v", res.RequeueAfter)
		}
	})
}

// The "already installed" decision must never be read off the site's Spec.Apps:
// site-init can skip an app it cannot find yet still leave the name in the spec,
// so trusting Spec.Apps skips a real install and blocks repairing such a site.
func TestSiteAppReconciler_Reconcile_IgnoresPoisonedSpecApps(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	ctx := context.Background()

	installed := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/method/login":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-sid"})
			w.WriteHeader(http.StatusOK)
		case "/api/method/frappe.installer.install_app":
			installed = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message": "Installed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
		Spec:       vyogotechv1.SiteAppSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}, AppName: "wiki"},
	}
	// The site's spec CLAIMS wiki, but it is not actually installed. The old code
	// short-circuited on this and never installed; the fix must install anyway.
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"},
			SiteName: "site1.example.com",
			Apps:     []string{"wiki"},
		},
		Status: vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "16.0.0"},
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "site1-admin", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("adminpass")},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(siteApp, site, bench, adminSecret).WithStatusSubresource(siteApp).Build()
	r := &SiteAppReconciler{
		Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		FrappeClient: NewFrappeClient(ts.URL, "Administrator", "adminpass"),
	}

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if !installed {
		t.Error("app must be installed even though site.Spec.Apps already lists it (Spec.Apps is desired state, not proof of install)")
	}
}

// Once a SiteApp has finished installing for its current generation, a later
// reconcile must not redo the work.
func TestSiteAppReconciler_Reconcile_SkipsWhenAlreadyReconciled(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	ctx := context.Background()

	installed := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/method/frappe.installer.install_app" {
			installed = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app1", Namespace: "default",
			Finalizers: []string{siteAppFinalizer},
		},
		Spec:   vyogotechv1.SiteAppSpec{SiteRef: &vyogotechv1.NamespacedName{Name: "site1"}, AppName: "wiki"},
		Status: vyogotechv1.SiteAppStatus{Phase: "Ready", ObservedGeneration: 0},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeSiteSpec{BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"}, SiteName: "site1.example.com"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(siteApp, site).WithStatusSubresource(siteApp).Build()
	r := &SiteAppReconciler{
		Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		FrappeClient: NewFrappeClient(ts.URL, "Administrator", "adminpass"),
	}

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if installed {
		t.Error("a SiteApp already Ready for its current generation must not reinstall")
	}
}

// A SiteApp carrying an FPMPackage must install via `fpm install` (prebuilt
// package) rather than a git clone; git stays the fallback for apps without one.
func TestSiteAppReconciler_Reconcile_FPMInstallPath(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	ctx := context.Background()

	skipBackup := false
	siteApp := &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default", Finalizers: []string{siteAppFinalizer}},
		Spec: vyogotechv1.SiteAppSpec{
			SiteRef:             &vyogotechv1.NamespacedName{Name: "site1"},
			AppName:             "wiki",
			FPMPackage:          "frappe/wiki==3.0.0",
			FPMRepo:             "ghcr.io/vyogotech/fpm",
			FPMRepoType:         "oci",
			BackupBeforeInstall: skipBackup, // skip preflight backup so the install Job is built now
		},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeSiteSpec{BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"}, SiteName: "site1.example.com"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "16.0.0"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(siteApp, site, bench).WithStatusSubresource(siteApp).Build()
	r := &SiteAppReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	job := &batchv1.Job{}
	if err := client.Get(ctx, types.NamespacedName{Name: "app1-app-install", Namespace: "default"}, job); err != nil {
		t.Fatalf("expected install Job to be created: %v", err)
	}
	script := strings.Join(job.Spec.Template.Spec.Containers[0].Command, "\n") +
		strings.Join(job.Spec.Template.Spec.Containers[0].Args, "\n")
	if !strings.Contains(script, `fpm install "$FPM_PACKAGE"`) {
		t.Error("install Job script must use `fpm install` when FPMPackage is set")
	}
	if !strings.Contains(script, `--type "${FPM_REPO_TYPE:-http}"`) {
		t.Error("install Job script must configure the repo backend type (OCI vs HTTP)")
	}
	// The package + repo must reach the Job as env.
	env := map[string]string{}
	authFrom := map[string]bool{}
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			authFrom[e.Name] = e.ValueFrom.SecretKeyRef.Name == "fpm-registry-auth"
		}
	}
	if env["FPM_PACKAGE"] != "frappe/wiki==3.0.0" || env["FPM_REPO"] != "ghcr.io/vyogotech/fpm" || env["FPM_REPO_TYPE"] != "oci" {
		t.Errorf("FPM env not passed to Job: %v", env)
	}
	// A private OCI registry needs credentials from the fpm-registry-auth Secret.
	if !authFrom["FPM_USERNAME"] || !authFrom["FPM_TOKEN"] {
		t.Error("OCI install must source FPM_USERNAME/FPM_TOKEN from the fpm-registry-auth Secret")
	}
}

func TestSiteAppReconciler_reloadBenchServingPods(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	// mkDeploy builds a bench deployment the way the operator does: the "component"
	// label lives on the pod template (and selector), NOT on the Deployment's own
	// metadata (which only carries app + bench). The reload must read it from the
	// template — this is the regression this test guards.
	mkDeploy := func(name, component string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "user-ns",
				Labels:    map[string]string{"app": "frappe", "bench": "b1"},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "frappe", "bench": "b1", "component": component}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frappe", "bench": "b1", "component": component}},
				},
			},
		}
	}

	guni := mkDeploy("b1-gunicorn", "gunicorn")
	worker := mkDeploy("b1-worker-default", "worker-default")
	scheduler := mkDeploy("b1-scheduler", "scheduler")
	nginx := mkDeploy("b1-nginx", "nginx")          // must NOT be rolled
	socketio := mkDeploy("b1-socketio", "socketio") // must NOT be rolled
	otherBench := mkDeploy("b2-gunicorn", "gunicorn")
	otherBench.Labels["bench"] = "b2"
	otherBench.Spec.Template.Labels["bench"] = "b2"

	client := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(guni, worker, scheduler, nginx, socketio, otherBench).Build()
	r := &SiteAppReconciler{Client: client, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
	ctx := context.Background()

	annKey := "vyogo.tech/reload-builder"
	getAnn := func(name string) string {
		d := &appsv1.Deployment{}
		_ = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "user-ns"}, d)
		return d.Spec.Template.Annotations[annKey]
	}

	r.reloadBenchServingPods(ctx, "b1", "user-ns", "builder", "gen-1")

	// serving components rolled
	for _, n := range []string{"b1-gunicorn", "b1-worker-default", "b1-scheduler"} {
		if got := getAnn(n); got != "gen-1" {
			t.Errorf("%s: expected reload annotation gen-1, got %q", n, got)
		}
	}
	// non-serving + other-bench untouched
	for _, n := range []string{"b1-nginx", "b1-socketio", "b2-gunicorn"} {
		if got := getAnn(n); got != "" {
			t.Errorf("%s: expected NO reload annotation, got %q", n, got)
		}
	}

	// idempotency: same generation must not change the resourceVersion (no roll loop)
	before := &appsv1.Deployment{}
	_ = client.Get(ctx, types.NamespacedName{Name: "b1-gunicorn", Namespace: "user-ns"}, before)
	r.reloadBenchServingPods(ctx, "b1", "user-ns", "builder", "gen-1")
	after := &appsv1.Deployment{}
	_ = client.Get(ctx, types.NamespacedName{Name: "b1-gunicorn", Namespace: "user-ns"}, after)
	if before.ResourceVersion != after.ResourceVersion {
		t.Errorf("expected no-op on same generation, resourceVersion changed %s -> %s", before.ResourceVersion, after.ResourceVersion)
	}

	// a new generation rolls again
	r.reloadBenchServingPods(ctx, "b1", "user-ns", "builder", "gen-2")
	if got := getAnn("b1-gunicorn"); got != "gen-2" {
		t.Errorf("expected reload annotation gen-2 after new generation, got %q", got)
	}
}
