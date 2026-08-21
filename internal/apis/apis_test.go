package apis

import (
	"reflect"
	"testing"

	"github.com/rmpato/poke/internal/config"
)

func TestDomainIsTheRegistrableDomain(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		{"api.acme.com", "acme.com"},
		{"api.staging.acme.com", "acme.com"},
		{"acme.com", "acme.com"},
		{"API.Acme.COM", "acme.com"},
		{"api.acme.com:8443", "acme.com"},

		// A multi-part public suffix is not two labels: acme.co.uk is one API,
		// and co.uk is not the domain of every British company at once.
		{"api.acme.co.uk", "acme.co.uk"},

		// github.io is itself a public suffix, so each user's pages site is its
		// own registrable domain rather than all of them being "github.io".
		{"rmpato.github.io", "rmpato.github.io"},

		// There is nothing above these to group by, so they are their own.
		{"localhost", "localhost"},
		{"localhost:3000", "localhost"},
		{"127.0.0.1:8080", "127.0.0.1"},
		{"10.0.0.5", "10.0.0.5"},
		{"[::1]:8080", "::1"},
		{"gateway", "gateway"},
		{"", ""},
	} {
		if got := Domain(tc.host); got != tc.want {
			t.Errorf("Domain(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestGuessEnvReadsTheSubdomain(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		// Unqualified means production. That is what api.acme.com means
		// everywhere, and filing most of a user's history under a caveat would
		// be worse than being occasionally wrong.
		{"api.acme.com", EnvProd},
		{"acme.com", EnvProd},
		{"www.acme.com", EnvProd},

		{"api.staging.acme.com", EnvStaging},
		{"staging.acme.com", EnvStaging},
		{"stg-api.acme.com", EnvStaging},
		{"dev-api.acme.com", EnvDev},
		{"api.dev.acme.com", EnvDev},
		{"qa.acme.com", EnvTest},
		{"uat.acme.com", EnvTest},
		{"sandbox.acme.com", EnvSandbox},

		// Longest match wins, or preprod would read as prod with a prefix.
		{"preprod.acme.com", EnvPreprod},
		{"pre-prod.acme.com", EnvPreprod},

		{"localhost:3000", EnvLocal},
		{"127.0.0.1", EnvLocal},
		{"api.acme.local", EnvLocal},
		{"acme.test", EnvLocal},
		{"10.0.0.5", EnvDev},

		// Only the labels above the registrable domain are read, so a company
		// that is actually called Staging is not mistaken for an environment:
		// staging.co.uk *is* the API, and its www is production.
		{"staging.co.uk", EnvProd},
		{"www.staging.co.uk", EnvProd},
		{"api.staging.staging.co.uk", EnvStaging},
	} {
		if got := GuessEnv(tc.host); got != tc.want {
			t.Errorf("GuessEnv(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

// Every inference is overridable, and an override always wins — otherwise a
// host pogo reads wrongly stays wrong forever.
func TestOverridesBeatGuesses(t *testing.T) {
	reg := config.APIRegistry{
		Names:   map[string]string{"acme.com": "Acme"},
		Domains: map[string]string{"localhost:3000": "acme.com"},
		Envs:    map[string]string{"api-2.acme.com": "staging"},
	}

	if ref := Classify("https://api-2.acme.com/v1", reg); ref.Env != "staging" || !ref.Pinned {
		t.Errorf("pinned env = %q pinned=%v, want staging and pinned", ref.Env, ref.Pinned)
	}
	// A guess is not a pin: the UI shows the difference, so the flag has to be
	// false when pogo worked it out itself.
	if ref := Classify("https://api.acme.com/v1", reg); ref.Pinned {
		t.Error("a guessed environment must not claim to be pinned")
	}
	if ref := Classify("http://localhost:3000/v1", reg); ref.Domain != "acme.com" {
		t.Errorf("moved host domain = %q, want acme.com", ref.Domain)
	}
	if ref := Classify("https://api.acme.com/v1", reg); ref.Name != "Acme" {
		t.Errorf("name = %q, want the display name", ref.Name)
	}
	if ref := Classify("", reg); ref != (Ref{}) {
		t.Errorf("Classify(\"\") = %+v, want the zero value", ref)
	}
}

func TestSummarizeGroupsAndOrders(t *testing.T) {
	reg := config.APIRegistry{}
	refs := []Ref{
		{Host: "api.acme.com", Domain: "acme.com", Env: EnvProd},
		{Host: "api.acme.com", Domain: "acme.com", Env: EnvProd},
		{Host: "api.staging.acme.com", Domain: "acme.com", Env: EnvStaging},
		{Host: "api2.staging.acme.com", Domain: "acme.com", Env: EnvStaging},
		{Host: "api.other.com", Domain: "other.com", Env: EnvProd},
		{Domain: ""}, // an unclassifiable request is dropped, not bucketed
	}

	got := Summarize(refs, reg)
	if len(got) != 2 {
		t.Fatalf("got %d APIs, want 2", len(got))
	}
	// Busiest API first: the sidebar is a place to find things, and the thing
	// you are most likely looking for is the one you call most.
	if got[0].Domain != "acme.com" || got[0].Count != 4 {
		t.Errorf("first API = %s (%d), want acme.com (4)", got[0].Domain, got[0].Count)
	}
	// Production first inside an API, then down the pipeline.
	var envs []string
	for _, e := range got[0].Envs {
		envs = append(envs, e.Name)
	}
	if !reflect.DeepEqual(envs, []string{EnvProd, EnvStaging}) {
		t.Errorf("envs = %v, want production first", envs)
	}
	if hosts := got[0].Envs[1].Hosts; !reflect.DeepEqual(hosts, []string{"api2.staging.acme.com", "api.staging.acme.com"}) &&
		len(hosts) != 2 {
		t.Errorf("staging hosts = %v, want both", hosts)
	}
}
