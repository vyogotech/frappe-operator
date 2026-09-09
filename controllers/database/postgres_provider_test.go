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

package database

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func pgTestSetup() (*PostgresProvider, *runtime.Scheme) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&batchv1.Job{}).Build()
	return NewPostgresProvider(vyogotechv1.DatabaseConfig{}, cl, scheme).(*PostgresProvider), scheme
}

func pgSite() *vyogotechv1.FrappeSite {
	return &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "pgsite", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "pg.example.com",
			DBConfig: vyogotechv1.DatabaseConfig{Provider: "postgres", Mode: "shared"},
		},
	}
}

func TestPostgresProvider_SharedLifecycle(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()

	// EnsureDatabase (shared): creates a password Secret + a provision Job, returns info.
	info, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if info.Provider != "postgres" || info.Host == "" || info.Name == "" || info.Port != "5432" {
		t.Fatalf("unexpected DatabaseInfo: %+v", info)
	}
	provJob := &batchv1.Job{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-db-provision", Namespace: "default"}, provJob); err != nil {
		t.Fatalf("expected provision Job: %v", err)
	}

	// IsReady is false until the provision Job succeeds.
	ready, err := p.IsReady(ctx, site)
	if err != nil {
		t.Fatalf("IsReady: %v", err)
	}
	if ready {
		t.Error("expected IsReady=false before the provision Job completes")
	}
	provJob.Status.Succeeded = 1
	if err := p.client.Status().Update(ctx, provJob); err != nil {
		t.Fatalf("update job status: %v", err)
	}
	if ready, err = p.IsReady(ctx, site); err != nil || !ready {
		t.Errorf("expected IsReady=true after Job success, got %v err=%v", ready, err)
	}

	// The provider writes the password via StringData; the real apiserver converts that
	// to Data. The fake client doesn't, so simulate it before reading credentials.
	pwSec := &corev1.Secret{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-db-password", Namespace: "default"}, pwSec); err != nil {
		t.Fatalf("get password secret: %v", err)
	}
	if pwSec.Data == nil {
		pwSec.Data = map[string][]byte{}
	}
	for k, v := range pwSec.StringData {
		pwSec.Data[k] = []byte(v)
	}
	if err := p.client.Update(ctx, pwSec); err != nil {
		t.Fatalf("update password secret: %v", err)
	}

	// GetCredentials returns the site password secret.
	creds, err := p.GetCredentials(ctx, site)
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}
	if creds.SecretName != "pgsite-db-password" || creds.Password == "" || creds.Username == "" {
		t.Errorf("unexpected credentials: %+v", creds)
	}
}

func TestPostgresProvider_DeletionPolicy(t *testing.T) {
	ctx := context.Background()

	// Retain (default): Cleanup is a no-op (no delete Job) to preserve tenant data.
	p, _ := pgTestSetup()
	site := pgSite()
	if _, err := p.EnsureDatabase(ctx, site); err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if err := p.Cleanup(ctx, site); err != nil {
		t.Fatalf("Cleanup (retain): %v", err)
	}
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-db-delete", Namespace: "default"}, &batchv1.Job{}); err == nil {
		t.Error("Retain policy must NOT create a delete Job")
	}

	// Delete: Cleanup creates a drop-database Job.
	p2, _ := pgTestSetup()
	site2 := pgSite()
	site2.Spec.DeletionPolicy = "Delete"
	if _, err := p2.EnsureDatabase(ctx, site2); err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if err := p2.Cleanup(ctx, site2); err != nil {
		t.Fatalf("Cleanup (delete): %v", err)
	}
	if err := p2.client.Get(ctx, types.NamespacedName{Name: "pgsite-db-delete", Namespace: "default"}, &batchv1.Job{}); err != nil {
		t.Errorf("Delete policy must create a drop-database Job: %v", err)
	}
}

