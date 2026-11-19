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
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// forceInstallFixture is a Ready site on a bench plus one SiteApp, the smallest
// world the install-Job path of the reconciler needs.
type forceInstallFixture struct {
	client client.Client
	r      *SiteAppReconciler
	req    ctrl.Request
	jobKey types.NamespacedName
}

func newForceInstallFixture(t *testing.T, siteApp *vyogotechv1.SiteApp, extra ...runtime.Object) *forceInstallFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	siteApp.Name, siteApp.Namespace = "app1", "default"
	siteApp.Finalizers = []string{siteAppFinalizer}
	skipBackup := false
	siteApp.Spec.BackupBeforeInstall = &skipBackup // build the install Job without waiting on a backup
	siteApp.Spec.SiteRef = &vyogotechv1.NamespacedName{Name: "site1"}
	siteApp.Spec.AppName = "wiki"

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "site1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeSiteSpec{BenchRef: &vyogotechv1.NamespacedName{Name: "bench1"}, SiteName: "site1.example.com"},
		Status:     vyogotechv1.FrappeSiteStatus{Phase: vyogotechv1.FrappeSitePhaseReady},
	}
	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench1", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "16.0.0"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(append([]runtime.Object{siteApp, site, bench}, extra...)...).
		WithStatusSubresource(siteApp).Build()
	return &forceInstallFixture{
		client: c,
		r:      &SiteAppReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10)},
		req:    ctrl.Request{NamespacedName: types.NamespacedName{Name: "app1", Namespace: "default"}},
		jobKey: types.NamespacedName{Name: "app1-app-install", Namespace: "default"},
	}
}

func (f *forceInstallFixture) reconcile(t *testing.T) ctrl.Result {
	t.Helper()
	res, err := f.r.Reconcile(context.Background(), f.req)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	return res
}

func (f *forceInstallFixture) job(t *testing.T) *batchv1.Job {
	t.Helper()
	job := &batchv1.Job{}
	if err := f.client.Get(context.Background(), f.jobKey, job); err != nil {
		t.Fatalf("expected an install Job: %v", err)
	}
	return job
}

func (f *forceInstallFixture) noJob(t *testing.T, why string) {
	t.Helper()
	err := f.client.Get(context.Background(), f.jobKey, &batchv1.Job{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("%s: expected no install Job, got err=%v", why, err)
	}
}

func (f *forceInstallFixture) siteApp(t *testing.T) *vyogotechv1.SiteApp {
	t.Helper()
	sa := &vyogotechv1.SiteApp{}
	if err := f.client.Get(context.Background(), f.req.NamespacedName, sa); err != nil {
		t.Fatalf("get SiteApp: %v", err)
	}
	return sa
}

func (f *forceInstallFixture) finishJob(t *testing.T, succeeded bool) {
	t.Helper()
	job := f.job(t)
	if succeeded {
		job.Status.Succeeded = 1
	} else {
		job.Status.Failed = 1
	}
	if err := f.client.Status().Update(context.Background(), job); err != nil {
		t.Fatalf("update Job status: %v", err)
	}
}

func jobEnvNames(job *batchv1.Job) []string {
	var names []string
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		names = append(names, e.Name)
	}
	return names
}

