package curlargs

import (
	"reflect"
	"strings"
	"testing"
)

func TestQuoteRoundTrip(t *testing.T) {
	// Anything Quote produces must survive Split unchanged. This is the
	// property that makes "copy as curl" safe to paste and safe to re-run.
	cases := [][]string{
		{"-X", "POST", "https://example.com"},
		{"-H", "Content-Type: application/json"},
		{"-d", `{"name":"Pato","note":"it's fine"}`},
		{"-d", "a b\tc"},
		{"--data", "quote\"inside"},
		{"-H", "X-Empty:"},
		{""},
		{"-d", `$(rm -rf /)`},
		{"-d", "back\\slash"},
		{"-H", "X: ünïcøde ✓"},
	}
	for _, args := range cases {
		line := Quote(args)
		got, err := Split(line)
		if err != nil {
			t.Fatalf("Split(%q) failed: %v", line, err)
		}
		if !reflect.DeepEqual(got, args) {
			t.Errorf("round trip of %q via %q gave %q", args, line, got)
		}
	}
}

func TestSplitHandlesRealCommands(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{`curl https://example.com`, []string{"curl", "https://example.com"}},
		{`curl -H "Accept: application/json" https://x.com`,
			[]string{"curl", "-H", "Accept: application/json", "https://x.com"}},
		{`curl -d '{"a":1}' https://x.com`, []string{"curl", "-d", `{"a":1}`, "https://x.com"}},
		{"curl \\\n  -sS \\\n  https://x.com", []string{"curl", "-sS", "https://x.com"}},
		{`curl -d "say \"hi\"" https://x.com`, []string{"curl", "-d", `say "hi"`, "https://x.com"}},
		{`  curl   https://x.com  `, []string{"curl", "https://x.com"}},
	}
	for _, tt := range tests {
		got, err := Split(tt.line)
		if err != nil {
			t.Fatalf("Split(%q): %v", tt.line, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Split(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestSplitRejectsUnbalancedQuotes(t *testing.T) {
	for _, line := range []string{`curl -d 'oops`, `curl -d "oops`} {
		if _, err := Split(line); err == nil {
			t.Errorf("Split(%q) should report an unbalanced quote", line)
		}
	}
}

// Split must not expand anything: pogo executes the result with exec, never
// through a shell, so treating $(...) as a substitution would invent a command
// injection path that does not otherwise exist.
func TestSplitDoesNotExpand(t *testing.T) {
	got, err := Split(`curl -d $(whoami) --url $HOME`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"curl", "-d", "$(whoami)", "--url", "$HOME"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Split expanded something: got %q, want %q", got, want)
	}
}

func TestStripCurl(t *testing.T) {
	for _, in := range [][]string{
		{"curl", "https://x.com"},
		{"pogo", "https://x.com"},
		{"/usr/bin/curl", "https://x.com"},
	} {
		if got := StripCurl(in); !reflect.DeepEqual(got, []string{"https://x.com"}) {
			t.Errorf("StripCurl(%q) = %q", in, got)
		}
	}
	// A command that does not start with the binary name is left alone.
	in := []string{"-X", "POST", "https://x.com"}
	if got := StripCurl(in); !reflect.DeepEqual(got, in) {
		t.Errorf("StripCurl(%q) = %q, want unchanged", in, got)
	}
}

func TestRenderMultilineKeepsOptionsWithValues(t *testing.T) {
	out := Render([]string{"-X", "POST", "-H", "Accept: application/json", "https://x.com"}, true)

	if !strings.Contains(out, "-X POST") {
		t.Errorf("option and value should share a line:\n%s", out)
	}
	if !strings.Contains(out, "-H 'Accept: application/json'") {
		t.Errorf("header should be quoted on one line:\n%s", out)
	}
	// The rendering must still parse back to the same arguments.
	args, err := Split(strings.ReplaceAll(out, "\\\n", " "))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"curl", "-X", "POST", "-H", "Accept: application/json", "https://x.com"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("multiline render did not round trip: %q", args)
	}
}
