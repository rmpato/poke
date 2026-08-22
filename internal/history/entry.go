// Package history defines the record pogo writes and pogo reads.
//
// The types here are the on-disk contract, so they are plain data: no
// filesystem access, no terminal awareness, no knowledge of Bubble Tea. Storage
// lives in internal/store, presentation in internal/tui.
package history

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/rmpato/pogo/internal/curlargs"
)

// Source records how an entry came to exist. History is immutable: replaying or
// editing a request appends a new entry pointing back at its parent rather than
// modifying the original.
type Source string

const (
	SourceRun    Source = "run"    // captured from `pogo curl`
	SourceReplay Source = "replay" // re-run unchanged from the TUI
	SourceEdit   Source = "edit"   // edited in the TUI, then run
	SourceImport Source = "import" // brought in from a HAR file, never run by pogo
)

// Entry is one captured request/response exchange.
type Entry struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Source    Source    `json:"source"`
	ParentID  string    `json:"parent_id,omitempty"`

	Command  Command   `json:"command"`
	Request  Request   `json:"request"`
	Response *Response `json:"response,omitempty"`

	// Duration is wall-clock time around the curl process, so it includes
	// process startup. It is what the user waited, which is the honest number.
	Duration Duration `json:"duration"`

	// Metrics is curl's own timing and transfer accounting, present only when
	// the installed curl was new enough to report it to a file.
	Metrics *Metrics `json:"metrics,omitempty"`

	// Env records which environment resolved this request's variables, so a
	// replay can say what it will be run against.
	Env string `json:"env,omitempty"`

	// API is the registrable domain this request reached, recorded at capture
	// because a templated URL cannot be classified afterwards: by the time pogo
	// reads the entry back, "{{base}}/users" has no host in it. Entries whose
	// URL is literal are classified live instead, so correcting a grouping
	// applies to history already written.
	API string `json:"api,omitempty"`

	Exit     int    `json:"exit"`
	Error    string `json:"error,omitempty"` // curl's diagnostic output, trimmed
	Favorite bool   `json:"favorite,omitempty"`
	Note     string `json:"note,omitempty"`

	// Collection groups saved requests. It is a plain name rather than a
	// hierarchy: the point is to find things again, not to model a filing
	// cabinet nobody asked for.
	Collection string `json:"collection,omitempty"`

	// Redacted marks an entry whose secrets were stripped at capture time and
	// are therefore permanently gone. Replaying it will not authenticate.
	Redacted bool `json:"redacted,omitempty"`
}

// Command is the invocation exactly as the user typed it, minus pogo's own
// options. It is the authoritative record: replay re-executes Args verbatim
// rather than rebuilding a command from parsed metadata, so a parser gap can
// never corrupt a replay.
type Command struct {
	Args []string `json:"args"`
	Dir  string   `json:"dir,omitempty"`
}

// String renders the command as a single shell-safe line.
func (c Command) String() string { return curlargs.Render(c.Args, false) }

// Multiline renders the command across continued lines for display and editing.
func (c Command) Multiline() string { return curlargs.Render(c.Args, true) }

// Request is the parsed view of the command. Everything here is best-effort
// metadata derived from Command.Args; see package curlargs.
type Request struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers []curlargs.Header   `json:"headers,omitempty"`
	Options []curlargs.Option   `json:"options,omitempty"`
	Body    *BodyRef            `json:"body,omitempty"`
	Flags   curlargs.Flags      `json:"flags,omitempty"`
	Parts   []curlargs.BodyPart `json:"parts,omitempty"`

	// Incomplete is set when curlargs could not fully understand the command.
	// The UI surfaces this rather than presenting a confident half-truth.
	Incomplete bool     `json:"incomplete,omitempty"`
	Unparsed   []string `json:"unparsed,omitempty"`
}