func jobEnvValue(job *batchv1.Job, name string) (string, bool) {
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

// Without the annotation the install Job is what it always was: the same env
// in the same order, no FORCE_INSTALL, no Job annotation, generation label
// only.
func TestSiteAppForceInstall_AbsentAnnotationLeavesJobUnchanged(t *testing.T) {
	f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
		Spec: vyogotechv1.SiteAppSpec{FPMPackage: "frappe/wiki==3.0.1", FPMRepo: "https://fpm.example.com"},
	})
	f.reconcile(t)

	job := f.job(t)
	want := []string{"APP_NAME", "SITE_NAME", "GIT_REPO", "GIT_BRANCH", "FPM_PACKAGE", "FPM_REPO", "FPM_REPO_TYPE", "FPM_VERSION", "AUTO_MIGRATE", "FRAPPE_VERSION", "USER"}
	if got := jobEnvNames(job); !reflect.DeepEqual(got, want) {
		t.Errorf("install Job env changed without the annotation:\n got %v\nwant %v", got, want)
	}
	if len(job.Annotations) != 0 {
		t.Errorf("install Job must carry no annotations without a force token, got %v", job.Annotations)
	}
	if got := job.Spec.Template.Spec.Containers[0].Command; len(got) != 3 || got[2] != siteAppInstallScript {
		t.Error("install Job must run siteAppInstallScript unmodified")
	}

	f.finishJob(t, true)
	f.reconcile(t)
	sa := f.siteApp(t)
	if sa.Status.Phase != "Ready" || sa.Status.ForceInstallToken != "" {
		t.Errorf("expected Ready with no force token recorded, got phase=%q token=%q", sa.Status.Phase, sa.Status.ForceInstallToken)
	}
	if meta.FindStatusCondition(sa.Status.Conditions, "ForceInstall") != nil {
		t.Error("ForceInstall condition must not appear without the annotation")
	}
}

// With the annotation the Job is told to force (FORCE_INSTALL=1) and records
// which token it runs for; a successful run moves the token into status.
func TestSiteAppForceInstall_AnnotationForcesInstallJob(t *testing.T) {
	f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{forceInstallAnnotation: "2026-11-20T10:00:00Z"}},
		Spec:       vyogotechv1.SiteAppSpec{FPMPackage: "frappe/wiki==3.0.1", FPMRepo: "https://fpm.example.com"},
	})
	f.reconcile(t)

	job := f.job(t)
	if v, ok := jobEnvValue(job, "FORCE_INSTALL"); !ok || v != "1" {
		t.Errorf("forced install Job must set FORCE_INSTALL=1, got %q (present=%v)", v, ok)
	}
	if got := job.Annotations[forceInstallAnnotation]; got != "2026-11-20T10:00:00Z" {
		t.Errorf("forced install Job must record its token, got %q", got)
	}
	// The token is free text and must stay out of labels.
	for k, v := range job.Labels {
		if strings.Contains(v, "2026-11-20T10") {
			t.Errorf("token leaked into label %s=%s", k, v)
		}
	}

	f.finishJob(t, true)
	f.reconcile(t)
	sa := f.siteApp(t)
	if sa.Status.Phase != "Ready" || sa.Status.ForceInstallToken != "2026-11-20T10:00:00Z" {
		t.Errorf("expected Ready with the honoured token recorded, got phase=%q token=%q", sa.Status.Phase, sa.Status.ForceInstallToken)
	}
	if c := meta.FindStatusCondition(sa.Status.Conditions, "ForceInstall"); c == nil || c.Status != metav1.ConditionTrue {
		t.Errorf("expected ForceInstall=True after a forced install, got %+v", c)
	}
}

// A token that a completed install already honoured is not acted on again:
// no Job, no status churn, however often the SiteApp is reconciled.
func TestSiteAppForceInstall_HonouredTokenDoesNotReinstall(t *testing.T) {
	f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{forceInstallAnnotation: "t1"}},
		Spec:       vyogotechv1.SiteAppSpec{FPMPackage: "frappe/wiki==3.0.1"},
		Status:     vyogotechv1.SiteAppStatus{Phase: "Ready", ForceInstallToken: "t1"},
	})
	before := f.siteApp(t).ResourceVersion
	for i := 0; i < 3; i++ {
		if res := f.reconcile(t); res.RequeueAfter != 0 {
			t.Fatalf("reconcile %d: an honoured token must not requeue, got %v", i, res)
		}
	}
	f.noJob(t, "honoured token")
	if after := f.siteApp(t).ResourceVersion; after != before {
		t.Errorf("an honoured token must not touch the SiteApp (resourceVersion %s -> %s)", before, after)
	}
}

