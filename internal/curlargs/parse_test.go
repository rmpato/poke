package curlargs

import (
	"reflect"
	"testing"
)

func TestParseMethodInference(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"bare url is GET", []string{"https://example.com"}, "GET"},
		{"explicit -X wins", []string{"-X", "PATCH", "https://example.com"}, "PATCH"},
		{"lowercase method is normalized", []string{"-X", "delete", "https://example.com"}, "DELETE"},
		{"data implies POST", []string{"-d", "a=1", "https://example.com"}, "POST"},
		{"json implies POST", []string{"--json", `{"a":1}`, "https://example.com"}, "POST"},
		{"form implies POST", []string{"-F", "file=@x.txt", "https://example.com"}, "POST"},
		{"upload implies PUT", []string{"-T", "x.txt", "https://example.com"}, "PUT"},
		{"head flag implies HEAD", []string{"-I", "https://example.com"}, "HEAD"},
		{"-G keeps GET despite data", []string{"-G", "-d", "a=1", "https://example.com"}, "GET"},
		{"-X overrides inference", []string{"-X", "PUT", "-d", "a=1", "https://example.com"}, "PUT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.args).Method; got != tt.want {
				t.Errorf("Parse(%q).Method = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseHeaders(t *testing.T) {
	spec := Parse([]string{
		"-H", "Content-Type: application/json",
		"--header", "X-Trace:  abc  ",
		"-H", "X-Empty;",
		"https://example.com",
	})

	want := []Header{
		{"Content-Type", "application/json"},
		{"X-Trace", "abc"},
		{"X-Empty", ""},
	}
	if !reflect.DeepEqual(spec.Headers, want) {
		t.Errorf("headers = %+v, want %+v", spec.Headers, want)
	}
}

func TestParseShortOptionClustering(t *testing.T) {
	// curl accepts -sSL as three switches, and lets a value-taking option end a
	// cluster with its value either attached or separate.
	tests := []struct {
		args       []string
		wantURL    string
		wantMethod string
		wantSilent bool
		wantFollow bool
		wantData   string
	}{
		{[]string{"-sSL", "https://example.com"}, "https://example.com", "GET", true, true, ""},
		{[]string{"-sd", "a=1", "https://example.com"}, "https://example.com", "POST", true, false, "a=1"},
		{[]string{"-da=1", "https://example.com"}, "https://example.com", "POST", false, false, "a=1"},
		{[]string{"-sSLXPOST", "https://example.com"}, "https://example.com", "POST", true, true, ""},
	}
	for _, tt := range tests {
		spec := Parse(tt.args)
		if spec.URL() != tt.wantURL {
			t.Errorf("Parse(%q).URL = %q, want %q", tt.args, spec.URL(), tt.wantURL)
		}
		if spec.Method != tt.wantMethod {
			t.Errorf("Parse(%q).Method = %q, want %q", tt.args, spec.Method, tt.wantMethod)
		}
		if spec.Flags.Silent != tt.wantSilent {
			t.Errorf("Parse(%q).Silent = %v, want %v", tt.args, spec.Flags.Silent, tt.wantSilent)
		}
		if spec.Flags.Follow != tt.wantFollow {
			t.Errorf("Parse(%q).Follow = %v, want %v", tt.args, spec.Flags.Follow, tt.wantFollow)
		}
		if tt.wantData != "" {
			if len(spec.Body) != 1 || spec.Body[0].Value != tt.wantData {
				t.Errorf("Parse(%q).Body = %+v, want value %q", tt.args, spec.Body, tt.wantData)
			}
		}
	}
}

func TestParseLongOptionWithEquals(t *testing.T) {
	spec := Parse([]string{"--request=PUT", "--data=name=pato", "https://example.com"})
	if spec.Method != "PUT" {
		t.Errorf("method = %q, want PUT", spec.Method)
	}
	if len(spec.Body) != 1 || spec.Body[0].Value != "name=pato" {
		t.Errorf("body = %+v, want name=pato", spec.Body)
	}
}

// An option pogo does not know must never swallow the URL. This is the property
// that keeps a parser gap from producing a wrong history entry.
func TestUnknownOptionDoesNotSwallowURL(t *testing.T) {
	spec := Parse([]string{"--some-future-curl-option", "https://example.com/users"})

	if got := spec.URL(); got != "https://example.com/users" {
		t.Errorf("URL = %q, want the real URL", got)
	}
	if len(spec.Unrecognized) == 0 {
		t.Error("unknown option should be recorded in Unrecognized so the UI can admit it")
	}
	if spec.Complete() {
		t.Error("a spec with unrecognized options must not claim to be complete")
	}
}

// When an unknown option really did take a value, that value must be quarantined
// rather than promoted to a URL.
func TestUnknownOptionValueIsNotMistakenForURL(t *testing.T) {
	spec := Parse([]string{"--some-future-option", "3", "https://example.com"})
	if got := spec.URL(); got != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", got)
	}
}

