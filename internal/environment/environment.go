// Package environment resolves {{variable}} references in a curl command.
//
// The point is not templating for its own sake. Expansion happens on the way to
// curl and nowhere else, so what gets stored is the template:
//
//	poke -H "Authorization: Bearer {{token}}" {{base}}/users
//
// runs against the real token and the real host, while the history entry keeps
// the braces. The secret never reaches history.jsonl, and a replay months later
// picks up whatever the variable holds then — which is usually what you wanted,
// since the token you captured has long since expired.
package environment

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// reference matches {{name}} with optional surrounding whitespace.
var reference = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}`)

// Set is the collection of named environments and which one is active.
type Set struct {
	Active       string                       `json:"active,omitempty"`
	Environments map[string]map[string]string `json:"environments"`
}

// Names returns the environment names in a stable order.
func (s Set) Names() []string {
	names := make([]string, 0, len(s.Environments))
	for name := range s.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Vars returns the variables of an environment, or nil when it does not exist.
func (s Set) Vars(name string) map[string]string {
	if name == "" {
		return nil
	}
	return s.Environments[name]
}

// Load reads the environment file. A missing file yields an empty set, because
// most people never create one and that must not be an error.
func Load(path string) (Set, error) {
	s := Set{Environments: map[string]map[string]string{}}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if s.Environments == nil {
		s.Environments = map[string]map[string]string{}
	}
	return s, nil
}

// Save writes the environment file with owner-only permissions: it holds the
// tokens that history deliberately does not.
func (s Set) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Expand substitutes variables into an argument list.
//
// Unknown references are left exactly as written rather than replaced with an
// empty string: a request to https://{{base}}/users is obviously broken and
// says so, while a request to https:///users looks like a different bug.
func Expand(args []string, vars map[string]string) (expanded []string, missing []string) {
	out := make([]string, len(args))
	seen := map[string]bool{}

	for i, arg := range args {
		out[i] = reference.ReplaceAllStringFunc(arg, func(match string) string {
			name := reference.FindStringSubmatch(match)[1]
			if v, ok := vars[name]; ok {
				return v
			}
			if !seen[name] {
				seen[name] = true
				missing = append(missing, name)
			}
			return match
		})
	}
	sort.Strings(missing)
	return out, missing
}

// References lists the variables a command mentions, in first-seen order. pogo
// uses it to show what a request depends on.
func References(args []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, arg := range args {
		for _, m := range reference.FindAllStringSubmatch(arg, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// UsesVariables reports whether a command references any variable at all.
func UsesVariables(args []string) bool {
	for _, arg := range args {
		if reference.MatchString(arg) {
			return true
		}
	}
	return false
}

// Describe renders a one-line summary of an environment for the UI.
func (s Set) Describe(name string) string {
	vars := s.Vars(name)
	if vars == nil {
		return "no such environment"
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(keys) > 4 {
		return fmt.Sprintf("%d variables: %s, …", len(keys), strings.Join(keys[:4], ", "))
	}
	return fmt.Sprintf("%d variables: %s", len(keys), strings.Join(keys, ", "))
}