// The full one-shot cycle within a single generation: install, force with a
// token, settle, then force again with a new token. Each token runs exactly
// one Job, and the previous Job (same fixed name, same generation label) is
// cleared out of the way first.
func TestSiteAppForceInstall_NewTokenRunsExactlyOneNewJob(t *testing.T) {
	f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
		Spec: vyogotechv1.SiteAppSpec{FPMPackage: "frappe/wiki==3.0.1"},
	})
	ctx := context.Background()

	// Plain install.
	f.reconcile(t)
	f.finishJob(t, true)
	f.reconcile(t)
	if _, forced := jobEnvValue(f.job(t), "FORCE_INSTALL"); forced {
		t.Fatal("the plain install must not be forced")
	}

	setToken := func(token string) {
		t.Helper()
		sa := f.siteApp(t)
		if sa.Annotations == nil {
			sa.Annotations = map[string]string{}
		}
		sa.Annotations[forceInstallAnnotation] = token
		if err := f.client.Update(ctx, sa); err != nil {
			t.Fatalf("annotate SiteApp: %v", err)
		}
	}

	for _, token := range []string{"t1", "t2"} {
		setToken(token)

		// First pass removes the finished Job that did not run for this token.
		if res := f.reconcile(t); res.RequeueAfter == 0 {
			t.Fatalf("token %s: expected a requeue after clearing the previous Job", token)
		}
		f.noJob(t, "token "+token+": previous Job")

		// Second pass creates the forced Job.
		f.reconcile(t)
		job := f.job(t)
		if job.Status.Succeeded != 0 {
			t.Fatalf("token %s: expected a new, unfinished Job", token)
		}
		if v, _ := jobEnvValue(job, "FORCE_INSTALL"); v != "1" {
			t.Fatalf("token %s: new Job must be forced", token)
		}
		if got := job.Annotations[forceInstallAnnotation]; got != token {
			t.Fatalf("token %s: Job records token %q", token, got)
		}
		if got := f.siteApp(t).Status.ForceInstallToken; got == token {
			t.Fatalf("token %s recorded as honoured before its Job finished", token)
		}

		// While it runs, nothing is replaced.
		f.reconcile(t)
		if f.job(t).ResourceVersion != job.ResourceVersion {
			t.Fatalf("token %s: running Job must be left alone", token)
		}

		f.finishJob(t, true)
		f.reconcile(t)
		sa := f.siteApp(t)
		if sa.Status.Phase != "Ready" || sa.Status.ForceInstallToken != token {
			t.Fatalf("token %s: expected Ready and honoured, got phase=%q token=%q", token, sa.Status.Phase, sa.Status.ForceInstallToken)
		}

		// Settled: further reconciles neither delete nor recreate the Job.
		done := f.job(t)
		for i := 0; i < 3; i++ {
			if res := f.reconcile(t); res.RequeueAfter != 0 {
				t.Fatalf("token %s: settled SiteApp must not requeue, got %v", token, res)
			}
		}
		if again := f.job(t); again.Status.Succeeded != 1 || again.ResourceVersion != done.ResourceVersion {
			t.Fatalf("token %s: completed forced install looped", token)
		}
	}
}

// A forced Job that failed is not retried on its own (same as any failed
// install); its token stays un-honoured, and a new token is the retry.
func TestSiteAppForceInstall_FailedForcedJobRetriesOnlyOnNewToken(t *testing.T) {
	f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{forceInstallAnnotation: "t1"}},
		Spec:       vyogotechv1.SiteAppSpec{FPMPackage: "frappe/wiki==3.0.1"},
	})
	ctx := context.Background()
	f.reconcile(t)
	f.finishJob(t, false)
	failed := f.job(t)

	for i := 0; i < 2; i++ {
		if _, err := f.r.Reconcile(ctx, f.req); err == nil || !strings.Contains(err.Error(), "JobFailed") {
			t.Fatalf("expected JobFailed, got %v", err)
		}
	}
	if again := f.job(t); again.Status.Failed != 1 || again.ResourceVersion != failed.ResourceVersion {
		t.Fatal("a failed forced Job must not be replaced for the same token")
	}
	if sa := f.siteApp(t); sa.Status.Phase != "Failed" || sa.Status.ForceInstallToken != "" {
		t.Fatalf("expected Failed with no honoured token, got phase=%q token=%q", sa.Status.Phase, sa.Status.ForceInstallToken)
	}

	sa := f.siteApp(t)
	sa.Annotations[forceInstallAnnotation] = "t2"
	if err := f.client.Update(ctx, sa); err != nil {
		t.Fatalf("annotate SiteApp: %v", err)
	}
	f.reconcile(t) // clears the failed Job
	f.reconcile(t) // creates the retry
	if got := f.job(t).Annotations[forceInstallAnnotation]; got != "t2" {
		t.Fatalf("expected a retry Job for the new token, got %q", got)
	}
}

