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

package main

import (
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
)

func int32Ptr(n int32) *int32 { return &n }

func Test_effectiveMaxFromBenches(t *testing.T) {
	tests := []struct {
		name    string
		fromEnv int
		items   []vyogotechv1alpha1.FrappeBench
		want    int
	}{
		{
			name:    "empty benches uses env",
			fromEnv: 10,
			items:   nil,
			want:    10,
		},
		{
			name:    "empty list uses env",
			fromEnv: 5,
			items:   []vyogotechv1alpha1.FrappeBench{},
			want:    5,
		},
		{
			name:    "bench override higher than env",
			fromEnv: 10,
			items: []vyogotechv1alpha1.FrappeBench{
				{Spec: vyogotechv1alpha1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(20)}},
			},
			want: 20,
		},
		{
			name:    "env higher than bench",
			fromEnv: 15,
			items: []vyogotechv1alpha1.FrappeBench{
				{Spec: vyogotechv1alpha1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(5)}},
			},
			want: 15,
		},
		{
			name:    "max across multiple benches",
			fromEnv: 10,
			items: []vyogotechv1alpha1.FrappeBench{
				{Spec: vyogotechv1alpha1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(8)}},
				{Spec: vyogotechv1alpha1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(25)}},
				{Spec: vyogotechv1alpha1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(12)}},
			},
			want: 25,
		},
		{
			name:    "nil siteReconcileConcurrency ignored",
			fromEnv: 10,
			items: []vyogotechv1alpha1.FrappeBench{
				{Spec: vyogotechv1alpha1.FrappeBenchSpec{SiteReconcileConcurrency: nil}},
			},
			want: 10,
		},
		{
			name:    "zero from env clamped to 1",
			fromEnv: 0,
			items:   nil,
			want:    1,
		},
		{
			name:    "negative from env clamped to 1",
			fromEnv: -1,
			items:   nil,
			want:    1,
		},
		{
			name:    "bench zero ignored",
			fromEnv: 10,
			items: []vyogotechv1alpha1.FrappeBench{
				{Spec: vyogotechv1alpha1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(0)}},
			},
			want: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveMaxFromBenches(tt.fromEnv, tt.items)
			if got != tt.want {
				t.Errorf("effectiveMaxFromBenches() = %d, want %d", got, tt.want)
			}
		})
	}
}
