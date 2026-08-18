// Command poke runs curl and remembers what it ran.
//
// poke is a wrapper, not a replacement: arguments are handed to the real curl
// binary untouched, so anything curl accepts, poke accepts, and the output on
// the terminal is curl's own. What poke adds is a local record of the exchange
// that `pogo` can browse, replay and edit.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"

	"github.com/rmpato/poke/internal/capture"
	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/history"
	"github.com/rmpato/poke/internal/runner"
	"github.com/rmpato/poke/internal/selfupdate"
	"github.com/rmpato/poke/internal/store"
	"github.com/rmpato/poke/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// pokeFlags are poke's own options. They live behind a --poke- prefix so that
// no curl option, present or future, can ever collide with them, and so that a
// reader of a shell history can tell at a glance which arguments were curl's.
type pokeFlags struct {
	noCapture bool
	note      string
}

func run(args []string) int {
	// The detached update-check child. Not part of the user interface: poke
	// spawns it so that checking for a release costs the user nothing.
	if len(args) == 1 && args[0] == selfupdate.RefreshFlag() {
		cfg, err := config.Load()
		if err != nil {
			return 1
		}
		return selfupdate.RefreshCache(cfg.Dir(), version.Version)
	}

	// poke intercepts help and version only in first position. Anywhere else
	// they belong to curl, whose own -h and -V mean the same thing.
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help":
			usage(os.Stdout)
			return 0
		case "-V", "--version":
			fmt.Println(version.Line("poke"))
			return 0
		case "--curl-help":
			return execCurl(append([]string{"--help"}, args[1:]...))
		case "--where":
			fmt.Println(config.DataDir())
			return 0
		case "--update":
			return selfupdate.CLI("poke", version.Version, os.Stdout, os.Stderr)
		case "--check-update":
			return selfupdate.CheckCLI("poke", version.Version, os.Stdout, os.Stderr)
		}
	}
	if len(args) == 0 {
		usage(os.Stdout)
		return 0
	}

	curlArgs, flags, err := extractFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "poke: %v\n", err)
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		// A broken config could silently disable a redaction rule the user
		// believes is protecting them, so this is fatal rather than ignored.
		fmt.Fprintf(os.Stderr, "poke: %s: %v\n", config.File(), err)
		return 2
	}
	if flags.noCapture {
		cfg.Capture.Disabled = true
	}

	var st *store.Store
	if !cfg.Capture.Disabled {
		if st, err = store.Open(cfg); err != nil {
			warn("history unavailable: %v", err)
			st = nil
		}
	}

	// Ctrl-C reaches curl directly through the terminal's process group. poke
	// stays out of the way so that curl performs its own cleanup and reports
	// its own status, exactly as it would unwrapped.
	signal.Ignore(syscall.SIGINT, syscall.SIGQUIT)

	dir, _ := os.Getwd()
	rec := capture.New(cfg, st)

	res, err := rec.Run(context.Background(), capture.Request{
		Args:        curlArgs,
		Source:      history.SourcePoke,
		Dir:         dir,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Stdin:       os.Stdin,
		StdoutIsTTY: isatty.IsTerminal(os.Stdout.Fd()),
	})

	if err != nil {
		if res == nil || res.Run == nil || !res.Run.Started {
			fmt.Fprintf(os.Stderr, "poke: %v\n", err)
			if strings.Contains(err.Error(), runner.ErrCurlMissing.Error()) {
				return 127 // conventional "command not found"
			}
			return 2
		}
		// curl ran; only the recording failed. The user's request succeeded and
		// its exit status is what matters, so this is a warning, not a failure.
		warn("could not record request: %v", err)
	}

	if flags.note != "" && res != nil && res.Entry != nil && st != nil {
		if err := st.SetNote(res.Entry.ID, flags.note); err != nil {
			warn("could not save note: %v", err)
		}
	}

	// A release notice is printed after the request, never before it, and only
	// to an interactive terminal so scripts parsing stderr are unaffected.
	if isatty.IsTerminal(os.Stderr.Fd()) && os.Getenv("POKE_QUIET") == "" {
		selfupdate.Notice(cfg.Dir(), version.Version,
			cfg.Update.CheckInterval(), !cfg.Update.Disabled, os.Stderr)
	}

	if res == nil || res.Run == nil {
		return 2
	}
	return res.Run.Exit
}

// extractFlags removes poke's own options from the argument list. Everything
// else is passed through untouched and in its original order.
func extractFlags(args []string) ([]string, pokeFlags, error) {
	var (
		flags pokeFlags
		out   = make([]string, 0, len(args))
	)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--poke-") {
			out = append(out, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "--poke-no-capture":
			flags.noCapture = true
		case "--poke-note":
			if !hasValue {
				if i+1 >= len(args) {
					return nil, flags, fmt.Errorf("option %s: requires parameter", name)
				}
				i++
				value = args[i]
			}
			flags.note = value
		default:
			return nil, flags, fmt.Errorf("option %s: is unknown", name)
		}
	}
	return out, flags, nil
}

// execCurl forwards to the real curl for pass-through subcommands such as
// --curl-help.
func execCurl(args []string) int {
	cfg, _ := config.Load()
	res, err := runner.Run(context.Background(), runner.Options{
		Binary: cfg.Capture.Curl,
		Args:   args,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "poke: %v\n", err)
		return 127
	}
	return res.Exit
}

// warn reports a poke-level problem without pretending the request failed.
// POKE_QUIET silences these for scripts that parse curl's stderr.
func warn(format string, args ...any) {
	if os.Getenv("POKE_QUIET") != "" {
		return
	}
	fmt.Fprintf(os.Stderr, "poke: "+format+"\n", args...)
}

func usage(w *os.File) {
	fmt.Fprintf(w, `poke %s — curl, but it remembers.

Usage:
  poke [curl options] <url>

poke passes every argument straight to curl, so any curl command works
unchanged. It records the request, the response and how long it took, and
`+"`pogo`"+` browses that history.

Examples:
  poke https://api.example.com/users
  poke -X POST -H 'Content-Type: application/json' \
       -d '{"name":"Pato"}' https://api.example.com/users
  poke -sSL -o page.html https://example.com

poke's own options (everything else belongs to curl):
  --poke-no-capture      run the request without recording it
  --poke-note <text>     attach a note to this entry
  --help, -h             this help (first argument only)
  --version, -V          print poke's version
  --curl-help            forward to `+"`curl --help`"+`
  --where                print the history directory
  --update               install the latest release
  --check-update         report whether a newer release exists

Environment:
  POKE_HOME              history directory (default $XDG_DATA_HOME/poke)
  POKE_CONFIG            config file (default $XDG_CONFIG_HOME/poke/config.json)
  POKE_CURL              curl binary to delegate to
  POKE_NO_CAPTURE        set to disable recording entirely
  POKE_REDACT            display | store | off
  POKE_QUIET             suppress poke's own warnings
  POKE_NO_UPDATE_CHECK   never look for new releases

History is stored unencrypted in %s and includes request headers,
which routinely carry credentials. See docs/security.md before sharing it.

Browse it with: pogo
`, version.Version, config.DataDir())
}
