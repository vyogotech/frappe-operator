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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

const (
	siteAppFinalizer = "vyogo.tech/siteapp-finalizer"
)

// SiteAppReconciler reconciles a SiteApp object
type SiteAppReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	FrappeClient *FrappeClient // Optional injected client for testing
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=siteapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=siteapps/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitebackups,verbs=get;list;watch;create
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch

// benchServingComponents are the bench deployment components that run Python
// application code and therefore load installed apps into a long-lived process.
// nginx (reverse proxy) and socketio (Node) don't import Frappe apps, so they
// don't need to roll when an app is added or removed.
var benchServingComponents = map[string]bool{
	"gunicorn":       true,
	"scheduler":      true,
	"worker-default": true,
	"worker-short":   true,
	"worker-long":    true,
}

// reloadBenchServingPods rolls the bench's Python serving deployments so a
// newly installed app on the shared sites PVC is picked up by the running
// processes. A long-running Python interpreter caches its sys.path finders, so
// an app added to sites/apps *after* the process started is invisible to it
// until a restart — the site then 500s with "ModuleNotFoundError: No module
// named '<app>'" on every request. install-app succeeding on the PVC is not
// enough; the gunicorn/worker/scheduler processes must be rolled.
//
// The roll is triggered by stamping a per-app annotation on each deployment's
// pod template. The value is stable for a given SiteApp generation, so repeated
// reconciles of the same app re-write the same value (a no-op that Kubernetes
// ignores — no restart loop), while a new app uses a distinct annotation key and
// rolls exactly once. Using a per-app key (rather than one shared key) avoids
// two apps ping-ponging the same annotation between their own values.
func (r *SiteAppReconciler) reloadBenchServingPods(ctx context.Context, benchName, benchNamespace, appName, reloadValue string) {
	logger := log.FromContext(ctx)
	var deploys appsv1.DeploymentList
	if err := r.List(ctx, &deploys, client.InNamespace(benchNamespace), client.MatchingLabels{"bench": benchName}); err != nil {
		logger.Error(err, "reload: failed to list bench deployments", "bench", benchName)
		return
	}
	annKey := "vyogo.tech/reload-" + appName
	for i := range deploys.Items {
		d := &deploys.Items[i]
		// The "component" label lives on the pod template / selector, not on the
		// Deployment's own metadata (which only carries app + bench), so read it
		// from the template.
		if !benchServingComponents[d.Spec.Template.Labels["component"]] {
			continue
		}
		if d.Spec.Template.Annotations[annKey] == reloadValue {
			continue // already rolled for this app install
		}
		patched := d.DeepCopy()
		if patched.Spec.Template.Annotations == nil {
			patched.Spec.Template.Annotations = map[string]string{}
		}
		patched.Spec.Template.Annotations[annKey] = reloadValue
		if err := r.Patch(ctx, patched, client.MergeFrom(d)); err != nil {
			logger.Error(err, "reload: failed to patch deployment", "deployment", d.Name)
			continue
		}
		logger.Info("reload: rolled bench serving deployment to pick up app", "deployment", d.Name, "app", appName)
	}
}

