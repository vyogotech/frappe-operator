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
	"net"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

const (
	siteDomainFinalizer = "vyogo.tech/sitedomain-finalizer"
)

// SiteDomainReconciler reconciles a SiteDomain object
type SiteDomainReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Optional injected DNS lookup function for testing
	DNSLookupFunc func(host string) ([]string, error)
}

//+kubebuilder:rbac:groups=vyogo.tech,resources=sitedomains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitedomains/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vyogo.tech,resources=sitedomains/finalizers,verbs=update
//+kubebuilder:rbac:groups=vyogo.tech,resources=frappesites,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *SiteDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	siteDomain := &vyogotechv1.SiteDomain{}
	if err := r.Get(ctx, req.NamespacedName, siteDomain); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if siteDomain.Spec.SiteRef == nil || siteDomain.Spec.SiteRef.Name == "" {
		return r.failReconciliation(ctx, siteDomain, "siteRef.name is required", "ValidationFailed")
	}
	if siteDomain.Spec.Domain == "" {
		return r.failReconciliation(ctx, siteDomain, "domain is required", "ValidationFailed")
	}

	siteNamespace := siteDomain.Spec.SiteRef.Namespace
	if siteNamespace == "" {
		siteNamespace = siteDomain.Namespace
	}

	// Fetch referenced FrappeSite
	site := &vyogotechv1.FrappeSite{}
	siteKey := types.NamespacedName{Name: siteDomain.Spec.SiteRef.Name, Namespace: siteNamespace}
	if err := r.Get(ctx, siteKey, site); err != nil {
		siteDomain.Status.Phase = "Pending"
		r.setCondition(siteDomain, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotFound",
			Message: fmt.Sprintf("Referenced FrappeSite %s not found: %v", siteKey.Name, err),
		})
		_ = r.updateStatus(ctx, siteDomain)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Handle Deletion & Finalizer
	if siteDomain.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(siteDomain, siteDomainFinalizer) {
			logger.Info("Cleaning up site domain", "domain", siteDomain.Spec.Domain, "site", site.Name)
			// Best-effort: remove the sites/<domain> alias symlink on the PVC.
			// The Ingress + Certificate are garbage-collected via owner refs.
			r.cleanupFrappeDomainAlias(ctx, siteDomain, site)
			latest := &vyogotechv1.SiteDomain{}
			if err := r.Get(ctx, req.NamespacedName, latest); err == nil {
				controllerutil.RemoveFinalizer(latest, siteDomainFinalizer)
				if err := r.Update(ctx, latest); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !controllerutil.ContainsFinalizer(siteDomain, siteDomainFinalizer) {
		controllerutil.AddFinalizer(siteDomain, siteDomainFinalizer)
		if err := r.Update(ctx, siteDomain); err != nil {
			return ctrl.Result{}, err
		}
	}

	if site.Status.Phase != vyogotechv1.FrappeSitePhaseReady {
		siteDomain.Status.Phase = "Pending"
		r.setCondition(siteDomain, metav1.Condition{
			Type:    "SiteReady",
			Status:  metav1.ConditionFalse,
			Reason:  "SiteNotReady",
			Message: fmt.Sprintf("Referenced FrappeSite %s is in phase %s", site.Name, site.Status.Phase),
		})
		_ = r.updateStatus(ctx, siteDomain)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Step 1: DNS Resolution Check
	lookupFunc := r.DNSLookupFunc
	if lookupFunc == nil {
		lookupFunc = net.LookupHost
	}

	dnsConfigured := false
	resolvedIPs, err := lookupFunc(siteDomain.Spec.Domain)
	if err == nil && len(resolvedIPs) > 0 {
		dnsConfigured = true
	}

	siteDomain.Status.DNSConfigured = dnsConfigured

	// Step 2: Create or Update Kubernetes Ingress for custom domain
	ingressName := fmt.Sprintf("%s-domain-%s", site.Name, siteDomain.Name)
	secretName := fmt.Sprintf("%s-tls", siteDomain.Name)
	if siteDomain.Spec.TLS != nil && siteDomain.Spec.TLS.SecretName != "" {
		secretName = siteDomain.Spec.TLS.SecretName
	}

	pathTypePrefix := networkingv1.PathTypePrefix
	// Mirror the primary-site ingress (frappesite_ingress.go): a sane default,
	// then whatever the site's ingress config specifies. Do NOT hardcode
	// ssl-redirect here — that belongs to the cluster/site. Forcing it "true"
	// 308-loops on proxy-fronted clusters (e.g. Cloudflare Flexible SSL, where
	// the edge speaks HTTP to the origin), which is why the platform ingresses
	// set ssl-redirect "false". Inheriting the site annotations lets that flow
	// through to custom domains too.
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-body-size": "100m",
	}
	if site.Spec.Ingress != nil && site.Spec.Ingress.Annotations != nil {
		for k, v := range site.Spec.Ingress.Annotations {
			annotations[k] = v
		}
	}

	if siteDomain.Spec.TLS != nil && siteDomain.Spec.TLS.IssuerRef != nil && siteDomain.Spec.TLS.IssuerRef.Name != "" {
		if siteDomain.Spec.TLS.IssuerRef.Kind == "ClusterIssuer" {
			annotations["cert-manager.io/cluster-issuer"] = siteDomain.Spec.TLS.IssuerRef.Name
		} else {
			annotations["cert-manager.io/issuer"] = siteDomain.Spec.TLS.IssuerRef.Name
		}
	}

	ingressClass := "nginx"
	if site.Spec.Ingress != nil && site.Spec.Ingress.ClassName != "" {
		ingressClass = site.Spec.Ingress.ClassName
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressName,
			Namespace:   siteDomain.Namespace,
			Labels:      map[string]string{"app": "frappe", "site": site.Name},
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []networkingv1.IngressRule{
				{
					Host: siteDomain.Spec.Domain,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathTypePrefix,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											// Must match the bench nginx Service the FrappeBench
											// controller creates: "<bench>-nginx" (see
											// frappebench_deployments.go ensureNginxService and the
											// primary-site ingress in frappesite_ingress.go). The old
											// "-bench-v2-nginx" name never existed, so the custom-domain
											// Ingress had no valid backend (503).
											Name: fmt.Sprintf("%s-nginx", site.Spec.BenchRef.Name),
											Port: networkingv1.ServiceBackendPort{Number: 8080},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{siteDomain.Spec.Domain},
					SecretName: secretName,
				},
			},
		},
	}
	_ = controllerutil.SetControllerReference(siteDomain, ingress, r.Scheme)

	var existingIngress networkingv1.Ingress
	err = r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: siteDomain.Namespace}, &existingIngress)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, ingress); err != nil {
			return r.failReconciliation(ctx, siteDomain, fmt.Sprintf("Failed to create custom domain Ingress: %v", err), "IngressCreationFailed")
		}
	} else if err == nil {
		existingIngress.Spec = ingress.Spec
		existingIngress.Annotations = ingress.Annotations
		if err := r.Update(ctx, &existingIngress); err != nil {
			return r.failReconciliation(ctx, siteDomain, fmt.Sprintf("Failed to update custom domain Ingress: %v", err), "IngressUpdateFailed")
		}
	}

	// Step 3: Register the custom domain with Frappe itself. The bench runs with
	// FRAPPE_SITE_NAME_HEADER=$host, so Frappe resolves the site from the Host
	// header and looks for sites/<host>/. The site directory is named the primary
	// domain (site.Spec.SiteName), so a request on the custom domain would 404
	// ("site does not exist") without an alias. We create a symlink
	// sites/<domain> -> <siteName> on the bench's shared sites PVC — the native
	// Frappe multitenant alias mechanism — so every bench pod resolves it.
	if err := r.ensureFrappeDomainAlias(ctx, siteDomain, site); err != nil {
		return r.failReconciliation(ctx, siteDomain, fmt.Sprintf("Failed to register domain alias with Frappe: %v", err), "AliasJobFailed")
	}

	siteDomain.Status.Phase = "Ready"
	siteDomain.Status.IngressName = ingressName
	siteDomain.Status.TLSCertificateIssued = true
	siteDomain.Status.ObservedGeneration = siteDomain.Generation
	r.setCondition(siteDomain, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "DomainActive",
		Message: fmt.Sprintf("Custom domain %s is active", siteDomain.Spec.Domain),
	})
	_ = r.updateStatus(ctx, siteDomain)

	return ctrl.Result{}, nil
}

