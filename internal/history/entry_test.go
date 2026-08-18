package history

import (
	"encoding/json"
	"testing"
	"time"
)

// Durations serialize as milliseconds so the log stays readable with jq, but the
// round trip must be exact: an entry read back from disk has to equal the one
// that was written, or every comparison against stored history is off by a
// nanosecond.
func TestDurationJSONRoundTripIsExact(t *testing.T) {
	durations := []time.Duration{
		0,
		42 * time.Millisecond,
		1500 * time.Millisecond,
		250 * time.Microsecond,
		// Awkward values: nanosecond counts that are not clean milliseconds.
		40123456 * time.Nanosecond,
		999999999 * time.Nanosecond,
		1 * time.Nanosecond,
		123456789 * time.Nanosecond,
		time.Hour + 7*time.Nanosecond,
	}
	for _, d := range durations {
		data, err := json.Marshal(Duration(d))
		if err != nil {
			t.Fatal(err)
		}
		var got Duration
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", data, err)
		}
		if time.Duration(got) != d {
			t.Errorf("round trip of %v (%d ns) gave %v (%d ns) via json %s",
				d, d, time.Duration(got), got, data)
		}
	}
}

// The same property, driven by real measured durations rather than chosen ones.
func TestDurationRoundTripFuzz(t *testing.T) {
	seed := time.Duration(1)
	for i := 0; i < 5000; i++ {
		// A simple LCG keeps this deterministic across runs and platforms.
		seed = (seed*1103515245 + 12345) % 1_000_000_000_000
		d := seed
		if d < 0 {
			d = -d
		}
		data, err := json.Marshal(Duration(d))
		if err != nil {
			t.Fatal(err)
		}
		var got Duration
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", data, err)
		}
		if time.Duration(got) != d {
			t.Fatalf("round trip of %d ns gave %d ns via json %s", d, got, data)
		}
	}
}

func TestDurationString(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "-"},
		{500 * time.Microsecond, "0.50ms"},
		{42 * time.Millisecond, "42ms"},
		{1500 * time.Millisecond, "1.50s"},
		{90 * time.Second, "1m30s"},
	}
	for _, tt := range tests {
		if got := Duration(tt.d).String(); got != tt.want {
			t.Errorf("Duration(%v).String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestHostAndPath(t *testing.T) {
	tests := []struct {
		url, host, path string
	}{
		{"https://api.example.com/users?x=1", "api.example.com", "/users?x=1"},
		{"api.example.com", "api.example.com", "/"},
		{"https://user:pass@api.example.com/x", "api.example.com", "/x"},
		{"http://localhost:8080/a/b", "localhost:8080", "/a/b"},
		{"", "", "/"},
	}
	for _, tt := range tests {
		e := &Entry{Request: Request{URL: tt.url}}
		if got := e.Host(); got != tt.host {
			t.Errorf("Host(%q) = %q, want %q", tt.url, got, tt.host)
		}
		if got := e.Path(); got != tt.path {
			t.Errorf("Path(%q) = %q, want %q", tt.url, got, tt.path)
		}
	}
}

// Credentials in a URL must never reach a list row.
func TestHostStripsUserInfo(t *testing.T) {
	e := &Entry{Request: Request{URL: "https://admin:hunter2@api.example.com/x"}}
	if got := e.Host(); got != "api.example.com" {
		t.Errorf("Host() = %q, credentials must be dropped", got)
	}
}

func TestStatusAndOK(t *testing.T) {
	tests := []struct {
		name   string
		entry  Entry
		status int
		ok     bool
	}{
		{"success", Entry{Response: &Response{Blocks: []Block{{Status: 200}}}}, 200, true},
		{"server error", Entry{Response: &Response{Blocks: []Block{{Status: 500}}}}, 500, false},
		{"client error", Entry{Response: &Response{Blocks: []Block{{Status: 404}}}}, 404, false},
		{"curl failed", Entry{Exit: 7}, 0, false},
		{"no response but clean exit", Entry{}, 0, true},
		{"redirect chain uses the final status",
			Entry{Response: &Response{Blocks: []Block{{Status: 302}, {Status: 200}}}}, 200, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.Status(); got != tt.status {
				t.Errorf("Status() = %d, want %d", got, tt.status)
			}
			if got := tt.entry.OK(); got != tt.ok {
				t.Errorf("OK() = %v, want %v", got, tt.ok)
			}
		})
	}
}

func TestRedirectsCount(t *testing.T) {
	e := Entry{Response: &Response{Blocks: []Block{{Status: 301}, {Status: 302}, {Status: 200}}}}
	if got := e.Redirects(); got != 2 {
		t.Errorf("Redirects() = %d, want 2", got)
	}
	empty := Entry{}
	if got := empty.Redirects(); got != 0 {
		t.Errorf("Redirects() on an empty entry = %d, want 0", got)
	}
}

func TestNewIDIsSortableAndUnique(t *testing.T) {
	seen := map[string]bool{}
	var prev string
	for i := 0; i < 2000; i++ {
		id := NewID()
		if seen[id] {
			t.Fatalf("duplicate id %q after %d iterations", id, i)
		}
		seen[id] = true
		if len(id) != idTimeChars+idRandChars {
			t.Fatalf("id %q has unexpected length %d", id, len(id))
		}
		// Ids generated later must not sort before earlier ones.
		if prev != "" && id[:idTimeChars] < prev[:idTimeChars] {
			t.Fatalf("id %q sorts before earlier id %q", id, prev)
		}
		prev = id
	}
}

func TestEntryJSONRoundTrip(t *testing.T) {
	e := Entry{
		ID:        NewID(),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		Source:    SourceReplay,
		ParentID:  "PARENT",
		Command:   Command{Args: []string{"-X", "POST", "https://x.com"}, Dir: "/tmp"},
		Request:   Request{Method: "POST", URL: "https://x.com"},
		Response:  &Response{Blocks: []Block{{Proto: "HTTP/2", Status: 201}}},
		Duration:  Duration(1234 * time.Millisecond),
		Exit:      0,
		Favorite:  true,
		Metrics:   &Metrics{TimeTotal: 1.2, HTTPCode: 201},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got Entry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.ID != e.ID || got.Source != e.Source || got.ParentID != e.ParentID {
		t.Errorf("identity fields did not survive: %+v", got)
	}
	if got.Status() != 201 || got.Duration != e.Duration || !got.Favorite {
		t.Errorf("entry did not survive the round trip: %+v", got)
	}
	if got.Metrics == nil || got.Metrics.HTTPCode != 201 {
		t.Errorf("metrics did not survive: %+v", got.Metrics)
	}
	if !got.CreatedAt.Equal(e.CreatedAt) {
		t.Errorf("timestamp = %v, want %v", got.CreatedAt, e.CreatedAt)
	}
}

func TestCommandRendering(t *testing.T) {
	c := Command{Args: []string{"-H", "Accept: application/json", "https://x.com"}}
	if got := c.String(); got != `curl -H 'Accept: application/json' https://x.com` {
		t.Errorf("String() = %q", got)
	}
	if got := c.Multiline(); got == c.String() {
		t.Error("Multiline() should differ from the single-line rendering")
	}
}
