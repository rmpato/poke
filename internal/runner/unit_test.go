package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseHeaderDumpSplitsBlocks(t *testing.T) {
	dump := "HTTP/1.1 302 Found\r\n" +
		"Location: /json\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n" +
		"HTTP/2 200 \r\n" +
		"content-type: application/json\r\n" +
		"x-request-id: abc\r\n" +
		"\r\n"

	blocks := ParseHeaderDump([]byte(dump))
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].Status != 302 || blocks[0].Reason != "Found" || blocks[0].Proto != "HTTP/1.1" {
		t.Errorf("first block = %+v", blocks[0])
	}
	// HTTP/2 has no reason phrase; the trailing space must not become one.
	if blocks[1].Status != 200 || blocks[1].Reason != "" || blocks[1].Proto != "HTTP/2" {
		t.Errorf("second block = %+v", blocks[1])
	}
	if v, ok := blocks[1].Header("Content-Type"); !ok || v != "application/json" {
		t.Errorf("header lookup should be case-insensitive, got %q %v", v, ok)
	}
}

func TestParseHeaderDumpWithoutStatusLine(t *testing.T) {
	// file:// and some other protocols dump headers with no status line.
	blocks := ParseHeaderDump([]byte("Content-Length: 42\r\nAccept-ranges: bytes\r\n"))
	if len(blocks) != 1 || blocks[0].Status != 0 || len(blocks[0].Headers) != 2 {
		t.Errorf("blocks = %+v, want one statusless block with two headers", blocks)
	}
}

func TestParseHeaderDumpEmpty(t *testing.T) {
	if got := ParseHeaderDump(nil); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		out         string
		wantMajor   int
		wantMinor   int
		expectError bool
	}{
		{"curl 8.6.0 (x86_64-apple-darwin23.0) libcurl/8.6.0\nRelease-Date", 8, 6, false},
		{"curl 7.81.0 (x86_64-pc-linux-gnu) libcurl/7.81.0", 7, 81, false},
		{"curl 8.11.1-DEV (x86_64) libcurl/8.11.1", 8, 11, false},
		{"wget 1.21", 0, 0, true},
		{"", 0, 0, true},
	}
	for _, tt := range tests {
		v, err := ParseVersion(tt.out)
		if tt.expectError {
			if err == nil {
				t.Errorf("ParseVersion(%q) should fail", tt.out)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): %v", tt.out, err)
			continue
		}
		if v.Major != tt.wantMajor || v.Minor != tt.wantMinor {
			t.Errorf("ParseVersion(%q) = %v, want %d.%d", tt.out, v, tt.wantMajor, tt.wantMinor)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		have, min Version
		want      bool
	}{
		{Version{8, 6}, Version{7, 67}, true},
		{Version{7, 67}, Version{7, 67}, true},
		{Version{7, 66}, Version{7, 67}, false},
		{Version{8, 3}, Version{8, 3}, true},
		{Version{8, 2}, Version{8, 3}, false},
		{Version{9, 0}, Version{8, 3}, true},
	}
	for _, tt := range tests {
		if got := tt.have.AtLeast(tt.min); got != tt.want {
			t.Errorf("%v.AtLeast(%v) = %v, want %v", tt.have, tt.min, got, tt.want)
		}
	}
}

func TestParseMetricsKeepsLastDocument(t *testing.T) {
	// curl appends one document per transfer; the last describes the response
	// whose body poke captured.
	data := []byte(`{"http_code":302,"time_total":0.1}
{"http_code":200,"time_total":0.25,"size_download":179}`)

	m := ParseMetrics(data)
	if m == nil {
		t.Fatal("no metrics parsed")
	}
	if m.HTTPCode != 200 || m.TimeTotal != 0.25 || m.SizeDownload != 179 {
		t.Errorf("metrics = %+v, want the final transfer", m)
	}
}

func TestParseMetricsTolerantOfTruncation(t *testing.T) {
	data := []byte(`{"http_code":200,"time_total":0.25}
{"http_code":500,"time_tot`)

	m := ParseMetrics(data)
	if m == nil || m.HTTPCode != 200 {
		t.Errorf("a truncated final document should leave earlier ones usable, got %+v", m)
	}
	if ParseMetrics(nil) != nil {
		t.Error("no data should yield no metrics")
	}
}

