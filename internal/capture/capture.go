// Package capture turns a curl invocation into a history entry.
//
// It is the seam between execution and storage, and it is deliberately the only
// place where the two meet. pogo uses it to record what the user typed; pogo
// uses it to replay and to run edited commands. Neither binary builds its own
// curl invocation, so a replay is not an approximation of the original request
// — it is the same code executing the same argv.
package capture

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rmpato/poke/internal/apis"
	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/environment"
	"github.com/rmpato/poke/internal/history"
	"github.com/rmpato/poke/internal/runner"
	"github.com/rmpato/poke/internal/store"
)

// Recorder runs curl and persists the result.
type Recorder struct {
	cfg   config.Config
	store *store.Store

	// vars expands {{variable}} references on the way to curl. It is applied to
	// what runs, never to what is stored.
	vars map[string]string
	env  string

	// api is the API a request should be filed under when its URL cannot say
	// so itself — a command whose host is behind a {{variable}}.
	api string
}

// New returns a Recorder. A nil store means "run but do not record", which is
// what --no-capture and an unwritable history directory both fall back to.
func New(cfg config.Config, st *store.Store) *Recorder {
	return &Recorder{cfg: cfg, store: st}
}

// WithEnvironment returns a Recorder that expands variables from the named
// environment. The Recorder is copied, so the caller's is unaffected.
func (r *Recorder) WithEnvironment(name string, vars map[string]string) *Recorder {
	c := *r
	c.env, c.vars = name, vars
	return &c
}

// WithAPI returns a Recorder that files what it records under an API, for the
// case where the URL is templated and the host is not knowable until the
// variables resolve. The Recorder is copied, so the caller's is unaffected.
func (r *Recorder) WithAPI(domain string) *Recorder {
	c := *r
	c.api = domain
	return &c
}

// Environment reports the active environment name, if any.
func (r *Recorder) Environment() string { return r.env }

// API reports the API requests are being filed under, if one was set.
func (r *Recorder) API() string { return r.api }

// Config exposes the configuration the recorder was built with.
func (r *Recorder) Config() config.Config { return r.cfg }

// Request describes one invocation to run and record.
type Request struct {
	Args     []string
	Source   history.Source
	ParentID string
	Dir      string

	// Stdout, Stderr and Stdin connect curl to a terminal. pogo leaves them nil
	// because it owns the screen; pogo passes the real files through.
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
	StdoutIsTTY bool
}

// Result pairs the recorded entry with the raw execution result. Entry is nil
// when capture was disabled; Run is present whenever curl actually ran.
type Result struct {
	Entry  *history.Entry
	Run    *runner.Result
	Stored bool

	// MissingVars lists {{variables}} the active environment did not define.
	// They were left in the command verbatim rather than blanked, so the
	// failure is visible instead of mysterious.
	MissingVars []string
}

// Run executes the request and, unless capture is disabled, records it.
//
// The error return is reserved for pogo's own failures. A request curl could
// not complete is a successful capture with a non-zero exit code, because a
// failed request is exactly the kind you want in your history.
func (r *Recorder) Run(ctx context.Context, req Request) (*Result, error) {
	// Variables are resolved for execution only. req.Args, which is what gets
	// recorded, keeps its {{braces}} so the history file never holds the token.
	runArgs, missing := environment.Expand(req.Args, r.vars)

	// Metadata describes the command as written, which is what the user will
	// recognize in the history list.
	spec := curlargs.Parse(req.Args)

	maxBody := r.cfg.Capture.MaxResponseBody
	if r.cfg.Capture.Disabled || r.store == nil {
		maxBody = 0
	}

	// The expanded command is what actually went out, so it is what says which
	// API this request reached. The stored command keeps its braces.
	runSpec := curlargs.Parse(runArgs)

	runRes, err := runner.Run(ctx, runner.Options{
		Binary:      r.cfg.Capture.Curl,
		Args:        runArgs,
		Dir:         req.Dir,
		Stdout:      req.Stdout,
		Stderr:      req.Stderr,
		Stdin:       req.Stdin,
		StdoutIsTTY: req.StdoutIsTTY,
		MaxBody:     maxBody,
		MaxStderr:   config.DefaultMaxStderr,
		Spec:        runSpec,
	})
	if err != nil {
		return &Result{Run: runRes, MissingVars: missing}, err
	}

	out := &Result{Run: runRes, MissingVars: missing}
	if r.cfg.Capture.Disabled || r.store == nil {
		return out, nil
	}

	entry, err := r.record(req, spec, runSpec, runRes)
	if err != nil {
		// Recording is best-effort by design. The request already happened and
		// the user already saw its output; failing the command now would make
		// pogo less reliable than the curl it wraps.
		return out, err
	}
	out.Entry = entry
	out.Stored = true
	return out, nil
}

// Replay re-runs a stored entry verbatim, recording the outcome as a new entry.
// The original is never touched: history is append-only, so "what happened when
// I ran this before" stays answerable.
func (r *Recorder) Replay(ctx context.Context, e *history.Entry) (*Result, error) {
	return r.Run(ctx, Request{
		Args:     e.Command.Args,
		Source:   history.SourceReplay,
		ParentID: e.ID,
		Dir:      workingDir(e.Command.Dir),
	})
}

// RunEdited executes a modified command derived from an existing entry.
func (r *Recorder) RunEdited(ctx context.Context, parent *history.Entry, args []string) (*Result, error) {
	var dir, parentID string
	if parent != nil {
		dir, parentID = workingDir(parent.Command.Dir), parent.ID
	}
	return r.Run(ctx, Request{
		Args:     args,
		Source:   history.SourceEdit,
		ParentID: parentID,
		Dir:      dir,
	})
}

