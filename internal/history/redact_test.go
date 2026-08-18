package history

import (
	"strings"
	"testing"

	"github.com/rmpato/poke/internal/curlargs"
)

func TestSensitiveHeaderDetection(t *testing.T) {
	p := DefaultPolicy()

	for _, name := range []string{"Authorization", "authorization", "COOKIE", "Set-Cookie",
		"X-Api-Key", "Proxy-Authorization", "x-auth-token"} {
		if !p.SensitiveHeader(name) {
			t.Errorf("%q should be treated as sensitive", name)
		}
	}
	for _, name := range []string{"Content-Type", "Accept", "User-Agent", "X-Request-Id"} {
		if p.SensitiveHeader(name) {
			t.Errorf("%q should not be treated as sensitive", name)
		}
	}

	// The list is extensible for site-specific header names.
	p.Headers = []string{"X-Internal-Signature"}
	if !p.SensitiveHeader("x-internal-signature") {
		t.Error("configured header names should be honored, case-insensitively")
	}
}

func TestMaskValueKeepsShapeNotSecret(t *testing.T) {
	got := MaskValue("Authorization", "Bearer sk-live-abcdefghijklmnop")

	if !strings.HasPrefix(got, "Bearer ") {
		t.Errorf("the auth scheme is useful and not secret; got %q", got)
	}
	if strings.Contains(got, "sk-live") {
		t.Errorf("the token leaked: %q", got)
	}
	if !strings.Contains(got, "chars") {
		t.Errorf("the masked value should hint at its length: %q", got)
	}
}

func TestMaskValueKeepsCookieNames(t *testing.T) {
	got := MaskValue("Cookie", "session=abc123; theme=dark")

	if !strings.Contains(got, "session=") || !strings.Contains(got, "theme=") {
		t.Errorf("cookie names are the reason you look at the header; got %q", got)
	}
	if strings.Contains(got, "abc123") || strings.Contains(got, "dark") {
		t.Errorf("cookie values leaked: %q", got)
	}
}

func TestMaskURL(t *testing.T) {
	p := DefaultPolicy()

	tests := []struct {
		in       string
		leaked   string
		mustKeep string
	}{
		{"https://api.example.com/x?access_token=secret123&page=2", "secret123", "page=2"},
		{"https://api.example.com/x?api_key=abc", "abc", "api_key="},
		{"https://user:hunter2@api.example.com/x", "hunter2", "user"},
		{"https://api.example.com/users?include=posts", "", "include=posts"},
	}
	for _, tt := range tests {
		got := p.MaskURL(tt.in)
		if tt.leaked != "" && strings.Contains(got, tt.leaked) {
			t.Errorf("MaskURL(%q) leaked %q: %q", tt.in, tt.leaked, got)
		}
		if !strings.Contains(got, tt.mustKeep) {
			t.Errorf("MaskURL(%q) lost %q: %q", tt.in, tt.mustKeep, got)
		}
	}
}

func TestMaskedLeavesOriginalUntouched(t *testing.T) {
	e := &Entry{
		Request: Request{
			Method:  "GET",
			URL:     "https://api.example.com/x?token=secret",
			Headers: []curlargs.Header{{Name: "Authorization", Value: "Bearer secret"}},
		},
		Command: Command{Args: []string{"-H", "Authorization: Bearer secret", "https://api.example.com"}},
		Response: &Response{Blocks: []Block{{
			Status:  200,
			Headers: []curlargs.Header{{Name: "Set-Cookie", Value: "sid=abc"}},
		}}},
	}

	masked := DefaultPolicy().Masked(e)

	if strings.Contains(masked.Request.Headers[0].Value, "secret") {
		t.Error("masked view still contains the token")
	}
	if strings.Contains(masked.Response.Blocks[0].Headers[0].Value, "abc") {
		t.Error("masked view still contains the cookie value")
	}
	// Masking is a view. Replay reads the original, so mutating it would break
	// authentication for every future replay.
	if e.Request.Headers[0].Value != "Bearer secret" {
		t.Error("Masked mutated the original entry")
	}
	if e.Command.Args[1] != "Authorization: Bearer secret" {
		t.Error("Masked mutated the original command")
	}
}

func TestMaskArgsProducesShareableCommand(t *testing.T) {
	p := DefaultPolicy()
	args := []string{
		"-X", "POST",
		"-H", "Authorization: Bearer sk-live-secret",
		"-H", "Content-Type: application/json",
		"-u", "admin:hunter2",
		"https://api.example.com/x?token=abc123",
	}

	got := p.MaskArgs(args)
	joined := strings.Join(got, " ")

	for _, secret := range []string{"sk-live-secret", "hunter2", "abc123"} {
		if strings.Contains(joined, secret) {
			t.Errorf("%q survived masking: %s", secret, joined)
		}
	}
	// Everything else must remain, or the command stops being recognizable.
	for _, keep := range []string{"-X", "POST", "Content-Type: application/json", "api.example.com"} {
		if !strings.Contains(joined, keep) {
			t.Errorf("masking removed %q: %s", keep, joined)
		}
	}
	if len(got) != len(args) {
		t.Errorf("masking changed the argument count: %d vs %d", len(got), len(args))
	}
}

