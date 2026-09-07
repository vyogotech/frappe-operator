package controllers

import (
	"context"
	"testing"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// Jobs used to be created with no resources at all in the namespaces that
// carry no LimitRange (the pooled bench namespaces), which made every app
// install, backup and migration BestEffort. Each builder must now hand the
// pod the bench's jobResources, falling back to the operator built-ins.
func TestJobBuilders_AlwaysSetResources(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	custom := &vyogotechv1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("333m"), corev1.ResourceMemory: resource.MustParse("333Mi")},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("3"), corev1.ResourceMemory: resource.MustParse("3333Mi")},
	}
	plain := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "bench", Namespace: "default"},
		Spec:       vyogotechv1.FrappeBenchSpec{FrappeVersion: "15"},
	}
	sized := plain.DeepCopy()
	sized.Spec.JobResources = &vyogotechv1.JobResources{Default: custom}

	assertResources := func(t *testing.T, what string, got corev1.ResourceRequirements, want corev1.ResourceRequirements) {
		t.Helper()
		for _, res := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
			if q := got.Requests[res]; q.IsZero() {
				t.Errorf("%s: no %s request", what, res)
			}
			if q := got.Limits[res]; q.IsZero() {
				t.Errorf("%s: no %s limit", what, res)
			}
			if w, ok := want.Limits[res]; ok {
				if g := got.Limits[res]; g.Cmp(w) != 0 {
					t.Errorf("%s: %s limit %s, want %s", what, res, g.String(), w.String())
				}
			}
		}
	}
	wantCustom := corev1.ResourceRequirements{Requests: custom.Requests, Limits: custom.Limits}

	t.Run("backup", func(t *testing.T) {
		r := &SiteBackupReconciler{Scheme: scheme}
		sb := &vyogotechv1.SiteBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"},
			Spec:       vyogotechv1.SiteBackupSpec{Site: "site.local"},
		}
		job := r.buildBackupJob(context.Background(), sb, plain)
		assertResources(t, "backup/built-in", job.Spec.Template.Spec.Containers[0].Resources, vyogotechv1.DefaultJobResources(vyogotechv1.JobKindBackup))
		job = r.buildBackupJob(context.Background(), sb, sized)
		assertResources(t, "backup/custom", job.Spec.Template.Spec.Containers[0].Resources, wantCustom)
	})

	t.Run("app install", func(t *testing.T) {
		r := &SiteAppReconciler{Scheme: scheme}
		sa := &vyogotechv1.SiteApp{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}}
		job := r.buildAppJob(sa, "a-install", "app-installer", "img", "bench-sites", "true", nil,
			vyogotechv1.ResolveJobResources(plain, vyogotechv1.JobKindAppInstall), 1, nil)
		assertResources(t, "appInstall/built-in", job.Spec.Template.Spec.Containers[0].Resources, vyogotechv1.DefaultJobResources(vyogotechv1.JobKindAppInstall))
		job = r.buildAppJob(sa, "a-install", "app-installer", "img", "bench-sites", "true", nil,
			vyogotechv1.ResolveJobResources(sized, vyogotechv1.JobKindAppInstall), 1, nil)
		assertResources(t, "appInstall/custom", job.Spec.Template.Spec.Containers[0].Resources, wantCustom)
	})

	t.Run("config apply", func(t *testing.T) {
		on := true
		sc := &vyogotechv1.SiteConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "default"},
			Spec:       vyogotechv1.SiteConfigSpec{MaintenanceMode: &on},
		}
		job, _ := buildConfigJob(sc, plain, "site.local", "img")
		assertResources(t, "config/built-in", job.Spec.Template.Spec.Containers[0].Resources, vyogotechv1.DefaultJobResources(vyogotechv1.JobKindMaintenance))
		job, _ = buildConfigJob(sc, sized, "site.local", "img")
		assertResources(t, "config/custom", job.Spec.Template.Spec.Containers[0].Resources, wantCustom)
	})
}
