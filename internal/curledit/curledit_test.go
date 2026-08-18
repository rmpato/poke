package curledit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rmpato/poke/internal/curlargs"
)

func formOf(t *testing.T, args []string, body string) (Form, *curlargs.Spec) {
	t.Helper()
	spec := curlargs.Parse(args)
	return FormOf(spec, body), spec
}

// The property that makes structured editing safe: options poke does not model
// survive an edit untouched. Regenerating a command from fields would drop them.
func TestApplyPreservesUnmodeledOptions(t *testing.T) {
	args := []string{
		"--cacert", "/etc/ssl/private.pem",
		"--resolve", "api.example.com:443:10.0.0.7",
		"-k", "--http2", "--unix-socket", "/var/run/api.sock",
		"-H", "Accept: application/json",
		"https://api.example.com/users",
	}
	have, _ := formOf(t, args, "")

	want := have
	want.Headers = []curlargs.Header{{Name: "Accept", Value: "application/xml"}}
	got := Apply(args, have, want)

	joined := strings.Join(got, " ")
	for _, keep := range []string{"--cacert /etc/ssl/private.pem", "--resolve api.example.com:443:10.0.0.7",
		"-k", "--http2", "--unix-socket /var/run/api.sock"} {
		if !strings.Contains(joined, keep) {
			t.Errorf("editing dropped %q: %s", keep, joined)
		}
	}
	if !strings.Contains(joined, "Accept: application/xml") {
		t.Errorf("the edit was not applied: %s", joined)
	}
	if strings.Contains(joined, "application/json") {
		t.Errorf("the old header value survived: %s", joined)
	}
}

