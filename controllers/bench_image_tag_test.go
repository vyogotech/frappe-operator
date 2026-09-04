package controllers

import "testing"

// benchImageTag must map a bare Frappe major onto the version-N tag the
// published bench images actually use; "v16" never existed on any registry.
func TestBenchImageTag(t *testing.T) {
	cases := map[string]string{
		"16":         "version-16",
		"15":         "version-15",
		"16.0.0":     "v16.0.0",
		"v16.0.0":    "v16.0.0",
		"version-16": "version-16",
		"develop":    "develop",
		"latest":     "latest",
		"":           "",
	}
	for in, want := range cases {
		if got := benchImageTag(in); got != want {
			t.Errorf("benchImageTag(%q) = %q, want %q", in, got, want)
		}
	}
}
