package runner

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireCurl skips a test when no curl is installed. poke delegates to the
// real binary, so these tests exercise the real integration rather than a mock
// of it; every server involved is a local httptest server.
func requireCurl(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is not installed")
	}
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "abc123")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"users":[{"id":1,"name":"Pato"}]}`))
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/json", http.StatusFound)
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"kaboom"}`))
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAll(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(body)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte{'P', 'N', 'G', 0x00, 0x01, 0x02})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func readAll(r *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func run(t *testing.T, opts Options) (*Result, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts.Stdout, opts.Stderr = &stdout, &stderr
	if opts.MaxBody == 0 {
		opts.MaxBody = 1 << 20
	}
	if opts.MaxStderr == 0 {
		opts.MaxStderr = 64 << 10
	}
	res, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res, &stdout, &stderr
}

func TestRunCapturesStatusHeadersAndBody(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	res, stdout, _ := run(t, Options{Args: []string{"-s", srv.URL + "/json"}})

	if res.Exit != 0 {
		t.Errorf("exit = %d, want 0", res.Exit)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Status != 200 {
		t.Fatalf("blocks = %+v, want one 200", res.Blocks)
	}
	if v, ok := res.Blocks[0].Header("X-Request-Id"); !ok || v != "abc123" {
		t.Errorf("response header not captured: %+v", res.Blocks[0].Headers)
	}
	if !strings.Contains(string(res.Body), `"name":"Pato"`) {
		t.Errorf("body not captured: %q", res.Body)
	}
	// The body must also have reached the caller's stdout untouched: capture is
	// a side effect, never a replacement for curl's output.
	if stdout.String() != string(res.Body) {
		t.Errorf("stdout %q differs from captured body %q", stdout.String(), res.Body)
	}
	if res.Duration <= 0 {
		t.Error("duration was not measured")
	}
}

func TestRunCapturesRedirectChain(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	res, _, _ := run(t, Options{Args: []string{"-sL", srv.URL + "/redirect"}})

	if len(res.Blocks) != 2 {
		t.Fatalf("got %d response blocks, want 2 (302 then 200)", len(res.Blocks))
	}
	if res.Blocks[0].Status != 302 || res.Blocks[1].Status != 200 {
		t.Errorf("chain = %d then %d, want 302 then 200", res.Blocks[0].Status, res.Blocks[1].Status)
	}
}

func TestRunReportsHTTPErrorsAsSuccessfulCaptures(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	res, _, _ := run(t, Options{Args: []string{"-s", srv.URL + "/boom"}})

	// curl exits 0 for a 500: the request worked, the server disagreed.
	if res.Exit != 0 {
		t.Errorf("exit = %d, want 0", res.Exit)
	}
	if res.Blocks[0].Status != 500 {
		t.Errorf("status = %d, want 500", res.Blocks[0].Status)
	}
	if !strings.Contains(string(res.Body), "kaboom") {
		t.Errorf("error body not captured: %q", res.Body)
	}
}

func TestRunPropagatesCurlFailure(t *testing.T) {
	requireCurl(t)

	// Port 0 is never listening; curl fails to connect and says so.
	res, _, _ := run(t, Options{Args: []string{"-s", "http://127.0.0.1:1/nope"}})

	if res.Exit == 0 {
		t.Error("a failed connection should produce a non-zero exit")
	}
	if got := ErrorText(res.Stderr, res.Exit); got == "" {
		t.Error("a failure should yield an explanation")
	}
}

func TestRunPassesArgumentsThroughVerbatim(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	res, _, _ := run(t, Options{Args: []string{
		"-s", "-X", "POST",
		"-H", "Content-Type: application/json",
		"-d", `{"name":"Pato"}`,
		srv.URL + "/echo",
	}})

	if res.Blocks[0].Status != 201 {
		t.Errorf("status = %d, want 201", res.Blocks[0].Status)
	}
	if string(res.Body) != `{"name":"Pato"}` {
		t.Errorf("server echoed %q; the request body did not arrive intact", res.Body)
	}
}

// The user's own -D must keep working: overriding it would break their command.
func TestRunHonoursUserDumpHeader(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)
	dump := filepath.Join(t.TempDir(), "headers.txt")

	res, _, _ := run(t, Options{Args: []string{"-s", "-D", dump, srv.URL + "/json"}})

	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("poke overrode the user's -D: %v", err)
	}
	if !strings.Contains(string(data), "200") {
		t.Errorf("user's dump file has no status line: %q", data)
	}
	if len(res.Blocks) == 0 || res.Blocks[0].Status != 200 {
		t.Error("poke should read the user's dump file instead of opening its own")
	}
}

