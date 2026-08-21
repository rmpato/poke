package environment

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExpand(t *testing.T) {
	vars := map[string]string{"token": "sk-live-abc", "base": "https://api.example.com"}

	args := []string{"-H", "Authorization: Bearer {{token}}", "{{base}}/users"}
	got, missing := Expand(args, vars)

	want := []string{"-H", "Authorization: Bearer sk-live-abc", "https://api.example.com/users"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Expand = %q, want %q", got, want)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

func TestExpandToleratesWhitespaceAndRepeats(t *testing.T) {
	got, _ := Expand([]string{"{{ base }}/a/{{base}}"}, map[string]string{"base": "X"})
	if got[0] != "X/a/X" {
		t.Errorf("Expand = %q", got[0])
	}
}

// An undefined variable is left as written. Replacing it with an empty string
// would turn https://{{base}}/users into https:///users — a request that fails
// for a reason nobody can see.
func TestExpandLeavesUnknownReferencesVisible(t *testing.T) {
	got, missing := Expand([]string{"{{base}}/users", "-H", "X: {{nope}}"}, map[string]string{})

	if got[0] != "{{base}}/users" {
		t.Errorf("unknown reference was substituted: %q", got[0])
	}
	if !reflect.DeepEqual(missing, []string{"base", "nope"}) {
		t.Errorf("missing = %v, want both names reported", missing)
	}
}

func TestExpandLeavesOrdinaryCommandsUntouched(t *testing.T) {
	args := []string{"-X", "POST", "-d", `{"a":1}`, "https://api.example.com/x?y={z}"}
	got, missing := Expand(args, map[string]string{"a": "b"})

	if !reflect.DeepEqual(got, args) {
		t.Errorf("a command with no variables was altered:\n got  %q\n want %q", got, args)
	}
	if missing != nil {
		t.Errorf("missing = %v", missing)
	}
}

func TestReferences(t *testing.T) {
	got := References([]string{"-H", "A: {{token}}", "{{base}}/x/{{token}}"})
	if !reflect.DeepEqual(got, []string{"token", "base"}) {
		t.Errorf("References = %v, want first-seen order without repeats", got)
	}
	if UsesVariables([]string{"https://x.com"}) {
		t.Error("UsesVariables should be false for a plain command")
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	set, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("a missing environments file must not be an error: %v", err)
	}
	if len(set.Names()) != 0 {
		t.Error("expected an empty set")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "environments.yaml")
	set := Set{
		Active: "prod",
		Shared: map[string]Vars{
			"prod":    {"ua": "pogo"},
			"staging": {"ua": "pogo"},
		},
		APIs: map[string]map[string]Vars{
			"example.com": {
				"prod":    {"token": "sk-live", "base": "https://api.example.com"},
				"staging": {"token": "sk-test", "base": "https://staging.example.com"},
			},
		},
	}
	if err := set.Save(path); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// This file holds the tokens that history deliberately does not.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("environments file mode = %o, want 600", perm)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active != "prod" || got.Vars("example.com", "staging")["token"] != "sk-test" {
		t.Errorf("round trip = %+v", got)
	}
	// A shared variable reaches an API that never mentioned it.
	if got.Vars("example.com", "staging")["ua"] != "pogo" {
		t.Error("a shared variable should apply to every API")
	}
	// Production first: the environment list reads like a pipeline backwards.
	if !reflect.DeepEqual(got.Names(), []string{"prod", "staging"}) {
		t.Errorf("Names = %v, want production first", got.Names())
	}
}

// An API's own value of a variable beats the shared one, because the more
// specific statement is the one the user meant.
func TestAPIVarsOverrideShared(t *testing.T) {
	set := Set{
		Shared: map[string]Vars{"staging": {"base": "https://shared.example", "ua": "pogo"}},
		APIs: map[string]map[string]Vars{
			"acme.com": {"staging": {"base": "https://api.staging.acme.com"}},
		},
	}
	vars := set.Vars("acme.com", "staging")
	if vars["base"] != "https://api.staging.acme.com" {
		t.Errorf("base = %q, want the API's own value", vars["base"])
	}
	if vars["ua"] != "pogo" {
		t.Errorf("ua = %q, want the shared value to survive", vars["ua"])
	}
	// An API with nothing of its own still gets the shared set.
	if set.Vars("other.com", "staging")["base"] != "https://shared.example" {
		t.Error("an API with no variables of its own should still see shared ones")
	}
	// An environment nobody defines is nil, which is how a typo is caught.
	if set.Vars("acme.com", "nope") != nil {
		t.Error("an unknown environment should resolve to nothing")
	}
}

func TestLoadReportsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "environments.yaml")
	_ = os.WriteFile(path, []byte("active: [oops\n"), 0o600)

	if _, err := Load(path); err == nil {
		t.Error("a malformed environments file should be reported, not ignored")
	}
}

func TestDescribe(t *testing.T) {
	set := Set{APIs: map[string]map[string]Vars{
		"acme.com": {"prod": {"a": "1", "b": "2"}},
	}}
	if got := set.Describe("acme.com", "prod"); !strings.Contains(got, "2 variables") {
		t.Errorf("Describe = %q", got)
	}
	if got := set.Describe("acme.com", "nope"); !strings.Contains(got, "no variables") {
		t.Errorf("Describe = %q", got)
	}
}
