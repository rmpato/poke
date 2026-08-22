package history

import (
	"fmt"
	"strings"

	"github.com/rmpato/pogo/internal/curlargs"
)

// Placeholder is what replaces a secret once it has been removed at capture
// time. It is distinct from the display mask so that "this was never stored"
// and "this is stored but hidden right now" never look the same.
const Placeholder = "<redacted by pogo>"

// Policy decides what counts as a secret and when it is removed.
//
// Two very different things are being controlled here:
//
//   - ModeDisplay (the default) stores the request as it was sent and hides
//     secrets in the UI. Replay works. Anyone who can read the history file can
//     read the secrets.
//   - ModeStore removes secrets before they ever reach disk. Nothing sensitive
//     is persisted; replaying such a request will fail to authenticate.
//
// The default is deliberately the honest-but-unsafe one: a request history that
// silently breaks replay would be worse than useless, so pogo keeps the data
// and documents the exposure instead of pretending it is harmless.
type Policy struct {
	Mode        Mode     `json:"mode" yaml:"mode,omitempty"`
	Headers     []string `json:"headers,omitempty" yaml:"headers,omitempty"`           // added to the defaults
	QueryParams []string `json:"query_params,omitempty" yaml:"query_params,omitempty"` // added to the defaults
	Off         bool     `json:"off,omitempty" yaml:"off,omitempty"`                   // treat nothing as sensitive

	// Hosts overrides the policy for particular hosts. Production credentials
	// and a local dev server rarely deserve the same treatment, and forcing one
	// setting on both means picking the wrong one somewhere.
	//
	//	{"hosts": {"api.stripe.com": {"mode": "store"},
	//	           "localhost:8080": {"off": true}}}
	//
	// A host key matches exactly, or as a suffix when written as ".example.com".
	Hosts map[string]HostPolicy `json:"hosts,omitempty" yaml:"hosts,omitempty"`
}

// HostPolicy is the subset of a policy that can be overridden per host.
type HostPolicy struct {
	Mode    Mode     `json:"mode,omitempty" yaml:"mode,omitempty"`
	Headers []string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Off     *bool    `json:"off,omitempty" yaml:"off,omitempty"`
}

// For returns the policy that applies to a host, merging any override over the
// defaults. The result is a plain Policy, so every call site keeps working
// without knowing that per-host rules exist.
func (p Policy) For(host string) Policy {
	if len(p.Hosts) == 0 || host == "" {
		return p
	}

	override, ok := p.Hosts[host]
	if !ok {
		// Suffix rules let one entry cover every subdomain.
		for pattern, candidate := range p.Hosts {
			if strings.HasPrefix(pattern, ".") && strings.HasSuffix(strings.ToLower(host), strings.ToLower(pattern)) {
				override, ok = candidate, true
				break
			}
		}
	}
	if !ok {
		return p
	}

	out := p
	if override.Mode != "" {
		out.Mode = override.Mode
	}
	if override.Off != nil {
		out.Off = *override.Off
	}
	if len(override.Headers) > 0 {
		out.Headers = append(append([]string(nil), p.Headers...), override.Headers...)
	}
	return out
}

// Mode selects when redaction happens.
type Mode string

const (
	ModeDisplay Mode = "display" // store secrets, hide them in the UI
	ModeStore   Mode = "store"   // never write secrets to disk
)

// DefaultPolicy hides secrets in the UI but keeps them on disk so replay works.
func DefaultPolicy() Policy { return Policy{Mode: ModeDisplay} }

// defaultSensitiveHeaders are matched case-insensitively.
var defaultSensitiveHeaders = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
	"api-key",
	"x-auth-token",
	"x-amz-security-token",
	"x-csrf-token",
	"x-xsrf-token",
}

// defaultSensitiveParams are query-string keys whose values are masked.
var defaultSensitiveParams = []string{
	"access_token", "api_key", "apikey", "auth", "code", "id_token",
	"password", "refresh_token", "secret", "signature", "sig", "token",
}