func TestParseBodySources(t *testing.T) {
	spec := Parse([]string{"-d", "@payload.json", "https://example.com"})
	if len(spec.Body) != 1 || spec.Body[0].File != "payload.json" {
		t.Fatalf("body = %+v, want file payload.json", spec.Body)
	}

	spec = Parse([]string{"-d", "@-", "https://example.com"})
	if !spec.Body[0].Stdin || !spec.ReadsStdin {
		t.Errorf("@- should be recognized as stdin, got %+v", spec.Body[0])
	}

	spec = Parse([]string{"-T", "-", "https://example.com"})
	if !spec.Body[0].Stdin || !spec.ReadsStdin {
		t.Errorf("-T - should be recognized as stdin, got %+v", spec.Body[0])
	}
}

func TestParseOutputRedirection(t *testing.T) {
	if got := Parse([]string{"-o", "out.html", "https://example.com"}).OutputFile; got != "out.html" {
		t.Errorf("OutputFile = %q, want out.html", got)
	}
	if !Parse([]string{"-O", "https://example.com/f.zip"}).RemoteName {
		t.Error("-O should set RemoteName")
	}
	if got := Parse([]string{"-D", "h.txt", "https://example.com"}).DumpHeader; got != "h.txt" {
		t.Errorf("DumpHeader = %q, want h.txt", got)
	}
}

func TestParseFlagsIncompleteCases(t *testing.T) {
	if !Parse([]string{"--next", "https://a.example.com", "https://b.example.com"}).Multi {
		t.Error("--next should mark the command as multi-request")
	}
	if !Parse([]string{"-K", "curlrc", "https://example.com"}).ConfigFile {
		t.Error("-K should mark the metadata as partial")
	}
}

func TestParseNoValueOnTrailingOption(t *testing.T) {
	// A malformed command must not panic; curl will report the error itself.
	spec := Parse([]string{"https://example.com", "-H"})
	if spec.URL() != "https://example.com" {
		t.Errorf("URL = %q", spec.URL())
	}
}

func TestParseCredentialsAndAgent(t *testing.T) {
	spec := Parse([]string{"-u", "user:pass", "-A", "pogo/1", "-e", "https://ref", "-b", "s=1", "https://example.com"})
	if spec.User != "user:pass" {
		t.Errorf("User = %q", spec.User)
	}
	if spec.UserAgent != "pogo/1" || spec.Referer != "https://ref" || spec.Cookie != "s=1" {
		t.Errorf("agent/referer/cookie not captured: %+v", spec)
	}
}

func TestLooksLikeURL(t *testing.T) {
	yes := []string{
		"https://example.com", "http://example.com/a?b=c", "example.com",
		"example.com/users", "localhost:8080/x", "127.0.0.1:9", "[::1]:8080/x",
		"ftp://files.example.com", "api.foo.co.uk",
	}
	no := []string{
		"", "-X", "POST", "3", "/tmp/file", "./rel", "~/x", "application/json",
		"user:pass", "a b",
	}
	for _, s := range yes {
		if !LooksLikeURL(s) {
			t.Errorf("LooksLikeURL(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if LooksLikeURL(s) {
			t.Errorf("LooksLikeURL(%q) = true, want false", s)
		}
	}
}

// A URL written with variables is still a URL. Without this, a request like
// `pogo '{{base}}/users'` records no URL at all and the history row is blank.
func TestLooksLikeURLAcceptsTemplates(t *testing.T) {
	yes := []string{"{{base}}", "{{base}}/users", "{{base}}/users/{{id}}", "https://{{host}}/x"}
	for _, s := range yes {
		if !LooksLikeURL(s) {
			t.Errorf("LooksLikeURL(%q) = false, want true", s)
		}
	}
	// An unclosed brace is not a template and must not become a URL.
	for _, s := range []string{"{{unclosed", "}}backwards", "{single}"} {
		if LooksLikeURL(s) {
			t.Errorf("LooksLikeURL(%q) = true, want false", s)
		}
	}
}

func TestParseTemplateURL(t *testing.T) {
	spec := Parse([]string{"-H", "Authorization: Bearer {{token}}", "{{base}}/users/42"})
	if got := spec.URL(); got != "{{base}}/users/42" {
		t.Errorf("URL = %q, want the template", got)
	}
	if !spec.Complete() {
		t.Error("a templated command should still parse cleanly")
	}
}
