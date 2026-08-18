// Package curledit applies structured edits to a curl command line.
//
// The obvious implementation — read a request into fields, then generate a
// fresh command from those fields — quietly destroys anything the fields do not
// model. A request carrying --cacert, --resolve, -k or --unix-socket would come
// back without them, and the replay would fail for reasons the user could not
// see.
//
// So editing works the other way round: the original argv is authoritative, and
// each change is applied to it in place. Change a header value and only that
// -H argument is rewritten. Everything poke does not understand travels along
// untouched, which is the same property that makes capture safe.
package curledit

import (
	"net/url"
	"sort"
	"strings"

	"github.com/rmpato/poke/internal/curlargs"
)

// Param is one query-string parameter.
type Param struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Form is the editable view of a request.
type Form struct {
	Method  string
	URL     string // without the query string; Query holds that
	Query   []Param
	Headers []curlargs.Header
	Body    string
}

// FormOf builds the editable view from a parsed command and its request body.
func FormOf(spec *curlargs.Spec, body string) Form {
	f := Form{
		Method:  spec.Method,
		Headers: append([]curlargs.Header(nil), spec.Headers...),
		Body:    body,
	}
	f.URL, f.Query = splitQuery(spec.URL())
	return f
}

// splitQuery separates a URL from its query parameters, preserving order and
// repeated keys, both of which are meaningful.
func splitQuery(raw string) (string, []Param) {
	base, query, ok := strings.Cut(raw, "?")
	if !ok || query == "" {
		return raw, nil
	}
	// A fragment stays attached to the base; curl does not send it anyway.
	frag := ""
	if q, f, ok := strings.Cut(query, "#"); ok {
		query, frag = q, "#"+f
	}

	var params []Param
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		params = append(params, Param{Key: decode(k), Value: decode(v)})
	}
	return base + frag, params
}

// JoinURL recombines a base URL and its parameters.
func (f Form) JoinURL() string {
	if len(f.Query) == 0 {
		return f.URL
	}
	base, frag, hasFrag := strings.Cut(f.URL, "#")

	parts := make([]string, 0, len(f.Query))
	for _, p := range f.Query {
		if p.Key == "" {
			continue
		}
		if p.Value == "" {
			parts = append(parts, encode(p.Key))
			continue
		}
		parts = append(parts, encode(p.Key)+"="+encode(p.Value))
	}
	if len(parts) == 0 {
		if hasFrag {
			return base + "#" + frag
		}
		return base
	}

	out := base + "?" + strings.Join(parts, "&")
	if hasFrag {
		out += "#" + frag
	}
	return out
}

// decode and encode keep values readable while editing. Percent-encoding is
// reapplied on the way out, but only where it is needed: rewriting an unchanged
// URL must not alter bytes the server might care about.
func decode(s string) string {
	if !strings.ContainsAny(s, "%+") {
		return s
	}
	if out, err := url.QueryUnescape(s); err == nil {
		return out
	}
	return s
}

func encode(s string) string {
	if s == url.QueryEscape(s) {
		return s
	}
	return url.QueryEscape(s)
}

// Apply rewrites args so that it expresses want instead of have.
//
// have must be the form as it was read from args; the difference between the
// two is what gets applied. Anything in args that neither form describes is
// preserved exactly.
func Apply(args []string, have, want Form) []string {
	out := append([]string(nil), args...)

	if want.Method != have.Method {
		out = setMethod(out, want.Method)
	}

	haveURL, wantURL := have.JoinURL(), want.JoinURL()
	if wantURL != haveURL {
		out = setURL(out, haveURL, wantURL)
	}

	out = applyHeaders(out, have.Headers, want.Headers)

	if want.Body != have.Body {
		out = setBody(out, want.Body)
	}
	return out
}

// setMethod rewrites an existing -X, or adds one.
func setMethod(args []string, method string) []string {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-X" || args[i] == "--request":
			if i+1 < len(args) {
				args[i+1] = method
				return args
			}
		case strings.HasPrefix(args[i], "--request="):
			args[i] = "--request=" + method
			return args
		case strings.HasPrefix(args[i], "-X") && len(args[i]) > 2:
			args[i] = "-X" + method
			return args
		}
	}
	// -I sends HEAD without -X; changing the method means it has to go, or curl
	// would still suppress the body.
	if method != "HEAD" {
		args = removeFlags(args, "-I", "--head")
	}
	return append([]string{"-X", method}, args...)
}

