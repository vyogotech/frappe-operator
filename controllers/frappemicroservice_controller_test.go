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
	"strings"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/controllers/database"
	"github.com/vyogotech/frappe-operator/pkg/resources"
)

const (
	msNS    = "tenant"
	msName  = "orders"
	msSite  = "acme"
	msBench = "erp-bench"
)

type msFixture struct {
	t      *testing.T
	ctx    context.Context
	client client.Client
	r      *FrappeMicroserviceReconciler
}

type msFixtureOpts struct {
	openShift    bool
	enforceHTTPS bool
	// mutate the objects before they are loaded into the fake client
	site  func(*vyogotechv1.FrappeSite)
	ms    func(*vyogotechv1.FrappeMicroservice)
	extra []client.Object
	// skipSite leaves the FrappeSite out of the cluster
	skipSite bool
}

func newMSFixture(t *testing.T, o msFixtureOpts) *msFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))

	bench := &vyogotechv1.FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: msBench, Namespace: msNS},
		Status:     vyogotechv1.FrappeBenchStatus{Phase: "Ready"},
	}
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: msSite, Namespace: msNS},
		Spec: vyogotechv1.FrappeSiteSpec{
			BenchRef:         &vyogotechv1.NamespacedName{Name: msBench},
			SiteName:         "erp.acme.com",
			IngressClassName: "traefik",
		},
		Status: vyogotechv1.FrappeSiteStatus{
			Phase:                     vyogotechv1.FrappeSitePhaseReady,
			ResolvedDomain:            "erp.acme.com",
			DatabaseHost:              "mariadb.db.svc.cluster.local",
			DatabasePort:              "3306",
			DatabaseName:              "_acme",
			DatabaseProvider:          "mariadb",
			DatabaseCredentialsSecret: "acme-db-password",
		},
	}
	if o.site != nil {
		o.site(site)
	}
	ms := &vyogotechv1.FrappeMicroservice{
		ObjectMeta: metav1.ObjectMeta{Name: msName, Namespace: msNS, Generation: 1},
		Spec: vyogotechv1.FrappeMicroserviceSpec{
			SiteRef:    vyogotechv1.LocalSiteReference{Name: msSite},
			Image:      vyogotechv1.ImageConfig{Repository: "ghcr.io/acme/orders", Tag: "1.0.0"},
			Deployment: &vyogotechv1.MicroserviceDeploymentSpec{Port: 8000, Replicas: int32Ptr(1)},
		},
	}
	if o.ms != nil {
		o.ms(ms)
	}
	dbPassword := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "acme-db-password", Namespace: msNS},
		Data:       map[string][]byte{"password": []byte("s3cret")},
	}

	objs := []client.Object{bench, ms, dbPassword}
	if !o.skipSite {
		objs = append(objs, site)
	}
	objs = append(objs, o.extra...)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&vyogotechv1.FrappeMicroservice{}, &vyogotechv1.FrappeSite{}, &appsv1.Deployment{}, &networkingv1.Ingress{}, &routev1.Route{}, &batchv1.Job{}, &batchv1.CronJob{}).
		WithIndex(&vyogotechv1.FrappeMicroservice{}, microserviceSiteRefIndex, indexMicroserviceBySiteRef).
		Build()

	// The MariaDB User CR the provider reads the DB user and password secret from.
	user := &unstructured.Unstructured{}
	user.SetGroupVersionKind(database.UserGVK)
	user.SetName(msSite + "-user")
	user.SetNamespace(msNS)
	_ = unstructured.SetNestedField(user.Object, "acme_user", "spec", "name")
	_ = unstructured.SetNestedField(user.Object, "acme-db-password", "spec", "passwordSecretKeyRef", "name")
	_ = unstructured.SetNestedField(user.Object, "password", "spec", "passwordSecretKeyRef", "key")
	if err := c.Create(context.Background(), user); err != nil {
		t.Fatalf("create user CR: %v", err)
	}

	return &msFixture{
		t:      t,
		ctx:    context.Background(),
		client: c,
		r: &FrappeMicroserviceReconciler{
			Client:       c,
			Scheme:       scheme,
			Recorder:     record.NewFakeRecorder(100),
			IsOpenShift:  o.openShift,
			EnforceHTTPS: o.enforceHTTPS,
		},
	}
}

func (f *msFixture) reconcile() ctrl.Result {
	f.t.Helper()
	res, err := f.r.Reconcile(f.ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: msName, Namespace: msNS}})
	if err != nil {
		f.t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func (f *msFixture) ms() *vyogotechv1.FrappeMicroservice {
	f.t.Helper()
	ms := &vyogotechv1.FrappeMicroservice{}
	if err := f.client.Get(f.ctx, types.NamespacedName{Name: msName, Namespace: msNS}, ms); err != nil {
		f.t.Fatalf("get microservice: %v", err)
	}
	return ms
}

func (f *msFixture) get(name string, obj client.Object) bool {
	f.t.Helper()
	err := f.client.Get(f.ctx, types.NamespacedName{Name: name, Namespace: msNS}, obj)
	if apierrors.IsNotFound(err) {
		return false
	}
	if err != nil {
		f.t.Fatalf("get %s: %v", name, err)
	}
	return true
}

func (f *msFixture) mustGet(name string, obj client.Object) {
	f.t.Helper()
	if !f.get(name, obj) {
		f.t.Fatalf("%T %s not found", obj, name)
	}
}

func (f *msFixture) markDeploymentReady() {
	f.t.Helper()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	dep.Status.ReadyReplicas = *dep.Spec.Replicas
	dep.Status.Replicas = *dep.Spec.Replicas
	if err := f.client.Status().Update(f.ctx, dep); err != nil {
		f.t.Fatalf("update deployment status: %v", err)
	}
}

func (f *msFixture) updateMS(mutate func(*vyogotechv1.FrappeMicroservice)) {
	f.t.Helper()
	ms := f.ms()
	mutate(ms)
	ms.Generation++
	if err := f.client.Update(f.ctx, ms); err != nil {
		f.t.Fatalf("update microservice: %v", err)
	}
}

func condition(ms *vyogotechv1.FrappeMicroservice, t string) *metav1.Condition {
	return meta.FindStatusCondition(ms.Status.Conditions, t)
}

func expectCondition(t *testing.T, ms *vyogotechv1.FrappeMicroservice, typ string, status metav1.ConditionStatus, reason string) {
	t.Helper()
	c := condition(ms, typ)
	if c == nil {
		t.Fatalf("condition %s missing; conditions: %+v", typ, ms.Status.Conditions)
	}
	if c.Status != status || (reason != "" && c.Reason != reason) {
		t.Fatalf("condition %s = %s/%s (%s), want %s/%s", typ, c.Status, c.Reason, c.Message, status, reason)
	}
}

// --- site gating -----------------------------------------------------------

func TestMicroservice_SiteNotFound(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{skipSite: true})
	res := f.reconcile()
	ms := f.ms()
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhasePending {
		t.Fatalf("phase = %s", ms.Status.Phase)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionSiteReady, metav1.ConditionFalse, "SiteNotFound")
	if res.RequeueAfter == 0 {
		t.Fatal("expected a requeue while the site is missing")
	}
	if f.get(msName, &appsv1.Deployment{}) {
		t.Fatal("no Deployment may be created without a site")
	}
}

