package harimport

import (
	"strings"
	"testing"

	"github.com/rmpato/poke/internal/history"
)

const sampleHAR = `{
  "log": {
    "version": "1.2",
    "entries": [
      {
        "startedDateTime": "2026-08-17T10:11:12.000Z",
        "time": 142.5,
        "request": {
          "method": "POST",
          "url": "https://api.example.com/users?team=core",
          "httpVersion": "HTTP/2",
          "headers": [
            {"name": ":authority", "value": "api.example.com"},
            {"name": ":method", "value": "POST"},
            {"name": "content-type", "value": "application/json"},
            {"name": "authorization", "value": "Bearer sk-live-abc"},
            {"name": "content-length", "value": "17"},
            {"name": "host", "value": "api.example.com"}
          ],
          "postData": {"mimeType": "application/json", "text": "{\"name\":\"Pato\"}"}
        },
        "response": {
          "status": 201,
          "statusText": "Created",
          "httpVersion": "HTTP/2",
          "headers": [{"name": "content-type", "value": "application/json"}],
          "content": {"size": 22, "mimeType": "application/json", "text": "{\"id\":45}"}
        }
      },
      {
        "startedDateTime": "2026-08-17T10:11:13.000Z",
        "time": 12,
        "request": {
          "method": "GET",
          "url": "https://api.example.com/health",
          "headers": []
        },
        "response": {
          "status": 200, "statusText": "OK", "headers": [],
          "content": {"size": 0, "mimeType": "text/plain", "text": ""}
        }
      }
    ]
  }
}`

func TestParseBuildsReplayableEntries(t *testing.T) {
	res, err := Parse(strings.NewReader(sampleHAR), Options{Collection: "devtools"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(res.Entries))
	}

	e := res.Entries[0]
	if e.Request.Method != "POST" || e.Status() != 201 {
		t.Errorf("entry = %s %d", e.Request.Method, e.Status())
	}
	if e.Source != history.SourceImport {
		t.Errorf("source = %q; an import must not look like something poke ran", e.Source)
	}
	if e.Collection != "devtools" {
		t.Errorf("collection = %q", e.Collection)
	}
	if e.Duration.String() != "142ms" {
		t.Errorf("duration = %s, want the HAR's timing", e.Duration)
	}
	if e.CreatedAt.Year() != 2026 || e.CreatedAt.Minute() != 11 {
		t.Errorf("timestamp = %v, want the one from the file", e.CreatedAt)
	}

	// The command has to be a real curl command, or none of replay, edit and
	// copy-as-curl work on an imported request.
	cmd := e.Command.String()
	for _, want := range []string{"-X POST", "https://api.example.com/users?team=core",
		"Content-Type: application/json", "--data-raw"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command is missing %q:\n%s", want, cmd)
		}
	}
}

// Browsers record pseudo-headers and hop-by-hop fields that curl sets itself;
// replaying them would be redundant at best and malformed at worst.
func TestParseDropsBrowserPseudoHeaders(t *testing.T) {
	res, err := Parse(strings.NewReader(sampleHAR), Options{})
	if err != nil {
		t.Fatal(err)
	}
	cmd := res.Entries[0].Command.String()

	for _, gone := range []string{":authority", ":method", "content-length", "host:"} {
		if strings.Contains(strings.ToLower(cmd), gone) {
			t.Errorf("%q should not survive import:\n%s", gone, cmd)
		}
	}
	// The headers that matter do survive.
	if !strings.Contains(cmd, "Authorization: Bearer sk-live-abc") {
		t.Errorf("a real header was dropped:\n%s", cmd)
	}
}

func TestParseKeepsBodies(t *testing.T) {
	res, _ := Parse(strings.NewReader(sampleHAR), Options{})
	e := res.Entries[0]

	if e.Request.Body == nil || e.Request.Body.Inline != `{"name":"Pato"}` {
		t.Errorf("request body = %+v", e.Request.Body)
	}
	if e.Response.Body == nil || e.Response.Body.Inline != `{"id":45}` {
		t.Errorf("response body = %+v", e.Response.Body)
	}
	// An empty response body is absent rather than an empty blob.
	if res.Entries[1].Response.Body != nil {
		t.Error("an empty body should not produce a payload reference")
	}
}

func TestParseRejectsUnusableInput(t *testing.T) {
	for name, in := range map[string]string{
		"not json":   `{"log":`,
		"no entries": `{"log":{"entries":[]}}`,
		"not a har":  `{"hello":"world"}`,
	} {
		if _, err := Parse(strings.NewReader(in), Options{}); err == nil {
			t.Errorf("%s should be rejected", name)
		}
	}
}

func TestParseSkipsEntriesWithoutURL(t *testing.T) {
	in := `{"log":{"entries":[
	  {"request":{"method":"GET","url":""},"response":{"status":0}},
	  {"request":{"method":"GET","url":"https://x.com"},"response":{"status":200}}
	]}}`

	res, err := Parse(strings.NewReader(in), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.Skipped != 1 {
		t.Errorf("entries=%d skipped=%d, want 1 and 1", len(res.Entries), res.Skipped)
	}
}