func TestPostgresProvider_DedicatedClusterShape(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	site.Spec.DBConfig.PostgresEngine = "percona"

	info, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase(dedicated): %v", err)
	}
	if info.Host == "" || info.Port != "5432" {
		t.Fatalf("unexpected DatabaseInfo: %+v", info)
	}

	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(PerconaPGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, cluster); err != nil {
		t.Fatalf("expected PerconaPGCluster: %v", err)
	}

	// The Percona CRD requires postgresVersion, instances[].dataVolumeClaimSpec and
	// backups.pgbackrest.repos — assert the operator emits all of them.
	if _, found, _ := unstructured.NestedInt64(cluster.Object, "spec", "postgresVersion"); !found {
		t.Error("cluster spec.postgresVersion missing")
	}
	repos, found, _ := unstructured.NestedSlice(cluster.Object, "spec", "backups", "pgbackrest", "repos")
	if !found || len(repos) == 0 {
		t.Error("cluster spec.backups.pgbackrest.repos missing")
	}
	insts, found, _ := unstructured.NestedSlice(cluster.Object, "spec", "instances")
	if !found || len(insts) == 0 {
		t.Fatal("cluster spec.instances missing")
	}
	if _, ok := insts[0].(map[string]interface{})["dataVolumeClaimSpec"]; !ok {
		t.Error("instances[0].dataVolumeClaimSpec missing")
	}

	// Users: the superuser (postgres) is exposed for the configure Job, plus the
	// app user. Every user name must be a DNS label (^[a-z0-9]([-a-z0-9]*[a-z0-9])?$),
	// and the app user must match generatePGUserName (for the pguser secret lookup).
	users, _, _ := unstructured.NestedSlice(cluster.Object, "spec", "users")
	if len(users) < 2 {
		t.Fatalf("cluster spec.users should include postgres + app user, got %d", len(users))
	}
	wantUser := p.generatePGUserName(site)
	var sawPostgres, sawApp bool
	for _, u := range users {
		name, _ := u.(map[string]interface{})["name"].(string)
		if !dnsLabel.MatchString(name) {
			t.Errorf("Percona user name %q is not a valid DNS label", name)
		}
		if name == "postgres" {
			sawPostgres = true
		}
		if name == wantUser {
			sawApp = true
		}
	}
	if !sawPostgres {
		t.Error("cluster spec.users must expose the postgres superuser for the configure job")
	}
	if !sawApp {
		t.Errorf("cluster spec.users must include the app user %q", wantUser)
	}
}

func TestPostgresProvider_DedicatedDefaultsToStackGres(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	// No PostgresEngine set, and no pre-existing cluster -> must default to StackGres

	info, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase(dedicated default): %v", err)
	}
	if info.Host == "" || info.Port != "5432" {
		t.Fatalf("unexpected DatabaseInfo: %+v", info)
	}

	sgCluster := &unstructured.Unstructured{}
	sgCluster.SetGroupVersionKind(SGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, sgCluster); err != nil {
		t.Fatalf("expected SGCluster to be created by default for new site: %v", err)
	}

	perconaCluster := &unstructured.Unstructured{}
	perconaCluster.SetGroupVersionKind(PerconaPGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, perconaCluster); err == nil {
		t.Error("PerconaPGCluster must NOT be created when defaulting to StackGres")
	}
}

func TestPostgresProvider_DedicatedGrandfathersExistingPercona(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	// No PostgresEngine set, but Percona cluster already exists in cluster
	existingPercona := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "pgv2.percona.com/v2",
			"kind":       "PerconaPGCluster",
			"metadata": map[string]interface{}{
				"name":      "pgsite-postgres",
				"namespace": "default",
			},
		},
	}
	if err := p.client.Create(ctx, existingPercona); err != nil {
		t.Fatalf("failed to seed existing Percona cluster: %v", err)
	}

	_, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase with grandfathered cluster: %v", err)
	}

	// Should not have created SGCluster
	sgCluster := &unstructured.Unstructured{}
	sgCluster.SetGroupVersionKind(SGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, sgCluster); err == nil {
		t.Error("SGCluster must NOT be created when a pre-existing Percona cluster is grandfathered")
	}
}

