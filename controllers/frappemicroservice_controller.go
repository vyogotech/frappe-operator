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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/controllers/database"
)

const (
	// microserviceLabel is set on every object a FrappeMicroservice owns and is
	// the Deployment/Service selector.
	microserviceLabel = "vyogo.tech/microservice"
	// bindingHashAnnotation on the pod template changes whenever the site
	// binding Secret changes, so credential rotation rolls the Deployment.
	bindingHashAnnotation = "vyogo.tech/site-binding-hash"
	// microserviceSiteRefIndex indexes FrappeMicroservices by spec.siteRef.name.
	microserviceSiteRefIndex = "spec.siteRef.name"

	microserviceContainerName = "microservice"
	microservicePortName      = "http"
	microserviceHealthPath    = "/health"

	microservicePendingRequeue = 30 * time.Second
	microserviceResync         = 5 * time.Minute
)

// FrappeMicroserviceReconciler reconciles a FrappeMicroservice object.
type FrappeMicroserviceReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	IsOpenShift  bool
	EnforceHTTPS bool
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=frappemicroservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappemicroservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappemicroservices/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappebenches,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs;cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes/custom-host,verbs=create;update;patch

// Reconcile drives a FrappeMicroservice towards: binding Secret, Deployment and
// ClusterIP Service always; Ingress or Routes on the site's domain only when
// spec.routing is set. Every child is controlled by the CR, so deleting the CR
// garbage-collects them.
func (r *FrappeMicroserviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ms := &vyogotechv1.FrappeMicroservice{}
	if err := r.Get(ctx, req.NamespacedName, ms); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !ms.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	base := ms.DeepCopy()
	result, err := r.reconcileMicroservice(ctx, ms)
	if err != nil {
		return ctrl.Result{}, err
	}

	ms.Status.ObservedGeneration = ms.Generation
	ms.Status.ServiceURL = ""
	if microserviceType(ms) == vyogotechv1.MicroserviceTypeDeployment {
		ms.Status.ServiceURL = microserviceServiceURL(ms)
	}
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseFailed {
		ms.Status.Phase = microservicePhase(ms)
	}
	// A merge patch carries no resourceVersion, so a cache that lags this
	// controller's own previous write does not turn into a conflict.
	if err := r.Status().Patch(ctx, ms, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if result.RequeueAfter == 0 && ms.Status.Phase == vyogotechv1.FrappeMicroservicePhasePending {
		result.RequeueAfter = microservicePendingRequeue
	}
	return result, nil
}

// reconcileMicroservice records conditions on ms.Status; the caller persists
// them. It returns an error only for unexpected API failures.
func (r *FrappeMicroserviceReconciler) reconcileMicroservice(ctx context.Context, ms *vyogotechv1.FrappeMicroservice) (ctrl.Result, error) {
	ms.Status.Phase = ""

	// 1. Site gating.
	site := &vyogotechv1.FrappeSite{}
	if err := r.Get(ctx, types.NamespacedName{Name: ms.Spec.SiteRef.Name, Namespace: ms.Namespace}, site); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		r.siteNotReady(ms, "SiteNotFound", fmt.Sprintf("FrappeSite %q not found in namespace %s", ms.Spec.SiteRef.Name, ms.Namespace))
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		r.siteNotReady(ms, "SiteNotReady", fmt.Sprintf("FrappeSite %s is in phase %q", site.Name, site.Status.Phase))
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	if site.Status.DatabaseProvider == "sqlite" {
		r.siteNotReady(ms, "UnsupportedDatabase", "FrappeSite uses SQLite; a microservice needs a network database it can share with the site")
		ms.Status.Phase = vyogotechv1.FrappeMicroservicePhaseFailed
		r.Recorder.Event(ms, corev1.EventTypeWarning, "UnsupportedDatabase", "bound FrappeSite uses SQLite")
		return ctrl.Result{}, nil
	}
	if site.Status.DatabaseProvider == "" || site.Status.DatabaseHost == "" {
		r.siteNotReady(ms, "DatabaseInfoUnavailable", fmt.Sprintf("FrappeSite %s has not recorded its database connection yet", site.Name))
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	if site.Spec.BenchRef == nil || site.Spec.BenchRef.Name == "" {
		r.siteNotReady(ms, "BenchNotFound", fmt.Sprintf("FrappeSite %s has no benchRef", site.Name))
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	bench := &vyogotechv1.FrappeBench{}
	benchKey := types.NamespacedName{Name: site.Spec.BenchRef.Name, Namespace: site.Spec.BenchRef.Namespace}
	if benchKey.Namespace == "" {
		benchKey.Namespace = site.Namespace
	}
	if err := r.Get(ctx, benchKey, bench); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		r.siteNotReady(ms, "BenchNotFound", fmt.Sprintf("FrappeBench %s not found", benchKey.Name))
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	provider, err := database.NewProvider(resolveSiteDBConfig(site, bench), r.Client, r.Scheme)
	if err != nil {
		r.siteNotReady(ms, "CredentialsUnavailable", fmt.Sprintf("cannot resolve the site's database provider: %v", err))
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	creds, err := provider.GetCredentials(ctx, site)
	if err != nil || creds == nil {
		r.siteNotReady(ms, "CredentialsUnavailable", fmt.Sprintf("cannot read the site's database credentials: %v", err))
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}

	// 2. Binding Secret.
	binding := microserviceBindingData(ms, site, benchKey, creds)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: microserviceBindingSecretName(ms), Namespace: ms.Namespace}}
	conflict, err := r.createOrUpdateOwned(ctx, ms, secret, func() error {
		secret.Labels = mergeLabels(secret.Labels, microserviceLabels(ms))
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = binding
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if conflict {
		r.nameConflict(ms, "Secret", secret.Name)
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	meta.SetStatusCondition(&ms.Status.Conditions, metav1.Condition{
		Type:               vyogotechv1.MicroserviceConditionSiteReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Bound",
		Message:            fmt.Sprintf("Bound to FrappeSite %s through Secret %s", site.Name, secret.Name),
		ObservedGeneration: ms.Generation,
	})

	hash := bindingHash(binding)
	switch microserviceType(ms) {
	case vyogotechv1.MicroserviceTypeJob:
		return r.reconcileJob(ctx, ms)
	case vyogotechv1.MicroserviceTypeCronJob:
		return r.reconcileCronJob(ctx, ms, hash)
	default:
		return r.reconcileDeploymentWorkload(ctx, ms, site, hash)
	}
}

// reconcileDeploymentWorkload handles type Deployment: Deployment, Service and
// optional routing.
func (r *FrappeMicroserviceReconciler) reconcileDeploymentWorkload(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, site *vyogotechv1.FrappeSite, hash string) (ctrl.Result, error) {
	// 3. Deployment.
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: ms.Name, Namespace: ms.Namespace}}
	conflict, err := r.createOrUpdateOwned(ctx, ms, dep, func() error {
		r.mutateDeployment(ctx, ms, dep, hash)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if conflict {
		r.nameConflict(ms, "Deployment", dep.Name)
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}

	// 4. Service.
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: ms.Name, Namespace: ms.Namespace}}
	conflict, err = r.createOrUpdateOwned(ctx, ms, svc, func() error {
		mutateMicroserviceService(ms, svc)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if conflict {
		r.nameConflict(ms, "Service", svc.Name)
		return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
	}
	setAvailableCondition(ms, dep)

	// 5. External routing.
	if err := r.reconcileRouting(ctx, ms, site); err != nil {
		return ctrl.Result{}, err
	}

	if microservicePhase(ms) == vyogotechv1.FrappeMicroservicePhaseReady {
		return ctrl.Result{RequeueAfter: microserviceResync}, nil
	}
	return ctrl.Result{RequeueAfter: microservicePendingRequeue}, nil
}

// --- binding -------------------------------------------------------------------

func microserviceBindingSecretName(ms *vyogotechv1.FrappeMicroservice) string {
	return ms.Name + "-site"
}

// microserviceBindingData is the full, stable contract between the operator and
// a frappe-microservice-lib image. CENTRAL_SITE_URL points at the bench's
// in-cluster nginx: the library sends FRAPPE_SITE as the Host header, which is
// how the bench resolves the site, so session checks never leave the cluster.
func microserviceBindingData(ms *vyogotechv1.FrappeMicroservice, site *vyogotechv1.FrappeSite, bench types.NamespacedName, creds *database.DatabaseCredentials) map[string][]byte {
	dbType := "mariadb"
	if p := strings.ToLower(site.Status.DatabaseProvider); p == "postgres" || p == "postgresql" {
		dbType = "postgres"
	}
	data := map[string]string{
		"FRAPPE_SITE":      site.Spec.SiteName,
		"CENTRAL_SITE_URL": fmt.Sprintf("http://%s-nginx.%s.svc.cluster.local:8080", bench.Name, bench.Namespace),
		"DB_TYPE":          dbType,
		"DB_HOST":          site.Status.DatabaseHost,
		"DB_PORT":          site.Status.DatabasePort,
		"DB_NAME":          site.Status.DatabaseName,
		"DB_USER":          creds.Username,
		"DB_PASSWORD":      creds.Password,
	}
	if microserviceType(ms) == vyogotechv1.MicroserviceTypeDeployment {
		data["PORT"] = strconv.Itoa(int(microservicePort(ms)))
	}
	out := make(map[string][]byte, len(data))
	for k, v := range data {
		out[k] = []byte(v)
	}
	return out
}

func bindingHash(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write(data[k])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// --- workload ------------------------------------------------------------------

func microserviceType(ms *vyogotechv1.FrappeMicroservice) vyogotechv1.MicroserviceType {
	if ms.Spec.Type == "" {
		return vyogotechv1.MicroserviceTypeDeployment
	}
	return ms.Spec.Type
}

func microservicePort(ms *vyogotechv1.FrappeMicroservice) int32 {
	if ms.Spec.Deployment == nil || ms.Spec.Deployment.Port == 0 {
		return 8000
	}
	return ms.Spec.Deployment.Port
}

func microserviceReplicas(ms *vyogotechv1.FrappeMicroservice) int32 {
	if ms.Spec.Deployment == nil || ms.Spec.Deployment.Replicas == nil {
		return 1
	}
	return *ms.Spec.Deployment.Replicas
}

func microserviceSelector(ms *vyogotechv1.FrappeMicroservice) map[string]string {
	return map[string]string{microserviceLabel: ms.Name}
}

func microserviceLabels(ms *vyogotechv1.FrappeMicroservice) map[string]string {
	return map[string]string{
		microserviceLabel:              ms.Name,
		"vyogo.tech/site":              ms.Spec.SiteRef.Name,
		"app.kubernetes.io/name":       ms.Name,
		"app.kubernetes.io/component":  "microservice",
		"app.kubernetes.io/managed-by": "frappe-operator",
	}
}

func mergeLabels(existing, desired map[string]string) map[string]string {
	if existing == nil {
		existing = map[string]string{}
	}
	for k, v := range desired {
		existing[k] = v
	}
	return existing
}

func microserviceImage(ms *vyogotechv1.FrappeMicroservice) string {
	tag := ms.Spec.Image.Tag
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s:%s", ms.Spec.Image.Repository, tag)
}

func (r *FrappeMicroserviceReconciler) mutateDeployment(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, dep *appsv1.Deployment, hash string) {
	labels := microserviceLabels(ms)
	dep.Labels = mergeLabels(dep.Labels, labels)
	// The selector is immutable; only set it on creation.
	if dep.Spec.Selector == nil {
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: microserviceSelector(ms)}
	}
	replicas := microserviceReplicas(ms)
	dep.Spec.Replicas = &replicas
	dep.Spec.Template.Labels = mergeLabels(dep.Spec.Template.Labels, labels)
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations[bindingHashAnnotation] = hash

	r.mutatePodSpec(ctx, ms, &dep.Spec.Template.Spec, true)
}

// mutatePodSpec sets the pod spec shared by every workload type. A
// long-running pod (Deployment) gets the HTTP port and /health probes; a batch
// pod (Job, CronJob) runs to completion and is never restarted in place.
func (r *FrappeMicroserviceReconciler) mutatePodSpec(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, pod *corev1.PodSpec, longRunning bool) {
	var res corev1.ResourceRequirements
	if ms.Spec.Resources != nil {
		res.Requests = ms.Spec.Resources.Requests
		res.Limits = ms.Spec.Resources.Limits
	}
	c := corev1.Container{
		Name:            microserviceContainerName,
		Image:           microserviceImage(ms),
		ImagePullPolicy: ms.Spec.Image.PullPolicy,
		Command:         ms.Spec.Command,
		Args:            ms.Spec.Args,
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: microserviceBindingSecretName(ms)}},
		}},
		Env:             ms.Spec.Env,
		Resources:       res,
		SecurityContext: ContainerSecurityContextForBench(r.IsOpenShift, nil),
	}
	if longRunning {
		probe := func(initialDelay, period int32) *corev1.Probe {
			return &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
					Path: microserviceHealthPath,
					Port: intstr.FromString(microservicePortName),
				}},
				InitialDelaySeconds: initialDelay,
				PeriodSeconds:       period,
				TimeoutSeconds:      5,
				FailureThreshold:    3,
			}
		}
		c.Ports = []corev1.ContainerPort{{
			Name:          microservicePortName,
			ContainerPort: microservicePort(ms),
			Protocol:      corev1.ProtocolTCP,
		}}
		c.LivenessProbe = probe(15, 20)
		c.ReadinessProbe = probe(5, 10)
	} else {
		pod.RestartPolicy = corev1.RestartPolicyNever
	}
	pod.ImagePullSecrets = ms.Spec.Image.PullSecrets
	pod.SecurityContext = PodSecurityContextForBench(ctx, r.Client, r.IsOpenShift, ms.Namespace, nil)
	pod.Containers = []corev1.Container{c}
}

