package curlargs

import "strings"

// LooksLikeURL reports whether a bare operand is plausibly a curl URL.
//
// curl treats every operand as a URL, but pogo cannot: if it mis-guessed the
// arity of an option it did not recognize, that option's *value* arrives here
// looking like an operand. Sniffing keeps such a value out of the URL field,
// where it would be actively misleading, at the cost of occasionally
// quarantining an exotic-but-real URL into Spec.Unrecognized.
//
// The schemeless forms curl accepts ("example.com", "localhost:8080/x") are
// allowed, which is why a hostname-shaped value such as "config.json" can still
// slip through. That is a knowingly loose edge; it only affects display.
func LooksLikeURL(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") || strings.ContainsAny(s, " \t\n") {
		return false
	}
	if strings.Contains(s, "://") {
		return true
	}
	// A {{variable}} reference stands in for whatever it resolves to, so the
	// usual shape tests cannot apply: "{{base}}/users" has no dot and no
	// scheme. In operand position a template is a URL — an option's value never
	// reaches here, because the option consumed it.
	if hasTemplate(s) {
		return true
	}
	// Filesystem paths are never schemeless URLs.
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "~") {
		return false
	}
	// [::1]:8080/path
	if strings.HasPrefix(s, "[") && strings.Contains(s, "]") {
		return true
	}

	host := s
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return false
	}
	hostname, port, hasPort := strings.Cut(host, ":")
	if hasPort && !isDigits(port) {
		return false
	}
	if hostname == "localhost" {
		return true
	}
	// Anything else needs a dotted name: "example.com", "10.0.0.1".
	return strings.Contains(hostname, ".") && !strings.HasSuffix(hostname, ".") &&
		!strings.HasPrefix(hostname, ".")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// hasTemplate reports whether a token contains a {{variable}} reference.
func hasTemplate(s string) bool {
	open := strings.Index(s, "{{")
	if open < 0 {
		return false
	}
	return strings.Contains(s[open+2:], "}}")
}
