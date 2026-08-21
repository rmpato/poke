package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/rmpato/poke/internal/apis"
	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/environment"
)

func newEnvCmd(app *app) *cobra.Command {
	var (
		domain string
		reveal bool
	)

	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage the environments {{variables}} resolve from",
		Long: `An environment name is global — "staging" means the same word everywhere —
but its values belong to one API, so {{base}} can be acme's staging host for an
acme request and the payments team's for a payments one.

  pogo env set staging --api acme.com base=https://api.staging.acme.com
  pogo env set staging --api acme.com token=sk_test_…
  pogo env use staging
  pogo curl '{{base}}/users' -H 'Authorization: Bearer {{token}}'

Variables are expanded on the way to curl and nowhere else: the history entry
keeps the braces, so the token never lands on disk with your requests. It lands
here instead — ` + "`pogo env`" + ` writes to a file of its own, mode 0600.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         func(*cobra.Command, []string) error { return listEnvs(reveal) },
	}
	cmd.PersistentFlags().StringVar(&domain, "api", "", "the API these variables belong to (default: shared by all)")
	cmd.PersistentFlags().BoolVar(&reveal, "reveal", false, "print values in full instead of masked")

	cmd.AddCommand(
		&cobra.Command{
			Use:          "list",
			Short:        "List environments and the variables they define",
			Args:         cobra.NoArgs,
			SilenceUsage: true,
			RunE:         func(*cobra.Command, []string) error { return listEnvs(reveal) },
		},
		&cobra.Command{
			Use:   "use <name>",
			Short: "Make an environment the active one",
			Long: `use records which environment new requests resolve against. Pass an empty
name to run with no environment at all.

A single request can override it without changing this: pogo curl --pogo-env
prod, or POGO_ENV=prod in front of the command.`,
			Args:         cobra.RangeArgs(0, 1),
			SilenceUsage: true,
			RunE: func(_ *cobra.Command, args []string) error {
				name := ""
				if len(args) > 0 {
					name = args[0]
				}
				set, err := environment.Load(config.EnvFile())
				if err != nil {
					return err
				}
				if name != "" && !set.Has(name) {
					return fmt.Errorf("no environment named %q — create it with: pogo env set %s KEY=VALUE", name, name)
				}
				set.Active = name
				if err := set.Save(config.EnvFile()); err != nil {
					return err
				}
				if name == "" {
					fmt.Println("no environment is active")
					return nil
				}
				fmt.Printf("active environment: %s\n", name)
				return nil
			},
		},
		&cobra.Command{
			Use:   "set <name> KEY=VALUE [KEY=VALUE...]",
			Short: "Define variables in an environment",
			Long: `set writes one or more variables. With --api they belong to that API only;
without it they are shared by every API, and an API's own value of the same
name wins over the shared one.

  pogo env set staging --api acme.com base=https://api.staging.acme.com
  pogo env set staging ua=pogo`,
			Args:         cobra.MinimumNArgs(2),
			SilenceUsage: true,
			RunE: func(_ *cobra.Command, args []string) error {
				name, assignments := args[0], args[1:]
				set, err := environment.Load(config.EnvFile())
				if err != nil {
					return err
				}
				scope := scopeOf(domain)
				for _, a := range assignments {
					key, value, ok := strings.Cut(a, "=")
					if !ok || key == "" {
						return fmt.Errorf("%q is not KEY=VALUE", a)
					}
					set.SetVar(scope, name, key, value)
				}
				if set.Active == "" {
					set.Active = name // the first environment you make is the one you meant
				}
				if err := set.Save(config.EnvFile()); err != nil {
					return err
				}
				fmt.Printf("%s: %d variable(s) set in %s\n", name, len(assignments), scopeLabel(scope))
				return nil
			},
		},
		&cobra.Command{
			Use:          "unset <name> KEY [KEY...]",
			Short:        "Remove variables from an environment",
			Args:         cobra.MinimumNArgs(2),
			SilenceUsage: true,
			RunE: func(_ *cobra.Command, args []string) error {
				name, keys := args[0], args[1:]
				set, err := environment.Load(config.EnvFile())
				if err != nil {
					return err
				}
				scope := scopeOf(domain)
				for _, key := range keys {
					set.UnsetVar(scope, name, key)
				}
				if !set.Has(set.Active) {
					set.Active = ""
				}
				if err := set.Save(config.EnvFile()); err != nil {
					return err
				}
				fmt.Printf("%s: %d variable(s) removed from %s\n", name, len(keys), scopeLabel(scope))
				return nil
			},
		},
	)
	return cmd
}

func scopeOf(domain string) string {
	if domain == "" {
		return ""
	}
	return apis.Domain(domain)
}

func scopeLabel(scope string) string {
	if scope == "" {
		return "the shared set"
	}
	return scope
}

func listEnvs(reveal bool) error {
	set, err := environment.Load(config.EnvFile())
	if err != nil {
		return err
	}

	names := set.Names()
	if len(names) == 0 {
		fmt.Fprintf(os.Stderr, "no environments yet — create one with:\n"+
			"  pogo env set staging --api acme.com base=https://api.staging.acme.com\n")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, name := range names {
		marker := "  "
		if name == set.Active {
			marker = "▸ "
		}
		fmt.Fprintf(w, "%s%s\n", marker, name)

		if vars := set.Shared[name]; len(vars) > 0 {
			fmt.Fprintf(w, "    shared\t%s\n", renderVars(vars, reveal))
		}
		for _, domain := range set.Domains() {
			if vars := set.APIs[domain][name]; len(vars) > 0 {
				fmt.Fprintf(w, "    %s\t%s\n", domain, renderVars(vars, reveal))
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\n%s\n", config.EnvFile())
	return nil
}

func renderVars(vars environment.Vars, reveal bool) string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+maskValue(vars[k], reveal))
	}
	return strings.Join(parts, "  ")
}

// maskValue hides a value unless asked. Environments hold the credentials that
// history deliberately does not, and `pogo env list` is the command someone
// runs while screen-sharing to explain what an environment is.
func maskValue(value string, reveal bool) string {
	if reveal || value == "" {
		return value
	}
	// A URL is not a secret and is the thing you most want to check at a
	// glance; anything else is treated as one.
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if len(value) <= 4 {
		return strings.Repeat("•", len(value))
	}
	return value[:2] + strings.Repeat("•", min(len(value)-4, 8)) + value[len(value)-2:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