// Forcing needs a package to replace the app with. On a git-source (or
// source-less) SiteApp the annotation is ignored: a settled SiteApp stays
// settled, a new install is built without FORCE_INSTALL, and the reason is
// reported once in a condition.
func TestSiteAppForceInstall_IgnoredWithoutFPMPackage(t *testing.T) {
	t.Run("settled", func(t *testing.T) {
		f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{forceInstallAnnotation: "t1"}},
			Spec:       vyogotechv1.SiteAppSpec{GitRepo: "https://github.com/frappe/wiki"},
			Status:     vyogotechv1.SiteAppStatus{Phase: "Ready"},
		})
		f.reconcile(t)
		f.noJob(t, "git-source SiteApp")
		sa := f.siteApp(t)
		if sa.Status.Phase != "Ready" || sa.Status.ForceInstallToken != "" {
			t.Errorf("expected Ready and no honoured token, got phase=%q token=%q", sa.Status.Phase, sa.Status.ForceInstallToken)
		}
		c := meta.FindStatusCondition(sa.Status.Conditions, "ForceInstall")
		if c == nil || c.Status != metav1.ConditionFalse || c.Reason != "FPMPackageRequired" {
			t.Fatalf("expected ForceInstall=False/FPMPackageRequired, got %+v", c)
		}
		// Reported once, not on every reconcile.
		rv := sa.ResourceVersion
		f.reconcile(t)
		f.reconcile(t)
		if got := f.siteApp(t).ResourceVersion; got != rv {
			t.Errorf("ignored annotation must not rewrite status on every reconcile (%s -> %s)", rv, got)
		}
		if n := len(f.r.Recorder.(*record.FakeRecorder).Events); n != 1 {
			t.Errorf("expected exactly one ForceInstallIgnored event, got %d", n)
		}
	})

	t.Run("first install", func(t *testing.T) {
		f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{forceInstallAnnotation: "t1"}},
			Spec:       vyogotechv1.SiteAppSpec{GitRepo: "https://github.com/frappe/wiki"},
		})
		f.reconcile(t)
		job := f.job(t)
		if _, ok := jobEnvValue(job, "FORCE_INSTALL"); ok {
			t.Error("a git-source install must never be forced")
		}
		if len(job.Annotations) != 0 {
			t.Errorf("a git-source install Job must carry no force token, got %v", job.Annotations)
		}
	})
}

// A forced reinstall happens inside one generation, so it needs a preflight
// backup of its own (the generation's backup already exists and would be
// reused as "done") and its own roll of the serving pods.
func TestSiteAppForceInstall_OwnBackupPerToken(t *testing.T) {
	f := newForceInstallFixture(t, &vyogotechv1.SiteApp{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{forceInstallAnnotation: "t1"}},
		Spec:       vyogotechv1.SiteAppSpec{FPMPackage: "frappe/wiki==3.0.1"},
		Status:     vyogotechv1.SiteAppStatus{Phase: "Ready", InstalledVersion: "frappe/wiki==3.0.1"},
	})
	ctx := context.Background()
	sa := f.siteApp(t)
	sa.Spec.BackupBeforeInstall = nil // default: back up first
	if err := f.client.Update(ctx, sa); err != nil {
		t.Fatalf("update SiteApp: %v", err)
	}

	f.reconcile(t)
	f.noJob(t, "install must wait for the preflight backup")
	want := "app1-pre-g0-f" + forceInstallSuffix("t1")
	if err := f.client.Get(ctx, types.NamespacedName{Name: want, Namespace: "default"}, &vyogotechv1.SiteBackup{}); err != nil {
		t.Fatalf("expected preflight backup %s for the forced reinstall: %v", want, err)
	}
	if forceInstallSuffix("t1") == forceInstallSuffix("t2") {
		t.Fatal("distinct tokens must map to distinct suffixes")
	}
}

