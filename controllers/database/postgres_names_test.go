package database

import (
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