func (r *SiteAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	siteApp := &vyogotechv1.SiteApp{}
	if err := r.Get(ctx, req.NamespacedName, siteApp); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if siteApp.Spec.SiteRef == nil || siteApp.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteApp, "siteRef is required", "ValidationFailed")
	}

	if siteApp.Spec.AppName == "" {
		return r.failReconciliation(ctx, siteApp, "appName is required", "ValidationFailed")
	}

	siteNamespace := siteApp.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteApp.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteApp.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteApp.Status.Phase = "Pending"
		r.setCondition(siteApp, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteApp)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Handle Deletion & Finalizer
	if siteApp.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(siteApp, siteAppFinalizer) {
			// Actually uninstall the app from the site before dropping the CR.
			// Just removing the finalizer would delete the console card but leave
			// the app installed on the shared PVC — a bad app then keeps 500ing the
			// desk. Run a bench uninstall-app Job and wait for it to complete.
			res, removable, err := r.reconcileAppUninstallJob(ctx, siteApp, site)
			if err != nil {
				return res, err
			}
			if !removable {
				// Job still running (or just created) — requeue without removing
				// the finalizer so the CR sticks around until uninstall finishes.
				return res, nil
			}

			latest := &vyogotechv1.SiteApp{}
			if err := r.Get(ctx, req.NamespacedName, latest); err == nil {
				controllerutil.RemoveFinalizer(latest, siteAppFinalizer)
				if err := r.Update(ctx, latest); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !controllerutil.ContainsFinalizer(siteApp, siteAppFinalizer) {
		controllerutil.AddFinalizer(siteApp, siteAppFinalizer)
		if err := r.Update(ctx, siteApp); err != nil {
			return ctrl.Result{}, err
		}
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteApp.Status.Phase = "Pending"
		r.setCondition(siteApp, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteApp)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	r.setCondition(siteApp, metav1.Condition{
		Type:    "SiteReady",
		Status:  metav1.ConditionTrue,
		Reason:  "SiteReady",
		Message: "Referenced FrappeSite is ready",
	})

	// Check if app is already installed on site
	alreadyInstalled := false
	for _, app := range site.Spec.Apps {
		if app == siteApp.Spec.AppName {
			alreadyInstalled = true
			break
		}
	}

	if alreadyInstalled {
		siteApp.Status.Phase = "Ready"
		siteApp.Status.ObservedGeneration = siteApp.Generation
		r.setCondition(siteApp, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AppInstalled",
			Message: fmt.Sprintf("App %s is already installed on site", siteApp.Spec.AppName),
		})
		_ = r.updateStatus(ctx, siteApp)
		return ctrl.Result{}, nil
	}

	// If a test client is injected, use it directly
	if r.FrappeClient != nil {
		siteApp.Status.Phase = "Installing"
		_ = r.updateStatus(ctx, siteApp)
		if err := r.FrappeClient.InstallApp(ctx, siteApp.Spec.AppName); err != nil {
			return r.failReconciliation(ctx, siteApp, fmt.Sprintf("Failed to install app in Frappe: %v", err), "AppInstallFailed")
		}
		siteApp.Status.Phase = "Ready"
		siteApp.Status.ObservedGeneration = siteApp.Generation
		r.setCondition(siteApp, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AppInstalled",
			Message: fmt.Sprintf("App %s successfully installed on site", siteApp.Spec.AppName),
		})
		_ = r.updateStatus(ctx, siteApp)
		return ctrl.Result{}, nil
	}

	// Reconcile Install K8s Job (handles both pre-existing bench apps and dynamic GitHub gitRepo fetching)
	return r.reconcileAppInstallJob(ctx, siteApp, site)
}