func mutateMicroserviceService(ms *vyogotechv1.FrappeMicroservice, svc *corev1.Service) {
	svc.Labels = mergeLabels(svc.Labels, microserviceLabels(ms))
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.Selector = microserviceSelector(ms)
	svc.Spec.Ports = []corev1.ServicePort{{
		Name:       microservicePortName,
		Port:       microservicePort(ms),
		TargetPort: intstr.FromString(microservicePortName),
		Protocol:   corev1.ProtocolTCP,
	}}
}

func microserviceServiceURL(ms *vyogotechv1.FrappeMicroservice) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", ms.Name, ms.Namespace, microservicePort(ms))
}

func setAvailableCondition(ms *vyogotechv1.FrappeMicroservice, dep *appsv1.Deployment) {
	desired := microserviceReplicas(ms)
	ready := dep.Status.ReadyReplicas
	ms.Status.ReadyReplicas = ready
	cond := metav1.Condition{Type: vyogotechv1.MicroserviceConditionAvailable, ObservedGeneration: ms.Generation}
	switch {
	case desired == 0:
		cond.Status, cond.Reason, cond.Message = metav1.ConditionTrue, "ScaledToZero", "Deployment is scaled to zero"
	case ready >= desired:
		cond.Status, cond.Reason, cond.Message = metav1.ConditionTrue, "ReplicasReady", fmt.Sprintf("%d/%d replicas ready", ready, desired)
	default:
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, "ReplicasNotReady", fmt.Sprintf("%d/%d replicas ready", ready, desired)
	}
	meta.SetStatusCondition(&ms.Status.Conditions, cond)
}