func TestPostgresProvider_StackGresClusterShape(t *testing.T) {
	p, _ := pgTestSetup()
	ctx := context.Background()
	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	site.Spec.DBConfig.PostgresEngine = "stackgres"

	info, err := p.EnsureDatabase(ctx, site)
	if err != nil {
		t.Fatalf("EnsureDatabase(stackgres): %v", err)
	}
	if info.Host != "pgsite-postgres.default.svc.cluster.local" || info.Port != "5432" {
		t.Fatalf("unexpected DatabaseInfo: %+v", info)
	}

	// 1. Assert SGCluster exists with correct shape
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(SGClusterGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "pgsite-postgres", Namespace: "default"}, cluster); err != nil {
		t.Fatalf("expected SGCluster: %v", err)
	}

	version, found, _ := unstructured.NestedString(cluster.Object, "spec", "postgres", "version")
	if !found || version != "16" {
		t.Errorf("expected spec.postgres.version='16', got %v", version)
	}
	profile, found, _ := unstructured.NestedString(cluster.Object, "spec", "sgInstanceProfile")
	if !found || profile != "frappe-postgres-dedicated-default" {
		t.Errorf("expected default instance profile, got %v", profile)
	}

	// 2. Assert supporting configs exist
	pgConfig := &unstructured.Unstructured{}
	pgConfig.SetGroupVersionKind(SGPostgresConfigGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "frappe-postgres-dedicated-defaults", Namespace: "default"}, pgConfig); err != nil {
		t.Fatalf("expected SGPostgresConfig: %v", err)
	}

	poolingConfig := &unstructured.Unstructured{}
	poolingConfig.SetGroupVersionKind(SGPoolingConfigGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "frappe-postgres-dedicated-pooling", Namespace: "default"}, poolingConfig); err != nil {
		t.Fatalf("expected SGPoolingConfig: %v", err)
	}

	instProfile := &unstructured.Unstructured{}
	instProfile.SetGroupVersionKind(SGInstanceProfileGVK)
	if err := p.client.Get(ctx, types.NamespacedName{Name: "frappe-postgres-dedicated-default", Namespace: "default"}, instProfile); err != nil {
		t.Fatalf("expected SGInstanceProfile: %v", err)
	}
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func TestPostgresProvider_GenerateDBName(t *testing.T) {
	p, _ := pgTestSetup()
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "n"},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: "My.Weird Site! Name"},
	}
	name := p.generateDBName(site)
	if len(name) == 0 || len(name) > 63 {
		t.Errorf("db name length out of range: %q (%d)", name, len(name))
	}
	if !strings.HasPrefix(name, "_") {
		t.Errorf("db name should start with _ (valid postgres identifier): %q", name)
	}
	if strings.ContainsAny(name, ". !") {
		t.Errorf("db name must be sanitized of special chars: %q", name)
	}
	// Deterministic per site.
	if name != p.generateDBName(site) {
		t.Error("generateDBName must be deterministic")
	}
}

func TestPostgresProvider_BenchInheritance(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&batchv1.Job{}).Build()

	// Provider constructed with bench-level config
	benchConfig := vyogotechv1.DatabaseConfig{
		Provider:       "postgres",
		Mode:           "dedicated",
		PostgresEngine: "percona",
	}
	p := NewPostgresProvider(benchConfig, cl, scheme).(*PostgresProvider)

	// Site has empty dbConfig
	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "inherited-site", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "inherited.example.com",
		},
	}

	delegate, err := p.getDelegate(context.Background(), site)
	if err != nil {
		t.Fatalf("getDelegate failed: %v", err)
	}

	if _, ok := delegate.(*PerconaPostgresProvider); !ok {
		t.Errorf("expected PerconaPostgresProvider via bench inheritance, got: %T", delegate)
	}
}

type mockNoKindMatchClient struct {
	client.Client
}

func (m *mockNoKindMatchClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	return &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "pgv2.percona.com", Kind: "PerconaPGCluster"}}
}

func TestPostgresProvider_NoKindMatchFallback(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	mockCl := &mockNoKindMatchClient{}
	p := NewPostgresProvider(vyogotechv1.DatabaseConfig{Mode: "dedicated"}, mockCl, scheme).(*PostgresProvider)

	site := &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "new-site", Namespace: "default"},
		Spec: vyogotechv1.FrappeSiteSpec{
			SiteName: "new.example.com",
			DBConfig: vyogotechv1.DatabaseConfig{Mode: "dedicated"},
		},
	}

	engine, err := p.resolvePostgresEngine(context.Background(), site)
	if err != nil {
		t.Fatalf("expected fallback to stackgres on NoKindMatchError, got err: %v", err)
	}
	if engine != "stackgres" {
		t.Errorf("expected engine 'stackgres', got: %q", engine)
	}
}