func (r *SiteAppReconciler) reconcileAppInstallJob(ctx context.Context, siteApp *vyogotechv1.SiteApp, site *vyogotechv1.FrappeSite) (ctrl.Result, error) {
	jobName := fmt.Sprintf("%s-app-install", siteApp.Name)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: siteApp.Namespace}, job)

	if errors.IsNotFound(err) {
		// Rollback safety: before installing/upgrading the app, take a full backup
		// and wait for it to succeed. The install job is not created until the
		// preflight backup is done, so a corrupt install can always be reverted.
		if siteApp.Spec.BackupBeforeInstall {
			backupName := fmt.Sprintf("%s-pre-g%d", siteApp.Name, siteApp.Generation)
			done, berr := ensurePreflightBackup(ctx, r.Client, siteApp.Namespace, site.Spec.SiteName, backupName)
			if berr != nil {
				return r.failReconciliation(ctx, siteApp, fmt.Sprintf("Pre-install backup failed: %v", berr), "PreBackupFailed")
			}
			if !done {
				siteApp.Status.Phase = "BackingUp"
				siteApp.Status.PreBackupRef = backupName
				r.setCondition(siteApp, metav1.Condition{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "BackingUp",
					Message: fmt.Sprintf("Taking pre-install backup %s before installing %s", backupName, siteApp.Spec.AppName),
				})
				_ = r.updateStatus(ctx, siteApp)
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			siteApp.Status.PreBackupRef = backupName
		}

		bench := &vyogotechv1.FrappeBench{}
		benchName := site.Spec.BenchRef.Name
		benchNamespace := site.Spec.BenchRef.Namespace
		if benchNamespace == "" {
			benchNamespace = site.Namespace
		}
		if err := r.Get(ctx, types.NamespacedName{Name: benchName, Namespace: benchNamespace}, bench); err != nil {
			return r.failReconciliation(ctx, siteApp, fmt.Sprintf("Failed to fetch referenced bench %s: %v", benchName, err), "BenchNotFound")
		}

		image := resolveBenchImage(bench)
		pvcName := fmt.Sprintf("%s-sites", bench.Name)

		script := `#!/bin/bash
set -e

mkdir -p /home/frappe/frappe-bench/logs 2>/dev/null || true
touch /tmp/bench.log 2>/dev/null || true
ln -sf /tmp/bench.log /home/frappe/frappe-bench/logs/bench.log 2>/dev/null || true

export HOME=/tmp
export PYTHONPATH="/tmp/pip:/home/frappe/frappe-bench/sites/apps:$PYTHONPATH"

# The runtime-apps dir must exist before we write into it. It is created here
# (not only inside the GIT_REPO branch) so the sitecustomize write never fails
# under 'set -e' when no git source is provided.
mkdir -p /home/frappe/frappe-bench/sites/apps 2>/dev/null || true

# Ensure sitecustomize.py exists to dynamically load all installed apps into sys.path
cat << "EOF" > /home/frappe/frappe-bench/sites/apps/sitecustomize.py
import sys, os
apps_dir = "/home/frappe/frappe-bench/sites/apps"
ignored = {"frappe", "erpnext", "custom_demo_app", "__pycache__"}
if os.path.exists(apps_dir):
    for entry in os.listdir(apps_dir):
        if entry.startswith(".") or entry in ignored:
            continue
        full_path = os.path.join(apps_dir, entry)
        if os.path.isdir(full_path) and full_path not in sys.path:
            sys.path.append(full_path)
EOF

cd /home/frappe/frappe-bench/sites

# Branch for frappe-official dependencies, derived from the bench's Frappe
# version (e.g. "15.0.0" -> "version-15"); falls back to the repo default.
DEP_BRANCH=""
if [ -n "$FRAPPE_VERSION" ]; then
  FV_MAJOR=$(echo "$FRAPPE_VERSION" | grep -oE '[0-9]+' | head -1)
  [ -n "$FV_MAJOR" ] && DEP_BRANCH="version-$FV_MAJOR"
fi

# clone_app <name> <git_repo> <git_branch> — clone + link + frontend build +
# pip deps + register in apps.txt/apps.pth. No-op if repo is empty.
clone_app() {
  local name="$1" repo="$2" branch="$3"
  [ -z "$repo" ] && return 0
  mkdir -p apps
  if [ ! -d "apps/$name" ]; then
    echo "Cloning $name from $repo (branch: ${branch:-default})..."
    if [ -n "$branch" ]; then
      git clone --depth 1 -b "$branch" "$repo" "apps/$name" 2>/dev/null || git clone --depth 1 "$repo" "apps/$name" || true
    else
      git clone --depth 1 "$repo" "apps/$name" || true
    fi
  fi
  ln -sf $(pwd)/apps/$name /home/frappe/frappe-bench/apps/$name 2>/dev/null || true
  if [ -d "apps/$name/$name/public" ]; then
    ln -sf $(pwd)/apps/$name/$name/public /home/frappe/frappe-bench/sites/assets/$name 2>/dev/null || true
  fi
  if [ -f "apps/$name/frontend/package.json" ] && [ ! -d "apps/$name/frontend/dist" ]; then
    echo "Building frontend assets for $name..."
    (cd "apps/$name/frontend" && (yarn build 2>/dev/null || npm run build 2>/dev/null || true))
  fi
  if [ -d "apps/$name" ]; then
    echo "Installing Python dependencies for $name..."
    pip install --target /tmp/pip $(pwd)/apps/$name 2>/dev/null || true
  fi
  rm -rf /tmp/pip/click /tmp/pip/click-*.dist-info 2>/dev/null || true
  if [ -f apps.txt ] && [ -w apps.txt ]; then
    grep -q "^$name$" apps.txt 2>/dev/null || echo "$name" >> apps.txt
  fi
  grep -q "^/home/frappe/frappe-bench/sites/apps/$name$" apps.pth 2>/dev/null || echo "/home/frappe/frappe-bench/sites/apps/$name" >> apps.pth
}

# read_required_apps <name> — print the app's required_apps (dependency app
# names) declared in its hooks.py, space-separated.
read_required_apps() {
  local hooks="apps/$1/$1/hooks.py"
  [ -f "$hooks" ] || return 0
  python3 - "$hooks" <<'PY' 2>/dev/null
import re, sys
c = open(sys.argv[1]).read()
m = re.search(r'required_apps\s*=\s*\[(.*?)\]', c, re.S)
if m:
    print(' '.join(re.findall(r'["\']([\w_]+)["\']', m.group(1))))
PY
}

# 1) Clone the target app (needed to read its dependency list).
clone_app "$APP_NAME" "$GIT_REPO" "$GIT_BRANCH"

# 2) Auto-install required_apps (dependencies) FIRST, in declared order, before
# the target app — e.g. Frappe HRMS declares required_apps=["erpnext"], so
# erpnext is installed first. frappe-official deps are cloned from
# github.com/frappe/<dep> at the bench's version branch.
INSTALLED=$(bench --site "$SITE_NAME" list-apps 2>/dev/null | awk '{print $1}')
for dep in $(read_required_apps "$APP_NAME"); do
  [ "$dep" = "frappe" ] && continue
  if echo "$INSTALLED" | grep -qx "$dep"; then
    echo "Dependency '$dep' already installed on $SITE_NAME — skipping."
    continue
  fi
  echo "Auto-installing dependency '$dep' (required by $APP_NAME)..."
  clone_app "$dep" "https://github.com/frappe/$dep" "$DEP_BRANCH"
  cd /home/frappe/frappe-bench
  bench --site "$SITE_NAME" install-app "$dep" --force
  # Rebuild the module→app map so the target app's doctypes (which may live in a
  # module the dependency just added, e.g. hrms's "HR") resolve correctly on the
  # first attempt instead of erroring "No module named frappe.core.doctype.*".
  bench --site "$SITE_NAME" clear-cache 2>/dev/null || true
  cd /home/frappe/frappe-bench/sites
done

# 3) Install the target app.
cd /home/frappe/frappe-bench
echo "Installing app $APP_NAME on site $SITE_NAME..."
# Do NOT swallow install failures: the SiteApp status must reflect reality.
bench --site "$SITE_NAME" install-app "$APP_NAME" --force

echo "Building assets for $APP_NAME..."
bench build --app "$APP_NAME" 2>/dev/null || true

echo "Clearing site cache..."
bench --site "$SITE_NAME" clear-cache 2>/dev/null || true

# Post-install health probe: a bad app (e.g. one incompatible with the bench's
# Frappe version) can leave the site unbootable so every desk request 500s. Do a
# cheap boot of the site and enumerate installed apps; if this errors the site is
# broken, so exit non-zero to fail the Job. The controller then marks the SiteApp
# Failed instead of Ready, surfacing the breakage instead of hiding it. No
# '|| true' here on purpose — its failure must fail the Job.
echo "Verifying site health after install..."
bench --site "$SITE_NAME" execute frappe.get_installed_apps
`

		env := []corev1.EnvVar{
			{Name: "APP_NAME", Value: siteApp.Spec.AppName},
			{Name: "SITE_NAME", Value: site.Spec.SiteName},
			{Name: "GIT_REPO", Value: siteApp.Spec.GitRepo},
			{Name: "GIT_BRANCH", Value: siteApp.Spec.GitBranch},
			{Name: "FRAPPE_VERSION", Value: bench.Spec.FrappeVersion},
			{Name: "USER", Value: "frappe"},
		}

		newJob := r.buildAppJob(siteApp, jobName, "app-installer", image, pvcName, script, env, int32(1), nil)

		// The install job is owned by the SiteApp so it is garbage-collected with
		// it. (The uninstall job cannot be — see reconcileAppUninstallJob.)
		if err := controllerutil.SetControllerReference(siteApp, newJob, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, newJob); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create install job: %w", err)
		}

		siteApp.Status.Phase = "Installing"
		_ = r.updateStatus(ctx, siteApp)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if job.Status.Succeeded > 0 {
		// The app now exists on the shared PVC, but the already-running
		// gunicorn/worker/scheduler processes won't see it until they restart
		// (Python caches sys.path finders). Roll them so the site doesn't 500
		// with ModuleNotFoundError. Idempotent: the value is stable per SiteApp
		// generation, so this only rolls once per install/upgrade.
		benchName := site.Spec.BenchRef.Name
		benchNamespace := site.Spec.BenchRef.Namespace
		if benchNamespace == "" {
			benchNamespace = site.Namespace
		}
		reloadValue := fmt.Sprintf("gen-%d", siteApp.Generation)
		r.reloadBenchServingPods(ctx, benchName, benchNamespace, siteApp.Spec.AppName, reloadValue)

		siteApp.Status.Phase = "Ready"
		siteApp.Status.ObservedGeneration = siteApp.Generation
		r.setCondition(siteApp, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AppInstalled",
			Message: fmt.Sprintf("App %s installed on site", siteApp.Spec.AppName),
		})
		_ = r.updateStatus(ctx, siteApp)
		return ctrl.Result{}, nil
	}

	if job.Status.Failed > 0 {
		return r.failReconciliation(ctx, siteApp, fmt.Sprintf("App installation job %s failed", jobName), "JobFailed")
	}

	siteApp.Status.Phase = "Installing"
	_ = r.updateStatus(ctx, siteApp)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// resolveBenchImage derives the bench container image used for app install /
