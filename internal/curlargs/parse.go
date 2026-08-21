// Package curlargs extracts human-meaningful metadata from a curl command line.
//
// It is deliberately *not* a curl reimplementation. pogo always executes the
// user's original argv with the real curl binary; this package exists only so
// that pogo can show "POST https://api.example.com/users" instead of a wall of
// shell tokens. Every failure mode here is therefore a display-quality issue,
// never a behavioral one, and the parser is written to degrade loudly rather
// than guess: anything it does not understand lands in Spec.Unrecognized so the
// UI can admit it, and a token is only promoted to a URL if it actually looks
// like one.
package curlargs

import (
	"strings"
)

// Header is a request header supplied with -H/--header.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Option is a single recognized curl option as the user wrote it, preserving
// order. Name keeps its leading dashes ("-H", "--header") so that rendering a
// command back out stays faithful to the original invocation.
type Option struct {
	Name     string `json:"name"`
	Value    string `json:"value,omitempty"`
	HasValue bool   `json:"has_value,omitempty"`
}

// BodyPart is one piece of request payload. curl accepts several payload
// flavors and allows repeating them, so this stays a list rather than a single
// blob. Value is kept exactly as written, including "@file" and "@-" forms; it
// is the runner's job to materialize those.
type BodyPart struct {
	Kind  string `json:"kind"`            // data, data-raw, data-binary, form, json, upload-file, ...
	Value string `json:"value"`           // literal argument as written
	File  string `json:"file,omitempty"`  // set when the value referenced @file
	Stdin bool   `json:"stdin,omitempty"` // set when the value referenced @- or -
}

// Flags collects the handful of switches worth surfacing in the UI.
type Flags struct {
	Head     bool `json:"head,omitempty"`     // -I / --head
	Follow   bool `json:"follow,omitempty"`   // -L / --location
	Silent   bool `json:"silent,omitempty"`   // -s / --silent
	Verbose  bool `json:"verbose,omitempty"`  // -v / --verbose
	Insecure bool `json:"insecure,omitempty"` // -k / --insecure
	Get      bool `json:"get,omitempty"`      // -G / --get
	Include  bool `json:"include,omitempty"`  // -i / --include
	Compress bool `json:"compressed,omitempty"`
}

// Spec is the metadata view of a curl command line.
type Spec struct {
	Method    string     `json:"method"`
	URLs      []string   `json:"urls,omitempty"`
	Headers   []Header   `json:"headers,omitempty"`
	Body      []BodyPart `json:"body,omitempty"`
	Options   []Option   `json:"options,omitempty"`
	User      string     `json:"user,omitempty"`       // -u (credentials)
	ProxyUser string     `json:"proxy_user,omitempty"` // -U (credentials)
	UserAgent string     `json:"user_agent,omitempty"`
	Referer   string     `json:"referer,omitempty"`
	Cookie    string     `json:"cookie,omitempty"`
	Flags     Flags      `json:"flags,omitempty"`

	// OutputFile / RemoteName record that the response body is not going to
	// stdout, which is how the runner knows body capture will come up empty.
	OutputFile string `json:"output_file,omitempty"` // -o
	RemoteName bool   `json:"remote_name,omitempty"` // -O
	DumpHeader string `json:"dump_header,omitempty"` // user's own -D

	// Unrecognized holds options this table did not know. Their presence means
	// the rest of the parse may be incomplete, and the UI says so.
	Unrecognized []string `json:"unrecognized,omitempty"`

	// Multi is set by --next: a single curl invocation carrying several
	// requests. pogo records it as one entry and flags the ambiguity.
	Multi bool `json:"multi,omitempty"`

	// ConfigFile is set by -K/--config: arguments live in a file pogo did not
	// read, so this metadata is knowingly partial.
	ConfigFile bool `json:"config_file,omitempty"`

	// ReadsStdin is set when a payload references stdin, which the runner must
	// tee rather than consume.
	ReadsStdin bool `json:"reads_stdin,omitempty"`
}

// URL returns the first URL in the command, or "" when none was found.
func (s *Spec) URL() string {
	if len(s.URLs) == 0 {
		return ""
	}
	return s.URLs[0]
}

// Complete reports whether the parse is trustworthy enough to present without
// a caveat in the UI.
func (s *Spec) Complete() bool {
	return len(s.Unrecognized) == 0 && !s.ConfigFile && !s.Multi && len(s.URLs) > 0
}