// The script side of the switch, as text: the early exits are gated on FORCE,
// FORCE needs an fpm package, and fpm is asked to overwrite.
func TestSiteAppInstallScriptForceInstallGates(t *testing.T) {
	for _, want := range []string{
		`if [ "${FORCE_INSTALL:-}" = "1" ]; then`,
		`if [ "$FORCE" != "1" ] && bench --site "$SITE_NAME" list-apps 2>/dev/null | awk '{print $1}' | grep -qx "$APP_NAME"; then`,
		`if [ "$FORCE" != "1" ] && { [ -z "$FPM_PACKAGE" ] || [ -z "$CURRENT_PKG" ] || [ "$CURRENT_PKG" = "$FPM_PACKAGE" ]; }; then`,
		`[ "$FORCE" = "1" ] && [ "$app" = "$APP_NAME" ] && REPLACE=1`,
		`FPM_INSTALL_FLAGS="--overwrite"`,
		`fpm install "$PKG_SPEC" --bench-path /home/frappe/frappe-bench --site "$SITE_NAME" $FPM_INSTALL_FLAGS`,
	} {
		if !strings.Contains(siteAppInstallScript, want) {
			t.Fatalf("install script lacks %q", want)
		}
	}
	// FORCE must be decided before the first check that consults it.
	if strings.Index(siteAppInstallScript, "\nFORCE=0\n") > strings.Index(siteAppInstallScript, `if [ "$FORCE" != "1" ] && bench`) {
		t.Fatal("FORCE is consulted before it is set")
	}
}

// installScriptSandbox runs siteAppInstallScript for real against a throwaway
// bench directory with stub `bench` and `fpm` commands, so the paths the script
// takes - and what ends up in sites/apps/<app> - are observed, not inferred
// from its text.
//
// The stub fpm behaves like fpm v4.6+: it keeps an app the bench already has
// (apps/<app>/<app>/hooks.py, or one importable from sites/apps) unless
// --overwrite is passed, in which case it replaces apps/<app> with a symlink
// into its store. Every file it ships contains ORIGIN=fpm.
type installScriptSandbox struct {
	t    *testing.T
	root string
}

func newInstallScriptSandbox(t *testing.T) *installScriptSandbox {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &installScriptSandbox{t: t, root: root}
	for _, d := range []string{"apps/frappe", "sites/apps", "sites/assets", "env/bin", "logs", "tmp", "stubs", "store"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.write("installed.txt", "frappe\nwiki\n", 0o644)
	s.write("sites/apps.txt", "frappe\nwiki\n", 0o644)
	// pip / python of the bench env: nothing to report, no wheels to stage.
	s.write("env/bin/pip", "#!/bin/bash\nexit 0\n", 0o755)
	s.write("env/bin/python", "#!/bin/bash\ncat >/dev/null\nexit 0\n", 0o755)
	s.write("stubs/bench", `#!/bin/bash
echo "bench $*" >> "$SANDBOX/calls.log"
case "$*" in
  *list-apps*) cat "$SANDBOX/installed.txt" ;;
esac
exit 0
`, 0o755)
	s.write("stubs/fpm", `#!/bin/bash
echo "fpm $*" >> "$SANDBOX/calls.log"
if [ "$1" = "install" ] && [ "$2" = "--help" ]; then
  [ -n "$FPM_STUB_NO_OVERWRITE" ] || echo "      --overwrite   Replace the app when the bench already has it"
  exit 0
fi
[ "$1" = "install" ] || exit 0
app=wiki
overwrite=0
for a in "$@"; do [ "$a" = "--overwrite" ] && overwrite=1; done
if [ "$overwrite" = "0" ]; then
  if [ -f "$SANDBOX/apps/$app/$app/hooks.py" ] || [ -f "$SANDBOX/sites/apps/$app/$app/hooks.py" ]; then
    echo "App '$app' is already in bench; fetching nothing and keeping the bench's copy."
    exit 0
  fi
fi
store="$SANDBOX/store/$app/3.0.1"
mkdir -p "$store/$app/public"
echo "ORIGIN=fpm" > "$store/$app/hooks.py"
rm -rf "$SANDBOX/apps/$app"
ln -s "$store" "$SANDBOX/apps/$app"
exit 0
`, 0o755)
	return s
}

func (s *installScriptSandbox) write(rel, content string, mode os.FileMode) {
	s.t.Helper()
	p := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		s.t.Fatal(err)
	}
}

