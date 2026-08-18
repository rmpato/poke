package curlargs

import (
	"errors"
	"strings"
)

// safeChars are the bytes that never need quoting in a POSIX shell.
const safeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
	"_@%+=:,./-"

// QuoteArg renders one argument so a POSIX shell reproduces it verbatim.
func QuoteArg(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return r > 127 || !strings.ContainsRune(safeChars, r)
	}) < 0 {
		return s
	}
	// Single quotes protect everything except a single quote itself, which has
	// to leave and re-enter the quoted run.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Quote renders an argument list as a single shell-safe line.
func Quote(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = QuoteArg(a)
	}
	return strings.Join(parts, " ")
}

// Render produces a copy-pasteable curl command. When multiline is set, each
// option is placed on its own continued line, which is how people actually read
// a long request.
func Render(args []string, multiline bool) string {
	if !multiline {
		return "curl " + Quote(args)
	}

	var b strings.Builder
	b.WriteString("curl")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		b.WriteString(" \\\n  ")
		b.WriteString(QuoteArg(arg))
		// Keep an option and its value on the same line: "-H 'Accept: ...'"
		// reads as one idea, and splitting them doubles the line count.
		if v, ok := valueFollows(args, i); ok {
			b.WriteString(" ")
			b.WriteString(QuoteArg(v))
			i++
		}
	}
	return b.String()
}

// valueFollows reports whether args[i] is an option whose value is args[i+1].
func valueFollows(args []string, i int) (string, bool) {
	if i+1 >= len(args) {
		return "", false
	}
	arg := args[i]
	if !strings.HasPrefix(arg, "-") || arg == "-" {
		return "", false
	}
	if strings.HasPrefix(arg, "--") {
		name, _, hasEq := strings.Cut(arg[2:], "=")
		if hasEq || !isLongWithValue(name) {
			return "", false
		}
		return args[i+1], true
	}
	// A short cluster consumes the next argument only if its final character
	// takes a value and the value was not already packed into the token.
	last := arg[len(arg)-1]
	if _, ok := shortWithValue[last]; ok && len(arg) == 2 {
		return args[i+1], true
	}
	return "", false
}

// ErrUnbalancedQuote is returned by Split when a quoted run never closes.
var ErrUnbalancedQuote = errors.New("unbalanced quote in command")

// Split tokenizes a shell-ish command line into arguments.
//
// It covers the quoting people actually type into a terminal (single quotes,
// double quotes with backslash escapes, bare backslash escapes, and line
// continuations) and deliberately stops there: no expansion of variables,
// globs, substitutions or operators. poke re-executes the result directly with
// exec, never through a shell, so silently "supporting" $(...) would create a
// command injection surface where there is currently none.
func Split(line string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		started bool
	)
	flush := func() {
		if started {
			args = append(args, cur.String())
			cur.Reset()
			started = false
		}
	}

	for i := 0; i < len(line); i++ {
		c := line[i]
		switch c {
		case ' ', '\t', '\n', '\r':
			flush()
		case '\\':
			if i+1 < len(line) {
				i++
				if line[i] == '\n' {
					continue // line continuation
				}
				cur.WriteByte(line[i])
				started = true
			}
		case '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return nil, ErrUnbalancedQuote
			}
			cur.WriteString(line[i+1 : i+1+end])
			started = true
			i += end + 1
		case '"':
			j := i + 1
			closed := false
			for ; j < len(line); j++ {
				if line[j] == '\\' && j+1 < len(line) {
					j++
					if line[j] == '\n' {
						continue
					}
					// Inside double quotes a backslash only escapes these.
					if strings.IndexByte("\"\\$`", line[j]) < 0 {
						cur.WriteByte('\\')
					}
					cur.WriteByte(line[j])
					continue
				}
				if line[j] == '"' {
					closed = true
					break
				}
				cur.WriteByte(line[j])
			}
			if !closed {
				return nil, ErrUnbalancedQuote
			}
			started = true
			i = j
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	flush()
	return args, nil
}

// StripCurl drops a leading "curl" (or "poke") token so a command copied out of
// the UI, a shell history, or an editor buffer can be fed straight back in.
func StripCurl(args []string) []string {
	if len(args) > 0 {
		switch strings.TrimSuffix(args[0], ".exe") {
		case "curl", "poke":
			return args[1:]
		}
		// Also handle an absolute path such as /usr/bin/curl.
		if i := strings.LastIndexByte(args[0], '/'); i >= 0 {
			switch args[0][i+1:] {
			case "curl", "poke":
				return args[1:]
			}
		}
	}
	return args
}
