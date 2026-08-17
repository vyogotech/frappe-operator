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
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

func pgTestSetup() (*PostgresProvider, *runtime.Scheme) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vyogotechv1.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&batchv1.Job{}).Build()
	return &PostgresProvider{client: cl, scheme: scheme}, scheme
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