func TestTeeWriterPassesThroughAndCaps(t *testing.T) {
	var dst bytes.Buffer
	w := &teeWriter{dst: &dst, limit: 10}

	payload := []byte(strings.Repeat("a", 100))
	n, err := w.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("Write = %d, %v; a short write would make curl think the transfer failed", n, err)
	}
	if dst.Len() != 100 {
		t.Errorf("destination got %d bytes, want all 100", dst.Len())
	}
	if len(w.captured()) != 10 || !w.truncated {
		t.Errorf("captured %d bytes, truncated=%v; want 10 and true", len(w.captured()), w.truncated)
	}
	if w.total != 100 {
		t.Errorf("total = %d, want 100", w.total)
	}
}

// poke stands in for the check curl cannot make once a pipe is in the way.
func TestTeeWriterBinaryGuard(t *testing.T) {
	var dst, warn bytes.Buffer
	w := &teeWriter{dst: &dst, limit: 1000, guardTTY: true, warn: &warn}

	w.Write([]byte("safe text\n"))
	if dst.Len() == 0 {
		t.Fatal("text output should pass through")
	}
	before := dst.Len()

	w.Write([]byte{'a', 0x00, 'b'})

	if !w.guardTripped {
		t.Error("a NUL byte should trip the guard, matching curl's own test")
	}
	if dst.Len() != before {
		t.Error("binary data must not reach the terminal after the guard trips")
	}
	if !strings.Contains(warn.String(), "Binary output") {
		t.Errorf("the user should see curl's warning, got %q", warn.String())
	}
	// It is still captured: history keeps what the terminal refused to show.
	if !bytes.Contains(w.captured(), []byte{0x00}) {
		t.Error("the payload should still be captured for history")
	}
}

func TestTeeWriterNoGuardWhenNotTTY(t *testing.T) {
	var dst bytes.Buffer
	w := &teeWriter{dst: &dst, limit: 1000} // guardTTY false: output is a pipe

	w.Write([]byte{'a', 0x00, 'b'})

	if w.guardTripped {
		t.Error("binary output down a pipe is normal and must not be blocked")
	}
	if dst.Len() != 3 {
		t.Errorf("destination got %d bytes, want all 3", dst.Len())
	}
}

func TestSanitizeStderrCollapsesProgressRedraws(t *testing.T) {
	// curl redraws its progress meter with carriage returns; only the final
	// state of each line is worth storing.
	in := []byte("  % Total\r  0  100\r100  200\ncurl: (7) Failed to connect\n")
	got := string(sanitizeStderr(in))

	if strings.Contains(got, "\r") {
		t.Errorf("carriage returns survived: %q", got)
	}
	if !strings.Contains(got, "curl: (7) Failed to connect") {
		t.Errorf("the actual error was lost: %q", got)
	}
	if strings.Contains(got, "0  100") {
		t.Errorf("intermediate redraw survived: %q", got)
	}
}

func TestErrorTextPrefersCurlDiagnostics(t *testing.T) {
	stderr := []byte("  % Total    % Received\ncurl: (6) Could not resolve host: nope.invalid\n")
	if got := ErrorText(stderr, 6); got != "curl: (6) Could not resolve host: nope.invalid" {
		t.Errorf("ErrorText = %q", got)
	}
}

// A successful request must not acquire an error message from progress output.
func TestErrorTextEmptyOnSuccess(t *testing.T) {
	stderr := []byte("  % Total    % Received % Xferd\n100  179  100  179    0     0   104k")
	if got := ErrorText(stderr, 0); got != "" {
		t.Errorf("ErrorText on success = %q, want empty", got)
	}
}

func TestErrorTextFallsBackToExitMeaning(t *testing.T) {
	if got := ErrorText(nil, 7); got != "failed to connect" {
		t.Errorf("ErrorText = %q, want the meaning of exit 7", got)
	}
	if got := ErrorText(nil, 0); got != "" {
		t.Errorf("ErrorText = %q, want empty", got)
	}
}