func TestMicroservice_SiteNotReady(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{site: func(s *vyogotechv1.FrappeSite) { s.Status.Phase = vyogotechv1.FrappeSitePhaseProvisioning }})
	f.reconcile()
	ms := f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionSiteReady, metav1.ConditionFalse, "SiteNotReady")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhasePending {
		t.Fatalf("phase = %s", ms.Status.Phase)
	}
}

func TestMicroservice_DatabaseInfoNotRecordedYet(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{site: func(s *vyogotechv1.FrappeSite) {
		s.Status.DatabaseProvider = ""
		s.Status.DatabaseHost = ""
	}})
	res := f.reconcile()
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionSiteReady, metav1.ConditionFalse, "DatabaseInfoUnavailable")
	if res.RequeueAfter == 0 {
		t.Fatal("expected a requeue until the site records its database info")
	}
}

func TestMicroservice_SQLiteSiteIsUnsupported(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{site: func(s *vyogotechv1.FrappeSite) {
		s.Status.DatabaseProvider = "sqlite"
		s.Status.DatabaseHost = ""
	}})
	f.reconcile()
	ms := f.ms()
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseFailed {
		t.Fatalf("phase = %s", ms.Status.Phase)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionSiteReady, metav1.ConditionFalse, "UnsupportedDatabase")
}

// --- binding, deployment, service -------------------------------------------

func TestMicroservice_InternalOnly(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.Deployment.Replicas = int32Ptr(2)
		ms.Spec.Image.PullSecrets = []corev1.LocalObjectReference{{Name: "regcred"}}
		ms.Spec.Image.PullPolicy = corev1.PullAlways
		ms.Spec.Env = []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "INFO"}}
		ms.Spec.Resources = &vyogotechv1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
		}
	}})
	f.reconcile()

	// Binding Secret
	secret := &corev1.Secret{}
	f.mustGet(msName+"-site", secret)
	want := map[string]string{
		"FRAPPE_SITE":      "erp.acme.com",
		"CENTRAL_SITE_URL": "http://erp-bench-nginx.tenant.svc.cluster.local:8080",
		"PORT":             "8000",
		"DB_TYPE":          "mariadb",
		"DB_HOST":          "mariadb.db.svc.cluster.local",
		"DB_PORT":          "3306",
		"DB_NAME":          "_acme",
		"DB_USER":          "acme_user",
		"DB_PASSWORD":      "s3cret",
	}
	for k, v := range want {
		if got := string(secret.Data[k]); got != v {
			t.Errorf("binding %s = %q, want %q", k, got, v)
		}
	}
	if len(secret.Data) != len(want) {
		t.Errorf("binding has unexpected keys: %v", keysOf(secret.Data))
	}
	expectControlledByMS(t, secret)

	// Deployment
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	expectControlledByMS(t, dep)
	if *dep.Spec.Replicas != 2 {
		t.Errorf("replicas = %d", *dep.Spec.Replicas)
	}
	pod := dep.Spec.Template.Spec
	if len(pod.Containers) != 1 {
		t.Fatalf("containers = %d", len(pod.Containers))
	}
	c := pod.Containers[0]
	if c.Image != "ghcr.io/acme/orders:1.0.0" || c.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("image = %s (%s)", c.Image, c.ImagePullPolicy)
	}
	if len(pod.ImagePullSecrets) != 1 || pod.ImagePullSecrets[0].Name != "regcred" {
		t.Errorf("pull secrets = %+v", pod.ImagePullSecrets)
	}
	if len(c.EnvFrom) != 1 || c.EnvFrom[0].SecretRef == nil || c.EnvFrom[0].SecretRef.Name != msName+"-site" {
		t.Errorf("envFrom = %+v", c.EnvFrom)
	}
	if len(c.Env) != 1 || c.Env[0].Name != "LOG_LEVEL" {
		t.Errorf("env = %+v", c.Env)
	}
	if len(c.Ports) != 1 || c.Ports[0].ContainerPort != 8000 || c.Ports[0].Name != "http" {
		t.Errorf("ports = %+v", c.Ports)
	}
	for name, p := range map[string]*corev1.Probe{"liveness": c.LivenessProbe, "readiness": c.ReadinessProbe} {
		if p == nil || p.HTTPGet == nil || p.HTTPGet.Path != "/health" || p.HTTPGet.Port.StrVal != "http" {
			t.Errorf("%s probe = %+v", name, p)
		}
	}
	if q := c.Resources.Requests[corev1.ResourceCPU]; q.String() != "100m" {
		t.Errorf("cpu request = %s", q.String())
	}
	if q := c.Resources.Limits[corev1.ResourceMemory]; q.String() != "512Mi" {
		t.Errorf("memory limit = %s", q.String())
	}
	if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Errorf("container security context must disallow privilege escalation: %+v", c.SecurityContext)
	}
	if pod.SecurityContext == nil || pod.SecurityContext.RunAsNonRoot == nil || !*pod.SecurityContext.RunAsNonRoot {
		t.Errorf("pod must run as non-root: %+v", pod.SecurityContext)
	}
	if dep.Spec.Template.Annotations[bindingVersionAnnotation] == "" {
		t.Error("pod template must carry the binding hash")
	}
	sel := dep.Spec.Selector.MatchLabels
	for k, v := range sel {
		if dep.Spec.Template.Labels[k] != v {
			t.Errorf("template labels %v do not match selector %v", dep.Spec.Template.Labels, sel)
		}
	}

	// Service
	svc := &corev1.Service{}
	f.mustGet(msName, svc)
	expectControlledByMS(t, svc)
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("service type = %s", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 8000 || svc.Spec.Ports[0].TargetPort.StrVal != "http" {
		t.Errorf("service ports = %+v", svc.Spec.Ports)
	}
	for k, v := range sel {
		if svc.Spec.Selector[k] != v {
			t.Errorf("service selector %v does not match deployment selector %v", svc.Spec.Selector, sel)
		}
	}

	// No external exposure
	if f.get(msName+"-ingress", &networkingv1.Ingress{}) {
		t.Fatal("no Ingress may exist without spec.routing")
	}

	ms := f.ms()
	if ms.Status.ServiceURL != "http://orders.tenant.svc.cluster.local:8000" {
		t.Errorf("serviceURL = %q", ms.Status.ServiceURL)
	}
	if condition(ms, vyogotechv1.MicroserviceConditionRouted) != nil {
		t.Error("Routed must not be reported when spec.routing is absent")
	}
	if len(ms.Status.ExternalURLs) != 0 {
		t.Errorf("externalURLs = %v", ms.Status.ExternalURLs)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionSiteReady, metav1.ConditionTrue, "")
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionAvailable, metav1.ConditionFalse, "")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhasePending {
		t.Errorf("phase = %s before the Deployment is ready", ms.Status.Phase)
	}
}