func TestStripOnlyRunsInStoreMode(t *testing.T) {
	build := func() *Entry {
		return &Entry{
			Request: Request{
				URL:     "https://api.example.com/x",
				Headers: []curlargs.Header{{Name: "Authorization", Value: "Bearer secret"}},
				Options: []curlargs.Option{{Name: "-u", Value: "admin:hunter2", HasValue: true}},
			},
			Command: Command{Args: []string{"-H", "Authorization: Bearer secret"}},
		}
	}

	// The default keeps secrets so that replay works; that is a documented
	// trade-off, not an oversight.
	e := build()
	DefaultPolicy().Strip(e)
	if e.Request.Headers[0].Value != "Bearer secret" || e.Redacted {
		t.Error("display mode must not strip anything from disk")
	}

	e = build()
	Policy{Mode: ModeStore}.Strip(e)
	if e.Request.Headers[0].Value != Placeholder {
		t.Errorf("store mode should remove the header value, got %q", e.Request.Headers[0].Value)
	}
	if e.Request.Options[0].Value != Placeholder {
		t.Errorf("store mode should remove credential options, got %q", e.Request.Options[0].Value)
	}
	if strings.Contains(strings.Join(e.Command.Args, " "), "secret") {
		t.Error("store mode left the secret in the recorded command")
	}
	if !e.Redacted {
		t.Error("a stripped entry must be marked so the UI can explain why replay will fail")
	}
}

func TestPolicyOffDisablesEverything(t *testing.T) {
	p := Policy{Off: true}
	if p.SensitiveHeader("Authorization") {
		t.Error("Off should disable header detection")
	}
	url := "https://api.example.com/x?token=abc"
	if p.MaskURL(url) != url {
		t.Error("Off should disable URL masking")
	}
}

// A secret can hide in the recorded option list as well as in the header list:
// -H 'Authorization: ...' is one option whose *argument* is the credential.
// Cleaning only one of the two would leave the token on disk in store mode and
// on screen in display mode.
func TestHeaderOptionValuesAreRedacted(t *testing.T) {
	const secret = "Bearer sk-live-hidden"

	build := func() *Entry {
		return &Entry{Request: Request{
			Options: []curlargs.Option{
				{Name: "-H", Value: "Authorization: " + secret, HasValue: true},
				{Name: "-H", Value: "Accept: application/json", HasValue: true},
				{Name: "--header", Value: "Cookie: sid=abc123", HasValue: true},
			},
		}}
	}

	masked := DefaultPolicy().Masked(build())
	if strings.Contains(masked.Request.Options[0].Value, "sk-live-hidden") {
		t.Errorf("masked option still shows the token: %q", masked.Request.Options[0].Value)
	}
	if !strings.Contains(masked.Request.Options[0].Value, "Authorization:") {
		t.Errorf("the header name should stay visible: %q", masked.Request.Options[0].Value)
	}
	if masked.Request.Options[1].Value != "Accept: application/json" {
		t.Errorf("a harmless header was altered: %q", masked.Request.Options[1].Value)
	}
	if strings.Contains(masked.Request.Options[2].Value, "abc123") {
		t.Errorf("cookie value leaked: %q", masked.Request.Options[2].Value)
	}

	e := build()
	Policy{Mode: ModeStore}.Strip(e)
	if strings.Contains(e.Request.Options[0].Value, "sk-live-hidden") {
		t.Errorf("store mode left the token in the option list: %q", e.Request.Options[0].Value)
	}
	if !e.Redacted {
		t.Error("the entry should be marked redacted")
	}
}

// Production credentials and a local dev server rarely deserve the same
// treatment; per-host rules let one machine hold both opinions.
func TestPerHostPolicy(t *testing.T) {
	p := Policy{
		Mode: ModeDisplay,
		Hosts: map[string]HostPolicy{
			"api.stripe.com": {Mode: ModeStore},
			".internal.dev":  {Off: boolPtr(true)},
			"api.acme.com":   {Headers: []string{"X-Acme-Signature"}},
		},
	}

	// An unlisted host keeps the default.
	if got := p.For("api.example.com").Mode; got != ModeDisplay {
		t.Errorf("default host mode = %q, want display", got)
	}
	// An exact match overrides it.
	if got := p.For("api.stripe.com").Mode; got != ModeStore {
		t.Errorf("stripe mode = %q, want store", got)
	}
	// A leading dot matches subdomains.
	if !p.For("build.internal.dev").Off {
		t.Error("a .suffix rule should cover subdomains")
	}
	if p.For("internal.dev.example.com").Off {
		t.Error("a .suffix rule must not match a host that merely contains it")
	}
	// Extra header names add to the defaults rather than replacing them.
	acme := p.For("api.acme.com")
	if !acme.SensitiveHeader("X-Acme-Signature") {
		t.Error("the host's extra header should be sensitive")
	}
	if !acme.SensitiveHeader("Authorization") {
		t.Error("the built-in list should still apply")
	}
}

// The per-host mode has to reach the code that writes to disk, not just the
// lookup function.
func TestPerHostStripAppliesAtCaptureTime(t *testing.T) {
	p := Policy{Mode: ModeDisplay, Hosts: map[string]HostPolicy{
		"api.stripe.com": {Mode: ModeStore},
	}}

	build := func(url string) *Entry {
		return &Entry{Request: Request{
			URL:     url,
			Headers: []curlargs.Header{{Name: "Authorization", Value: "Bearer secret"}},
		}}
	}

	stripe := build("https://api.stripe.com/v1/charges")
	p.Strip(stripe)
	if stripe.Request.Headers[0].Value != Placeholder {
		t.Errorf("stripe should be stripped at capture time, got %q", stripe.Request.Headers[0].Value)
	}

	other := build("https://api.example.com/x")
	p.Strip(other)
	if other.Request.Headers[0].Value != "Bearer secret" {
		t.Error("an unlisted host should keep the default display mode")
	}
}

func boolPtr(b bool) *bool { return &b }