// ensureFrappeDomainAlias creates an idempotent Job that symlinks
// sites/<domain> -> <siteName> on the bench's shared sites PVC, so Frappe's
// $host-based multitenant routing resolves the custom domain to the site.
// The symlink lives on the shared PVC, so a single run serves every bench pod.
func (r *SiteDomainReconciler) ensureFrappeDomainAlias(ctx context.Context, siteDomain *vyogotechv1.SiteDomain, site *vyogotechv1.FrappeSite) error {
	jobName := fmt.Sprintf("%s-domain-alias", siteDomain.Name)

	// Idempotent: leave an existing Job alone unless it failed, in which case
	// recreate it so the alias eventually lands. `ln -sfn` is itself idempotent.
	existing := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: siteDomain.Namespace}, existing)
	if err == nil {
		if existing.Status.Failed > 0 && existing.Status.Succeeded == 0 {
			bg := metav1.DeletePropagationBackground
			_ = r.Delete(ctx, existing, &client.DeleteOptions{PropagationPolicy: &bg})
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	job := r.domainAliasJob(siteDomain, site, jobName, false)
	_ = controllerutil.SetControllerReference(siteDomain, job, r.Scheme)
	if err := r.Create(ctx, job); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// cleanupFrappeDomainAlias fires a best-effort Job that removes the sites/<domain>
// symlink when the SiteDomain is deleted. It carries no owner reference (so it
// outlives the SiteDomain it is cleaning up after) and self-deletes via TTL.
func (r *SiteDomainReconciler) cleanupFrappeDomainAlias(ctx context.Context, siteDomain *vyogotechv1.SiteDomain, site *vyogotechv1.FrappeSite) {
	jobName := fmt.Sprintf("%s-domain-unalias", siteDomain.Name)
	existing := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: siteDomain.Namespace}, existing); err == nil {
		return
	}
	job := r.domainAliasJob(siteDomain, site, jobName, true)
	_ = r.Create(ctx, job)
}