// uninstall Jobs from the bench's ImageConfig, falling back to the default
// operator image.
func resolveBenchImage(bench *vyogotechv1.FrappeBench) string {
	image := "ghcr.io/vyogotech/erpnext-for-operator:version-15"
	if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
		tag := bench.Spec.ImageConfig.Tag
		if tag == "" {
			tag = "latest"
		}
		image = fmt.Sprintf("%s:%s", bench.Spec.ImageConfig.Repository, tag)
	}
	return image
}

// buildAppJob assembles the Job that runs a bench command against the site on
// the bench's shared sites PVC. It is the single source of truth for the pod
// shape (image, sites PVC mount at frappe-sites subPath, security context,
// RestartPolicy Never) shared by the install and uninstall paths. The caller
// sets the owner reference (or deliberately does not — see the uninstall path).
func (r *SiteAppReconciler) buildAppJob(siteApp *vyogotechv1.SiteApp, jobName, containerName, image, pvcName, script string, env []corev1.EnvVar, backoffLimit int32, ttlSecondsAfterFinished *int32) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: siteApp.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "frappe-operator",
				"vyogo.tech/siteapp":           siteApp.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "frappe-operator",
						"vyogo.tech/siteapp":           siteApp.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  ptr.To(int64(1000)),
						RunAsGroup: ptr.To(int64(1000)),
						FSGroup:    ptr.To(int64(1000)),
					},
					Containers: []corev1.Container{
						{
							Name:            containerName,
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"bash", "-c", script},
							Env:             env,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sites",
									MountPath: "/home/frappe/frappe-bench/sites",
									SubPath:   "frappe-sites",
								},
								{
									Name:      "bench-logs",
									MountPath: "/home/frappe/frappe-bench/logs",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sites",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
						{
							Name: "bench-logs",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
}

// reconcileAppUninstallJob drives a bench uninstall-app Job to completion during
// SiteApp deletion. It returns (result, removable, err):
//   - removable=false: the uninstall is still in flight (or was just created);
//     the caller must requeue with result and keep the finalizer.
//   - removable=true: the uninstall finished (success) or was definitively given
//     up on (Job failed / bench gone); the caller may remove the finalizer.
//
// On a successful uninstall the bench's Python serving deployments are rolled so
// the desk drops the now-removed app from memory (gunicorn/scheduler/workers
// cache the installed-apps list — see reloadBenchServingPods). That roll is
// best-effort and never blocks finalizer removal.
func (r *SiteAppReconciler) reconcileAppUninstallJob(ctx context.Context, siteApp *vyogotechv1.SiteApp, site *vyogotechv1.FrappeSite) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)
	jobName := fmt.Sprintf("%s-app-uninstall", siteApp.Name)

	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: siteApp.Namespace}, job)

	if errors.IsNotFound(err) {
		bench := &vyogotechv1.FrappeBench{}
		benchName := ""
		benchNamespace := site.Namespace
		if site.Spec.BenchRef != nil {
			benchName = site.Spec.BenchRef.Name
			if site.Spec.BenchRef.Namespace != "" {
				benchNamespace = site.Spec.BenchRef.Namespace
			}
		}
		if benchName == "" {
			// No bench to uninstall from — nothing on the PVC to clean up. Let the
			// deletion proceed rather than wedging the resource forever.
			logger.Info("Uninstall: site has no benchRef, skipping uninstall job", "app", siteApp.Spec.AppName)
			r.Recorder.Event(siteApp, corev1.EventTypeWarning, "UninstallSkipped", "Site has no benchRef; app was not actively uninstalled")
			return ctrl.Result{}, true, nil
		}
		if err := r.Get(ctx, types.NamespacedName{Name: benchName, Namespace: benchNamespace}, bench); err != nil {
			// Bench gone means its DB/pods/PVC are gone too, so the app is already
			// effectively uninstalled. Allow deletion to complete.
			logger.Info("Uninstall: referenced bench not found, treating app as already removed", "bench", benchName, "app", siteApp.Spec.AppName)
			r.Recorder.Eventf(siteApp, corev1.EventTypeWarning, "UninstallSkipped", "Referenced bench %s not found; app was not actively uninstalled", benchName)
			return ctrl.Result{}, true, nil
		}

		image := resolveBenchImage(bench)
		pvcName := fmt.Sprintf("%s-sites", bench.Name)

		script := `#!/bin/bash
set -e

export HOME=/tmp
export PYTHONPATH="/tmp/pip:/home/frappe/frappe-bench/sites/apps:$PYTHONPATH"

cd /home/frappe/frappe-bench

echo "Uninstalling app $APP_NAME from site $SITE_NAME..."
# --yes bypasses the interactive confirmation prompt; --force removes the app
# even if other installed apps declare it as a dependency (dependents are the
# caller's problem, not a reason to leave a broken app wedged on the site).
bench --site "$SITE_NAME" uninstall-app "$APP_NAME" --yes --force

echo "Clearing site cache..."
bench --site "$SITE_NAME" clear-cache 2>/dev/null || true
`

		env := []corev1.EnvVar{
			{Name: "APP_NAME", Value: siteApp.Spec.AppName},
			{Name: "SITE_NAME", Value: site.Spec.SiteName},
			{Name: "USER", Value: "frappe"},
		}

		// NOTE: no owner reference. The SiteApp is mid-deletion (DeletionTimestamp
		// set), and the API server rejects creating a child with a
		// blockOwnerDeletion owner reference to an object being deleted. Instead the
		// Job self-cleans via TTLSecondsAfterFinished once it finishes.
		newJob := r.buildAppJob(siteApp, jobName, "app-uninstaller", image, pvcName, script, env, int32(2), ptr.To(int32(300)))

		if err := r.Create(ctx, newJob); err != nil {
			return ctrl.Result{}, false, fmt.Errorf("failed to create uninstall job: %w", err)
		}

		logger.Info("Created uninstall job", "job", jobName, "app", siteApp.Spec.AppName, "site", site.Name)
		siteApp.Status.Phase = "Uninstalling"
		_ = r.updateStatus(ctx, siteApp)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
	} else if err != nil {
		return ctrl.Result{}, false, err
	}

	if job.Status.Succeeded > 0 {
		// App is gone from the PVC, but the running gunicorn/worker/scheduler
		// processes still hold it in memory. Roll them so the desk recovers.
		// Best-effort: a failed patch must not wedge the finalizer.
		benchName := ""
		benchNamespace := site.Namespace
		if site.Spec.BenchRef != nil {
			benchName = site.Spec.BenchRef.Name
			if site.Spec.BenchRef.Namespace != "" {
				benchNamespace = site.Spec.BenchRef.Namespace
			}
		}
		if benchName != "" {
			reloadValue := fmt.Sprintf("uninstall-gen-%d", siteApp.Generation)
			r.reloadBenchServingPods(ctx, benchName, benchNamespace, siteApp.Spec.AppName, reloadValue)
		}
		logger.Info("Uninstall job succeeded", "job", jobName, "app", siteApp.Spec.AppName)
		return ctrl.Result{}, true, nil
	}

	if job.Status.Failed > 0 {
		// Bounded give-up: the Job exhausted its backoffLimit. Surface the failure
		// loudly (status + event) but still let the CR be deleted so the user isn't
		// left with an un-removable card.
		msg := fmt.Sprintf("Uninstall job %s failed; app %s may still be installed on the site", jobName, siteApp.Spec.AppName)
		logger.Error(fmt.Errorf("uninstall job failed"), msg)
		siteApp.Status.Phase = "Failed"
		r.setCondition(siteApp, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "UninstallFailed",
			Message: msg,
		})
		_ = r.updateStatus(ctx, siteApp)
		r.Recorder.Event(siteApp, corev1.EventTypeWarning, "UninstallFailed", msg)
		return ctrl.Result{}, true, nil
	}

	// Job still running.
	siteApp.Status.Phase = "Uninstalling"
	_ = r.updateStatus(ctx, siteApp)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
}

func (r *SiteAppReconciler) updateStatus(ctx context.Context, siteApp *vyogotechv1.SiteApp) error {
	return r.Status().Update(ctx, siteApp)
}

func (r *SiteAppReconciler) setCondition(siteApp *vyogotechv1.SiteApp, condition metav1.Condition) {
	condition.ObservedGeneration = siteApp.Generation
	meta.SetStatusCondition(&siteApp.Status.Conditions, condition)
}

func (r *SiteAppReconciler) failReconciliation(ctx context.Context, siteApp *vyogotechv1.SiteApp, msg, reason string) (ctrl.Result, error) {
	siteApp.Status.Phase = "Failed"
	r.setCondition(siteApp, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	_ = r.updateStatus(ctx, siteApp)
	return ctrl.Result{}, fmt.Errorf("%s: %s", reason, msg)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteApp{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