func (s *installScriptSandbox) read(rel string) string {
	b, err := os.ReadFile(filepath.Join(s.root, rel))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// staleCopy puts an app copy on the "sites volume" the way a manual copy
// would: code, but whatever marker the caller chooses (none = unmarked).
func (s *installScriptSandbox) staleCopy(marker string) {
	s.write("sites/apps/wiki/wiki/hooks.py", "ORIGIN=stale\n", 0o644)
	if marker != "" {
		s.write("sites/apps/wiki/.fpm_package", marker+"\n", 0o644)
	}
}

func (s *installScriptSandbox) run(env ...string) (string, error) {
	s.t.Helper()
	script := strings.ReplaceAll(siteAppInstallScript, "/home/frappe/frappe-bench", s.root)
	script = strings.ReplaceAll(script, "/tmp", s.root+"/tmp")
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = s.root
	cmd.Env = append([]string{
		"PATH=" + filepath.Join(s.root, "stubs") + ":" + os.Getenv("PATH"),
		"SANDBOX=" + s.root,
		"APP_NAME=wiki",
		"SITE_NAME=site1.example.com",
		"AUTO_MIGRATE=true",
	}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *installScriptSandbox) assertReplacedByFpm(out string) {
	s.t.Helper()
	if got := s.read("sites/apps/wiki/wiki/hooks.py"); got != "ORIGIN=fpm" {
		s.t.Fatalf("sites/apps/wiki must hold the fpm package's code, got %q\n%s", got, out)
	}
	if got := s.read("sites/apps/wiki/.fpm_package"); got != "frappe/wiki==3.0.1" {
		s.t.Fatalf(".fpm_package marker = %q, want the requested package\n%s", got, out)
	}
	calls := s.read("calls.log")
	if !strings.Contains(calls, "fpm install frappe/wiki==3.0.1") || !strings.Contains(calls, "--overwrite") {
		s.t.Fatalf("expected `fpm install ... --overwrite`, calls:\n%s", calls)
	}
	if !strings.Contains(calls, "bench --site site1.example.com migrate") {
		s.t.Fatalf("autoMigrate must still run after a forced reinstall, calls:\n%s", calls)
	}
	leftovers, _ := filepath.Glob(filepath.Join(s.root, "sites/apps/.wiki.replaced.*"))
	tmps, _ := filepath.Glob(filepath.Join(s.root, "sites/apps/wiki.tmp.*"))
	if len(leftovers)+len(tmps) != 0 {
		s.t.Fatalf("forced reinstall left scratch dirs behind: %v %v", leftovers, tmps)
	}
}

// The bug, reproduced: an unmarked copy on the volume plus the app in
// list-apps makes a normal run exit "nothing to do" and leave the stale code.
func TestSiteAppInstallScriptRun_UnmarkedCopyIsKeptWithoutForce(t *testing.T) {
	s := newInstallScriptSandbox(t)
	s.staleCopy("")
	out, err := s.run("FPM_PACKAGE=frappe/wiki==3.0.1")
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already installed on site1.example.com (verified via list-apps); nothing to do.") {
		t.Fatalf("expected the already-installed early exit, got:\n%s", out)
	}
	if got := s.read("sites/apps/wiki/wiki/hooks.py"); got != "ORIGIN=stale" {
		t.Fatalf("without force the copy on the volume must be untouched, got %q", got)
	}
	if strings.Contains(s.read("calls.log"), "fpm install") {
		t.Fatal("without force fpm must not be invoked for an already-installed app")
	}
}

// FORCE_INSTALL=1 bypasses that exit (and the shared-bench fast path) and ends
// with the requested package in sites/apps/<app>, marker written, migrate run.
func TestSiteAppInstallScriptRun_ForceReplacesUnmarkedCopy(t *testing.T) {
	s := newInstallScriptSandbox(t)
	s.staleCopy("")
	out, err := s.run("FPM_PACKAGE=frappe/wiki==3.0.1", "FORCE_INSTALL=1")
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "nothing to do") || strings.Contains(out, "site-level install only") {
		t.Fatalf("forced run took an early exit:\n%s", out)
	}
	s.assertReplacedByFpm(out)
}