// sensitiveOptions are curl options whose argument is a credential.
var sensitiveOptions = map[string]bool{
	"-u": true, "--user": true,
	"-U": true, "--proxy-user": true,
	"-b": true, "--cookie": true,
	"--oauth2-bearer":         true,
	"--pass":                  true,
	"--tlspassword":           true,
	"--proxy-tlspassword":     true,
	"--proxy-header":          true,
	"--aws-sigv4":             true,
	"--socks5-gssapi-service": true,
}

// SensitiveHeader reports whether a header name is treated as a secret.
func (p Policy) SensitiveHeader(name string) bool {
	if p.Off {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(name))
	for _, h := range defaultSensitiveHeaders {
		if n == h {
			return true
		}
	}
	for _, h := range p.Headers {
		if n == strings.ToLower(strings.TrimSpace(h)) {
			return true
		}
	}
	return false
}

// SensitiveOption reports whether a curl option carries a credential.
func (p Policy) SensitiveOption(name string) bool {
	if p.Off {
		return false
	}
	return sensitiveOptions[name]
}

func (p Policy) sensitiveParam(key string) bool {
	if p.Off {
		return false
	}
	k := strings.ToLower(key)
	for _, s := range defaultSensitiveParams {
		if k == s {
			return true
		}
	}
	for _, s := range p.QueryParams {
		if k == strings.ToLower(strings.TrimSpace(s)) {
			return true
		}
	}
	return false
}

// authSchemes are kept in cleartext when masking an Authorization header: the
// scheme is diagnostically useful ("is this Bearer or Basic?") and is not the
// secret.
var authSchemes = []string{"Bearer", "Basic", "Digest", "Token", "ApiKey", "AWS4-HMAC-SHA256"}

// MaskValue hides a secret while keeping the shape of it visible, because
// "Bearer ●●●● (184 chars)" tells a developer something useful and the raw
// token tells a shoulder-surfer something more useful still.
func MaskValue(headerName, value string) string {
	if value == "" {
		return ""
	}
	if strings.EqualFold(headerName, "cookie") || strings.EqualFold(headerName, "set-cookie") {
		return maskCookie(value)
	}
	for _, scheme := range authSchemes {
		if len(value) > len(scheme) && strings.EqualFold(value[:len(scheme)], scheme) &&
			value[len(scheme)] == ' ' {
			return scheme + " " + dots(value[len(scheme)+1:])
		}
	}
	return dots(value)
}

// maskCookie keeps cookie *names* (which are rarely secret and often the whole
// reason you are looking at the header) and masks their values.
func maskCookie(v string) string {
	parts := strings.Split(v, ";")
	for i, p := range parts {
		lead := ""
		trimmed := strings.TrimLeft(p, " ")
		lead = p[:len(p)-len(trimmed)]
		if name, val, ok := strings.Cut(trimmed, "="); ok {
			parts[i] = lead + name + "=" + dots(val)
		} else {
			parts[i] = p
		}
	}
	return strings.Join(parts, ";")
}

func dots(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("%s (%d chars)", strings.Repeat("●", 8), len(s))
}

// MaskURL hides credentials embedded in a URL's userinfo and in query
// parameters known to carry tokens.
func (p Policy) MaskURL(raw string) string {
	if p.Off || raw == "" {
		return raw
	}
	out := raw

	// scheme://user:pass@host -> scheme://user:●●●@host
	if scheme, rest, ok := strings.Cut(out, "://"); ok {
		authority := rest
		tail := ""
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			authority, tail = rest[:i], rest[i:]
		}
		if at := strings.LastIndexByte(authority, '@'); at >= 0 {
			userinfo := authority[:at]
			if user, _, hasPass := strings.Cut(userinfo, ":"); hasPass {
				userinfo = user + ":" + strings.Repeat("●", 6)
			}
			authority = userinfo + "@" + authority[at+1:]
		}
		out = scheme + "://" + authority + tail
	}

	base, query, hasQuery := strings.Cut(out, "?")
	if !hasQuery {
		return out
	}
	frag := ""
	if q, f, ok := strings.Cut(query, "#"); ok {
		query, frag = q, "#"+f
	}
	pairs := strings.Split(query, "&")
	for i, pair := range pairs {
		if k, v, ok := strings.Cut(pair, "="); ok && v != "" && p.sensitiveParam(k) {
			pairs[i] = k + "=" + strings.Repeat("●", 8)
		}
	}
	return base + "?" + strings.Join(pairs, "&") + frag
}

