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

package v1

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalSiteReference names a FrappeSite in the same namespace as the referring
// object. It deliberately has no namespace field: a microservice can only bind
// to a site in its own namespace (Secrets, Ingress TLS secrets and same-host
// OpenShift Routes are all namespace-local).
type LocalSiteReference struct {
	// Name of the FrappeSite.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// MicroserviceRouting exposes the microservice on the bound site's domain.
type MicroserviceRouting struct {
	// Paths are URL path prefixes on the site's domain that are routed to this
	// microservice instead of the monolith. Matching uses Ingress `Prefix`
	// semantics: `/orders` matches `/orders` and `/orders/...`, not
	// `/orders-report`.
	//
	// Validation only looks at this object. Two microservices claiming the same
	// path on one site are not detected by the operator; the ingress controller
	// (or an API gateway in front of it) decides which one wins.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MaxLength=256
	// +listType=set
	// +kubebuilder:validation:XValidation:rule="self.all(p, p.matches('^(/[A-Za-z0-9._~%-]+)+$'))",message="each path must be an absolute path made of non-empty segments of [A-Za-z0-9._~%-], with no trailing slash; '/' is not allowed"
	// +kubebuilder:validation:XValidation:rule="self.all(p, !p.matches('/[.][.]?(/|$)'))",message="paths must not contain dot segments ('.' or '..')"
	// +kubebuilder:validation:XValidation:rule="self.all(p, !['/api/method/login', '/login', '/app', '/assets', '/files', '/private', '/socket.io'].exists(r, p == r || p.startsWith(r + '/') || r.startsWith(p + '/')))",message="paths must not overlap reserved monolith routes: /api/method/login, /login, /app, /assets, /files, /private, /socket.io"
	Paths []string `json:"paths"`
}

// MicroserviceType selects the workload the operator runs the image as.
// +kubebuilder:validation:Enum=Deployment;Job;CronJob
type MicroserviceType string

const (
	// MicroserviceTypeDeployment is a long-running HTTP service: Deployment,
	// ClusterIP Service and optional routing on the site's domain.
	MicroserviceTypeDeployment MicroserviceType = "Deployment"
	// MicroserviceTypeJob runs the image to completion once per spec change.
	MicroserviceTypeJob MicroserviceType = "Job"
	// MicroserviceTypeCronJob runs the image on a schedule.
	MicroserviceTypeCronJob MicroserviceType = "CronJob"
)

// MicroserviceDeploymentSpec configures type Deployment.
type MicroserviceDeploymentSpec struct {
	// Port the container listens on and the Service exposes. The operator passes
	// it to the container as PORT.
	// +kubebuilder:default=8000
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// Replicas of the Deployment. With autoscaling set, the autoscaler owns the
	// replica count and this value applies only once autoscaling is removed.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Routing exposes path prefixes on the site's domain. When omitted, the
	// microservice is only reachable in-cluster through its ClusterIP Service.
	// +optional
	Routing *MicroserviceRouting `json:"routing,omitempty"`

	// Autoscaling lets a HorizontalPodAutoscaler (CPU or memory) or a KEDA
	// ScaledObject (event-driven), both owned by the operator, scale the
	// Deployment. The operator then sets the Deployment's replicas only when it
	// creates it, so it and the autoscaler never fight over the count.
	// +optional
	Autoscaling *MicroserviceAutoscaling `json:"autoscaling,omitempty"`
}

// MicroserviceAutoscaling configures horizontal autoscaling of a Deployment
// microservice. Set CPU and/or memory targets for a HorizontalPodAutoscaler, or
// keda for a KEDA ScaledObject.
// +kubebuilder:validation:XValidation:rule="!has(self.minReplicas) || self.minReplicas <= self.maxReplicas",message="minReplicas must not exceed maxReplicas"
// +kubebuilder:validation:XValidation:rule="has(self.keda) || !has(self.minReplicas) || self.minReplicas >= 1",message="minReplicas must be at least 1 unless keda is set (only KEDA can scale to zero)"
// +kubebuilder:validation:XValidation:rule="has(self.keda) ? size(self.keda.triggers) > 0 : (has(self.targetCPUUtilizationPercentage) || has(self.targetMemoryUtilizationPercentage))",message="set a CPU or memory utilization target, or keda with at least one trigger"
// +kubebuilder:validation:XValidation:rule="!has(self.keda) || (!has(self.targetCPUUtilizationPercentage) && !has(self.targetMemoryUtilizationPercentage))",message="keda cannot be combined with CPU or memory targets; use a cpu or memory KEDA trigger instead"
type MicroserviceAutoscaling struct {
	// MinReplicas is the lower bound. Defaults to 1.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the upper bound.
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas"`

	// TargetCPUUtilizationPercentage is the average CPU utilization, relative
	// to resources.requests.cpu, the HorizontalPodAutoscaler aims for.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +optional
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// TargetMemoryUtilizationPercentage is the average memory utilization,
	// relative to resources.requests.memory, the HorizontalPodAutoscaler aims
	// for.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +optional
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`

	// Keda scales the Deployment with a KEDA ScaledObject instead of a
	// HorizontalPodAutoscaler. Needs KEDA installed in the cluster.
	// +optional
	Keda *MicroserviceKEDA `json:"keda,omitempty"`
}

// MicroserviceKEDA configures the KEDA ScaledObject.
type MicroserviceKEDA struct {
	// PollingInterval in seconds between trigger checks. KEDA defaults to 30.
	// +kubebuilder:validation:Minimum=1
	// +optional
	PollingInterval *int32 `json:"pollingInterval,omitempty"`

	// CooldownPeriod in seconds to wait after the last active trigger before
	// scaling back to minReplicas. KEDA defaults to 300.
	// +kubebuilder:validation:Minimum=0
	// +optional
	CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`

	// Triggers decide the replica count, e.g. a redis list length.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Triggers []MicroserviceKEDATrigger `json:"triggers"`
}

// MicroserviceKEDATrigger is one KEDA scaler.
type MicroserviceKEDATrigger struct {
	// Type of the KEDA scaler, e.g. redis, cpu, prometheus.
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`

	// Name of the trigger, unique within the ScaledObject. Optional.
	// +optional
	Name string `json:"name,omitempty"`

	// MetricType is how KEDA compares the metric with its target. Needed by
	// the cpu and memory scalers.
	// +kubebuilder:validation:Enum=Utilization;Value;AverageValue
	// +optional
	MetricType string `json:"metricType,omitempty"`

	// Metadata is the scaler configuration, passed to KEDA as is.
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`

	// AuthenticationRef names a KEDA TriggerAuthentication in this namespace
	// that supplies the scaler's credentials.
	// +optional
	AuthenticationRef string `json:"authenticationRef,omitempty"`
}

// MicroserviceJobSpec holds the run settings of type Job, and of every Job a
// CronJob starts.
type MicroserviceJobSpec struct {
	// BackoffLimit is the number of retries before a run is marked failed.
	// Kubernetes defaults it to 6.
	// +kubebuilder:validation:Minimum=0
	// +optional
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`

	// ActiveDeadlineSeconds bounds how long a run may take.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`

	// TTLSecondsAfterFinished lets Kubernetes delete a finished Job. The
	// operator remembers the run in status and does not start it again.
	// +kubebuilder:validation:Minimum=0
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
}

// MicroserviceCronJobSpec configures type CronJob.
type MicroserviceCronJobSpec struct {
	// Schedule in cron format, e.g. "0 2 * * *".
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// TimeZone for the schedule, e.g. "Australia/Sydney". Defaults to the
	// kube-controller-manager's time zone.
	// +optional
	TimeZone *string `json:"timeZone,omitempty"`

	// ConcurrencyPolicy for overlapping runs.
	// +kubebuilder:validation:Enum=Allow;Forbid;Replace
	// +kubebuilder:default=Forbid
	// +optional
	ConcurrencyPolicy batchv1.ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	// Suspend pauses future runs.
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	// StartingDeadlineSeconds after a missed schedule, beyond which the run is
	// skipped.
	// +kubebuilder:validation:Minimum=0
	// +optional
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`

	// SuccessfulJobsHistoryLimit is how many finished successful Jobs to keep.
	// +kubebuilder:validation:Minimum=0
	// +optional
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`

	// FailedJobsHistoryLimit is how many finished failed Jobs to keep.
	// +kubebuilder:validation:Minimum=0
	// +optional
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty"`
}

// FrappeMicroserviceSpec defines the desired state of FrappeMicroservice.
// +kubebuilder:validation:XValidation:rule="!has(self.deployment) || !has(self.deployment.autoscaling) || !has(self.deployment.autoscaling.targetCPUUtilizationPercentage) || (has(self.resources) && has(self.resources.requests) && 'cpu' in self.resources.requests)",message="targetCPUUtilizationPercentage requires resources.requests.cpu"
// +kubebuilder:validation:XValidation:rule="!has(self.deployment) || !has(self.deployment.autoscaling) || !has(self.deployment.autoscaling.keda) || self.deployment.autoscaling.keda.triggers.all(t, t.type != 'cpu' || (has(self.resources) && has(self.resources.requests) && 'cpu' in self.resources.requests))",message="a cpu keda trigger requires resources.requests.cpu"
// +kubebuilder:validation:XValidation:rule="!has(self.deployment) || !has(self.deployment.autoscaling) || !has(self.deployment.autoscaling.keda) || self.deployment.autoscaling.keda.triggers.all(t, t.type != 'memory' || (has(self.resources) && has(self.resources.requests) && 'memory' in self.resources.requests))",message="a memory keda trigger requires resources.requests.memory"
// +kubebuilder:validation:XValidation:rule="!has(self.deployment) || !has(self.deployment.autoscaling) || !has(self.deployment.autoscaling.targetMemoryUtilizationPercentage) || (has(self.resources) && has(self.resources.requests) && 'memory' in self.resources.requests)",message="targetMemoryUtilizationPercentage requires resources.requests.memory"
// +kubebuilder:validation:XValidation:rule="self.type == 'Deployment' || !has(self.deployment)",message="deployment is only valid when type is Deployment"
// +kubebuilder:validation:XValidation:rule="self.type != 'Deployment' || !has(self.job)",message="job is only valid when type is Job or CronJob"
// +kubebuilder:validation:XValidation:rule="(self.type == 'CronJob') == has(self.cronJob)",message="cronJob is required when type is CronJob, and only valid then"
// +kubebuilder:validation:XValidation:rule="self.type == 'Deployment' || (has(self.command) && size(self.command) > 0)",message="command is required when type is Job or CronJob"
type FrappeMicroserviceSpec struct {
	// SiteRef is the FrappeSite (in this namespace) the microservice binds to.
	// The operator generates the site binding Secret from it. Immutable.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="siteRef is immutable"
	SiteRef LocalSiteReference `json:"siteRef"`

	// Image of the microservice, typically built on frappe-microservice-lib.
	// +kubebuilder:validation:XValidation:rule="has(self.repository) && self.repository != ''",message="image.repository is required"
	Image ImageConfig `json:"image"`

	// Type of workload. Immutable.
	// +kubebuilder:default=Deployment
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type is immutable"
	// +optional
	Type MicroserviceType `json:"type,omitempty"`

	// Command overrides the image entrypoint. Required for Job and CronJob,
	// because the library's default entrypoint starts a long-running server.
	// +optional
	Command []string `json:"command,omitempty"`

	// Args passed to the command.
	// +optional
	Args []string `json:"args,omitempty"`

	// Resources for the microservice container.
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// Env adds environment variables to the container. Operator-managed binding
	// keys (FRAPPE_SITE, CENTRAL_SITE_URL, PORT, DB_*) cannot be set here.
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:XValidation:rule="self.all(e, !(e.name in ['FRAPPE_SITE', 'CENTRAL_SITE_URL', 'PORT']) && !e.name.startsWith('DB_'))",message="env must not set operator-managed binding keys: FRAPPE_SITE, CENTRAL_SITE_URL, PORT, DB_*"
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Deployment settings. Only valid when type is Deployment.
	// +optional
	Deployment *MicroserviceDeploymentSpec `json:"deployment,omitempty"`

	// Job run settings. Valid when type is Job or CronJob.
	// +optional
	Job *MicroserviceJobSpec `json:"job,omitempty"`

	// CronJob schedule. Required when type is CronJob.
	// +optional
	CronJob *MicroserviceCronJobSpec `json:"cronJob,omitempty"`
}

// FrappeMicroservicePhase is a coarse summary of the microservice state.
// Deployment and CronJob use Pending/Ready; Job uses Pending, Running,
// Succeeded and Failed. Failed is also used when the site cannot be supported.
// +kubebuilder:validation:Enum=Pending;Ready;Running;Succeeded;Failed
type FrappeMicroservicePhase string

const (
	FrappeMicroservicePhasePending   FrappeMicroservicePhase = "Pending"
	FrappeMicroservicePhaseReady     FrappeMicroservicePhase = "Ready"
	FrappeMicroservicePhaseRunning   FrappeMicroservicePhase = "Running"
	FrappeMicroservicePhaseSucceeded FrappeMicroservicePhase = "Succeeded"
	FrappeMicroservicePhaseFailed    FrappeMicroservicePhase = "Failed"
)

// Condition types reported on FrappeMicroservice.
const (
	// MicroserviceConditionSiteReady: the bound site is Ready and its binding
	// Secret is in sync.
	MicroserviceConditionSiteReady = "SiteReady"
	// MicroserviceConditionAvailable: the Deployment has the desired number of
	// ready replicas and the Service exists.
	MicroserviceConditionAvailable = "Available"
	// MicroserviceConditionRouted: only set when spec.routing is present. On
	// OpenShift it is True once every Route is admitted; on Kubernetes once the
	// Ingress has a load balancer address.
	MicroserviceConditionRouted = "Routed"
	// MicroserviceConditionComplete (type Job): True once the run succeeded;
	// False with reason Running or Failed otherwise.
	MicroserviceConditionComplete = "Complete"
	// MicroserviceConditionScheduled (type CronJob): True once the CronJob
	// exists; reason Suspended when it is paused.
	MicroserviceConditionScheduled = "Scheduled"
	// MicroserviceConditionAutoscalingReady: only set when
	// spec.deployment.autoscaling is present. True once the HorizontalPodAutoscaler
	// or ScaledObject exists and is active; False with reason KEDANotInstalled
	// when keda is requested but the cluster has no KEDA CRDs.
	MicroserviceConditionAutoscalingReady = "AutoscalingReady"
)

// MicroserviceAutoscalingStatus is the observed state of the autoscaler.
type MicroserviceAutoscalingStatus struct {
	// CurrentReplicas is the number of pods the Deployment runs now.
	CurrentReplicas int32 `json:"currentReplicas"`
	// DesiredReplicas is the count the autoscaler asks for.
	DesiredReplicas int32 `json:"desiredReplicas"`
}

// FrappeMicroserviceStatus defines the observed state of FrappeMicroservice.
type FrappeMicroserviceStatus struct {
	// Phase is Ready when SiteReady and Available are True and, if routed,
	// Routed is True.
	// +optional
	Phase FrappeMicroservicePhase `json:"phase,omitempty"`

	// ReadyReplicas reported by the Deployment (type Deployment).
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// ServiceURL is the in-cluster URL of the microservice (type Deployment).
	// +optional
	ServiceURL string `json:"serviceURL,omitempty"`

	// ExternalURLs lists the routed URLs on the site's domain.
	// +optional
	ExternalURLs []string `json:"externalURLs,omitempty"`

	// JobName is the Job of the current run (type Job).
	// +optional
	JobName string `json:"jobName,omitempty"`

	// LastScheduleTime of the CronJob (type CronJob).
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// LastSuccessfulTime of the CronJob (type CronJob).
	// +optional
	LastSuccessfulTime *metav1.Time `json:"lastSuccessfulTime,omitempty"`

	// ActiveJobs is the number of running Jobs started by the CronJob.
	// +optional
	ActiveJobs int32 `json:"activeJobs,omitempty"`

	// Autoscaling reports the replica counts while spec.deployment.autoscaling
	// is set (type Deployment).
	// +optional
	Autoscaling *MicroserviceAutoscalingStatus `json:"autoscaling,omitempty"`

	// ObservedGeneration is the generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions: SiteReady always; Available and Routed (Deployment);
	// Complete (Job); Scheduled (CronJob).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=fms
//+kubebuilder:validation:XValidation:rule="self.spec.type != 'Deployment' || size(self.metadata.name) <= 63",message="a Deployment microservice name must be at most 63 characters (it names the Service)"
//+kubebuilder:validation:XValidation:rule="self.spec.type != 'CronJob' || size(self.metadata.name) <= 52",message="a CronJob microservice name must be at most 52 characters"
//+kubebuilder:printcolumn:name="Site",type="string",JSONPath=".spec.siteRef.name"
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas"
//+kubebuilder:printcolumn:name="Service URL",type="string",JSONPath=".status.serviceURL"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FrappeMicroservice runs a frappe-microservice-lib image bound to an existing
// FrappeSite through a generated site binding Secret, as a Deployment (with a
// ClusterIP Service and optional routing on the site's domain), a Job or a
// CronJob.
type FrappeMicroservice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FrappeMicroserviceSpec   `json:"spec,omitempty"`
	Status FrappeMicroserviceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FrappeMicroserviceList contains a list of FrappeMicroservice
type FrappeMicroserviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FrappeMicroservice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FrappeMicroservice{}, &FrappeMicroserviceList{})
}