// workingDir falls back to the current directory when the recorded one is gone,
// which happens routinely with worktrees and temporary checkouts.
func workingDir(dir string) string {
	if dir == "" {
		return ""
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	return dir
}

func (r *Recorder) record(req Request, spec, runSpec *curlargs.Spec, run *runner.Result) (*history.Entry, error) {
	source := req.Source
	if source == "" {
		source = history.SourceRun
	}

	e := &history.Entry{
		ID:        history.NewID(),
		CreatedAt: time.Now().UTC(),
		Source:    source,
		ParentID:  req.ParentID,
		Command:   history.Command{Args: req.Args, Dir: req.Dir},
		Request:   history.FromSpec(spec),
		Duration:  history.Duration(run.Duration),
		Metrics:   run.Metrics,
		Env:       r.env,
		API:       r.apiFor(runSpec),
		Exit:      run.Exit,
		Error:     runner.ErrorText(run.Stderr, run.Exit),
	}

	if body, origin, total := r.requestBody(spec, req.Dir, run); len(body) > 0 {
		ref, err := r.store.PutBody(e.ID, store.KindRequest, body, total, origin)
		if err != nil {
			return nil, err
		}
		e.Request.Body = ref
	}

	if len(run.Blocks) > 0 || len(run.Body) > 0 {
		resp := &history.Response{Blocks: run.Blocks}
		if len(run.Body) > 0 {
			ref, err := r.store.PutBody(e.ID, store.KindResponse, run.Body, run.BodySize, "stdout")
			if err != nil {
				return nil, err
			}
			resp.Body = ref
		}
		if b := lastBlock(run.Blocks); b != nil {
			if ct, ok := b.Header("Content-Type"); ok {
				resp.ContentType = ct
			}
		}
		e.Response = resp
	}

	// A resolved URL would defeat the whole point of storing the template, so
	// it is dropped whenever expansion actually substituted something.
	if e.Metrics != nil && environment.UsesVariables(req.Args) {
		e.Metrics.EffectiveURL = ""
	}

	// Redaction, when configured to happen before anything reaches disk.
	r.cfg.Redact.Strip(e)

	if err := r.store.Append(e); err != nil {
		return nil, err
	}
	// Housekeeping runs after the entry is safely on disk, and its failure is
	// never the user's problem.
	if r.store.ShouldCompact() {
		_, _ = r.store.Compact()
	}
	return e, nil
}

func lastBlock(blocks []history.Block) *history.Block {
	if len(blocks) == 0 {
		return nil
	}
	return &blocks[len(blocks)-1]
}

// requestBody reconstructs what curl sent, from the command line and from any
// payload that arrived on stdin.
//
// This is a reconstruction, not an observation: curl does not report the body it
// built. For -d it is exact; for multipart -F it is a description, because
// rebuilding curl's boundary-delimited encoding would be inventing detail pogo
// never witnessed.
func (r *Recorder) requestBody(spec *curlargs.Spec, dir string, run *runner.Result) ([]byte, string, int64) {
	if len(spec.Body) == 0 {
		return nil, "", 0
	}
	max := r.cfg.Capture.MaxRequestBody

	var (
		pieces  []string
		origins []string
		total   int64
	)

	for _, part := range spec.Body {
		switch {
		case part.Stdin:
			if len(run.RequestBody) > 0 {
				pieces = append(pieces, string(run.RequestBody))
				total += int64(len(run.RequestBody))
			}
			origins = append(origins, "stdin")

		case part.File != "":
			data, size, err := readCapped(resolve(dir, part.File), max)
			if err != nil {
				pieces = append(pieces, "<unreadable: "+part.File+">")
			} else {
				pieces = append(pieces, string(data))
				total += size
			}
			origins = append(origins, "@"+part.File)

		case part.Kind == "form" || part.Kind == "form-string":
			pieces = append(pieces, "-F "+part.Value)
			origins = append(origins, "multipart")
			total += int64(len(part.Value))

		default:
			pieces = append(pieces, part.Value)
			origins = append(origins, part.Kind)
			total += int64(len(part.Value))
		}
	}

	sep := "&" // how curl joins repeated -d payloads
	if spec.Body[0].Kind == "form" || spec.Body[0].Kind == "form-string" {
		sep = "\n"
	}
	body := []byte(strings.Join(pieces, sep))
	if int64(len(body)) > max {
		body = body[:max]
	}
	return body, strings.Join(dedupe(origins), ","), total
}

func resolve(dir, path string) string {
	if dir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

func readCapped(path string, max int64) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	var size int64
	if fi, err := f.Stat(); err == nil {
		size = fi.Size()
	}
	data, err := io.ReadAll(io.LimitReader(f, max))
	if err != nil {
		return nil, size, err
	}
	if size < int64(len(data)) {
		size = int64(len(data))
	}
	return data, size, nil
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// apiFor decides which API an entry is filed under.
//
// An explicit --api always wins: the user has said something the URL cannot.
// Otherwise the API is read off the command that actually ran, which is the
// expanded one — the stored command may still say {{base}}, and a template is
// not a host.
func (r *Recorder) apiFor(runSpec *curlargs.Spec) string {
	if r.api != "" {
		return r.api
	}
	if runSpec == nil {
		return ""
	}
	return apis.Classify(runSpec.URL(), r.cfg.APIs).Domain
}