// Response is the captured result. Blocks holds one element per response in a
// redirect chain, so -L requests keep their full story; Final is the one that
// produced the body.
type Response struct {
	Blocks      []Block  `json:"blocks,omitempty"`
	Body        *BodyRef `json:"body,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
}

// Block is a single HTTP response head.
type Block struct {
	Proto   string            `json:"proto,omitempty"`
	Status  int               `json:"status"`
	Reason  string            `json:"reason,omitempty"`
	Headers []curlargs.Header `json:"headers,omitempty"`
}

// BodyRef points at a payload held outside the index. Bodies are kept in
// separate blob files so that loading history stays proportional to the number
// of requests rather than to how much data they moved.
type BodyRef struct {
	Size      int64  `json:"size"`
	Stored    int64  `json:"stored"`              // bytes actually written to the blob
	Truncated bool   `json:"truncated,omitempty"` // Stored < Size, capped by config
	Binary    bool   `json:"binary,omitempty"`
	Blob      string `json:"blob,omitempty"` // blob file name, empty when inline-only
	Inline    string `json:"inline,omitempty"`
	Origin    string `json:"origin,omitempty"` // "stdout", "-d", "@file", "stdin", ...
}

// Duration is a time.Duration that serializes as milliseconds, which keeps the
// JSONL readable by eye and by jq.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(time.Duration(d).Seconds()*1000, 'f', -1, 64)), nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	ms, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return err
	}
	// Round rather than truncate. Nanoseconds divided by a million is rarely
	// exact in binary, so multiplying back lands a hair under the original
	// value and truncation would lose a nanosecond on every load — enough to
	// make a stored entry differ from the one just written.
	*d = Duration(time.Duration(math.Round(ms * float64(time.Millisecond))))
	return nil
}

func (d Duration) String() string {
	td := time.Duration(d)
	switch {
	case td == 0:
		return "-"
	case td < time.Millisecond:
		return strconv.FormatFloat(float64(td.Microseconds())/1000, 'f', 2, 64) + "ms"
	case td < time.Second:
		return strconv.Itoa(int(td.Milliseconds())) + "ms"
	case td < time.Minute:
		return strconv.FormatFloat(td.Seconds(), 'f', 2, 64) + "s"
	default:
		return td.Round(time.Second).String()
	}
}

// Status returns the final HTTP status code, or 0 when none was captured.
func (e *Entry) Status() int {
	if b := e.FinalBlock(); b != nil {
		return b.Status
	}
	return 0
}

// FinalBlock returns the last response head, i.e. the one after all redirects.
func (e *Entry) FinalBlock() *Block {
	if e.Response == nil || len(e.Response.Blocks) == 0 {
		return nil
	}
	return &e.Response.Blocks[len(e.Response.Blocks)-1]
}

// Redirects reports how many hops preceded the final response.
func (e *Entry) Redirects() int {
	if e.Response == nil || len(e.Response.Blocks) == 0 {
		return 0
	}
	return len(e.Response.Blocks) - 1
}

// OK reports whether curl exited cleanly and, when a status was captured, that
// it was not an error status.
func (e *Entry) OK() bool {
	if e.Exit != 0 {
		return false
	}
	if s := e.Status(); s != 0 {
		return s < 400
	}
	return true
}

// Host returns the host portion of the URL for compact display.
func (e *Entry) Host() string { return HostOf(e.Request.URL) }

// Path returns the path (and query) portion of the URL.
func (e *Entry) Path() string { return PathOf(e.Request.URL) }

// HostOf extracts the host from a URL, falling back to the whole string when it
// cannot be isolated. It takes a string rather than an Entry so that callers can
// mask a URL first and still split the result.
func HostOf(u string) string {
	if u == "" {
		return ""
	}
	if _, rest, ok := strings.Cut(u, "://"); ok {
		u = rest
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.IndexByte(u, '@'); i >= 0 {
		u = u[i+1:] // strip userinfo: credentials never belong in a list row
	}
	return u
}

// PathOf extracts the path and query from a URL.
func PathOf(u string) string {
	if _, rest, ok := strings.Cut(u, "://"); ok {
		u = rest
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		return u[i:]
	}
	return "/"
}

// Header returns the first request header matching name, case-insensitively.
func (r Request) Header(name string) (string, bool) {
	for _, h := range r.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value, true
		}
	}
	return "", false
}

// Header returns the first response header matching name.
func (b Block) Header(name string) (string, bool) {
	for _, h := range b.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value, true
		}
	}
	return "", false
}

// FromSpec projects a parsed command line onto the stored request shape.
func FromSpec(s *curlargs.Spec) Request {
	return Request{
		Method:     s.Method,
		URL:        s.URL(),
		Headers:    s.Headers,
		Options:    s.Options,
		Flags:      s.Flags,
		Parts:      s.Body,
		Incomplete: !s.Complete(),
		Unparsed:   s.Unrecognized,
	}
}