// --- routing -------------------------------------------------------------------

func microserviceIngressName(ms *vyogotechv1.FrappeMicroservice) string {
	return ms.Name + "-ingress"
}

func microserviceRouteName(ms *vyogotechv1.FrappeMicroservice, path string) string {
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%s-%s", ms.Name, hex.EncodeToString(sum[:])[:8])
}

func microserviceRoutingPaths(ms *vyogotechv1.FrappeMicroservice) []string {
	if ms.Spec.Deployment == nil || ms.Spec.Deployment.Routing == nil {
		return nil
	}
	return ms.Spec.Deployment.Routing.Paths
}

func (r *FrappeMicroserviceReconciler) reconcileRouting(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, site *vyogotechv1.FrappeSite) error {
	paths := microserviceRoutingPaths(ms)
	useRoutes := siteUsesRoute(r.IsOpenShift, site)

	if len(paths) == 0 {
		meta.RemoveStatusCondition(&ms.Status.Conditions, vyogotechv1.MicroserviceConditionRouted)
		ms.Status.ExternalURLs = nil
		return r.pruneRouting(ctx, ms, false, nil)
	}

	routed := func(status metav1.ConditionStatus, reason, msg string) {
		meta.SetStatusCondition(&ms.Status.Conditions, metav1.Condition{
			Type: vyogotechv1.MicroserviceConditionRouted, Status: status, Reason: reason, Message: msg, ObservedGeneration: ms.Generation,
		})
	}

	if !siteIngressEnabled(site) {
		routed(metav1.ConditionFalse, "SiteIngressDisabled", fmt.Sprintf("FrappeSite %s has ingress disabled, so there is no domain to route on", site.Name))
		ms.Status.ExternalURLs = nil
		return r.pruneRouting(ctx, ms, false, nil)
	}
	domain := site.Status.ResolvedDomain
	if domain == "" {
		routed(metav1.ConditionFalse, "SiteDomainUnavailable", fmt.Sprintf("FrappeSite %s has no resolved domain yet", site.Name))
		ms.Status.ExternalURLs = nil
		return nil
	}

	if useRoutes {
		return r.reconcileRoutes(ctx, ms, site, domain, paths, routed)
	}
	return r.reconcileIngress(ctx, ms, site, domain, paths, routed)
}