func TestMicroservice_ReadyOnceDeploymentIsReady(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.reconcile()
	f.markDeploymentReady()
	res := f.reconcile()

	ms := f.ms()
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseReady {
		t.Fatalf("phase = %s; conditions %+v", ms.Status.Phase, ms.Status.Conditions)
	}
	if ms.Status.ReadyReplicas != 1 {
		t.Errorf("readyReplicas = %d", ms.Status.ReadyReplicas)
	}
	if ms.Status.ObservedGeneration != ms.Generation {
		t.Errorf("observedGeneration = %d, generation = %d", ms.Status.ObservedGeneration, ms.Generation)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionAvailable, metav1.ConditionTrue, "")
	if res.RequeueAfter == 0 {
		t.Error("a Ready microservice still resyncs periodically to pick up credential rotation")
	}
}

func TestMicroservice_ZeroReplicasIsAvailable(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Deployment.Replicas = int32Ptr(0) }})
	f.reconcile()
	ms := f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionAvailable, metav1.ConditionTrue, "ScaledToZero")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseReady {
		t.Errorf("phase = %s", ms.Status.Phase)
	}
}

func TestMicroservice_PortFlowsToContainerServiceAndBinding(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Deployment.Port = 9090 }})
	f.reconcile()
	secret := &corev1.Secret{}
	f.mustGet(msName+"-site", secret)
	if string(secret.Data["PORT"]) != "9090" {
		t.Errorf("PORT = %q", secret.Data["PORT"])
	}
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort != 9090 {
		t.Errorf("containerPort = %d", dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
	}
	svc := &corev1.Service{}
	f.mustGet(msName, svc)
	if svc.Spec.Ports[0].Port != 9090 {
		t.Errorf("service port = %d", svc.Spec.Ports[0].Port)
	}
	if got := f.ms().Status.ServiceURL; got != "http://orders.tenant.svc.cluster.local:9090" {
		t.Errorf("serviceURL = %q", got)
	}
}

func TestMicroservice_DefaultImageTagIsLatest(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Image.Tag = "" }})
	f.reconcile()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if img := dep.Spec.Template.Spec.Containers[0].Image; img != "ghcr.io/acme/orders:latest" {
		t.Errorf("image = %s", img)
	}
}

func TestMicroservice_PostgresSiteBindsPostgres(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{site: func(s *vyogotechv1.FrappeSite) {
		s.Status.DatabaseProvider = "postgres"
		s.Status.DatabasePort = "5432"
		s.Spec.DBConfig = vyogotechv1.DatabaseConfig{Provider: "external", ConnectionSecretRef: &corev1.SecretReference{Name: "pg-conn"}}
	}, extra: []client.Object{&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pg-conn", Namespace: msNS},
		Data:       map[string][]byte{"username": []byte("pguser"), "password": []byte("pgpass")},
	}}})
	f.reconcile()
	secret := &corev1.Secret{}
	f.mustGet(msName+"-site", secret)
	if string(secret.Data["DB_TYPE"]) != "postgres" || string(secret.Data["DB_USER"]) != "pguser" || string(secret.Data["DB_PASSWORD"]) != "pgpass" {
		t.Errorf("binding = %v", stringData(secret.Data))
	}
}

func TestMicroservice_CredentialRotationRollsDeployment(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.reconcile()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	before := dep.Spec.Template.Annotations[bindingVersionAnnotation]

	src := &corev1.Secret{}
	f.mustGet("acme-db-password", src)
	src.Data["password"] = []byte("rotated")
	if err := f.client.Update(f.ctx, src); err != nil {
		t.Fatal(err)
	}
	f.reconcile()

	secret := &corev1.Secret{}
	f.mustGet(msName+"-site", secret)
	if string(secret.Data["DB_PASSWORD"]) != "rotated" {
		t.Fatalf("binding not re-synced: DB_PASSWORD = %q", secret.Data["DB_PASSWORD"])
	}
	f.mustGet(msName, dep)
	if after := dep.Spec.Template.Annotations[bindingVersionAnnotation]; after == before {
		t.Fatal("binding hash must change so the Deployment rolls")
	}
}

func TestMicroservice_ReconcileIsIdempotent(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.reconcile()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	rv := dep.ResourceVersion
	f.reconcile()
	f.mustGet(msName, dep)
	if dep.ResourceVersion != rv {
		t.Fatal("a second reconcile with no changes must not rewrite the Deployment")
	}
}

func TestMicroservice_SpecChangesUpdateDeployment(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.reconcile()
	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.Image.Tag = "2.0.0"
		ms.Spec.Deployment.Replicas = int32Ptr(3)
	})
	f.reconcile()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if dep.Spec.Template.Spec.Containers[0].Image != "ghcr.io/acme/orders:2.0.0" || *dep.Spec.Replicas != 3 {
		t.Fatalf("deployment not updated: %s x%d", dep.Spec.Template.Spec.Containers[0].Image, *dep.Spec.Replicas)
	}
}