// The image ships the app in apps/<app> as well. Without --overwrite fpm keeps
// that directory and the relocate step would copy the IMAGE's code onto the
// volume under the package's marker; the forced path must end with fpm's.
func TestSiteAppInstallScriptRun_ForceWhenImageAlsoShipsTheApp(t *testing.T) {
	s := newInstallScriptSandbox(t)
	s.staleCopy("")
	s.write("apps/wiki/wiki/hooks.py", "ORIGIN=image\n", 0o644)
	out, err := s.run("FPM_PACKAGE=frappe/wiki==3.0.1", "FORCE_INSTALL=1")
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	s.assertReplacedByFpm(out)
}

// A marker that already names the requested package does not stop a forced
// run: the marker can be right while the files are not.
func TestSiteAppInstallScriptRun_ForceReplacesEvenWhenMarkerMatches(t *testing.T) {
	s := newInstallScriptSandbox(t)
	s.staleCopy("frappe/wiki==3.0.1")
	out, err := s.run("FPM_PACKAGE=frappe/wiki==3.0.1", "FORCE_INSTALL=1")
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	s.assertReplacedByFpm(out)
}

// Defence in depth: FORCE_INSTALL without an fpm package changes nothing.
func TestSiteAppInstallScriptRun_ForceIgnoredWithoutFPMPackage(t *testing.T) {
	s := newInstallScriptSandbox(t)
	s.staleCopy("")
	out, err := s.run("FORCE_INSTALL=1")
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "FORCE_INSTALL is set but there is no FPM package to install; ignoring it.") ||
		!strings.Contains(out, "nothing to do.") {
		t.Fatalf("expected FORCE_INSTALL to be ignored and the normal early exit taken, got:\n%s", out)
	}
	if got := s.read("sites/apps/wiki/wiki/hooks.py"); got != "ORIGIN=stale" {
		t.Fatalf("copy on the volume must be untouched, got %q", got)
	}
}

// An fpm too old to overwrite must fail the Job loudly and leave the volume
// alone, rather than silently keeping the stale copy and stamping a marker.
func TestSiteAppInstallScriptRun_ForceFailsOnFpmWithoutOverwrite(t *testing.T) {
	s := newInstallScriptSandbox(t)
	s.staleCopy("")
	out, err := s.run("FPM_PACKAGE=frappe/wiki==3.0.1", "FORCE_INSTALL=1", "FPM_STUB_NO_OVERWRITE=1")
	if err == nil {
		t.Fatalf("expected the script to fail, got:\n%s", out)
	}
	if !strings.Contains(out, "a forced reinstall needs fpm v4.6.0 or newer") {
		t.Fatalf("expected a clear error, got:\n%s", out)
	}
	if got := s.read("sites/apps/wiki/wiki/hooks.py"); got != "ORIGIN=stale" {
		t.Fatalf("copy on the volume must be untouched on failure, got %q", got)
	}
	if s.read("sites/apps/wiki/.fpm_package") != "" {
		t.Fatal("no marker may be written when the forced reinstall did not happen")
	}
}