// With -o the body never reaches stdout, so there is nothing to tee. poke must
// notice rather than record an empty body as if it were the response.
func TestRunWithOutputFileReportsBodyNotOnStdout(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)
	out := filepath.Join(t.TempDir(), "out.json")

	res, stdout, _ := run(t, Options{Args: []string{"-s", "-o", out, srv.URL + "/json"}})

	if res.BodyStdout {
		t.Error("BodyStdout should be false when -o redirects the body")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty with -o, got %q", stdout.String())
	}
	data, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(data), "Pato") {
		t.Errorf("curl's -o output is missing or wrong: %v %q", err, data)
	}
}

func TestRunCapturesRequestBodyFromStdin(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	payload := `{"from":"stdin"}`
	res, _, _ := run(t, Options{
		Args:  []string{"-s", "-X", "POST", "-d", "@-", srv.URL + "/echo"},
		Stdin: strings.NewReader(payload),
	})

	if string(res.RequestBody) != payload {
		t.Errorf("captured stdin payload = %q, want %q", res.RequestBody, payload)
	}
	if string(res.Body) != payload {
		t.Errorf("server received %q, want %q", res.Body, payload)
	}
}

func TestRunTruncatesLargeBodiesButPassesThemThrough(t *testing.T) {
	requireCurl(t)
	big := strings.Repeat("x", 50_000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(big))
	}))
	defer srv.Close()

	res, stdout, _ := run(t, Options{Args: []string{"-s", srv.URL}, MaxBody: 1000})

	if len(res.Body) != 1000 || !res.BodyCapped {
		t.Errorf("captured %d bytes, capped=%v; want 1000 and capped", len(res.Body), res.BodyCapped)
	}
	if res.BodySize != int64(len(big)) {
		t.Errorf("BodySize = %d, want the full %d", res.BodySize, len(big))
	}
	// The cap applies to what poke stores, never to what the user receives.
	if stdout.Len() != len(big) {
		t.Errorf("stdout got %d bytes, want the full %d", stdout.Len(), len(big))
	}
}

func TestRunCollectsMetricsWhenCurlSupportsThem(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	path, _ := exec.LookPath("curl")
	v, err := BinaryVersion(path)
	if err != nil {
		t.Skip("cannot determine curl version")
	}

	res, _, _ := run(t, Options{Args: []string{"-s", srv.URL + "/json"}})

	if !v.AtLeast(minWriteOutFile) {
		if res.Metrics != nil {
			t.Error("timings must not be reported for a curl that cannot produce them")
		}
		t.Skipf("curl %s predates --write-out %%output{}", v)
	}
	if res.Metrics == nil {
		t.Fatal("curl supports the write-out file form, but no metrics were captured")
	}
	if res.Metrics.TimeTotal <= 0 {
		t.Errorf("time_total = %v, want a positive duration", res.Metrics.TimeTotal)
	}
	if res.Metrics.HTTPCode != 200 {
		t.Errorf("metrics http_code = %d, want 200", res.Metrics.HTTPCode)
	}
}

// A user's own --write-out belongs to them; poke must not overwrite it, even at
// the cost of losing its timing data.
func TestRunLeavesUserWriteOutAlone(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	res, stdout, _ := run(t, Options{Args: []string{
		"-s", "-o", os.DevNull, "-w", "STATUS=%{http_code}", srv.URL + "/json",
	}})

	if !strings.Contains(stdout.String(), "STATUS=200") {
		t.Errorf("the user's --write-out did not reach stdout: %q", stdout.String())
	}
	if res.Metrics != nil {
		t.Error("poke should skip its own metrics rather than fight the user's -w")
	}
}

func TestRunMissingBinary(t *testing.T) {
	_, err := Run(context.Background(), Options{
		Binary: "definitely-not-a-real-curl-binary",
		Args:   []string{"https://example.com"},
	})
	if err == nil || !strings.Contains(err.Error(), ErrCurlMissing.Error()) {
		t.Errorf("err = %v, want a missing-curl error", err)
	}
}