// redactOption decides what to do with a recorded curl option.
//
// Two separate things can be sensitive: options whose whole argument is a
// credential (-u, --oauth2-bearer), and header options whose argument happens to
// carry one (-H 'Authorization: ...'). Missing the second kind leaves the secret
// sitting in the recorded option list even after the header list was cleaned,
// which is exactly the sort of near-miss that makes redaction untrustworthy.
func (p Policy) redactOption(name, value string, replace func(string) string) (string, bool) {
	if value == "" {
		return value, false
	}
	if p.SensitiveOption(name) {
		return replace(value), true
	}
	if isHeaderOpt(name) {
		hname, hvalue, ok := strings.Cut(value, ":")
		if ok && p.SensitiveHeader(hname) {
			return hname + ": " + replace(strings.TrimSpace(hvalue)), true
		}
	}
	return value, false
}

// Masked returns a display copy of an entry with secrets hidden. The original
// is never mutated: the caller is showing a view, not changing history.
func (p Policy) Masked(e *Entry) *Entry {
	p = p.For(HostOf(e.Request.URL))
	if p.Off {
		return e
	}
	c := *e
	c.Request = p.maskRequest(e.Request)
	c.Command = Command{Args: p.MaskArgs(e.Command.Args), Dir: e.Command.Dir}
	if e.Response != nil {
		r := *e.Response
		r.Blocks = make([]Block, len(e.Response.Blocks))
		for i, b := range e.Response.Blocks {
			nb := b
			nb.Headers = p.maskHeaders(b.Headers)
			r.Blocks[i] = nb
		}
		c.Response = &r
	}
	return &c
}

func (p Policy) maskRequest(r Request) Request {
	out := r
	out.URL = p.MaskURL(r.URL)
	out.Headers = p.maskHeaders(r.Headers)
	out.Options = make([]curlargs.Option, len(r.Options))
	for i, o := range r.Options {
		if o.HasValue {
			o.Value, _ = p.redactOption(o.Name, o.Value, func(v string) string {
				return MaskValue(headerNameOf(o.Name, o.Value), v)
			})
		}
		out.Options[i] = o
	}
	return out
}

func (p Policy) maskHeaders(hs []curlargs.Header) []curlargs.Header {
	if hs == nil {
		return nil
	}
	out := make([]curlargs.Header, len(hs))
	for i, h := range hs {
		if p.SensitiveHeader(h.Name) {
			h.Value = MaskValue(h.Name, h.Value)
		}
		out[i] = h
	}
	return out
}

// MaskArgs returns a copy of a command line with secret-bearing arguments
// masked. This is what "copy as redacted curl" produces: safe to paste into a
// bug report, and obviously not runnable.
func (p Policy) MaskArgs(args []string) []string {
	if p.Off {
		return args
	}
	out := make([]string, len(args))
	copy(out, args)

	for i := 0; i < len(out); i++ {
		arg := out[i]
		switch {
		case p.SensitiveOption(arg) && i+1 < len(out):
			out[i+1] = dots(out[i+1])
			i++
		case strings.HasPrefix(arg, "--"):
			if name, value, ok := strings.Cut(arg[2:], "="); ok && p.SensitiveOption("--"+name) {
				out[i] = "--" + name + "=" + dots(value)
			}
		case isHeaderOpt(arg) && i+1 < len(out):
			out[i+1] = p.maskHeaderArg(out[i+1])
			i++
		case strings.HasPrefix(arg, "-H") && len(arg) > 2:
			out[i] = "-H" + p.maskHeaderArg(arg[2:])
		case strings.HasPrefix(arg, "-u") && len(arg) > 2:
			out[i] = "-u" + dots(arg[2:])
		case curlargs.LooksLikeURL(arg):
			out[i] = p.MaskURL(arg)
		}
	}
	return out
}

