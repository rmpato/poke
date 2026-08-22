package cli

import (
	"reflect"
	"testing"

	"github.com/rmpato/pogo/internal/curlargs"
)

// The wrapper's whole promise is that curl sees exactly what you typed. A curl
// command line is full of things that look like flags to another parser —
// -sSL, --data-raw, a bare --, arguments that begin with a dash — and every one
// of them has to survive untouched.
func TestCurlArgumentsSurviveUnchanged(t *testing.T) {
	args := []string{
		"-sSL", "--compressed", "-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer sk-live-xyz",
		"--data-raw", `{"note":"--not-a-flag"}`,
		"-o", "-", "--", "https://api.acme.com/v1/orders",
	}
	want := append([]string(nil), args...)

	got, flags, err := extractFlags(args)
	if err != nil {
		t.Fatalf("extractFlags: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("curl args were rewritten:\n got %q\nwant %q", got, want)
	}
	if flags != (pogoFlags{}) {
		t.Errorf("flags = %+v, want none of pogo's own", flags)
	}
}

func TestPogoFlagsAreExtracted(t *testing.T) {
	args := []string{
		"--pogo-env", "staging",
		"-sS",
		"--pogo-note=first login",
		"--pogo-no-capture",
		"--pogo-api", "acme.com",
		"https://api.acme.com/",
	}

	got, flags, err := extractFlags(args)
	if err != nil {
		t.Fatalf("extractFlags: %v", err)
	}
	if want := []string{"-sS", "https://api.acme.com/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("curl args = %q, want %q", got, want)
	}
	want := pogoFlags{noCapture: true, note: "first login", env: "staging", api: "acme.com"}
	if flags != want {
		t.Errorf("flags = %+v, want %+v", flags, want)
	}
}

func TestPogoFlagErrors(t *testing.T) {
	if _, _, err := extractFlags([]string{"--pogo-env"}); err == nil {
		t.Error("a flag with no value should be reported, not silently dropped")
	}
	if _, _, err := extractFlags([]string{"--pogo-nope", "x"}); err == nil {
		t.Error("an unknown --pogo- flag should be reported rather than passed to curl")
	}
}

// The bare-URL shorthand is decided by LooksLikeURL before Cobra runs, so it
// must never claim a subcommand name or one of pogo's own flags.
func TestShorthandNeverSwallowsACommand(t *testing.T) {
	for _, name := range []string{
		"curl", "list", "import-har", "api", "env", "config",
		"compact", "update", "where", "help", "completion",
		"--help", "-h", "--version", "lst",
	} {
		if curlargs.LooksLikeURL(name) {
			t.Errorf("%q would be treated as a URL and handed to curl", name)
		}
	}

	for _, url := range []string{
		"https://api.acme.com/v1", "http://localhost:3000/health",
		"localhost:8080/x", "api.acme.com/v1", "{{base}}/users",
	} {
		if !curlargs.LooksLikeURL(url) {
			t.Errorf("%q should be recognised as a request", url)
		}
	}
}