func (r *FrappeMicroserviceReconciler) reconcileIngress(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, site *vyogotechv1.FrappeSite, domain string, paths []string, routed func(metav1.ConditionStatus, string, string)) error {
	tls := effectiveTLS(r.EnforceHTTPS, site)
	ing := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: microserviceIngressName(ms), Namespace: ms.Namespace}}
	conflict, err := r.createOrUpdateOwned(ctx, ms, ing, func() error {
		ing.Labels = mergeLabels(ing.Labels, microserviceLabels(ms))
		if ing.Annotations == nil {
			ing.Annotations = map[string]string{}
		}
		ing.Annotations["nginx.ingress.kubernetes.io/proxy-body-size"] = "100m"
		if tls {
			ing.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
			ing.Annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] = "true"
		} else {
			delete(ing.Annotations, "nginx.ingress.kubernetes.io/ssl-redirect")
			delete(ing.Annotations, "nginx.ingress.kubernetes.io/force-ssl-redirect")
		}
		className := siteIngressClassName(site)
		ing.Spec.IngressClassName = &className
		pathType := networkingv1.PathTypePrefix
		httpPaths := make([]networkingv1.HTTPIngressPath, 0, len(paths))
		for _, p := range paths {
			httpPaths = append(httpPaths, networkingv1.HTTPIngressPath{
				Path:     p,
				PathType: &pathType,
				Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
					Name: ms.Name,
					Port: networkingv1.ServiceBackendPort{Number: microservicePort(ms)},
				}},
			})
		}
		ing.Spec.Rules = []networkingv1.IngressRule{{
			Host:             domain,
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: httpPaths}},
		}}
		// Reference the site's certificate Secret without a cert-manager
		// annotation: the site's own Ingress already owns that Certificate.
		ing.Spec.TLS = nil
		if tls {
			ing.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{domain}, SecretName: siteTLSSecretName(site)}}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if conflict {
		routed(metav1.ConditionFalse, "NameConflict", fmt.Sprintf("Ingress %s exists and is not owned by this FrappeMicroservice", ing.Name))
		ms.Status.ExternalURLs = nil
		return nil
	}

	// Switching from Routes to an Ingress (e.g. routeConfig.enabled=false)
	// leaves stale Routes behind otherwise.
	if err := r.pruneRouting(ctx, ms, true, nil); err != nil {
		return err
	}

	scheme := "http"
	if tls {
		scheme = "https"
	}
	ms.Status.ExternalURLs = externalURLs(scheme, domain, paths)
	if len(ing.Status.LoadBalancer.Ingress) > 0 {
		routed(metav1.ConditionTrue, "LoadBalancerAssigned", "Ingress has a load balancer address. Path conflicts with other Ingresses are not detected on Kubernetes.")
	} else {
		routed(metav1.ConditionFalse, "IngressPending", "waiting for the ingress controller to publish an address")
	}
	return nil
}