// setURL replaces the URL wherever it appears, whether as an operand or as the
// value of --url.
func setURL(args []string, old, replacement string) []string {
	for i := 0; i < len(args); i++ {
		if args[i] == old {
			// Guard against replacing the *value* of an option that happens to
			// equal the URL, such as -e <referer>.
			if i > 0 && takesValue(args[i-1]) && args[i-1] != "--url" {
				continue
			}
			args[i] = replacement
			return args
		}
		if strings.HasPrefix(args[i], "--url=") && args[i][len("--url="):] == old {
			args[i] = "--url=" + replacement
			return args
		}
	}
	// The old URL was not found verbatim (it may have been assembled from
	// --url plus globbing). Appending is the honest fallback: curl uses the
	// last URL, and the user sees the result before running it.
	return append(args, replacement)
}

// applyHeaders rewrites, removes and adds -H arguments to match want.
func applyHeaders(args []string, have, want []curlargs.Header) []string {
	// Match by position within same-named headers, so two headers with the same
	// name stay distinguishable.
	type key struct {
		name string
		n    int
	}
	index := func(hs []curlargs.Header) map[key]string {
		seen := map[string]int{}
		out := map[key]string{}
		for _, h := range hs {
			lower := strings.ToLower(h.Name)
			out[key{lower, seen[lower]}] = h.Value
			seen[lower]++
		}
		return out
	}
	haveIdx, wantIdx := index(have), index(want)

	seen := map[string]int{}
	out := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		name, _, ok := headerArg(args, i)
		if !ok {
			out = append(out, args[i])
			continue
		}
		width := 1
		if args[i] == "-H" || args[i] == "--header" {
			width = 2
		}

		lower := strings.ToLower(name)
		k := key{lower, seen[lower]}
		seen[lower]++

		newValue, kept := wantIdx[k]
		if !kept {
			i += width - 1 // dropped
			continue
		}
		if newValue == haveIdx[k] {
			out = append(out, args[i:i+width]...)
			i += width - 1
			continue
		}
		out = append(out, "-H", name+": "+newValue)
		i += width - 1
	}

	// Anything in want that was not present before is appended, in order.
	existing := map[key]bool{}
	for k := range haveIdx {
		existing[k] = true
	}
	counts := map[string]int{}
	for _, h := range want {
		lower := strings.ToLower(h.Name)
		k := key{lower, counts[lower]}
		counts[lower]++
		if !existing[k] {
			out = append(out, "-H", h.Name+": "+h.Value)
		}
	}
	return out
}

// headerArg reports whether args[i] starts a header option, and what it says.
func headerArg(args []string, i int) (name, value string, ok bool) {
	var raw string
	switch {
	case args[i] == "-H" || args[i] == "--header":
		if i+1 >= len(args) {
			return "", "", false
		}
		raw = args[i+1]
	case strings.HasPrefix(args[i], "-H") && len(args[i]) > 2:
		raw = args[i][2:]
	case strings.HasPrefix(args[i], "--header="):
		raw = args[i][len("--header="):]
	default:
		return "", "", false
	}

	n, v, found := strings.Cut(raw, ":")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(n), strings.TrimSpace(v), true
}

// setBody rewrites the payload, adding -d when the request had none.
func setBody(args []string, body string) []string {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-d", "--data", "--data-raw", "--data-binary", "--data-ascii", "--json":
			if i+1 < len(args) {
				args[i+1] = body
				return args
			}
		}
		for _, prefix := range []string{"--data=", "--data-raw=", "--data-binary=", "--data-ascii=", "--json="} {
			if strings.HasPrefix(args[i], prefix) {
				args[i] = prefix + body
				return args
			}
		}
		if strings.HasPrefix(args[i], "-d") && len(args[i]) > 2 {
			args[i] = "-d" + body
			return args
		}
	}
	if body == "" {
		return args
	}
	return append(args, "-d", body)
}

func removeFlags(args []string, flags ...string) []string {
	drop := map[string]bool{}
	for _, f := range flags {
		drop[f] = true
	}
	out := args[:0]
	for _, a := range args {
		if !drop[a] {
			out = append(out, a)
		}
	}
	return out
}

// takesValue reports whether an argument is an option that consumes the next
// one, so a value is never mistaken for an operand.
func takesValue(arg string) bool {
	return curlargs.OptionTakesValue(arg)
}

// SortedParams returns query parameters ordered by key, for display only.
func SortedParams(params []Param) []Param {
	out := append([]Param(nil), params...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
