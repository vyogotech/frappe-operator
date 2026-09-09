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
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFrappeBenchValidateCreate(t *testing.T) {
	tests := []struct {
		name    string
		bench   *FrappeBench
		wantErr bool
	}{
		{
			name: "valid bench",
			bench: &FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bench",
				},
				Spec: FrappeBenchSpec{
					FrappeVersion: "version-15",
					Apps: []AppSource{
						{Name: "frappe", Source: "git", GitURL: "https://github.com/frappe/frappe"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing frappe version",
			bench: &FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bench",
				},
				Spec: FrappeBenchSpec{
					Apps: []AppSource{
						{Name: "frappe", Source: "git", GitURL: "https://github.com/frappe/frappe"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no apps",
			bench: &FrappeBench{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bench",
				},
				Spec: FrappeBenchSpec{
					FrappeVersion: "version-15",
					Apps:          []AppSource{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.bench.ValidateCreate(context.TODO(), tt.bench)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFrappeSiteValidateCreate(t *testing.T) {
	tests := []struct {
		name    string
		site    *FrappeSite
		wantErr bool
	}{
		{
			name: "valid site",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-site",
				},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
					BenchRef: &NamespacedName{
						Name: "test-bench",
					},
					DBConfig: DatabaseConfig{
						Mode: "shared",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid site with empty DBConfig",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-site",
				},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
					BenchRef: &NamespacedName{
						Name: "test-bench",
					},
					DBConfig: DatabaseConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "empty site name",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-site",
				},
				Spec: FrappeSiteSpec{
					SiteName: "",
					BenchRef: &NamespacedName{
						Name: "test-bench",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing bench ref",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-site",
				},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid db mode",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-site",
				},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
					BenchRef: &NamespacedName{
						Name: "test-bench",
					},
					DBConfig: DatabaseConfig{
						Mode: "invalid",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "dedicated mode without mariadb ref",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-site",
				},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
					BenchRef: &NamespacedName{
						Name: "test-bench",
					},
					DBConfig: DatabaseConfig{
						Mode: "dedicated",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "postgres shared is valid without any ref",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
					BenchRef: &NamespacedName{Name: "test-bench"},
					DBConfig: DatabaseConfig{Provider: "postgres", Mode: "shared"},
				},
			},
			wantErr: false,
		},
		{
			name: "postgres dedicated is valid without a ref (operator provisions cluster)",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
					BenchRef: &NamespacedName{Name: "test-bench"},
					DBConfig: DatabaseConfig{Provider: "postgres", Mode: "dedicated"},
				},
			},
			wantErr: false,
		},
		{
			name: "postgres provider with a mariadbRef is rejected",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
				Spec: FrappeSiteSpec{
					SiteName: "test.local",
					BenchRef: &NamespacedName{Name: "test-bench"},
					DBConfig: DatabaseConfig{
						Provider:   "postgres",
						MariaDBRef: &NamespacedName{Name: "mdb"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid deletionPolicy is rejected",
			site: &FrappeSite{
				ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
				Spec: FrappeSiteSpec{
					SiteName:       "test.local",
					BenchRef:       &NamespacedName{Name: "test-bench"},
					DeletionPolicy: "Purge",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.site.ValidateCreate(context.TODO(), tt.site)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFrappeBenchValidateUpdate(t *testing.T) {
	validBench := &FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "test-bench"},
		Spec: FrappeBenchSpec{
			FrappeVersion: "version-15",
			Apps:          []AppSource{{Name: "frappe", Source: "git", GitURL: "https://github.com/frappe/frappe"}},
		},
	}
	invalidBench := &FrappeBench{
		ObjectMeta: metav1.ObjectMeta{Name: "test-bench"},
		Spec:       FrappeBenchSpec{},
	}
	_, err := validBench.ValidateUpdate(context.TODO(), validBench, validBench)
	if err != nil {
		t.Errorf("ValidateUpdate(valid) error = %v", err)
	}
	_, err = invalidBench.ValidateUpdate(context.TODO(), invalidBench, invalidBench)
	if err == nil {
		t.Error("ValidateUpdate(invalid) expected error")
	}
}

func TestFrappeBenchValidateDelete(t *testing.T) {
	b := &FrappeBench{ObjectMeta: metav1.ObjectMeta{Name: "test-bench"}}
	warnings, err := b.ValidateDelete(context.TODO(), b)
	if err != nil {
		t.Errorf("ValidateDelete() error = %v", err)
	}
	if warnings != nil {
		t.Errorf("ValidateDelete() expected nil warnings, got %v", warnings)
	}
}

func TestFrappeSiteValidateUpdate(t *testing.T) {
	validSite := &FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
		Spec: FrappeSiteSpec{
			SiteName: "test.local",
			BenchRef: &NamespacedName{Name: "test-bench"},
			DBConfig: DatabaseConfig{Mode: "shared"},
		},
	}
	invalidSite := &FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
		Spec:       FrappeSiteSpec{},
	}
	_, err := validSite.ValidateUpdate(context.TODO(), validSite, validSite)
	if err != nil {
		t.Errorf("ValidateUpdate(valid) error = %v", err)
	}
	_, err = invalidSite.ValidateUpdate(context.TODO(), invalidSite, invalidSite)
	if err == nil {
		t.Error("ValidateUpdate(invalid) expected error")
	}
}

func TestFrappeSiteValidateDelete(t *testing.T) {
	s := &FrappeSite{ObjectMeta: metav1.ObjectMeta{Name: "test-site"}}
	warnings, err := s.ValidateDelete(context.TODO(), s)
	if err != nil {
		t.Errorf("ValidateDelete() error = %v", err)
	}
	if warnings != nil {
		t.Errorf("ValidateDelete() expected nil warnings, got %v", warnings)
	}
}

func TestFrappeSiteValidatePostgresEngine(t *testing.T) {
	baseSite := func(provider, mode, engine string) *FrappeSite {
		return &FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
			Spec: FrappeSiteSpec{
				SiteName: "test.local",
				BenchRef: &NamespacedName{Name: "test-bench"},
				DBConfig: DatabaseConfig{
					Provider:       provider,
					Mode:           mode,
					PostgresEngine: engine,
				},
			},
		}
	}

	tests := []struct {
		name     string
		site     *FrappeSite
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid stackgres engine on postgres",
			site:    baseSite("postgres", "dedicated", "stackgres"),
			wantErr: false,
		},
		{
			name:    "valid percona engine on postgres",
			site:    baseSite("postgres", "dedicated", "percona"),
			wantErr: false,
		},
		{
			name:    "invalid engine value",
			site:    baseSite("postgres", "dedicated", "mysql"),
			wantErr: true,
		},
		{
			name:    "postgresEngine on mariadb provider rejected",
			site:    baseSite("mariadb", "shared", "stackgres"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.site.ValidateCreate(context.TODO(), tt.site)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFrappeSiteValidateUpdate_PostgresEngineImmutability(t *testing.T) {
	oldSite := &FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
		Spec: FrappeSiteSpec{
			SiteName: "test.local",
			BenchRef: &NamespacedName{Name: "test-bench"},
			DBConfig: DatabaseConfig{
				Provider:       "postgres",
				Mode:           "dedicated",
				PostgresEngine: "percona",
			},
		},
	}

	// Changing to stackgres should fail
	newSiteChanged := oldSite.DeepCopy()
	newSiteChanged.Spec.DBConfig.PostgresEngine = "stackgres"

	_, err := newSiteChanged.ValidateUpdate(context.TODO(), oldSite, newSiteChanged)
	if err == nil {
		t.Error("ValidateUpdate expected error when changing postgresEngine on dedicated cluster, got nil")
	}

	// Keeping percona should succeed
	newSiteSame := oldSite.DeepCopy()
	_, err = newSiteSame.ValidateUpdate(context.TODO(), oldSite, newSiteSame)
	if err != nil {
		t.Errorf("ValidateUpdate expected success when keeping postgresEngine, got %v", err)
	}

	// Dedicated site created with empty postgresEngine (defaults to stackgres)
	oldSiteDefault := &FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "test-site-default"},
		Spec: FrappeSiteSpec{
			SiteName: "test.local",
			BenchRef: &NamespacedName{Name: "test-bench"},
			DBConfig: DatabaseConfig{
				Provider: "postgres",
				Mode:     "dedicated",
			},
		},
	}

	// Attempting to change to percona should fail
	newSiteSwitchPercona := oldSiteDefault.DeepCopy()
	newSiteSwitchPercona.Spec.DBConfig.PostgresEngine = "percona"
	_, err = newSiteSwitchPercona.ValidateUpdate(context.TODO(), oldSiteDefault, newSiteSwitchPercona)
	if err == nil {
		t.Error("ValidateUpdate expected error when changing defaulted postgresEngine to percona, got nil")
	}

	// Updating other fields without changing default engine should succeed
	newSiteDefaultSame := oldSiteDefault.DeepCopy()
	newSiteDefaultSame.Spec.SiteName = "new-name.local"
	_, err = newSiteDefaultSame.ValidateUpdate(context.TODO(), oldSiteDefault, newSiteDefaultSame)
	if err != nil {
		t.Errorf("ValidateUpdate expected success when keeping default engine, got %v", err)
	}
}

// TestFrappeSiteValidateIngress_HTTPSPolicyIsOperatorWide asserts that the
// webhook itself does not reject ssl-redirect/force-ssl-redirect overrides:
// whether HTTPS is mandatory is the operator-wide FRAPPE_ENFORCE_HTTPS setting
// (see controllers/tls_policy.go), which this type-level validator has no
// access to. Enforcement happens in the FrappeSite controller, which strips
// any insecure override before creating the Ingress when the policy is on.
func TestFrappeSiteValidateIngress_HTTPSPolicyIsOperatorWide(t *testing.T) {
	site := &FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: "test-site"},
		Spec: FrappeSiteSpec{
			SiteName: "test.local",
			BenchRef: &NamespacedName{Name: "test-bench"},
			Ingress: &IngressConfig{
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/ssl-redirect": "false",
				},
			},
		},
	}

	if _, err := site.ValidateCreate(context.TODO(), site); err != nil {
		t.Errorf("expected ValidateCreate to allow ssl-redirect=false (enforcement is operator-wide, not webhook-level), got: %v", err)
	}

	site.Spec.Ingress.Annotations = map[string]string{
		"nginx.ingress.kubernetes.io/force-ssl-redirect": "false",
	}
	if _, err := site.ValidateCreate(context.TODO(), site); err != nil {
		t.Errorf("expected ValidateCreate to allow force-ssl-redirect=false, got: %v", err)
	}

	site.Spec.Ingress.Annotations = map[string]string{
		"nginx.ingress.kubernetes.io/proxy-body-size": "50m",
	}
	if _, err := site.ValidateCreate(context.TODO(), site); err != nil {
		t.Errorf("expected ValidateCreate to succeed with valid annotations, got: %v", err)
	}
}
