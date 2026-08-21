// Package environment resolves {{variable}} references in a curl command.
//
// The point is not templating for its own sake. Expansion happens on the way to
// curl and nowhere else, so what gets stored is the template:
//
//	pogo curl -H "Authorization: Bearer {{token}}" {{base}}/users
//
// runs against the real token and the real host, while the history entry keeps
// the braces. The secret never reaches history.jsonl, and a replay months later
// picks up whatever the variable holds then — which is usually what you wanted,
// since the token you captured has long since expired.
//
// Variables are organised the way APIs actually are: an environment *name* is
// global, so "staging" means the same word everywhere, but its *values* belong
// to one API. `{{base}}` in staging is acme's staging host for an acme request
// and the payments team's staging host for a payments one, and neither of them
// has to be called `acme_staging_base`.
package environment

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rmpato/poke/internal/apis"
)

// reference matches {{name}} with optional surrounding whitespace.
var reference = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}`)

// Vars is one environment's variables for one API.
type Vars map[string]string

// Set is every environment pogo knows about, and which one is active.
//
// Shared holds variables that are true whatever you are calling — a user
// agent, a company-wide proxy. APIs holds the ones that are not, keyed by
// registrable domain and then by environment name. An API's value wins over a
// shared one of the same name, because the more specific statement is the one
// the user meant.
type Set struct {
	Active string                     `yaml:"active,omitempty"`
	Shared map[string]Vars            `yaml:"shared,omitempty"`
	APIs   map[string]map[string]Vars `yaml:"apis,omitempty"`
}

// Names returns every environment name that has variables anywhere, ordered
// the way a pipeline reads: production first.
func (s Set) Names() []string {
	seen := map[string]bool{}
	for name := range s.Shared {
		seen[name] = true
	}
	for _, envs := range s.APIs {
		for name := range envs {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sortEnvNames(names)
	return names
}

// EnvsFor returns the environments one API defines variables for.
func (s Set) EnvsFor(domain string) []string {
	envs := s.APIs[strings.ToLower(domain)]
	names := make([]string, 0, len(envs))
	for name := range envs {
		names = append(names, name)
	}
	sortEnvNames(names)
	return names
}

// Domains returns every API that has variables, alphabetically.
func (s Set) Domains() []string {
	out := make([]string, 0, len(s.APIs))
	for domain := range s.APIs {
		out = append(out, domain)
	}
	sort.Strings(out)
	return out
}

func sortEnvNames(names []string) {
	sort.Slice(names, func(i, j int) bool {
		ri, rj := apis.EnvRank(names[i]), apis.EnvRank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
}

// Vars returns the variables in force for one API in one environment: the
// shared set for that environment, with the API's own values layered over it.
//
// A nil result means the environment does not exist at all, which is different
// from an environment that exists and defines nothing — the first is a typo
// worth reporting, the second is fine.
func (s Set) Vars(domain, env string) Vars {
	if env == "" {
		return nil
	}
	shared, hasShared := s.Shared[env]
	own, hasOwn := s.APIs[strings.ToLower(domain)][env]
	if !hasShared && !hasOwn {
		return nil
	}

	out := make(Vars, len(shared)+len(own))
	for k, v := range shared {
		out[k] = v
	}
	for k, v := range own {
		out[k] = v
	}
	return out
}

// Has reports whether an environment name exists anywhere in the set.
func (s Set) Has(env string) bool {
	if env == "" {
		return false
	}
	if _, ok := s.Shared[env]; ok {
		return true
	}
	for _, envs := range s.APIs {
		if _, ok := envs[env]; ok {
			return true
		}
	}
	return false
}

// SetVar records one variable. An empty domain writes to the shared set.
func (s *Set) SetVar(domain, env, name, value string) {
	if env == "" || name == "" {
		return
	}
	if domain == "" {
		if s.Shared == nil {
			s.Shared = map[string]Vars{}
		}
		if s.Shared[env] == nil {
			s.Shared[env] = Vars{}
		}
		s.Shared[env][name] = value
		return
	}
	domain = strings.ToLower(domain)
	if s.APIs == nil {
		s.APIs = map[string]map[string]Vars{}
	}
	if s.APIs[domain] == nil {
		s.APIs[domain] = map[string]Vars{}
	}
	if s.APIs[domain][env] == nil {
		s.APIs[domain][env] = Vars{}
	}
	s.APIs[domain][env][name] = value
}

// UnsetVar removes one variable, and any container it leaves empty, so that
// deleting the last variable of an environment removes the environment rather
// than leaving an empty stanza that still shows up in every picker.
func (s *Set) UnsetVar(domain, env, name string) {
	if domain == "" {
		delete(s.Shared[env], name)
		if len(s.Shared[env]) == 0 {
			delete(s.Shared, env)
		}
		return
	}
	domain = strings.ToLower(domain)
	delete(s.APIs[domain][env], name)
	if len(s.APIs[domain][env]) == 0 {
		delete(s.APIs[domain], env)
	}
	if len(s.APIs[domain]) == 0 {
		delete(s.APIs, domain)
	}
}

// Load reads the environment file. A missing file yields an empty set, because
// most people never create one and that must not be an error.
func Load(path string) (Set, error) {
	var s Set

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Save writes the environment file with owner-only permissions: it holds the
// tokens that history deliberately does not.
func (s Set) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	header := "# pogo environments. This file holds credentials; it is not history.\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o600)
}

// Expand substitutes variables into an argument list.
//
// Unknown references are left exactly as written rather than replaced with an
// empty string: a request to https://{{base}}/users is obviously broken and
// says so, while a request to https:///users looks like a different bug.
func Expand(args []string, vars Vars) (expanded []string, missing []string) {
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

// Describe renders a one-line summary of what an environment holds for an API.
func (s Set) Describe(domain, env string) string {
	vars := s.Vars(domain, env)
	if vars == nil {
		return "no variables"
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