func TestMicroservice_NameConflictIsNotTakenOver(t *testing.T) {
	foreign := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: msName, Namespace: msNS, Labels: map[string]string{"owner": "someone-else"}},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 1234}}},
	}
	f := newMSFixture(t, msFixtureOpts{extra: []client.Object{foreign}})
	f.reconcile()

	svc := &corev1.Service{}
	f.mustGet(msName, svc)
	if svc.Spec.Ports[0].Port != 1234 || metav1.GetControllerOf(svc) != nil {
		t.Fatal("a Service this CR does not own must be left untouched")
	}
	ms := f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionAvailable, metav1.ConditionFalse, "NameConflict")
	if ms.Status.Phase == vyogotechv1.FrappeMicroservicePhaseReady {
		t.Fatal("phase must not be Ready with a name conflict")
	}
}

// --- Kubernetes Ingress -------------------------------------------------------

func withPaths(paths ...string) func(*vyogotechv1.FrappeMicroservice) {
	return func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.Deployment.Routing = &vyogotechv1.MicroserviceRouting{Paths: paths}
	}
}

func TestMicroservice_RoutedIngress(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withPaths("/orders", "/api/resource/Sales%20Order")})
	f.reconcile()

	ing := &networkingv1.Ingress{}
	f.mustGet(msName+"-ingress", ing)
	expectControlledByMS(t, ing)
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "traefik" {
		t.Errorf("ingressClassName = %v", ing.Spec.IngressClassName)
	}
	if len(ing.Spec.Rules) != 1 || ing.Spec.Rules[0].Host != "erp.acme.com" {
		t.Fatalf("rules = %+v", ing.Spec.Rules)
	}
	paths := ing.Spec.Rules[0].HTTP.Paths
	if len(paths) != 2 {
		t.Fatalf("paths = %+v", paths)
	}
	for i, want := range []string{"/orders", "/api/resource/Sales%20Order"} {
		p := paths[i]
		if p.Path != want || p.PathType == nil || *p.PathType != networkingv1.PathTypePrefix ||
			p.Backend.Service == nil || p.Backend.Service.Name != msName || p.Backend.Service.Port.Number != 8000 {
			t.Errorf("path[%d] = %+v", i, p)
		}
	}
	if len(ing.Spec.TLS) != 0 {
		t.Errorf("no TLS when the site has none: %+v", ing.Spec.TLS)
	}
	for k := range ing.Annotations {
		if strings.HasPrefix(k, "cert-manager.io/") {
			t.Errorf("must not carry cert-manager annotation %s", k)
		}
	}

	// Not routed until the ingress controller publishes an address.
	f.markDeploymentReady()
	f.reconcile()
	ms := f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionRouted, metav1.ConditionFalse, "IngressPending")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhasePending {
		t.Fatalf("phase = %s while the Ingress has no address", ms.Status.Phase)
	}

	f.mustGet(msName+"-ingress", ing)
	ing.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "192.168.1.111"}}
	if err := f.client.Status().Update(f.ctx, ing); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	ms = f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionRouted, metav1.ConditionTrue, "")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseReady {
		t.Fatalf("phase = %s; conditions %+v", ms.Status.Phase, ms.Status.Conditions)
	}
	wantURLs := []string{"http://erp.acme.com/orders", "http://erp.acme.com/api/resource/Sales%20Order"}
	if strings.Join(ms.Status.ExternalURLs, ",") != strings.Join(wantURLs, ",") {
		t.Errorf("externalURLs = %v", ms.Status.ExternalURLs)
	}
}

func TestMicroservice_RoutedIngressReusesSiteTLS(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{
		ms: withPaths("/orders"),
		site: func(s *vyogotechv1.FrappeSite) {
			s.Spec.TLS = vyogotechv1.TLSConfig{Enabled: true, SecretName: "acme-cert"}
		},
	})
	f.reconcile()
	ing := &networkingv1.Ingress{}
	f.mustGet(msName+"-ingress", ing)
	if len(ing.Spec.TLS) != 1 || ing.Spec.TLS[0].SecretName != "acme-cert" || ing.Spec.TLS[0].Hosts[0] != "erp.acme.com" {
		t.Fatalf("tls = %+v", ing.Spec.TLS)
	}
	for k := range ing.Annotations {
		if strings.HasPrefix(k, "cert-manager.io/") {
			t.Fatalf("must not request a second certificate via %s", k)
		}
	}
	if ing.Annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] != "true" {
		t.Errorf("HTTPS redirect annotations missing: %v", ing.Annotations)
	}
	if got := f.ms().Status.ExternalURLs; len(got) != 1 || got[0] != "https://erp.acme.com/orders" {
		t.Errorf("externalURLs = %v", got)
	}
}

func TestMicroservice_EnforcedHTTPSAddsTLS(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withPaths("/orders"), enforceHTTPS: true})
	f.reconcile()
	ing := &networkingv1.Ingress{}
	f.mustGet(msName+"-ingress", ing)
	if len(ing.Spec.TLS) != 1 || ing.Spec.TLS[0].SecretName != "acme-tls" {
		t.Fatalf("tls = %+v", ing.Spec.TLS)
	}
}

func TestMicroservice_RemovingRoutingPrunesIngress(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withPaths("/orders")})
	f.reconcile()
	f.mustGet(msName+"-ingress", &networkingv1.Ingress{})

	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Deployment.Routing = nil })
	f.reconcile()
	if f.get(msName+"-ingress", &networkingv1.Ingress{}) {
		t.Fatal("Ingress must be deleted when routing is removed")
	}
	ms := f.ms()
	if condition(ms, vyogotechv1.MicroserviceConditionRouted) != nil {
		t.Error("Routed must be cleared when routing is removed")
	}
	if len(ms.Status.ExternalURLs) != 0 {
		t.Errorf("externalURLs = %v", ms.Status.ExternalURLs)
	}
}