func isHeaderOpt(a string) bool { return a == "-H" || a == "--header" || a == "--proxy-header" }

// headerNameOf returns the header name carried by a header option, so the mask
// can keep an auth scheme or a set of cookie names visible.
func headerNameOf(optName, value string) string {
	if !isHeaderOpt(optName) {
		return ""
	}
	name, _, _ := strings.Cut(value, ":")
	return name
}

func (p Policy) maskHeaderArg(v string) string {
	name, value, ok := strings.Cut(v, ":")
	if !ok || !p.SensitiveHeader(name) {
		return v
	}
	return name + ": " + MaskValue(name, strings.TrimSpace(value))
}

// Strip removes secrets from an entry destined for disk, in place. It is only
// called when Mode is ModeStore, and it marks the entry so the UI can explain
// why replaying it will not authenticate.
func (p Policy) Strip(e *Entry) {
	p = p.For(HostOf(e.Request.URL))
	if p.Off || p.Mode != ModeStore {
		return
	}
	stripped := false

	for i, h := range e.Request.Headers {
		if p.SensitiveHeader(h.Name) {
			e.Request.Headers[i].Value = Placeholder
			stripped = true
		}
	}
	for i, o := range e.Request.Options {
		if !o.HasValue {
			continue
		}
		if v, changed := p.redactOption(o.Name, o.Value, func(string) string { return Placeholder }); changed {
			e.Request.Options[i].Value = v
			stripped = true
		}
	}
	if e.Response != nil {
		for bi, b := range e.Response.Blocks {
			for hi, h := range b.Headers {
				if p.SensitiveHeader(h.Name) {
					e.Response.Blocks[bi].Headers[hi].Value = Placeholder
					stripped = true
				}
			}
		}
	}

	if masked := p.MaskURL(e.Request.URL); masked != e.Request.URL {
		e.Request.URL = masked
		stripped = true
	}
	if args := p.stripArgs(e.Command.Args); args != nil {
		e.Command.Args = args
		stripped = true
	}
	if stripped {
		e.Redacted = true
	}
}

// stripArgs replaces secret arguments with Placeholder, returning nil when
// there was nothing to remove.
func (p Policy) stripArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	changed := false

	for i := 0; i < len(out); i++ {
		arg := out[i]
		switch {
		case p.SensitiveOption(arg) && i+1 < len(out):
			out[i+1], changed = Placeholder, true
			i++
		case strings.HasPrefix(arg, "--"):
			if name, _, ok := strings.Cut(arg[2:], "="); ok && p.SensitiveOption("--"+name) {
				out[i], changed = "--"+name+"="+Placeholder, true
			}
		case isHeaderOpt(arg) && i+1 < len(out):
			if name, _, ok := strings.Cut(out[i+1], ":"); ok && p.SensitiveHeader(name) {
				out[i+1], changed = name+": "+Placeholder, true
			}
			i++
		case strings.HasPrefix(arg, "-H") && len(arg) > 2:
			if name, _, ok := strings.Cut(arg[2:], ":"); ok && p.SensitiveHeader(name) {
				out[i], changed = "-H"+name+": "+Placeholder, true
			}
		case strings.HasPrefix(arg, "-u") && len(arg) > 2:
			out[i], changed = "-u"+Placeholder, true
		case curlargs.LooksLikeURL(arg):
			if m := p.MaskURL(arg); m != arg {
				out[i], changed = m, true
			}
		}
	}
	if !changed {
		return nil
	}
	return out
}