// domainAliasJob builds the create-or-remove alias Job. It passes the site name
// and domain as env vars (never interpolated into the shell) and rejects a domain
// containing a path separator, so a hostile domain value cannot escape sites/.
func (r *SiteDomainReconciler) domainAliasJob(siteDomain *vyogotechv1.SiteDomain, site *vyogotechv1.FrappeSite, jobName string, cleanup bool) *batchv1.Job {
	pvcName := fmt.Sprintf("%s-sites", site.Spec.BenchRef.Name)
	backoff := int32(4)
	ttl := int32(600)
	runAsUser := int64(1000)

	script := `set -e
case "$DOMAIN" in */*|..|"") echo "invalid domain: $DOMAIN"; exit 1;; esac
cd /sites
if [ ! -d "$SITE_NAME" ]; then echo "site dir $SITE_NAME not found"; exit 1; fi
if [ "$DOMAIN" = "$SITE_NAME" ]; then echo "domain equals site; nothing to do"; exit 0; fi
ln -sfn "$SITE_NAME" "$DOMAIN"
echo "aliased $DOMAIN -> $SITE_NAME"; ls -ld "$DOMAIN"`
	if cleanup {
		script = `set -e
case "$DOMAIN" in */*|..|"") echo "invalid domain: $DOMAIN"; exit 0;; esac
cd /sites
if [ -L "$DOMAIN" ]; then rm -f "$DOMAIN"; echo "removed alias $DOMAIN"; else echo "no alias $DOMAIN"; fi`
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: siteDomain.Namespace,
			Labels:    map[string]string{"app": "frappe", "site": site.Name, "sitedomain": siteDomain.Name},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:   corev1.RestartPolicyOnFailure,
					SecurityContext: &corev1.PodSecurityContext{FSGroup: &runAsUser},
					Containers: []corev1.Container{{
						Name:    "alias",
						Image:   "busybox:latest",
						Command: []string{"sh", "-c", script},
						Env: []corev1.EnvVar{
							{Name: "SITE_NAME", Value: site.Spec.SiteName},
							{Name: "DOMAIN", Value: siteDomain.Spec.Domain},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name: "sites", MountPath: "/sites", SubPath: "frappe-sites",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "sites",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
						},
					}},
				},
			},
		},
	}
}

func (r *SiteDomainReconciler) failReconciliation(ctx context.Context, siteDomain *vyogotechv1.SiteDomain, msg, reason string) (ctrl.Result, error) {
	siteDomain.Status.Phase = "Failed"
	r.setCondition(siteDomain, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if r.Recorder != nil {
		r.Recorder.Event(siteDomain, corev1.EventTypeWarning, reason, msg)
	}
	_ = r.updateStatus(ctx, siteDomain)
	return ctrl.Result{}, fmt.Errorf("%s", msg)
}

func (r *SiteDomainReconciler) setCondition(siteDomain *vyogotechv1.SiteDomain, condition metav1.Condition) {
	condition.ObservedGeneration = siteDomain.Generation
	meta.SetStatusCondition(&siteDomain.Status.Conditions, condition)
}

func (r *SiteDomainReconciler) updateStatus(ctx context.Context, siteDomain *vyogotechv1.SiteDomain) error {
	latest := &vyogotechv1.SiteDomain{}
	if err := r.Get(ctx, types.NamespacedName{Name: siteDomain.Name, Namespace: siteDomain.Namespace}, latest); err != nil {
		return err
	}
	latest.Status = siteDomain.Status
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("sitedomain-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&vyogotechv1.SiteDomain{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