func TestMicroservice_ChangingPathsUpdatesIngress(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: withPaths("/orders")})
	f.reconcile()
	f.updateMS(withPaths("/invoices"))
	f.reconcile()
	ing := &networkingv1.Ingress{}
	f.mustGet(msName+"-ingress", ing)
	if p := ing.Spec.Rules[0].HTTP.Paths; len(p) != 1 || p[0].Path != "/invoices" {
		t.Fatalf("paths = %+v", p)
	}
}

func TestMicroservice_SiteIngressDisabled(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{
		ms: withPaths("/orders"),
		site: func(s *vyogotechv1.FrappeSite) {
			s.Spec.Ingress = &vyogotechv1.IngressConfig{Enabled: resources.BoolPtr(false)}
		},
	})
	f.reconcile()
	if f.get(msName+"-ingress", &networkingv1.Ingress{}) {
		t.Fatal("no Ingress when the site itself is not exposed")
	}
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionRouted, metav1.ConditionFalse, "SiteIngressDisabled")
}

func TestMicroservice_ForeignIngressIsNotTakenOver(t *testing.T) {
	foreign := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: msName + "-ingress", Namespace: msNS}}
	f := newMSFixture(t, msFixtureOpts{ms: withPaths("/orders"), extra: []client.Object{foreign}})
	f.reconcile()
	ing := &networkingv1.Ingress{}
	f.mustGet(msName+"-ingress", ing)
	if metav1.GetControllerOf(ing) != nil || len(ing.Spec.Rules) != 0 {
		t.Fatal("an Ingress this CR does not own must be left untouched")
	}
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionRouted, metav1.ConditionFalse, "NameConflict")
}

// --- OpenShift Routes ---------------------------------------------------------

func (f *msFixture) routes() []routev1.Route {
	f.t.Helper()
	list := &routev1.RouteList{}
	if err := f.client.List(f.ctx, list, client.InNamespace(msNS), client.MatchingLabels{microserviceLabel: msName}); err != nil {
		f.t.Fatal(err)
	}
	return list.Items
}

func TestMicroservice_OpenShiftRoutes(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{
		openShift: true,
		ms:        withPaths("/orders", "/invoices"),
		site:      func(s *vyogotechv1.FrappeSite) { s.Spec.RouteConfig = &vyogotechv1.RouteConfig{Host: "apps.acme.com"} },
	})
	f.reconcile()
	if f.get(msName+"-ingress", &networkingv1.Ingress{}) {
		t.Fatal("OpenShift uses Routes, not an Ingress")
	}
	routes := f.routes()
	if len(routes) != 2 {
		t.Fatalf("routes = %d", len(routes))
	}
	byPath := map[string]routev1.Route{}
	for _, rt := range routes {
		byPath[rt.Spec.Path] = rt
	}
	for _, p := range []string{"/orders", "/invoices"} {
		rt, ok := byPath[p]
		if !ok {
			t.Fatalf("no route for %s", p)
		}
		expectControlledByMS(t, &rt)
		if rt.Spec.Host != "apps.acme.com" || rt.Spec.To.Name != msName || rt.Spec.Port.TargetPort.StrVal != "http" {
			t.Errorf("route %s = %+v", p, rt.Spec)
		}
		if rt.Spec.TLS == nil || rt.Spec.TLS.Termination != routev1.TLSTerminationEdge ||
			rt.Spec.TLS.InsecureEdgeTerminationPolicy != routev1.InsecureEdgeTerminationPolicyRedirect {
			t.Errorf("route %s tls = %+v", p, rt.Spec.TLS)
		}
	}

	f.markDeploymentReady()
	f.reconcile()
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionRouted, metav1.ConditionFalse, "RoutePending")

	// One admitted, one rejected by the router: the conflict surfaces.
	for _, rt := range f.routes() {
		admitted := corev1.ConditionTrue
		reason := ""
		if rt.Spec.Path == "/invoices" {
			admitted, reason = corev1.ConditionFalse, "HostAlreadyClaimed"
		}
		rt.Status.Ingress = []routev1.RouteIngress{{
			Host:       rt.Spec.Host,
			Conditions: []routev1.RouteIngressCondition{{Type: routev1.RouteAdmitted, Status: admitted, Reason: reason}},
		}}
		if err := f.client.Status().Update(f.ctx, &rt); err != nil {
			t.Fatal(err)
		}
	}
	f.reconcile()
	ms := f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionRouted, metav1.ConditionFalse, "RouteNotAdmitted")
	if c := condition(ms, vyogotechv1.MicroserviceConditionRouted); !strings.Contains(c.Message, "HostAlreadyClaimed") || !strings.Contains(c.Message, "/invoices") {
		t.Errorf("message should name the rejected path and router reason: %q", c.Message)
	}
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhasePending {
		t.Errorf("phase = %s", ms.Status.Phase)
	}

	// Dropping the rejected path prunes its Route and the rest is routed.
	f.updateMS(withPaths("/orders"))
	f.reconcile()
	routes = f.routes()
	if len(routes) != 1 || routes[0].Spec.Path != "/orders" {
		t.Fatalf("routes after prune = %+v", routes)
	}
	ms = f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionRouted, metav1.ConditionTrue, "")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseReady {
		t.Errorf("phase = %s", ms.Status.Phase)
	}
	if got := ms.Status.ExternalURLs; len(got) != 1 || got[0] != "https://apps.acme.com/orders" {
		t.Errorf("externalURLs = %v", got)
	}

	// Removing routing deletes every Route.
	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Deployment.Routing = nil })
	f.reconcile()
	if n := len(f.routes()); n != 0 {
		t.Fatalf("routes after removing routing = %d", n)
	}
}

func TestMicroservice_OpenShiftPassthroughIsRejected(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{
		openShift: true,
		ms:        withPaths("/orders"),
		site: func(s *vyogotechv1.FrappeSite) {
			s.Spec.RouteConfig = &vyogotechv1.RouteConfig{TLSTermination: "passthrough"}
		},
	})
	f.reconcile()
	if n := len(f.routes()); n != 0 {
		t.Fatalf("path-based routes cannot use passthrough; got %d routes", n)
	}
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionRouted, metav1.ConditionFalse, "PassthroughUnsupported")
}

// --- watches -------------------------------------------------------------------

