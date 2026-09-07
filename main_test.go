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

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"k8s.io/client-go/rest"
)

func int32Ptr(n int32) *int32 { return &n }

func TestGetMaxConcurrentReconciles(t *testing.T) {
	tests := []struct {
		name string
		env  string
		set  bool
		want int
	}{
		{name: "unset uses default", set: false, want: defaultMaxConcurrentReconciles},
		{name: "valid override", env: "12", set: true, want: 12},
		{name: "zero ignored", env: "0", set: true, want: defaultMaxConcurrentReconciles},
		{name: "negative ignored", env: "-3", set: true, want: defaultMaxConcurrentReconciles},
		{name: "garbage ignored", env: "abc", set: true, want: defaultMaxConcurrentReconciles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FRAPPE_MAX_CONCURRENT_RECONCILES", "")
			if tt.set {
				t.Setenv("FRAPPE_MAX_CONCURRENT_RECONCILES", tt.env)
			}
			if got := getMaxConcurrentReconciles(); got != tt.want {
				t.Errorf("getMaxConcurrentReconciles() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyClientThrottle(t *testing.T) {
	// Unset env leaves the rest.Config untouched (controller-runtime defaults preserved).
	t.Run("unset preserves config", func(t *testing.T) {
		cfg := &rest.Config{QPS: 20, Burst: 30}
		qps, burst := applyClientThrottle(cfg)
		if qps != 20 || burst != 30 {
			t.Errorf("applyClientThrottle() = (%v,%d), want (20,30)", qps, burst)
		}
	})
	// Valid env overrides QPS/Burst.
	t.Run("env overrides", func(t *testing.T) {
		t.Setenv("FRAPPE_CLIENT_QPS", "75")
		t.Setenv("FRAPPE_CLIENT_BURST", "150")
		cfg := &rest.Config{QPS: 20, Burst: 30}
		qps, burst := applyClientThrottle(cfg)
		if qps != 75 || burst != 150 {
			t.Errorf("applyClientThrottle() = (%v,%d), want (75,150)", qps, burst)
		}
	})
	// Invalid values are ignored.
	t.Run("garbage ignored", func(t *testing.T) {
		t.Setenv("FRAPPE_CLIENT_QPS", "nope")
		t.Setenv("FRAPPE_CLIENT_BURST", "-5")
		cfg := &rest.Config{QPS: 20, Burst: 30}
		qps, burst := applyClientThrottle(cfg)
		if qps != 20 || burst != 30 {
			t.Errorf("applyClientThrottle() = (%v,%d), want (20,30)", qps, burst)
		}
	})
}

func Test_effectiveMaxFromBenches(t *testing.T) {
	tests := []struct {
		name    string
		fromEnv int
		items   []vyogotechv1.FrappeBench
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
			items:   []vyogotechv1.FrappeBench{},
			want:    5,
		},
		{
			name:    "bench override higher than env",
			fromEnv: 10,
			items: []vyogotechv1.FrappeBench{
				{Spec: vyogotechv1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(20)}},
			},
			want: 20,
		},
		{
			name:    "env higher than bench",
			fromEnv: 15,
			items: []vyogotechv1.FrappeBench{
				{Spec: vyogotechv1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(5)}},
			},
			want: 15,
		},
		{
			name:    "max across multiple benches",
			fromEnv: 10,
			items: []vyogotechv1.FrappeBench{
				{Spec: vyogotechv1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(8)}},
				{Spec: vyogotechv1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(25)}},
				{Spec: vyogotechv1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(12)}},
			},
			want: 25,
		},
		{
			name:    "nil siteReconcileConcurrency ignored",
			fromEnv: 10,
			items: []vyogotechv1.FrappeBench{
				{Spec: vyogotechv1.FrappeBenchSpec{SiteReconcileConcurrency: nil}},
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
			items: []vyogotechv1.FrappeBench{
				{Spec: vyogotechv1.FrappeBenchSpec{SiteReconcileConcurrency: int32Ptr(0)}},
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
