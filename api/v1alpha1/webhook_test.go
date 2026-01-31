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

package v1alpha1

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