func TestMicroservice_SiteEventsMapToBoundMicroservices(t *testing.T) {
	other := &vyogotechv1.FrappeMicroservice{
		ObjectMeta: metav1.ObjectMeta{Name: "pdf", Namespace: msNS},
		Spec:       vyogotechv1.FrappeMicroserviceSpec{SiteRef: vyogotechv1.LocalSiteReference{Name: "another-site"}},
	}
	f := newMSFixture(t, msFixtureOpts{extra: []client.Object{other}})
	site := &vyogotechv1.FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: msSite, Namespace: msNS}}
	reqs := f.r.requestsForSite(f.ctx, site)
	if len(reqs) != 1 || reqs[0].Name != msName {
		t.Fatalf("requests = %+v", reqs)
	}
}

func TestMicroservice_CredentialSecretEventsMapToBoundMicroservices(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	src := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "acme-db-password", Namespace: msNS}}
	if reqs := f.r.requestsForSecret(f.ctx, src); len(reqs) != 1 || reqs[0].Name != msName {
		t.Fatalf("requests = %+v", reqs)
	}
	unrelated := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "something-else", Namespace: msNS}}
	if reqs := f.r.requestsForSecret(f.ctx, unrelated); len(reqs) != 0 {
		t.Fatalf("unrelated secret mapped to %+v", reqs)
	}
}

// --- helpers -------------------------------------------------------------------

func expectControlledByMS(t *testing.T, obj metav1.Object) {
	t.Helper()
	ref := metav1.GetControllerOf(obj)
	if ref == nil || ref.Kind != "FrappeMicroservice" || ref.Name != msName {
		t.Fatalf("%s must be controlled by the FrappeMicroservice, got %+v", obj.GetName(), ref)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func stringData(m map[string][]byte) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = string(v)
	}
	return out
}

// The status write must not depend on the resourceVersion of the object the
// reconcile started from: the cache often lags the controller's own writes,
// and an Update would fail with a conflict on every such pass.
func TestMicroservice_StatusWriteToleratesStaleResourceVersion(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.r.Client = interceptor.NewClient(f.client.(client.WithWatch), interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return apierrors.NewConflict(vyogotechv1.GroupVersion.WithResource("frappemicroservices").GroupResource(), obj.GetName(), nil)
		},
	})
	f.reconcile()
	if f.ms().Status.ServiceURL == "" {
		t.Fatal("status was not written")
	}
}

// --- command / args ------------------------------------------------------------

func TestMicroservice_CommandAndArgsFlowIntoTheDeployment(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.Command = []string{"python", "-m", "orders"}
		ms.Spec.Args = []string{"--workers", "2"}
	}})
	f.reconcile()
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	c := dep.Spec.Template.Spec.Containers[0]
	if strings.Join(c.Command, " ") != "python -m orders" || strings.Join(c.Args, " ") != "--workers 2" {
		t.Fatalf("command/args = %v %v", c.Command, c.Args)
	}
}

// --- Job ---------------------------------------------------------------------

func asJob(mutate ...func(*vyogotechv1.FrappeMicroservice)) func(*vyogotechv1.FrappeMicroservice) {
	return func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.Type = vyogotechv1.MicroserviceTypeJob
		ms.Spec.Deployment = nil
		ms.Spec.Command = []string{"python", "-m", "orders.backfill"}
		ms.Spec.Args = []string{"--since", "2026-01-01"}
		ms.Spec.Job = &vyogotechv1.MicroserviceJobSpec{
			BackoffLimit:            int32Ptr(2),
			ActiveDeadlineSeconds:   int64Ptr(600),
			TTLSecondsAfterFinished: int32Ptr(3600),
		}
		for _, m := range mutate {
			m(ms)
		}
	}
}

func (f *msFixture) jobs() []batchv1.Job {
	f.t.Helper()
	list := &batchv1.JobList{}
	if err := f.client.List(f.ctx, list, client.InNamespace(msNS), client.MatchingLabels{microserviceLabel: msName}); err != nil {
		f.t.Fatal(err)
	}
	return list.Items
}

func (f *msFixture) onlyJob() *batchv1.Job {
	f.t.Helper()
	jobs := f.jobs()
	if len(jobs) != 1 {
		f.t.Fatalf("expected exactly one Job, got %d", len(jobs))
	}
	return &jobs[0]
}

func (f *msFixture) setJobStatus(job *batchv1.Job, status batchv1.JobStatus) {
	f.t.Helper()
	job.Status = status
	if err := f.client.Status().Update(f.ctx, job); err != nil {
		f.t.Fatal(err)
	}
}

func TestMicroservice_JobRunsOnce(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asJob()})
	f.reconcile()

	job := f.onlyJob()
	expectControlledByMS(t, job)
	if !strings.HasPrefix(job.Name, msName+"-") || len(job.Name) > 63 {
		t.Errorf("job name = %q", job.Name)
	}
	if *job.Spec.BackoffLimit != 2 || *job.Spec.ActiveDeadlineSeconds != 600 || *job.Spec.TTLSecondsAfterFinished != 3600 {
		t.Errorf("run settings = backoff %v deadline %v ttl %v", *job.Spec.BackoffLimit, *job.Spec.ActiveDeadlineSeconds, *job.Spec.TTLSecondsAfterFinished)
	}
	pod := job.Spec.Template.Spec
	if pod.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy = %s", pod.RestartPolicy)
	}
	c := pod.Containers[0]
	if strings.Join(c.Command, " ") != "python -m orders.backfill" || strings.Join(c.Args, " ") != "--since 2026-01-01" {
		t.Errorf("command/args = %v %v", c.Command, c.Args)
	}
	if len(c.EnvFrom) != 1 || c.EnvFrom[0].SecretRef.Name != msName+"-site" {
		t.Errorf("envFrom = %+v", c.EnvFrom)
	}
	if len(c.Ports) != 0 || c.LivenessProbe != nil || c.ReadinessProbe != nil {
		t.Error("a Job container has no ports or probes")
	}
	if c.SecurityContext == nil || pod.SecurityContext == nil {
		t.Error("Job pods use the restricted security context")
	}

	// No long-running objects.
	if f.get(msName, &appsv1.Deployment{}) || f.get(msName, &corev1.Service{}) {
		t.Fatal("a Job creates no Deployment or Service")
	}
	secret := &corev1.Secret{}
	f.mustGet(msName+"-site", secret)
	if _, ok := secret.Data["PORT"]; ok {
		t.Error("PORT is only bound for Deployments")
	}

	ms := f.ms()
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseRunning {
		t.Fatalf("phase = %s", ms.Status.Phase)
	}
	if ms.Status.JobName != job.Name {
		t.Errorf("status.jobName = %q, want %q", ms.Status.JobName, job.Name)
	}
	if ms.Status.ServiceURL != "" {
		t.Errorf("serviceURL = %q for a Job", ms.Status.ServiceURL)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionComplete, metav1.ConditionFalse, "Running")

	// A second reconcile does not start another run.
	f.reconcile()
	if f.onlyJob().Name != job.Name {
		t.Fatal("the same spec must not create another Job")
	}

	f.setJobStatus(job, batchv1.JobStatus{Succeeded: 1, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}})
	f.reconcile()
	ms = f.ms()
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseSucceeded {
		t.Fatalf("phase = %s", ms.Status.Phase)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionComplete, metav1.ConditionTrue, "Succeeded")
}