func (r *FrappeMicroserviceReconciler) reconcileRoutes(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, site *vyogotechv1.FrappeSite, domain string, paths []string, routed func(metav1.ConditionStatus, string, string)) error {
	termination := siteRouteTermination(site)
	if termination == routev1.TLSTerminationPassthrough {
		routed(metav1.ConditionFalse, "PassthroughUnsupported", "the site's Route uses passthrough TLS, which does not support path-based routing")
		ms.Status.ExternalURLs = nil
		return r.pruneRouting(ctx, ms, false, nil)
	}
	host := siteRouteHost(site, domain)

	keep := map[string]bool{}
	var pending, rejected []string
	for _, p := range paths {
		path := p
		rt := &routev1.Route{ObjectMeta: metav1.ObjectMeta{Name: microserviceRouteName(ms, path), Namespace: ms.Namespace}}
		keep[rt.Name] = true
		conflict, err := r.createOrUpdateOwned(ctx, ms, rt, func() error {
			rt.Labels = mergeLabels(rt.Labels, microserviceLabels(ms))
			rt.Spec.Host = host
			rt.Spec.Path = path
			rt.Spec.To = routev1.RouteTargetReference{Kind: "Service", Name: ms.Name}
			rt.Spec.Port = &routev1.RoutePort{TargetPort: intstr.FromString(microservicePortName)}
			rt.Spec.TLS = &routev1.TLSConfig{
				Termination:                   termination,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			}
			rt.Spec.WildcardPolicy = routev1.WildcardPolicyNone
			return nil
		})
		if err != nil {
			return err
		}
		if conflict {
			routed(metav1.ConditionFalse, "NameConflict", fmt.Sprintf("Route %s exists and is not owned by this FrappeMicroservice", rt.Name))
			ms.Status.ExternalURLs = nil
			return nil
		}
		switch admitted, reason := routeAdmission(rt); {
		case admitted == corev1.ConditionFalse:
			rejected = append(rejected, fmt.Sprintf("%s: %s", path, reason))
		case admitted != corev1.ConditionTrue:
			pending = append(pending, path)
		}
	}
	if err := r.pruneRouting(ctx, ms, false, keep); err != nil {
		return err
	}

	ms.Status.ExternalURLs = externalURLs("https", host, paths)
	switch {
	case len(rejected) > 0:
		routed(metav1.ConditionFalse, "RouteNotAdmitted", "router rejected: "+strings.Join(rejected, "; "))
	case len(pending) > 0:
		routed(metav1.ConditionFalse, "RoutePending", "waiting for the router to admit: "+strings.Join(pending, ", "))
	default:
		routed(metav1.ConditionTrue, "RoutesAdmitted", fmt.Sprintf("%d route(s) admitted", len(paths)))
	}
	return nil
}

