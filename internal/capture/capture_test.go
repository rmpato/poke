package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rmpato/pogo/internal/config"
	"github.com/rmpato/pogo/internal/history"
	"github.com/rmpato/pogo/internal/store"
)

func requireCurl(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is not installed")
	}
}

func newRecorder(t *testing.T) (*Recorder, *store.Store, config.Config) {
	t.Helper()
	cfg := config.ForDir(t.TempDir())
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, st), st, cfg
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(buf[:n])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunRecordsEntry(t *testing.T) {
	requireCurl(t)
	rec, st, _ := newRecorder(t)
	srv := testServer(t)

	res, err := rec.Run(context.Background(), Request{
		Args:   []string{"-s", "-H", "X-Test: yes", srv.URL + "/json"},
		Source: history.SourceRun,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Stored || res.Entry == nil {
		t.Fatal("the request was not recorded")
	}

	loaded, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("history has %d entries, want 1", len(loaded.Entries))
	}

	e := loaded.Entries[0]
	if e.Request.Method != "GET" || e.Status() != 200 {
		t.Errorf("entry = %+v", e.Request)
	}
	if v, ok := e.Request.Header("X-Test"); !ok || v != "yes" {
		t.Errorf("request header not recorded: %+v", e.Request.Headers)
	}
	if e.Duration == 0 {
		t.Error("duration was not recorded")
	}

	body, err := st.Body(e.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("stored response body = %q", body)
	}
}

func TestRunRecordsRequestBody(t *testing.T) {
	requireCurl(t)
	rec, st, _ := newRecorder(t)
	srv := testServer(t)

	res, err := rec.Run(context.Background(), Request{
		Args: []string{"-s", "-X", "POST", "-d", `{"name":"Pato"}`, srv.URL + "/echo"},
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := st.Body(res.Entry.Request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"name":"Pato"}` {
		t.Errorf("recorded request body = %q", body)
	}
	if res.Entry.Request.Method != "POST" {
		t.Errorf("method = %q, want POST", res.Entry.Request.Method)
	}
}

func TestRunReadsRequestBodyFromFile(t *testing.T) {
	requireCurl(t)
	rec, st, _ := newRecorder(t)
	srv := testServer(t)

	dir := t.TempDir()
	payload := `{"from":"file"}`
	if err := os.WriteFile(filepath.Join(dir, "body.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := rec.Run(context.Background(), Request{
		Args: []string{"-s", "-X", "POST", "-d", "@body.json", srv.URL + "/echo"},
		Dir:  dir, // curl resolves @file relative to the working directory
	})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := st.Body(res.Entry.Request.Body)
	if string(body) != payload {
		t.Errorf("recorded body = %q, want %q", body, payload)
	}
}

// A failed request is exactly the kind you want in your history.
func TestRunRecordsFailures(t *testing.T) {
	requireCurl(t)
	rec, st, _ := newRecorder(t)

	res, err := rec.Run(context.Background(), Request{
		Args: []string{"-s", "http://127.0.0.1:1/nope"},
	})
	if err != nil {
		t.Fatalf("a failed request should still be a successful capture: %v", err)
	}
	if res.Entry == nil {
		t.Fatal("nothing was recorded")
	}
	if res.Entry.Exit == 0 {
		t.Error("exit code was not recorded")
	}
	if res.Entry.Error == "" {
		t.Error("no explanation was recorded")
	}

	loaded, _ := st.Load()
	if len(loaded.Entries) != 1 {
		t.Errorf("history has %d entries, want the failure", len(loaded.Entries))
	}
}

// Replay must run the original argv and leave the original entry alone.
func TestReplayAppendsWithoutMutating(t *testing.T) {
	requireCurl(t)
	rec, st, _ := newRecorder(t)
	srv := testServer(t)

	first, err := rec.Run(context.Background(), Request{Args: []string{"-s", srv.URL + "/json"}})
	if err != nil {
		t.Fatal(err)
	}
	original := *first.Entry

	second, err := rec.Replay(context.Background(), first.Entry)
	if err != nil {
		t.Fatal(err)
	}

	if second.Entry.ID == original.ID {
		t.Error("replay reused the original id instead of creating a new entry")
	}
	if second.Entry.Source != history.SourceReplay {
		t.Errorf("source = %q, want replay", second.Entry.Source)
	}
	if second.Entry.ParentID != original.ID {
		t.Errorf("parent = %q, want %q", second.Entry.ParentID, original.ID)
	}

	loaded, _ := st.Load()
	if len(loaded.Entries) != 2 {
		t.Fatalf("history has %d entries, want 2", len(loaded.Entries))
	}
	for _, e := range loaded.Entries {
		if e.ID == original.ID {
			if !e.CreatedAt.Equal(original.CreatedAt) || e.Duration != original.Duration {
				t.Error("replay modified the original entry; history must be append-only")
			}
		}
	}
}

func TestRunEditedRecordsNewEntry(t *testing.T) {
	requireCurl(t)
	rec, _, _ := newRecorder(t)
	srv := testServer(t)

	parent, err := rec.Run(context.Background(), Request{Args: []string{"-s", srv.URL + "/json"}})
	if err != nil {
		t.Fatal(err)
	}

	edited, err := rec.RunEdited(context.Background(), parent.Entry,
		[]string{"-s", "-X", "POST", "-d", "changed", srv.URL + "/echo"})
	if err != nil {
		t.Fatal(err)
	}

	if edited.Entry.Source != history.SourceEdit {
		t.Errorf("source = %q, want edit", edited.Entry.Source)
	}
	if edited.Entry.Request.Method != "POST" {
		t.Errorf("the edited command did not take effect: %+v", edited.Entry.Request)
	}
	if edited.Entry.ParentID != parent.Entry.ID {
		t.Error("the edited entry should point back at what it came from")
	}
}

func TestCaptureDisabledStillRunsRequest(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	cfg := config.ForDir(t.TempDir())
	cfg.Capture.Disabled = true
	st, _ := store.Open(cfg)
	rec := New(cfg, st)

	res, err := rec.Run(context.Background(), Request{Args: []string{"-s", srv.URL + "/json"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Exit != 0 {
		t.Errorf("the request should still have run, exit = %d", res.Run.Exit)
	}
	if res.Stored || res.Entry != nil {
		t.Error("nothing should have been recorded")
	}

	loaded, _ := st.Load()
	if len(loaded.Entries) != 0 {
		t.Errorf("history has %d entries, want none", len(loaded.Entries))
	}
}

// With redaction set to store mode, no secret may reach the disk.
func TestStoreModeRedactionKeepsSecretsOffDisk(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	cfg := config.ForDir(t.TempDir())
	cfg.Redact.Mode = history.ModeStore
	st, _ := store.Open(cfg)
	rec := New(cfg, st)

	const secret = "sk-live-do-not-store-me"
	if _, err := rec.Run(context.Background(), Request{
		Args: []string{"-s", "-H", "Authorization: Bearer " + secret, srv.URL + "/json"},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(st.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatal("the token was written to the history file despite store-mode redaction")
	}

	loaded, _ := st.Load()
	if !loaded.Entries[0].Redacted {
		t.Error("the entry should be marked redacted so the UI can explain a failed replay")
	}
}

// In the default mode the secret is stored on purpose, so that replay works.
// This test documents that trade-off rather than leaving it implicit.
func TestDisplayModeStoresSecretsSoReplayWorks(t *testing.T) {
	requireCurl(t)
	rec, st, _ := newRecorder(t)
	srv := testServer(t)

	const secret = "sk-live-kept-on-purpose"
	if _, err := rec.Run(context.Background(), Request{
		Args: []string{"-s", "-H", "Authorization: Bearer " + secret, srv.URL + "/json"},
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(st.Path())
	if !strings.Contains(string(data), secret) {
		t.Error("display mode is documented to store the real header so replay authenticates")
	}
}

// The point of variables: the command that runs carries the secret, the command
// that gets stored does not.
func TestEnvironmentKeepsSecretsOutOfHistory(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	cfg := config.ForDir(t.TempDir())
	st, _ := store.Open(cfg)
	rec := New(cfg, st).WithEnvironment("test", map[string]string{
		"token": "sk-live-never-store-me",
		"base":  srv.URL,
	})

	res, err := rec.Run(context.Background(), Request{
		Args: []string{"-s", "-H", "Authorization: Bearer {{token}}", "{{base}}/json"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The request really was made with the resolved values.
	if res.Run.Exit != 0 || res.Entry.Status() != 200 {
		t.Fatalf("the expanded request did not reach the server: exit=%d status=%d",
			res.Run.Exit, res.Entry.Status())
	}

	data, err := os.ReadFile(st.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-live-never-store-me") {
		t.Fatal("the resolved token was written to history")
	}
	if !strings.Contains(string(data), "{{token}}") {
		t.Error("history should keep the template so a replay can resolve it again")
	}
	if res.Entry.Env != "test" {
		t.Errorf("entry should record which environment resolved it, got %q", res.Entry.Env)
	}
}

// A replay resolves against the environment selected now, which is what makes
// an expired token a non-problem.
func TestReplayResolvesVariablesAgain(t *testing.T) {
	requireCurl(t)
	srv := testServer(t)

	cfg := config.ForDir(t.TempDir())
	st, _ := store.Open(cfg)
	base := New(cfg, st)

	first, err := base.WithEnvironment("old", map[string]string{"base": srv.URL}).
		Run(context.Background(), Request{Args: []string{"-s", "{{base}}/json"}})
	if err != nil {
		t.Fatal(err)
	}

	// A different environment, pointing somewhere that does not exist.
	replayed, err := base.WithEnvironment("broken", map[string]string{"base": "http://127.0.0.1:1"}).
		Replay(context.Background(), first.Entry)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Run.Exit == 0 {
		t.Error("the replay should have used the new environment's value")
	}
}

func TestMissingVariablesAreReported(t *testing.T) {
	requireCurl(t)
	rec, _, _ := newRecorder(t)

	res, err := rec.Run(context.Background(), Request{
		Args: []string{"-s", "http://127.0.0.1:1/{{nope}}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.MissingVars) != 1 || res.MissingVars[0] != "nope" {
		t.Errorf("MissingVars = %v, want [nope]", res.MissingVars)
	}
}
