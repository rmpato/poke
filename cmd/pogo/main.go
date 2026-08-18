// Command pogo is a terminal UI over the request history poke records.
//
// It browses, inspects, replays, edits and compares requests. Replaying goes
// through the same execution path poke uses, so pogo never builds a curl
// invocation of its own.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/rmpato/poke/internal/capture"
	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/harimport"
	"github.com/rmpato/poke/internal/selfupdate"
	"github.com/rmpato/poke/internal/store"
	"github.com/rmpato/poke/internal/tui"
	"github.com/rmpato/poke/internal/version"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("pogo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { usage(os.Stderr) }

	var (
		showVersion = fs.Bool("version", false, "print version and exit")
		showPath    = fs.Bool("path", false, "print the history directory and exit")
		list        = fs.Bool("list", false, "print history as text instead of opening the UI")
		asJSON      = fs.Bool("json", false, "with --list, emit JSON Lines")
		limit       = fs.Int("n", 20, "with --list, how many entries to print")
		filter      = fs.String("filter", "", "with --list, a pogo search expression")
		compact     = fs.Bool("compact", false, "rewrite the history log, dropping deleted entries")
		importHAR   = fs.String("import-har", "", "import a browser HAR export into history")
		collection  = fs.String("collection", "", "with --import-har, file the imported requests under this name")
		initConfig  = fs.Bool("init-config", false, "write a default config file and exit")
		update      = fs.Bool("update", false, "install the latest release")
		checkUpdate = fs.Bool("check-update", false, "report whether a newer release exists")
		help        = fs.Bool("help", false, "show help")
	)

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			usage(os.Stdout)
			return 0
		}
		return 2
	}
	if *help {
		usage(os.Stdout)
		return 0
	}
	if *showVersion {
		fmt.Println(version.Line("pogo"))
		return 0
	}
	if *showPath {
		fmt.Println(config.DataDir())
		return 0
	}
	if *update {
		return selfupdate.CLI("pogo", version.Version, os.Stdout, os.Stderr)
	}
	if *checkUpdate {
		return selfupdate.CheckCLI("pogo", version.Version, os.Stdout, os.Stderr)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pogo: %s: %v\n", config.File(), err)
		return 2
	}
	if *initConfig {
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "pogo: %v\n", err)
			return 1
		}
		fmt.Println("wrote", config.File())
		return 0
	}

	st, err := store.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pogo: %v\n", err)
		return 1
	}

	if *importHAR != "" {
		return runImportHAR(st, *importHAR, *collection)
	}

	if *compact {
		stats, err := st.Compact()
		if err != nil {
			fmt.Fprintf(os.Stderr, "pogo: %v\n", err)
			return 1
		}
		fmt.Printf("compacted to %d entries\n", stats.Live)
		return 0
	}

	// A TUI needs a terminal. When stdout is a pipe, print the history instead
	// of failing: `pogo --list | grep 500` should work like any other tool.
	if *list || !isatty.IsTerminal(os.Stdout.Fd()) {
		return printList(st, *filter, *limit, *asJSON)
	}

	rec := capture.New(cfg, st)
	model := tui.New(cfg, st, rec)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "pogo: %v\n", err)
		return 1
	}
	return 0
}

// printList is pogo's non-interactive face.
func printList(st *store.Store, filter string, limit int, asJSON bool) int {
	res, err := st.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pogo: %v\n", err)
		return 1
	}

	query := tui.ParseQuery(filter)
	entries := res.Entries[:0:0]
	for _, e := range res.Entries {
		if query.Empty() || query.Match(e) {
			entries = append(entries, e)
		}
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		for _, e := range entries {
			if err := enc.Encode(e); err != nil {
				return 1
			}
		}
		return 0
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "no requests recorded yet — run: poke https://example.com")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, e := range entries {
		status := "-"
		if s := e.Status(); s > 0 {
			status = fmt.Sprintf("%d", s)
		} else if e.Exit != 0 {
			status = fmt.Sprintf("exit %d", e.Exit)
		}
		star := " "
		if e.Favorite {
			star = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			star,
			e.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			e.Request.Method,
			status,
			e.Duration.String(),
			e.Request.URL)
	}
	if err := w.Flush(); err != nil {
		return 1
	}
	return 0
}

// runImportHAR brings a browser's export into history.
//
// Imported entries are recorded as such and are never presented as something
// poke ran; what they gain is everything else pogo does — search, inspection,
// editing, replay and diffing.
func runImportHAR(st *store.Store, path, collection string) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pogo: %v\n", err)
		return 1
	}
	defer func() { _ = f.Close() }()

	if collection == "" {
		collection = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	res, err := harimport.Parse(f, harimport.Options{Collection: collection})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pogo: %s: %v\n", path, err)
		return 1
	}

	for _, e := range res.Entries {
		// Payloads arrive inline from the parser; hand them to the store so the
		// large ones end up as blobs like any other capture.
		if ref := e.Request.Body; ref != nil {
			if stored, err := st.PutBody(e.ID, store.KindRequest, []byte(ref.Inline), ref.Size, "har"); err == nil {
				e.Request.Body = stored
			}
		}
		if e.Response != nil && e.Response.Body != nil {
			ref := e.Response.Body
			if stored, err := st.PutBody(e.ID, store.KindResponse, []byte(ref.Inline), ref.Size, "har"); err == nil {
				e.Response.Body = stored
			}
		}
		if err := st.Append(e); err != nil {
			fmt.Fprintf(os.Stderr, "pogo: %v\n", err)
			return 1
		}
	}

	fmt.Printf("imported %d requests into collection %q\n", len(res.Entries), collection)
	if res.Skipped > 0 {
		fmt.Printf("skipped %d entries with no URL\n", res.Skipped)
	}
	fmt.Println("browse them with: pogo")
	return 0
}

func usage(w *os.File) {
	fmt.Fprintf(w, `pogo %s — browse, replay and compare the requests poke recorded.

Usage:
  pogo [flags]

With no flags, pogo opens the terminal UI. When stdout is not a terminal it
prints the history as text instead, so it composes with the rest of your shell:

  pogo --list -n 50 --filter 'status:5xx host:api.example.com'
  pogo --json -n 100 | jq 'select(.duration > 500)'

Flags:
  --list                 print history as text instead of opening the UI
  --json                 with --list, emit one JSON object per line
  -n <count>             with --list, how many entries to print (default 20)
  --filter <expr>        with --list, a search expression (see below)
  --import-har <file>    import a browser HAR export (devtools → save all as HAR)
  --collection <name>    with --import-har, file the imports under this name
  --compact              rewrite the history log, dropping deleted entries
  --init-config          write a default config file and exit
  --path                 print the history directory
  --update               install the latest release
  --check-update         report whether a newer release exists
  --version              print pogo's version
  --help                 this help

Search expressions (in the UI, press / — same syntax):
  users/42               free text, matched against method, URL and status
  method:POST            filter by method
  status:4xx             filter by status class, or status:404 for one code
  host:api.example.com   filter by host
  collection:auth        filter by collection
  is:starred             only starred requests
  is:failed              only failures

Keys (press ? in the UI for the full list):
  ↑↓/jk navigate   ⏎ inspect   r replay   e edit   / search
  y copy   s star   c collection   x delete   d compare   t group   q quit

History lives in %s
and includes request headers, which routinely carry credentials.
Secrets are masked on screen; see docs/security.md for what is stored.
`, version.Version, config.DataDir())
}