// routeAdmission summarises a Route's Admitted condition across routers.
func routeAdmission(rt *routev1.Route) (corev1.ConditionStatus, string) {
	status := corev1.ConditionUnknown
	for _, ing := range rt.Status.Ingress {
		for _, c := range ing.Conditions {
			if c.Type != routev1.RouteAdmitted {
				continue
			}
			if c.Status == corev1.ConditionFalse {
				reason := c.Reason
				if c.Message != "" {
					reason = fmt.Sprintf("%s (%s)", c.Reason, c.Message)
				}
				return corev1.ConditionFalse, reason
			}
			if c.Status == corev1.ConditionTrue {
				status = corev1.ConditionTrue
			}
		}
	}
	return status, ""
}

// pruneRouting deletes routing objects owned by ms that are no longer wanted:
// the Ingress unless keepIngress, and (on OpenShift) every owned Route whose
// name is not in keepRoutes. Objects it does not control are never touched.
func (r *FrappeMicroserviceReconciler) pruneRouting(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, keepIngress bool, keepRoutes map[string]bool) error {
	if !keepIngress {
		ing := &networkingv1.Ingress{}
		err := r.Get(ctx, types.NamespacedName{Name: microserviceIngressName(ms), Namespace: ms.Namespace}, ing)
		switch {
		case err == nil && metav1.IsControlledBy(ing, ms):
			if err := r.Delete(ctx, ing); client.IgnoreNotFound(err) != nil {
				return err
			}
		case err != nil && !apierrors.IsNotFound(err):
			return err
		}
	}
	if !r.IsOpenShift {
		return nil
	}
	list := &routev1.RouteList{}
	if err := r.List(ctx, list, client.InNamespace(ms.Namespace), client.MatchingLabels{microserviceLabel: ms.Name}); err != nil {
		return err
	}
	for i := range list.Items {
		rt := &list.Items[i]
		if keepRoutes[rt.Name] || !metav1.IsControlledBy(rt, ms) {
			continue
		}
		if err := r.Delete(ctx, rt); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func externalURLs(scheme, host string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, fmt.Sprintf("%s://%s%s", scheme, host, p))
	}
	return out
}