// bodyKinds maps payload-bearing options to the Kind recorded on a BodyPart.
var bodyKinds = map[string]string{
	"data": "data", "data-raw": "data-raw", "data-binary": "data-binary",
	"data-ascii": "data-ascii", "data-urlencode": "data-urlencode",
	"form": "form", "form-string": "form-string", "json": "json",
	"upload-file": "upload-file", "url-query": "url-query",
}

// shortToLong maps the short options pogo interprets onto their long names, so
// the rest of the parser only has to reason about one spelling.
var shortToLong = map[byte]string{
	'X': "request", 'H': "header", 'd': "data", 'F': "form", 'T': "upload-file",
	'u': "user", 'U': "proxy-user", 'A': "user-agent", 'e': "referer",
	'b': "cookie", 'o': "output", 'D': "dump-header", 'K': "config",
	'I': "head", 'L': "location", 's': "silent", 'v': "verbose",
	'k': "insecure", 'G': "get", 'i': "include", 'O': "remote-name",
	':': "next",
}

// Parse interprets a curl argument list (without the leading "curl").
//
// It never returns an error: an unparseable command still produced a real HTTP
// request, and refusing to record it would be worse than recording it roughly.
func Parse(args []string) *Spec {
	s := &Spec{}
	explicitMethod := ""
	hasBody := false

	// apply folds one recognized option, in long-name form, into the spec.
	apply := func(display, name, value string, hasValue bool) {
		s.Options = append(s.Options, Option{Name: display, Value: value, HasValue: hasValue})

		if kind, ok := bodyKinds[name]; ok {
			hasBody = true
			s.Body = append(s.Body, newBodyPart(kind, value, s))
			return
		}
		switch name {
		case "request":
			explicitMethod = value
		case "header", "proxy-header":
			if name == "header" {
				if h, ok := parseHeader(value); ok {
					s.Headers = append(s.Headers, h)
				}
			}
		case "url":
			s.URLs = append(s.URLs, value)
		case "user":
			s.User = value
		case "proxy-user":
			s.ProxyUser = value
		case "user-agent":
			s.UserAgent = value
		case "referer":
			s.Referer = value
		case "cookie":
			s.Cookie = value
		case "output":
			s.OutputFile = value
		case "remote-name", "remote-name-all":
			s.RemoteName = true
		case "dump-header":
			s.DumpHeader = value
		case "config":
			s.ConfigFile = true
		case "next":
			s.Multi = true
		case "head":
			s.Flags.Head = true
		case "location", "location-trusted":
			s.Flags.Follow = true
		case "silent":
			s.Flags.Silent = true
		case "verbose":
			s.Flags.Verbose = true
		case "insecure":
			s.Flags.Insecure = true
		case "get":
			s.Flags.Get = true
		case "include":
			s.Flags.Include = true
		case "compressed":
			s.Flags.Compress = true
		}
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case strings.HasPrefix(arg, "--"):
			name, value, hasEq := strings.Cut(arg[2:], "=")
			switch {
			case isLongWithValue(name):
				if !hasEq {
					// The value is the next argument. A missing one means the
					// command was malformed; curl will have complained already.
					if i+1 < len(args) {
						i++
						value = args[i]
					}
				}
				apply(arg, name, value, true)
			case isLongBoolean(name):
				apply("--"+name, name, "", false)
			default:
				s.Unrecognized = append(s.Unrecognized, arg)
				// Assume a switch: curl's unknown-to-us options are mostly
				// switches, and guessing "takes a value" would swallow a real
				// URL. A value that lands in operands fails the URL sniff test
				// below and is quarantined instead.
				s.Options = append(s.Options, Option{Name: arg})
			}

		case len(arg) > 1 && arg[0] == '-':
			i += parseCluster(arg, args, i, s, apply)

		default:
			s.addOperand(arg)
		}
	}

	s.Method = inferMethod(explicitMethod, s, hasBody)
	return s
}