// TestPerconaProvider_StalledClusterSurfacesError covers the failure mode where
// the Percona operator never reconciles a cluster we created (e.g. it runs in
// single-namespace mode and is not watching this namespace). Previously IsReady
// returned (false, nil) forever and the site sat in Provisioning with no
// explanation.
func TestPerconaProvider_StalledClusterSurfacesError(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	site.Spec.DBConfig.PostgresEngine = "percona"

	newCluster := func(age time.Duration, state string) *unstructured.Unstructured {
		obj := map[string]interface{}{
			"apiVersion": "pgv2.percona.com/v2",
			"kind":       "PerconaPGCluster",
			"metadata": map[string]interface{}{
				"name":              "pgsite-postgres",
				"namespace":         "default",
				"creationTimestamp": metav1.NewTime(time.Now().Add(-age)).UTC().Format(time.RFC3339),
			},
		}
		if state != "" {
			obj["status"] = map[string]interface{}{"state": state}
		}
		u := &unstructured.Unstructured{Object: obj}
		return u
	}

	tests := []struct {
		name      string
		age       time.Duration
		state     string
		wantErr   bool
		errSubstr string
	}{
		{name: "fresh cluster with no status is still starting", age: 30 * time.Second, state: "", wantErr: false},
		{name: "long-blank status reports the operator is not reconciling", age: 30 * time.Minute, state: "", wantErr: true, errSubstr: "no status"},
		{name: "explicit error state is surfaced", age: time.Minute, state: "error", wantErr: true, errSubstr: "reported state"},
		{name: "initializing is normal progress", age: time.Minute, state: "initializing", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).
				WithRuntimeObjects(newCluster(tt.age, tt.state)).Build()
			p := NewPerconaPostgresProvider(vyogotechv1.DatabaseConfig{}, cl, scheme)

			ready, err := p.IsReady(context.Background(), site)
			if ready {
				t.Error("expected IsReady=false")
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got: %v", tt.errSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestPostgresProvider_RejectsEngineSwitch verifies the controller-side backstop
// that stops an explicit postgresEngine change from provisioning a second empty
// cluster and orphaning the live one. This runs even where the admission
// webhook is not deployed.
func TestPostgresProvider_RejectsEngineSwitch(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))

	existingSG := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "stackgres.io/v1",
		"kind":       "SGCluster",
		"metadata":   map[string]interface{}{"name": "pgsite-postgres", "namespace": "default"},
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existingSG).Build()
	p := NewPostgresProvider(vyogotechv1.DatabaseConfig{}, cl, scheme).(*PostgresProvider)

	site := pgSite()
	site.Spec.DBConfig.Mode = "dedicated"
	site.Spec.DBConfig.PostgresEngine = "percona" // switching away from the live StackGres cluster

	_, err := p.resolvePostgresEngine(context.Background(), site)
	if err == nil {
		t.Fatal("expected an error when switching engine on a site that already has an SGCluster")
	}
	if !strings.Contains(err.Error(), "SGCluster") {
		t.Errorf("error should name the existing cluster kind, got: %v", err)
	}

	// Keeping the engine that matches the existing cluster must still work.
	site.Spec.DBConfig.PostgresEngine = "stackgres"
	if engine, err := p.resolvePostgresEngine(context.Background(), site); err != nil || engine != "stackgres" {
		t.Errorf("expected stackgres with no error, got %q err=%v", engine, err)
	}
}

// TestPerconaImagesAreFullyQualified guards against a regression that made
// dedicated Percona mode impossible on OpenShift. An unqualified reference such
// as "percona/percona-postgresql-operator:tag" is resolved through the
// cluster's unqualified-search-registries list; on OpenShift that does not
// begin with Docker Hub, so CRI-O resolved it to registry.connect.redhat.com
// and every pod sat in ImagePullBackOff with "name unknown: Image not found".
func TestPerconaImagesAreFullyQualified(t *testing.T) {
	for _, img := range []string{
		defaultPerconaPostgresImage,
		defaultPerconaPGBouncerImage,
		defaultPerconaPGBackRestImage,
	} {
		host := strings.SplitN(img, "/", 2)[0]
		if !strings.Contains(host, ".") && host != "localhost" {
			t.Errorf("image %q is unqualified: registry host %q has no dot, so the cluster's "+
				"unqualified-search-registries list decides where it resolves", img, host)
		}
	}
}
