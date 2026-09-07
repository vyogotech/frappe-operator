package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// JobResources sizes the one-off Jobs the operator runs against a bench: bench
// and site initialisation, app install/uninstall, backup, restore, migration,
// cron runs and the small maintenance helpers (domain alias, config apply,
// site delete, database provisioning).
//
// Every entry is optional. A Job takes the most specific entry set here, then
// Default, then the operator's built-in sizing (see ResolveJobResources) - so
// a Job never runs without requests and limits, which would otherwise make it
// BestEffort: the first thing evicted under node pressure and invisible to the
// scheduler's bin-packing.
type JobResources struct {
	// Default applies to every Job for which no more specific entry is set.
	// +optional
	Default *ResourceRequirements `json:"default,omitempty"`

	// BenchInit sizes the bench initialisation Job (assets/apps bootstrap).
	// +optional
	BenchInit *ResourceRequirements `json:"benchInit,omitempty"`

	// SiteInit sizes site creation (bench new-site, optional app installs, migrate).
	// +optional
	SiteInit *ResourceRequirements `json:"siteInit,omitempty"`

	// AppInstall sizes SiteApp install and uninstall Jobs (pip/fpm install + migrate).
	// +optional
	AppInstall *ResourceRequirements `json:"appInstall,omitempty"`

	// Backup sizes one-time and scheduled backup Jobs.
	// +optional
	Backup *ResourceRequirements `json:"backup,omitempty"`

	// Restore sizes restore Jobs.
	// +optional
	Restore *ResourceRequirements `json:"restore,omitempty"`

	// Migration sizes SiteMigration Jobs (bench migrate).
	// +optional
	Migration *ResourceRequirements `json:"migration,omitempty"`

	// Cron sizes SiteCron Jobs.
	// +optional
	Cron *ResourceRequirements `json:"cron,omitempty"`

	// Maintenance sizes the small helpers: site delete, domain alias, config
	// apply and database provisioning/cleanup Jobs.
	// +optional
	Maintenance *ResourceRequirements `json:"maintenance,omitempty"`
}

// JobKind names a class of operator-run Job for resource resolution.
type JobKind string

const (
	JobKindBenchInit   JobKind = "benchInit"
	JobKindSiteInit    JobKind = "siteInit"
	JobKindAppInstall  JobKind = "appInstall"
	JobKindBackup      JobKind = "backup"
	JobKindRestore     JobKind = "restore"
	JobKindMigration   JobKind = "migration"
	JobKindCron        JobKind = "cron"
	JobKindMaintenance JobKind = "maintenance"
)

func requirements(cpuReq, memReq, cpuLim, memLim string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpuReq),
			corev1.ResourceMemory: resource.MustParse(memReq),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpuLim),
			corev1.ResourceMemory: resource.MustParse(memLim),
		},
	}
}

// DefaultJobResources is the operator's built-in sizing for a Job kind, used
// when neither the bench's jobResources entry for that kind nor its Default
// is set.
//
// Requests are kept modest so Jobs schedule beside serving pods on a busy
// node; limits are where the real work happens. The heavy kinds (site init,
// app install, migration, restore) run `bench migrate`, which on ERPNext-sized
// sites peaks well past 1Gi, so they get 3Gi headroom and two CPUs.
func DefaultJobResources(kind JobKind) corev1.ResourceRequirements {
	switch kind {
	case JobKindBenchInit:
		return requirements("100m", "512Mi", "1", "2Gi")
	case JobKindSiteInit:
		return requirements("250m", "512Mi", "2", "3Gi")
	case JobKindAppInstall, JobKindMigration, JobKindRestore:
		return requirements("250m", "512Mi", "2", "3Gi")
	case JobKindBackup, JobKindCron:
		return requirements("100m", "256Mi", "1", "1Gi")
	default: // JobKindMaintenance and anything unknown
		return requirements("50m", "128Mi", "500m", "512Mi")
	}
}

// ResolveJobResources returns the resources a Job of the given kind should
// run with on this bench: the kind-specific entry, else Default, else the
// operator built-in. A nil bench (a Job built without one in scope) yields
// the built-in sizing.
func ResolveJobResources(bench *FrappeBench, kind JobKind) corev1.ResourceRequirements {
	if bench == nil || bench.Spec.JobResources == nil {
		return DefaultJobResources(kind)
	}
	jr := bench.Spec.JobResources
	var specific *ResourceRequirements
	switch kind {
	case JobKindBenchInit:
		specific = jr.BenchInit
	case JobKindSiteInit:
		specific = jr.SiteInit
	case JobKindAppInstall:
		specific = jr.AppInstall
	case JobKindBackup:
		specific = jr.Backup
	case JobKindRestore:
		specific = jr.Restore
	case JobKindMigration:
		specific = jr.Migration
	case JobKindCron:
		specific = jr.Cron
	case JobKindMaintenance:
		specific = jr.Maintenance
	}
	if specific == nil {
		specific = jr.Default
	}
	if specific == nil {
		return DefaultJobResources(kind)
	}
	return corev1.ResourceRequirements{Requests: specific.Requests, Limits: specific.Limits}
}
