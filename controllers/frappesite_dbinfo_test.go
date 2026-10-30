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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
	"github.com/vyogotech/frappe-operator/controllers/database"
)

func TestRecordDatabaseInfo(t *testing.T) {
	t.Run("copies host, port, name and provider into status", func(t *testing.T) {
		site := &vyogotechv1.FrappeSite{}
		recordDatabaseInfo(site, &database.DatabaseInfo{Host: "mariadb.db.svc", Port: "3306", Name: "_abc", Provider: "mariadb"}, "mariadb")
		s := site.Status
		if s.DatabaseHost != "mariadb.db.svc" || s.DatabasePort != "3306" || s.DatabaseName != "_abc" || s.DatabaseProvider != "mariadb" {
			t.Fatalf("unexpected status %+v", s)
		}
	})

	t.Run("falls back to the configured provider when the info has none", func(t *testing.T) {
		site := &vyogotechv1.FrappeSite{}
		recordDatabaseInfo(site, &database.DatabaseInfo{Host: "pg", Port: "5432", Name: "db"}, "postgres")
		if site.Status.DatabaseProvider != "postgres" {
			t.Fatalf("expected provider postgres, got %q", site.Status.DatabaseProvider)
		}
	})

	t.Run("a nil info leaves status untouched", func(t *testing.T) {
		site := &vyogotechv1.FrappeSite{Status: vyogotechv1.FrappeSiteStatus{DatabaseHost: "keep"}}
		recordDatabaseInfo(site, nil, "mariadb")
		if site.Status.DatabaseHost != "keep" {
			t.Fatalf("status was modified: %+v", site.Status)
		}
	})
}

func TestSiteReadyAndUnchanged(t *testing.T) {
	ready := func(mutate func(*vyogotechv1.FrappeSite)) *vyogotechv1.FrappeSite {
		s := &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Generation: 3},
			Status: vyogotechv1.FrappeSiteStatus{
				Phase:              vyogotechv1.FrappeSitePhaseReady,
				ObservedGeneration: 3,
				DatabaseProvider:   "mariadb",
				DatabaseHost:       "mariadb",
			},
		}
		if mutate != nil {
			mutate(s)
		}
		return s
	}

	cases := []struct {
		name string
		site *vyogotechv1.FrappeSite
		want bool
	}{
		{"ready and unchanged", ready(nil), true},
		{"generation changed", ready(func(s *vyogotechv1.FrappeSite) { s.Generation = 4 }), false},
		{"not ready", ready(func(s *vyogotechv1.FrappeSite) { s.Status.Phase = vyogotechv1.FrappeSitePhaseProvisioning }), false},
		{"being deleted", ready(func(s *vyogotechv1.FrappeSite) {
			now := metav1.Now()
			s.DeletionTimestamp = &now
		}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := siteReadyAndUnchanged(tc.site, ""); got != tc.want {
				t.Fatalf("siteReadyAndUnchanged() = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("site-version annotation change forces a reconcile", func(t *testing.T) {
		s := ready(func(s *vyogotechv1.FrappeSite) { s.Status.ObservedSiteVersion = "1" })
		if siteReadyAndUnchanged(s, "2") {
			t.Fatal("expected a reconcile when the site version changes")
		}
	})
}

func TestNeedsDatabaseInfoBackfill(t *testing.T) {
	if !needsDatabaseInfoBackfill(&vyogotechv1.FrappeSite{}) {
		t.Fatal("a site without a recorded provider needs a backfill")
	}
	sqlite := &vyogotechv1.FrappeSite{Status: vyogotechv1.FrappeSiteStatus{DatabaseProvider: "sqlite"}}
	if needsDatabaseInfoBackfill(sqlite) {
		t.Fatal("a sqlite site has no host to record and must not be backfilled repeatedly")
	}
	recorded := &vyogotechv1.FrappeSite{Status: vyogotechv1.FrappeSiteStatus{DatabaseProvider: "mariadb", DatabaseHost: "db"}}
	if needsDatabaseInfoBackfill(recorded) {
		t.Fatal("a recorded site must not be backfilled")
	}
}
