package database

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vyogotechv1 "github.com/vyogotech/frappe-operator/api/v1"
)

// bench-v17/pgtest-e2efull-vyogo-tech hashes (FNV-32a) to a value whose top
// nibble is zero. With an unpadded %x that is a 7-character string, and the
// [:8] slice in generateDBName panicked the reconciler for the first Postgres
// site ever placed on the v17 pool. Any site name has a 1-in-16 chance.
const shortHashSiteName = "pgtest-e2efull-vyogo-tech"

func pgSiteNamed(name string) *vyogotechv1.FrappeSite {
	return &vyogotechv1.FrappeSite{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "bench-v17"},
		Spec:       vyogotechv1.FrappeSiteSpec{SiteName: strings.ReplaceAll(name, "-vyogo-tech", ".vyogo.tech")},
	}
}

func TestPostgresHashStringIsAlwaysEightCharacters(t *testing.T) {
	p := &PostgresProvider{}
	for _, in := range []string{"bench-v17/" + shortHashSiteName, "a", "", "bench-v16/erpnext-xyuxsc-vyogo-tech"} {
		if got := p.hashString(in); len(got) != 8 {
			t.Fatalf("hashString(%q) = %q (len %d), want 8 hex characters", in, got, len(got))
		}
	}
	// Prove the fixture really is a leading-zero case, so the test keeps
	// meaning something if FNV or the name changes.
	if got := p.hashString("bench-v17/" + shortHashSiteName); !strings.HasPrefix(got, "0") {
		t.Fatalf("fixture no longer hashes to a leading zero: %q", got)
	}
}

func TestPostgresNamesDoNotPanicOnShortHash(t *testing.T) {
	p := &PostgresProvider{}
	site := pgSiteNamed(shortHashSiteName)
	db := p.generateDBName(site)
	if !strings.HasPrefix(db, "_0") || len(db) > 63 {
		t.Fatalf("generateDBName = %q", db)
	}
	if user := p.generatePGUserName(site); len(user) != 9 || !strings.HasPrefix(user, "u0") {
		t.Fatalf("generatePGUserName = %q, want u + 8 hex characters", user)
	}
}

// A shared-mode site must be able to reach any PostgreSQL. Deriving the host
// solely from "<postgresRef>-pgbouncer" encodes Percona's topology and locks out
// every other operator: CloudNativePG publishes "<cluster>-rw", StackGres
// "<cluster>", and a managed service is an arbitrary name. dbConfig.host exists
// on the CRD for exactly this and was previously ignored.
func TestGetSharedHostPort(t *testing.T) {
	p := &PostgresProvider{}

	siteWith := func(host, port, refName, refNS string) *vyogotechv1.FrappeSite {
		db := vyogotechv1.DatabaseConfig{Host: host, Port: port}
		if refName != "" {
			db.PostgresRef = &vyogotechv1.NamespacedName{Name: refName, Namespace: refNS}
		}
		return &vyogotechv1.FrappeSite{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "sites"},
			Spec:       vyogotechv1.FrappeSiteSpec{DBConfig: db},
		}
	}

	tests := []struct {
		name     string
		site     *vyogotechv1.FrappeSite
		wantHost string
		wantPort string
	}{
		{
			name:     "explicit host wins over the ref",
			site:     siteWith("db.example.com", "", "frappe-postgres", "frappe-pg"),
			wantHost: "db.example.com",
			wantPort: "5432",
		},
		{
			// An external host must not be suffixed with .svc.cluster.local.
			name:     "explicit host is used verbatim",
			site:     siteWith("my-cnpg-rw.frappe-pg.svc.cluster.local", "", "", ""),
			wantHost: "my-cnpg-rw.frappe-pg.svc.cluster.local",
			wantPort: "5432",
		},
		{
			name:     "explicit port is honoured",
			site:     siteWith("db.example.com", "6432", "", ""),
			wantHost: "db.example.com",
			wantPort: "6432",
		},
		{
			name:     "falls back to the percona convention",
			site:     siteWith("", "", "frappe-postgres", "frappe-pg"),
			wantHost: "frappe-postgres-pgbouncer.frappe-pg.svc.cluster.local",
			wantPort: "5432",
		},
		{
			name:     "ref without a namespace uses the site's",
			site:     siteWith("", "", "cluster-a", ""),
			wantHost: "cluster-a-pgbouncer.sites.svc.cluster.local",
			wantPort: "5432",
		},
		{
			name:     "no host and no ref keeps the historical default",
			site:     siteWith("", "", "", ""),
			wantHost: "frappe-postgres-pgbouncer.sites.svc.cluster.local",
			wantPort: "5432",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := p.getSharedHostPort(context.Background(), tt.site)
			if err != nil {
				t.Fatalf("getSharedHostPort: %v", err)
			}
			if host != tt.wantHost || port != tt.wantPort {
				t.Errorf("got (%q, %q), want (%q, %q)", host, port, tt.wantHost, tt.wantPort)
			}
		})
	}
}
