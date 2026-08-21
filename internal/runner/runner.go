// Package runner executes curl and captures what happened.
//
// pogo is a wrapper, not a reimplementation: the user's argv is handed to the
// real curl binary untouched, and capture is bolted on through side channels
// that do not change how curl behaves. Both pogo and pogo drive this package,
// so a replayed request goes through exactly the same code path as the original
// invocation.
//
// Three side channels are used:
//
//   - Response status and headers come from -D writing to a temporary file.
//     This is supported by every curl in living memory and touches neither
//     stdout nor stderr.
//   - The response body is teed as it flows through to the user's stdout.
//   - Duration and exit status are measured around the process itself.
//
// Handing curl a pipe instead of the terminal changes two of its behaviors,
// both of which are restored deliberately and documented in docs/security.md:
// the progress meter (re-suppressed with --no-progress-meter) and the refusal
// to spray binary data at a terminal (re-implemented in the tee, matching
// curl's own NUL-byte test and exit code 23).
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/history"
)

// DefaultBinary is used when no curl path is configured.
const DefaultBinary = "curl"

var (
	// minNoProgressMeter is the first curl release with --no-progress-meter. On
	// anything older pogo declines to capture the body rather than pass an
	// option curl would reject.
	minNoProgressMeter = Version{7, 67}

	// minWriteOutFile is the first curl release whose --write-out can redirect
	// to a file with %output{}. Below it there is no way to collect curl's
	// timing breakdown without writing into the user's stdout or stderr, so
	// pogo records no timings at all rather than inventing them.
	minWriteOutFile = Version{8, 3}
)

// Options describes one curl invocation.
type Options struct {
	// Binary overrides the curl executable. Empty means "curl" from PATH.
	Binary string

	// Args is the user's argument list, without the leading "curl". It is
	// executed verbatim; pogo only appends its own capture options.
	Args []string

	// Dir is the working directory, which matters because curl arguments
	// routinely reference relative files (-d @body.json).
	Dir string

	// Stdout and Stderr receive curl's output. Nil discards, which is what pogo
	// wants: it is driving the terminal itself.
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// StdoutIsTTY tells the runner whether the caller's stdout is a terminal.
	// It cannot be inferred from Stdout, which is by then a pipe.
	StdoutIsTTY bool

	MaxBody   int64 // captured response bytes; 0 disables body capture
	MaxStderr int64

	// Spec, when set, avoids parsing Args twice. It is metadata only.
	Spec *curlargs.Spec

	Env []string
}

// Result is everything pogo learned from one invocation.
type Result struct {
	Args     []string
	Exit     int
	Duration time.Duration

	Blocks      []history.Block // one per response head, redirects included
	Body        []byte
	BodySize    int64 // bytes seen, which may exceed len(Body)
	BodyCapped  bool
	BodyStdout  bool // whether the body was even observable on stdout
	Stderr      []byte
	RequestBody []byte // materialized from stdin when the command read it

	// Metrics is curl's own timing and transfer accounting. It is nil when the
	// installed curl cannot write it to a file, or when the user supplied their
	// own --write-out, whose output belongs to them.
	Metrics *history.Metrics

	// BinaryGuard reports that pogo suppressed binary output to the terminal,
	// standing in for the check curl could not make through a pipe.
	BinaryGuard bool

	// Started reports whether curl actually ran. When false, Exit and the rest
	// are meaningless and the error explains why.
	Started bool
}

// ErrCurlMissing is returned when the curl binary cannot be found.
var ErrCurlMissing = errors.New("curl not found in PATH")