// --- status helpers -------------------------------------------------------------

func (r *FrappeMicroserviceReconciler) siteNotReady(ms *vyogotechv1.FrappeMicroservice, reason, msg string) {
	meta.SetStatusCondition(&ms.Status.Conditions, metav1.Condition{
		Type: vyogotechv1.MicroserviceConditionSiteReady, Status: metav1.ConditionFalse, Reason: reason, Message: msg, ObservedGeneration: ms.Generation,
	})
}

func (r *FrappeMicroserviceReconciler) nameConflict(ms *vyogotechv1.FrappeMicroservice, kind, name string) {
	msg := fmt.Sprintf("%s %s exists and is not owned by this FrappeMicroservice; it was left untouched", kind, name)
	meta.SetStatusCondition(&ms.Status.Conditions, metav1.Condition{
		Type: vyogotechv1.MicroserviceConditionAvailable, Status: metav1.ConditionFalse, Reason: "NameConflict", Message: msg, ObservedGeneration: ms.Generation,
	})
	r.Recorder.Event(ms, corev1.EventTypeWarning, "NameConflict", msg)
}

// microservicePhase derives the phase from the conditions of the workload type.
func microservicePhase(ms *vyogotechv1.FrappeMicroservice) vyogotechv1.FrappeMicroservicePhase {
	if !meta.IsStatusConditionTrue(ms.Status.Conditions, vyogotechv1.MicroserviceConditionSiteReady) {
		return vyogotechv1.FrappeMicroservicePhasePending
	}
	switch microserviceType(ms) {
	case vyogotechv1.MicroserviceTypeJob:
		c := meta.FindStatusCondition(ms.Status.Conditions, vyogotechv1.MicroserviceConditionComplete)
		switch {
		case c == nil:
			return vyogotechv1.FrappeMicroservicePhasePending
		case c.Status == metav1.ConditionTrue:
			return vyogotechv1.FrappeMicroservicePhaseSucceeded
		case c.Reason == "Failed":
			return vyogotechv1.FrappeMicroservicePhaseFailed
		case c.Reason == "Running":
			return vyogotechv1.FrappeMicroservicePhaseRunning
		}
		return vyogotechv1.FrappeMicroservicePhasePending
	case vyogotechv1.MicroserviceTypeCronJob:
		if meta.IsStatusConditionTrue(ms.Status.Conditions, vyogotechv1.MicroserviceConditionScheduled) {
			return vyogotechv1.FrappeMicroservicePhaseReady
		}
		return vyogotechv1.FrappeMicroservicePhasePending
	}
	if !meta.IsStatusConditionTrue(ms.Status.Conditions, vyogotechv1.MicroserviceConditionSiteReady) ||
		!meta.IsStatusConditionTrue(ms.Status.Conditions, vyogotechv1.MicroserviceConditionAvailable) {
		return vyogotechv1.FrappeMicroservicePhasePending
	}
	if c := meta.FindStatusCondition(ms.Status.Conditions, vyogotechv1.MicroserviceConditionRouted); c != nil && c.Status != metav1.ConditionTrue {
		return vyogotechv1.FrappeMicroservicePhasePending
	}
	return vyogotechv1.FrappeMicroservicePhaseReady
}

