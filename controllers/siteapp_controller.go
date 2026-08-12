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
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *SiteAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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
			logger.Info("Uninstalling site app", "app", siteApp.Spec.AppName, "site", site.Name)
			siteApp.Status.Phase = "Uninstalling"
			_ = r.updateStatus(ctx, siteApp)

			if r.FrappeClient != nil {
				_ = r.FrappeClient.UninstallApp(ctx, siteApp.Spec.AppName)
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
		bench := &vyogotechv1.FrappeBench{}
		benchName := site.Spec.BenchRef.Name
		benchNamespace := site.Spec.BenchRef.Namespace
		if benchNamespace == "" {
			benchNamespace = site.Namespace
		}
		if err := r.Get(ctx, types.NamespacedName{Name: benchName, Namespace: benchNamespace}, bench); err != nil {
			return r.failReconciliation(ctx, siteApp, fmt.Sprintf("Failed to fetch referenced bench %s: %v", benchName, err), "BenchNotFound")
		}

		image := "ghcr.io/vyogotech/erpnext-for-operator:version-15"
		if bench.Spec.ImageConfig != nil && bench.Spec.ImageConfig.Repository != "" {
			tag := bench.Spec.ImageConfig.Tag
			if tag == "" {
				tag = "latest"
			}
			image = fmt.Sprintf("%s:%s", bench.Spec.ImageConfig.Repository, tag)
		}

		pvcName := fmt.Sprintf("%s-sites", bench.Name)
		backoffLimit := int32(1)

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

if [ -n "$GIT_REPO" ]; then
  mkdir -p apps
  if [ ! -d "apps/$APP_NAME" ]; then
    echo "Cloning $APP_NAME from $GIT_REPO..."
    git clone --depth 1 ${GIT_BRANCH:+-b $GIT_BRANCH} "$GIT_REPO" "apps/$APP_NAME" || true
  fi
  ln -sf $(pwd)/apps/$APP_NAME /home/frappe/frappe-bench/apps/$APP_NAME 2>/dev/null || true
  if [ -d "apps/$APP_NAME/$APP_NAME/public" ]; then
    echo "Creating asset symlink for $APP_NAME..."
    ln -sf $(pwd)/apps/$APP_NAME/$APP_NAME/public /home/frappe/frappe-bench/sites/assets/$APP_NAME 2>/dev/null || true
  fi
  # Only build frontend if dist/ doesn't already exist (skip in prod - pre-built assets expected)
  if [ -f "apps/$APP_NAME/frontend/package.json" ] && [ ! -d "apps/$APP_NAME/frontend/dist" ]; then
    echo "Building frontend assets for $APP_NAME (no pre-built dist found)..."
    (cd "apps/$APP_NAME/frontend" && (yarn build 2>/dev/null || npm run build 2>/dev/null || true))
  elif [ -f "apps/$APP_NAME/frontend/package.json" ] && [ -d "apps/$APP_NAME/frontend/dist" ]; then
    echo "Skipping frontend build for $APP_NAME (pre-built dist already exists)."
  fi
  if [ -d "apps/$APP_NAME" ]; then
    echo "Installing Python dependencies for $APP_NAME..."
    pip install --target /tmp/pip $(pwd)/apps/$APP_NAME 2>/dev/null || true
    rm -rf /tmp/pip/click /tmp/pip/click-*.dist-info 2>/dev/null || true
  fi
fi

if [ -f apps.txt ] && [ -w apps.txt ]; then
  if ! grep -q "^$APP_NAME$" apps.txt 2>/dev/null; then
    echo "$APP_NAME" >> apps.txt || true
  fi
fi

if ! grep -q "^/home/frappe/frappe-bench/sites/apps/$APP_NAME$" apps.pth 2>/dev/null; then
  echo "/home/frappe/frappe-bench/sites/apps/$APP_NAME" >> apps.pth || true
fi

cd /home/frappe/frappe-bench
echo "Installing app $APP_NAME on site $SITE_NAME..."
# Do NOT swallow install failures: if the app is not in the bench and no git
# source was provided (or the install genuinely fails), the job must fail so the
# SiteApp status reflects reality instead of a false success.
bench --site "$SITE_NAME" install-app "$APP_NAME" --force

echo "Building assets for $APP_NAME..."
bench build --app "$APP_NAME" 2>/dev/null || true

echo "Clearing site cache..."
bench --site "$SITE_NAME" clear-cache 2>/dev/null || true
`

		newJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: siteApp.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "frappe-operator",
					"vyogo.tech/siteapp":           siteApp.Name,
				},
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: &backoffLimit,
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
								Name:            "app-installer",
								Image:           image,
								ImagePullPolicy: corev1.PullIfNotPresent,
								Command:         []string{"bash", "-c", script},
								Env: []corev1.EnvVar{
									{Name: "APP_NAME", Value: siteApp.Spec.AppName},
									{Name: "SITE_NAME", Value: site.Spec.SiteName},
									{Name: "GIT_REPO", Value: siteApp.Spec.GitRepo},
									{Name: "GIT_BRANCH", Value: siteApp.Spec.GitBranch},
									{Name: "USER", Value: "frappe"},
								},
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