func TestMicroservice_FailedJob(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asJob()})
	f.reconcile()
	f.setJobStatus(f.onlyJob(), batchv1.JobStatus{Failed: 3, Conditions: []batchv1.JobCondition{{
		Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded", Message: "Job has reached the specified backoff limit",
	}}})
	f.reconcile()
	ms := f.ms()
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseFailed {
		t.Fatalf("phase = %s", ms.Status.Phase)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionComplete, metav1.ConditionFalse, "Failed")
	if c := condition(ms, vyogotechv1.MicroserviceConditionComplete); !strings.Contains(c.Message, "BackoffLimitExceeded") {
		t.Errorf("message should carry the Job's reason: %q", c.Message)
	}
	// A failed run is not retried by the operator; the Job's backoffLimit governs retries.
	f.reconcile()
	if n := len(f.jobs()); n != 1 {
		t.Fatalf("jobs = %d", n)
	}
}

func TestMicroservice_JobSpecChangeStartsANewRun(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asJob()})
	f.reconcile()
	first := f.onlyJob().Name

	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Image.Tag = "1.0.1" })
	f.reconcile()
	second := f.onlyJob()
	if second.Name == first {
		t.Fatal("a spec change must start a new Job")
	}
	if second.Spec.Template.Spec.Containers[0].Image != "ghcr.io/acme/orders:1.0.1" {
		t.Errorf("image = %s", second.Spec.Template.Spec.Containers[0].Image)
	}
	ms := f.ms()
	if ms.Status.JobName != second.Name || ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseRunning {
		t.Errorf("status = %s %s", ms.Status.JobName, ms.Status.Phase)
	}
}

func TestMicroservice_CredentialRotationDoesNotRerunAJob(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asJob()})
	f.reconcile()
	first := f.onlyJob().Name

	src := &corev1.Secret{}
	f.mustGet("acme-db-password", src)
	src.Data["password"] = []byte("rotated")
	if err := f.client.Update(f.ctx, src); err != nil {
		t.Fatal(err)
	}
	f.reconcile()

	if f.onlyJob().Name != first {
		t.Fatal("a one-time Job must not re-run because credentials rotated")
	}
	secret := &corev1.Secret{}
	f.mustGet(msName+"-site", secret)
	if string(secret.Data["DB_PASSWORD"]) != "rotated" {
		t.Fatal("the binding is still re-synced")
	}
}

func TestMicroservice_FinishedJobRemovedByTTLIsNotRecreated(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asJob()})
	f.reconcile()
	job := f.onlyJob()
	f.setJobStatus(job, batchv1.JobStatus{Succeeded: 1, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}})
	f.reconcile()

	// ttlSecondsAfterFinished: the Job controller deletes the finished Job.
	if err := f.client.Delete(f.ctx, job); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	if n := len(f.jobs()); n != 0 {
		t.Fatalf("a finished run must not be recreated after TTL cleanup; jobs = %d", n)
	}
	if ms := f.ms(); ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseSucceeded {
		t.Fatalf("phase = %s", ms.Status.Phase)
	}
}

func TestMicroservice_LongJobNamesAreTruncated(t *testing.T) {
	long := strings.Repeat("j", 80)
	f := newMSFixture(t, msFixtureOpts{ms: asJob()})
	ms := &vyogotechv1.FrappeMicroservice{}
	f.mustGet(msName, ms)
	if name := microserviceJobName(&vyogotechv1.FrappeMicroservice{ObjectMeta: metav1.ObjectMeta{Name: long}, Spec: ms.Spec}); len(name) > 63 {
		t.Fatalf("job name %q is %d characters", name, len(name))
	}
}

// --- CronJob -------------------------------------------------------------------

func asCronJob(mutate ...func(*vyogotechv1.FrappeMicroservice)) func(*vyogotechv1.FrappeMicroservice) {
	return func(ms *vyogotechv1.FrappeMicroservice) {
		asJob()(ms)
		ms.Spec.Type = vyogotechv1.MicroserviceTypeCronJob
		tz := "Australia/Sydney"
		ms.Spec.CronJob = &vyogotechv1.MicroserviceCronJobSpec{
			Schedule:                   "0 2 * * *",
			TimeZone:                   &tz,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: int32Ptr(2),
			FailedJobsHistoryLimit:     int32Ptr(1),
			StartingDeadlineSeconds:    int64Ptr(300),
		}
		for _, m := range mutate {
			m(ms)
		}
	}
}