// createOrUpdateOwned creates or updates obj with mutate, unless an object of
// that name already exists and is not controlled by ms. In that case it
// returns conflict=true and leaves the object alone.
func (r *FrappeMicroserviceReconciler) createOrUpdateOwned(ctx context.Context, ms *vyogotechv1.FrappeMicroservice, obj client.Object, mutate func() error) (bool, error) {
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj); err == nil {
		if !metav1.IsControlledBy(obj, ms) {
			return true, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return false, err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := mutate(); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(ms, obj, r.Scheme)
	})
	return false, err
}

// --- watches ---------------------------------------------------------------------

func indexMicroserviceBySiteRef(obj client.Object) []string {
	ms, ok := obj.(*vyogotechv1.FrappeMicroservice)
	if !ok || ms.Spec.SiteRef.Name == "" {
		return nil
	}
	return []string{ms.Spec.SiteRef.Name}
}

func (r *FrappeMicroserviceReconciler) microservicesForSite(ctx context.Context, namespace, site string) []reconcile.Request {
	list := &vyogotechv1.FrappeMicroserviceList{}
	if err := r.List(ctx, list, client.InNamespace(namespace), client.MatchingFields{microserviceSiteRefIndex: site}); err != nil {
		log.FromContext(ctx).Error(err, "listing FrappeMicroservices for site", "site", site)
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, ms := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: ms.Name, Namespace: ms.Namespace}})
	}
	return reqs
}

// requestsForSite maps a FrappeSite event to the microservices bound to it.
func (r *FrappeMicroserviceReconciler) requestsForSite(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.microservicesForSite(ctx, obj.GetNamespace(), obj.GetName())
}

// requestsForSecret maps a change to a site's database credentials Secret to
// the microservices bound to that site, so the binding is re-synced.
func (r *FrappeMicroserviceReconciler) requestsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	sites := &vyogotechv1.FrappeSiteList{}
	if err := r.List(ctx, sites, client.InNamespace(obj.GetNamespace())); err != nil {
		log.FromContext(ctx).Error(err, "listing FrappeSites for secret", "secret", obj.GetName())
		return nil
	}
	var reqs []reconcile.Request
	for _, site := range sites.Items {
		if site.Status.DatabaseCredentialsSecret == obj.GetName() {
			reqs = append(reqs, r.microservicesForSite(ctx, site.Namespace, site.Name)...)
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *FrappeMicroserviceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("frappemicroservice-controller")
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &vyogotechv1.FrappeMicroservice{}, microserviceSiteRefIndex, indexMicroserviceBySiteRef); err != nil {
		return err
	}
	b := ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.FrappeMicroservice{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Watches(&vyogotechv1.FrappeSite{}, handler.EnqueueRequestsFromMapFunc(r.requestsForSite)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.requestsForSecret))
	if r.IsOpenShift {
		b = b.Owns(&routev1.Route{})
	}
	return b.Complete(r)
}