func TestApplyMethod(t *testing.T) {
	tests := []struct {
		name string
		args []string
		to   string
		want []string
	}{
		{"rewrites an existing -X",
			[]string{"-X", "POST", "https://x.com"}, "PUT",
			[]string{"-X", "PUT", "https://x.com"}},
		{"rewrites an attached -X",
			[]string{"-XPOST", "https://x.com"}, "PUT",
			[]string{"-XPUT", "https://x.com"}},
		{"rewrites --request=",
			[]string{"--request=POST", "https://x.com"}, "PATCH",
			[]string{"--request=PATCH", "https://x.com"}},
		{"adds -X when there was none",
			[]string{"https://x.com"}, "DELETE",
			[]string{"-X", "DELETE", "https://x.com"}},
		{"drops -I when leaving HEAD",
			[]string{"-I", "https://x.com"}, "GET",
			[]string{"-X", "GET", "https://x.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			have, _ := formOf(t, tt.args, "")
			want := have
			want.Method = tt.to
			if got := Apply(append([]string(nil), tt.args...), have, want); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Apply = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyURLAndQuery(t *testing.T) {
	args := []string{"-H", "Accept: application/json", "https://api.example.com/users?page=1&sort=name"}
	have, _ := formOf(t, args, "")

	if have.URL != "https://api.example.com/users" {
		t.Fatalf("URL = %q, query should be split out", have.URL)
	}
	if len(have.Query) != 2 || have.Query[0].Key != "page" || have.Query[1].Value != "name" {
		t.Fatalf("query = %+v", have.Query)
	}

	want := have
	want.Query = []Param{{"page", "2"}, {"sort", "name"}, {"include", "profile"}}
	got := Apply(append([]string(nil), args...), have, want)

	last := got[len(got)-1]
	if last != "https://api.example.com/users?page=2&sort=name&include=profile" {
		t.Errorf("URL = %q", last)
	}
}

// Query order is preserved because APIs occasionally care, and because a
// reordered URL in the history would look like a change the user did not make.
func TestQueryOrderAndRepeatsPreserved(t *testing.T) {
	have, _ := formOf(t, []string{"https://x.com/a?b=2&a=1&b=3"}, "")
	if len(have.Query) != 3 {
		t.Fatalf("query = %+v, repeats must be kept", have.Query)
	}
	if have.JoinURL() != "https://x.com/a?b=2&a=1&b=3" {
		t.Errorf("round trip = %q", have.JoinURL())
	}
}

func TestApplyHeaders(t *testing.T) {
	args := []string{
		"-H", "Accept: application/json",
		"-H", "Authorization: Bearer old",
		"-H", "X-Trace: 1",
		"https://x.com",
	}
	have, _ := formOf(t, args, "")

	want := have
	want.Headers = []curlargs.Header{
		{Name: "Accept", Value: "application/json"},  // unchanged
		{Name: "Authorization", Value: "Bearer new"}, // changed
		// X-Trace removed
		{Name: "X-New", Value: "yes"}, // added
	}
	got := Apply(append([]string(nil), args...), have, want)
	joined := strings.Join(got, " ")

	if !strings.Contains(joined, "Authorization: Bearer new") {
		t.Errorf("changed header missing: %s", joined)
	}
	if strings.Contains(joined, "Bearer old") {
		t.Errorf("old value survived: %s", joined)
	}
	if strings.Contains(joined, "X-Trace") {
		t.Errorf("removed header survived: %s", joined)
	}
	if !strings.Contains(joined, "X-New: yes") {
		t.Errorf("added header missing: %s", joined)
	}
	// An untouched header keeps its original argument form.
	if !strings.Contains(joined, "Accept: application/json") {
		t.Errorf("untouched header was disturbed: %s", joined)
	}
}

func TestApplyHeadersHandlesAttachedForm(t *testing.T) {
	args := []string{"-HAccept: application/json", "https://x.com"}
	have, _ := formOf(t, args, "")

	want := have
	want.Headers = []curlargs.Header{{Name: "Accept", Value: "text/plain"}}
	got := Apply(append([]string(nil), args...), have, want)

	if strings.Join(got, " ") != "-H Accept: text/plain https://x.com" {
		t.Errorf("Apply = %q", got)
	}
}

// Two headers with the same name must stay independently editable.
func TestApplyRepeatedHeaderNames(t *testing.T) {
	args := []string{"-H", "X-Tag: a", "-H", "X-Tag: b", "https://x.com"}
	have, _ := formOf(t, args, "")

	want := have
	want.Headers = []curlargs.Header{{Name: "X-Tag", Value: "a"}, {Name: "X-Tag", Value: "CHANGED"}}
	got := Apply(append([]string(nil), args...), have, want)
	joined := strings.Join(got, " ")

	if !strings.Contains(joined, "X-Tag: a") || !strings.Contains(joined, "X-Tag: CHANGED") {
		t.Errorf("Apply = %q", joined)
	}
	if strings.Contains(joined, "X-Tag: b") {
		t.Errorf("the second header was not updated: %q", joined)
	}
}

func TestApplyBody(t *testing.T) {
	args := []string{"-X", "POST", "-d", `{"a":1}`, "https://x.com"}
	have, _ := formOf(t, args, `{"a":1}`)

	want := have
	want.Body = `{"a":2}`
	got := Apply(append([]string(nil), args...), have, want)

	if !reflect.DeepEqual(got, []string{"-X", "POST", "-d", `{"a":2}`, "https://x.com"}) {
		t.Errorf("Apply = %q", got)
	}
}

func TestApplyBodyAddsFlagWhenMissing(t *testing.T) {
	args := []string{"https://x.com"}
	have, _ := formOf(t, args, "")

	want := have
	want.Body = "hello=world"
	got := Apply(append([]string(nil), args...), have, want)

	if strings.Join(got, " ") != "https://x.com -d hello=world" {
		t.Errorf("Apply = %q", got)
	}
}

// Editing nothing must change nothing, byte for byte.
func TestApplyNoChangeIsIdentity(t *testing.T) {
	args := []string{"-sSL", "-X", "POST", "-H", "Accept: application/json",
		"-d", `{"a":1}`, "--cacert", "x.pem", "https://x.com/a?b=1"}
	have, _ := formOf(t, args, `{"a":1}`)

	got := Apply(append([]string(nil), args...), have, have)
	if !reflect.DeepEqual(got, args) {
		t.Errorf("an empty edit changed the command:\n got  %q\n want %q", got, args)
	}
}

// A value that happens to equal the URL must not be mistaken for the operand.
func TestApplyURLDoesNotRewriteAnOptionValue(t *testing.T) {
	args := []string{"-e", "https://x.com/a", "https://x.com/a"}
	have, _ := formOf(t, args, "")

	want := have
	want.URL = "https://x.com/b"
	got := Apply(append([]string(nil), args...), have, want)

	if got[1] != "https://x.com/a" {
		t.Errorf("the referer was rewritten: %q", got)
	}
	if got[2] != "https://x.com/b" {
		t.Errorf("the URL was not rewritten: %q", got)
	}
}