func TestMicroservice_CronJob(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asCronJob()})
	f.reconcile()

	cj := &batchv1.CronJob{}
	f.mustGet(msName, cj)
	expectControlledByMS(t, cj)
	s := cj.Spec
	if s.Schedule != "0 2 * * *" || s.TimeZone == nil || *s.TimeZone != "Australia/Sydney" || s.ConcurrencyPolicy != batchv1.ForbidConcurrent {
		t.Errorf("schedule settings = %q %v %s", s.Schedule, s.TimeZone, s.ConcurrencyPolicy)
	}
	if *s.SuccessfulJobsHistoryLimit != 2 || *s.FailedJobsHistoryLimit != 1 || *s.StartingDeadlineSeconds != 300 {
		t.Errorf("history/deadline = %v %v %v", *s.SuccessfulJobsHistoryLimit, *s.FailedJobsHistoryLimit, *s.StartingDeadlineSeconds)
	}
	if s.Suspend == nil || *s.Suspend {
		t.Errorf("suspend = %v", s.Suspend)
	}
	js := s.JobTemplate.Spec
	if *js.BackoffLimit != 2 || *js.ActiveDeadlineSeconds != 600 || *js.TTLSecondsAfterFinished != 3600 {
		t.Error("run settings must be applied to every scheduled Job")
	}
	pod := js.Template.Spec
	if pod.RestartPolicy != corev1.RestartPolicyNever || strings.Join(pod.Containers[0].Command, " ") != "python -m orders.backfill" {
		t.Errorf("pod = %s %v", pod.RestartPolicy, pod.Containers[0].Command)
	}
	if js.Template.Annotations[bindingVersionAnnotation] == "" {
		t.Error("the job template carries the binding hash so future runs see rotated credentials")
	}
	if f.get(msName, &appsv1.Deployment{}) || f.get(msName, &corev1.Service{}) || len(f.jobs()) != 0 {
		t.Fatal("a CronJob creates no Deployment, Service or immediate Job")
	}

	ms := f.ms()
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseReady {
		t.Fatalf("phase = %s; conditions %+v", ms.Status.Phase, ms.Status.Conditions)
	}
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionScheduled, metav1.ConditionTrue, "Scheduled")

	last := metav1.NewTime(metav1.Now().Rfc3339Copy().Time)
	cj.Status.LastScheduleTime = &last
	cj.Status.LastSuccessfulTime = &last
	cj.Status.Active = []corev1.ObjectReference{{Name: msName + "-29000000"}}
	if err := f.client.Status().Update(f.ctx, cj); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	ms = f.ms()
	if ms.Status.LastScheduleTime == nil || !ms.Status.LastScheduleTime.Equal(&last) ||
		ms.Status.LastSuccessfulTime == nil || ms.Status.ActiveJobs != 1 {
		t.Errorf("cron status = %v %v %d", ms.Status.LastScheduleTime, ms.Status.LastSuccessfulTime, ms.Status.ActiveJobs)
	}
}

func TestMicroservice_CronJobDefaultsAndSuspend(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asCronJob(func(ms *vyogotechv1.FrappeMicroservice) {
		ms.Spec.CronJob = &vyogotechv1.MicroserviceCronJobSpec{Schedule: "*/5 * * * *"}
	})})
	f.reconcile()
	cj := &batchv1.CronJob{}
	f.mustGet(msName, cj)
	if cj.Spec.ConcurrencyPolicy != batchv1.ForbidConcurrent {
		t.Errorf("concurrencyPolicy defaults to Forbid, got %s", cj.Spec.ConcurrencyPolicy)
	}

	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.CronJob.Suspend = boolPtr(true) })
	f.reconcile()
	f.mustGet(msName, cj)
	if cj.Spec.Suspend == nil || !*cj.Spec.Suspend {
		t.Fatal("suspend must be applied")
	}
	ms := f.ms()
	expectCondition(t, ms, vyogotechv1.MicroserviceConditionScheduled, metav1.ConditionTrue, "Suspended")
	if ms.Status.Phase != vyogotechv1.FrappeMicroservicePhaseReady {
		t.Errorf("phase = %s", ms.Status.Phase)
	}
}

func TestMicroservice_CronJobPicksUpRotatedCredentials(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{ms: asCronJob()})
	f.reconcile()
	cj := &batchv1.CronJob{}
	f.mustGet(msName, cj)
	before := cj.Spec.JobTemplate.Spec.Template.Annotations[bindingVersionAnnotation]

	src := &corev1.Secret{}
	f.mustGet("acme-db-password", src)
	src.Data["password"] = []byte("rotated")
	if err := f.client.Update(f.ctx, src); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	f.mustGet(msName, cj)
	if cj.Spec.JobTemplate.Spec.Template.Annotations[bindingVersionAnnotation] == before {
		t.Fatal("the job template must change when the binding changes")
	}
}

func TestMicroservice_ForeignCronJobIsNotTakenOver(t *testing.T) {
	foreign := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: msName, Namespace: msNS},
		Spec:       batchv1.CronJobSpec{Schedule: "1 1 1 1 1"},
	}
	f := newMSFixture(t, msFixtureOpts{ms: asCronJob(), extra: []client.Object{foreign}})
	f.reconcile()
	cj := &batchv1.CronJob{}
	f.mustGet(msName, cj)
	if cj.Spec.Schedule != "1 1 1 1 1" || metav1.GetControllerOf(cj) != nil {
		t.Fatal("a CronJob this CR does not own must be left untouched")
	}
	expectCondition(t, f.ms(), vyogotechv1.MicroserviceConditionScheduled, metav1.ConditionFalse, "NameConflict")
}

// A child update can race the cache (e.g. the Deployment controller just
// wrote the Deployment). That conflict is routine: requeue quietly instead of
// returning an error, which controller-runtime logs as "Reconciler error".
func TestMicroservice_ChildUpdateConflictRequeuesWithoutError(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.reconcile() // create everything

	f.r.Client = interceptor.NewClient(f.client.(client.WithWatch), interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*appsv1.Deployment); ok {
				return apierrors.NewConflict(appsv1.Resource("deployments"), obj.GetName(), nil)
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	f.updateMS(func(ms *vyogotechv1.FrappeMicroservice) { ms.Spec.Image.Tag = "2.0.0" })

	res, err := f.r.Reconcile(f.ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: msName, Namespace: msNS}})
	if err != nil {
		t.Fatalf("a conflict must not surface as a reconcile error: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatal("a conflict must requeue")
	}
}

// The pod template must not carry anything derived from the database
// credentials: it follows the binding Secret's resourceVersion, which changes
// when the Secret changes and reveals nothing about its contents.
func TestMicroservice_RolloutMarkerIsNotDerivedFromCredentials(t *testing.T) {
	f := newMSFixture(t, msFixtureOpts{})
	f.reconcile()
	secret := &corev1.Secret{}
	f.mustGet(msName+"-site", secret)
	dep := &appsv1.Deployment{}
	f.mustGet(msName, dep)
	if got := dep.Spec.Template.Annotations[bindingVersionAnnotation]; got != secret.ResourceVersion {
		t.Fatalf("marker = %q, want the binding Secret's resourceVersion %q", got, secret.ResourceVersion)
	}
}
