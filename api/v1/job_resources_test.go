package v1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var allJobKinds = []JobKind{
	JobKindBenchInit, JobKindSiteInit, JobKindAppInstall, JobKindBackup,
	JobKindRestore, JobKindMigration, JobKindCron, JobKindMaintenance,
}

// No Job the operator creates may be BestEffort: every kind resolves to
// non-empty requests AND limits even when the bench says nothing.
func TestResolveJobResources_BuiltInsAreComplete(t *testing.T) {
	for _, kind := range allJobKinds {
		for _, bench := range []*FrappeBench{nil, {}, {Spec: FrappeBenchSpec{JobResources: &JobResources{}}}} {
			got := ResolveJobResources(bench, kind)
			for _, res := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
				if q := got.Requests[res]; q.IsZero() {
					t.Errorf("%s: built-in request for %s is empty", kind, res)
				}
				if q := got.Limits[res]; q.IsZero() {
					t.Errorf("%s: built-in limit for %s is empty", kind, res)
				}
				if got.Requests.Cpu().Cmp(*got.Limits.Cpu()) > 0 || got.Requests.Memory().Cmp(*got.Limits.Memory()) > 0 {
					t.Errorf("%s: request exceeds limit: %v", kind, got)
				}
			}
		}
	}
}

func TestResolveJobResources_Precedence(t *testing.T) {
	rr := func(mem string) *ResourceRequirements {
		return &ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(mem)},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(mem)},
		}
	}
	bench := &FrappeBench{Spec: FrappeBenchSpec{JobResources: &JobResources{
		Default:    rr("7Gi"),
		AppInstall: rr("9Gi"),
	}}}

	appInstall := ResolveJobResources(bench, JobKindAppInstall).Limits
	if got := appInstall.Memory().String(); got != "9Gi" {
		t.Errorf("specific entry must win over Default, got %s", got)
	}
	backup := ResolveJobResources(bench, JobKindBackup).Limits
	if got := backup.Memory().String(); got != "7Gi" {
		t.Errorf("Default must apply to kinds with no specific entry, got %s", got)
	}
	// An entry that only sets memory does not inherit the built-in CPU: the
	// operator hands the bench's block to the pod as-is, so the author owns it.
	if !backup.Cpu().IsZero() {
		t.Error("a bench-supplied block must be used verbatim, not merged with built-ins")
	}
}

func TestDefaultJobResources_HeavyKindsHaveMigrateHeadroom(t *testing.T) {
	min := resource.MustParse("2Gi")
	for _, kind := range []JobKind{JobKindSiteInit, JobKindAppInstall, JobKindMigration, JobKindRestore} {
		limits := DefaultJobResources(kind).Limits
		if limits.Memory().Cmp(min) < 0 {
			t.Errorf("%s runs bench migrate; its built-in memory limit must be at least %s", kind, min.String())
		}
	}
}