// Run executes curl and captures the exchange.
//
// A non-nil error means pogo failed (curl missing, temp file unwritable), not
// that the request failed: a 500 response or a DNS failure is a successful
// capture with a non-zero Exit.
func Run(ctx context.Context, opts Options) (*Result, error) {
	bin := opts.Binary
	if bin == "" {
		bin = DefaultBinary
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrCurlMissing, bin)
	}

	spec := opts.Spec
	if spec == nil {
		spec = curlargs.Parse(opts.Args)
	}

	res := &Result{Args: opts.Args}

	// --- header side channel -------------------------------------------------
	// The user's own -D wins: overriding it would break their command. When
	// they have one, pogo reads their file instead of opening a second.
	dumpPath := spec.DumpHeader
	args := append([]string(nil), opts.Args...)
	switch dumpPath {
	case "":
		tmp, err := os.CreateTemp("", "pogo-headers-*")
		if err != nil {
			return nil, fmt.Errorf("create header capture file: %w", err)
		}
		dumpPath = tmp.Name()
		_ = tmp.Close()
		defer func() { _ = os.Remove(dumpPath) }()
		args = append(args, "-D", dumpPath)
	case "-":
		dumpPath = "" // headers are going to stdout; nothing to read back
	}

	// --- metrics side channel ------------------------------------------------
	// curl reports its own timings through --write-out. Directing that output
	// to a file keeps stdout and stderr untouched, but %output{} only exists in
	// curl 8.3 and later; older curls simply contribute no timing data. A user
	// who passed their own --write-out keeps it: that output is theirs.
	metricsPath := ""
	if !hasWriteOut(spec) {
		if v, err := BinaryVersion(path); err == nil && v.AtLeast(minWriteOutFile) {
			tmp, err := os.CreateTemp("", "pogo-metrics-*.json")
			if err == nil {
				metricsPath = tmp.Name()
				_ = tmp.Close()
				defer func() { _ = os.Remove(metricsPath) }()
				args = append(args, "--write-out", "%output{>>"+metricsPath+"}%{json}")
			}
		}
	}

	// --- body side channel ---------------------------------------------------
	// The body is only observable when it is actually heading for stdout.
	bodyOnStdout := spec.OutputFile == "-" || (spec.OutputFile == "" && !spec.RemoteName)
	res.BodyStdout = bodyOnStdout

	capture := bodyOnStdout && opts.MaxBody > 0
	if capture && opts.StdoutIsTTY {
		// Teeing means curl writes into a pipe, and a pipe makes curl turn its
		// progress meter on. Suppress it explicitly to restore what the user
		// would have seen. Without support for the option, skip capture rather
		// than hand curl an argument it will reject.
		if v, err := BinaryVersion(path); err == nil && v.AtLeast(minNoProgressMeter) {
			args = append(args, "--no-progress-meter")
		} else {
			capture = false
		}
	}

	tee := &teeWriter{
		dst:   opts.Stdout,
		limit: opts.MaxBody,
		// curl refuses to write binary data to a terminal unless the user asked
		// for it with "-o -". Through a pipe curl cannot tell, so pogo makes
		// the same check on its behalf.
		guardTTY: opts.StdoutIsTTY && spec.OutputFile == "",
		warn:     opts.Stderr,
	}
	errTee := &teeWriter{dst: opts.Stderr, limit: opts.MaxStderr}

	// --- process -------------------------------------------------------------
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stderr = errTee
	if capture {
		cmd.Stdout = tee
	} else {
		cmd.Stdout = opts.Stdout // untouched: curl sees the terminal directly
	}

	var stdinTee *teeReader
	if opts.Stdin != nil {
		if spec.ReadsStdin && opts.MaxBody > 0 {
			// The payload is arriving on stdin, so capture it in passing.
			stdinTee = &teeReader{src: opts.Stdin, limit: opts.MaxBody}
			cmd.Stdin = stdinTee
		} else {
			cmd.Stdin = opts.Stdin
		}
	}

	start := time.Now()
	err = cmd.Run()
	res.Duration = time.Since(start)
	res.Started = true

	switch e := err.(type) {
	case nil:
		res.Exit = 0
	case *exec.ExitError:
		res.Exit = e.ExitCode()
	default:
		// curl never started: a missing binary, an unreadable directory.
		res.Started = false
		return res, fmt.Errorf("run curl: %w", err)
	}

	// --- collect -------------------------------------------------------------
	res.Body = tee.captured()
	res.BodySize = tee.total
	res.BodyCapped = tee.truncated
	res.Stderr = sanitizeStderr(errTee.captured())
	res.BinaryGuard = tee.guardTripped
	if stdinTee != nil {
		res.RequestBody = stdinTee.captured()
	}
	if res.BinaryGuard && res.Exit == 0 {
		// Match the exit status curl reports when it declines to write binary
		// output to a terminal (CURLE_WRITE_ERROR).
		res.Exit = 23
	}
	if dumpPath != "" {
		if data, err := os.ReadFile(dumpPath); err == nil {
			res.Blocks = ParseHeaderDump(data)
		}
	}
	if metricsPath != "" {
		if data, err := os.ReadFile(metricsPath); err == nil {
			res.Metrics = ParseMetrics(data)
		}
	}

	return res, nil
}