// parseCluster handles a short-option token, which may pack several switches
// and optionally end in a value-taking option ("-sSLH" + next arg, or "-dfoo").
// It returns how many extra arguments were consumed.
func parseCluster(arg string, args []string, i int, s *Spec, apply func(display, name, value string, hasValue bool)) int {
	for j := 1; j < len(arg); j++ {
		c := arg[j]
		long, known := shortToLong[c]

		if _, ok := shortWithValue[c]; ok {
			value := ""
			consumed := 0
			if j+1 < len(arg) {
				value = arg[j+1:] // -dfoo
			} else if i+1 < len(args) {
				value = args[i+1] // -d foo
				consumed = 1
			}
			if !known {
				// Known to take a value, but not one pogo interprets. Record it
				// so the option list stays faithful, without inventing meaning.
				s.Options = append(s.Options, Option{Name: "-" + string(c), Value: value, HasValue: true})
			} else {
				apply("-"+string(c), long, value, true)
			}
			return consumed
		}

		if _, ok := shortBoolean[c]; ok {
			if known {
				apply("-"+string(c), long, "", false)
			} else {
				s.Options = append(s.Options, Option{Name: "-" + string(c)})
			}
			continue
		}

		// Unknown short option: assume a switch and keep scanning the cluster.
		s.Unrecognized = append(s.Unrecognized, "-"+string(c))
		s.Options = append(s.Options, Option{Name: "-" + string(c)})
	}
	return 0
}

// addOperand classifies a non-option token. curl treats every operand as a URL,
// but pogo may have mis-guessed the arity of an option it did not recognize, so
// a token that does not look like a URL is quarantined rather than promoted.
func (s *Spec) addOperand(arg string) {
	if LooksLikeURL(arg) {
		s.URLs = append(s.URLs, arg)
		return
	}
	s.Unrecognized = append(s.Unrecognized, arg)
}

func newBodyPart(kind, value string, s *Spec) BodyPart {
	p := BodyPart{Kind: kind, Value: value}
	switch {
	case kind == "upload-file":
		if value == "-" {
			p.Stdin, s.ReadsStdin = true, true
		} else {
			p.File = value
		}
	case strings.HasPrefix(value, "@"):
		// --data-urlencode also accepts "name@file"; the plain "@file" form is
		// the common one and the only one pogo resolves.
		if value == "@-" {
			p.Stdin, s.ReadsStdin = true, true
		} else {
			p.File = value[1:]
		}
	}
	return p
}

func parseHeader(v string) (Header, bool) {
	// "Name: value", "Name;" (send empty), or "@file" (curl 7.55+).
	if strings.HasPrefix(v, "@") {
		return Header{}, false
	}
	if name, value, ok := strings.Cut(v, ":"); ok {
		return Header{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)}, true
	}
	if strings.HasSuffix(v, ";") {
		return Header{Name: strings.TrimSpace(strings.TrimSuffix(v, ";"))}, true
	}
	return Header{}, false
}

// inferMethod reproduces curl's own defaulting rules.
func inferMethod(explicit string, s *Spec, hasBody bool) string {
	if explicit != "" {
		return strings.ToUpper(explicit)
	}
	switch {
	case s.Flags.Head:
		return "HEAD"
	case hasUpload(s):
		return "PUT"
	case s.Flags.Get:
		return "GET" // -G moves any payload into the query string
	case hasBody:
		return "POST"
	default:
		return "GET"
	}
}

func hasUpload(s *Spec) bool {
	for _, b := range s.Body {
		if b.Kind == "upload-file" {
			return true
		}
	}
	return false
}

func isLongWithValue(name string) bool {
	_, ok := longWithValue[name]
	return ok
}

func isLongBoolean(name string) bool {
	if _, ok := longBoolean[name]; ok {
		return true
	}
	// curl accepts --no-<switch> for any boolean option.
	if rest, ok := strings.CutPrefix(name, "no-"); ok {
		_, ok := longBoolean[rest]
		return ok
	}
	return false
}

// OptionTakesValue reports whether an argument is a curl option that consumes
// the following argument. It is exported for editing, where knowing that
// -e <referer> is a value and not an operand prevents a rewrite from landing in
// the wrong place.
func OptionTakesValue(arg string) bool {
	switch {
	case strings.HasPrefix(arg, "--"):
		name, _, hasEq := strings.Cut(arg[2:], "=")
		return !hasEq && isLongWithValue(name)
	case len(arg) > 1 && arg[0] == '-':
		last := arg[len(arg)-1]
		_, ok := shortWithValue[last]
		return ok && len(arg) == 2
	}
	return false
}
