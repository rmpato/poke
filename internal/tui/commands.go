package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// command is one thing pogo can do.
//
// Actions are data rather than a switch statement because three separate parts
// of the UI need the same list: the key handler dispatches into it, the palette
// searches it, and the help screen is generated from it. When those three drift
// apart the result is a feature nobody can find — which is the problem this
// whole registry exists to prevent.
type command struct {
	id    string
	title string
	desc  string
	keys  string
	group string

	// motion marks navigation that belongs in the help reference but would only
	// be noise in a palette: nobody searches for "move down".
	motion bool

	// when reports whether the command applies right now. Unavailable commands
	// are shown greyed rather than hidden, so the palette teaches what exists
	// even when it cannot be used yet.
	when func(*Model) bool

	run func(*Model) tea.Cmd
}

func (c command) available(m *Model) bool { return c.when == nil || c.when(m) }

// hasSelection is the usual precondition: most commands act on a request.
func hasSelection(m *Model) bool { return m.selected() != nil }

// commands is the single source of truth for what pogo can do.
func (m *Model) commands() []command {
	return []command{
		// --- the request under the cursor ---
		{
			id: "inspect", group: "Request", keys: "⏎",
			title: "Inspect request",
			desc:  "request, response, headers, bodies, timing",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doInspect() },
		},
		{
			id: "replay", group: "Request", keys: "r",
			title: "Replay request",
			desc:  "run it again, recorded as a new entry",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doReplay() },
		},
		{
			id: "edit", group: "Request", keys: "e",
			title: "Edit and run",
			desc:  "change method, URL, query, headers or body, then run it",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doEdit() },
		},
		{
			id: "copy", group: "Request", keys: "y",
			title: "Copy…",
			desc:  "curl command, URL, headers, request or response body",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doCopy() },
		},
		{
			id: "diff", group: "Request", keys: "d",
			title: "Compare with…",
			desc:  "mark this request, then press d on another to diff them",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doDiff() },
		},
		{
			id: "delete", group: "Request", keys: "x",
			title: "Delete request",
			desc:  "remove it and its payloads from history",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doDelete() },
		},

		// --- organizing ---
		{
			id: "star", group: "Organize", keys: "s",
			title: "Star / unstar",
			desc:  "keep the requests that matter; they survive the history cap",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doStar() },
		},
		{
			id: "collection", group: "Organize", keys: "c",
			title: "Add to collection…",
			desc:  "file this request under a name you can filter and group by",
			when:  hasSelection,
			run:   func(m *Model) tea.Cmd { return m.doCollection() },
		},
		{
			id: "group", group: "Organize", keys: "t",
			title: "Change grouping",
			desc:  "chronological, by host, by collection",
			run:   func(m *Model) tea.Cmd { return m.doGroup() },
		},

		// --- finding ---
		{
			id: "search", group: "Find", keys: "/",
			title: "Search…",
			desc:  "free text, or method: status: host: collection: is:",
			run:   func(m *Model) tea.Cmd { return m.doSearch() },
		},
		{
			id: "clear", group: "Find", keys: "esc",
			title: "Clear search",
			desc:  "show everything again",
			when:  func(m *Model) bool { return m.query.Raw != "" },
			run:   func(m *Model) tea.Cmd { return m.doClearSearch() },
		},
		{
			id: "starred", group: "Find", keys: "",
			title: "Show starred only",
			desc:  "the same as searching is:starred",
			run:   func(m *Model) tea.Cmd { return m.applyFilter("is:starred") },
		},
		{
			id: "failed", group: "Find", keys: "",
			title: "Show failures only",
			desc:  "anything that failed or came back 400 or worse",
			run:   func(m *Model) tea.Cmd { return m.applyFilter("is:failed") },
		},

		// --- view ---
		{
			id: "sidebar", group: "View", keys: "\\",
			title: "Toggle sidebar",
			desc:  "filters, collections and hosts down the side",
			run:   func(m *Model) tea.Cmd { return m.doToggleSidebar() },
		},
		{
			id: "reveal", group: "View", keys: "S",
			title: "Reveal / hide secrets",
			desc:  "show masked tokens, cookies and API keys in full",
			run:   func(m *Model) tea.Cmd { return m.doReveal() },
		},
		{
			id: "body", group: "View", keys: "v",
			title: "Change body view",
			desc:  "JSON tree, pretty-printed, or raw bytes",
			when:  func(m *Model) bool { return m.screen == screenDetail },
			run:   func(m *Model) tea.Cmd { return m.doBodyMode() },
		},

		// --- application ---
		{
			id: "env", group: "App", keys: "E",
			title: "Switch environment…",
			desc:  "resolve {{variables}} from a different environment",
			run:   func(m *Model) tea.Cmd { return m.doEnv() },
		},
		{
			id: "update", group: "App", keys: "u",
			title: "Install update",
			desc:  "download and verify the newer release",
			when:  func(m *Model) bool { return m.updateVersion != "" },
			run:   func(m *Model) tea.Cmd { return m.doUpdate() },
		},
		{
			id: "help", group: "App", keys: "?",
			title: "Keyboard reference",
			desc:  "every key, grouped by what it is for",
			run:   func(m *Model) tea.Cmd { return m.doHelp() },
		},
		{
			id: "quit", group: "App", keys: "q",
			title: "Quit",
			desc:  "",
			run:   func(m *Model) tea.Cmd { return tea.Quit },
		},

		// --- inspecting and editing: keys that only exist inside a screen, so
		// they belong in the reference rather than in a palette of verbs ---
		{id: "panes", group: "View", keys: "tab / 1–5", title: "Move between panes", motion: true},
		{id: "fold", group: "View", keys: "space", title: "Fold or unfold a JSON node", motion: true},

		{id: "edit-field", group: "Edit", keys: "⏎", title: "Edit the focused field", motion: true},
		{id: "edit-method", group: "Edit", keys: "← →", title: "Change the method", motion: true},
		{id: "edit-remove", group: "Edit", keys: "ctrl+d", title: "Remove a header or parameter", motion: true},
		{id: "edit-raw", group: "Edit", keys: "ctrl+t", title: "Switch between fields and raw curl", motion: true},
		{id: "edit-editor", group: "Edit", keys: "ctrl+e", title: "Hand the command to $EDITOR", motion: true},
		{id: "edit-run", group: "Edit", keys: "ctrl+r", title: "Run it as a new entry", motion: true},

		// --- motions: in the reference, not in the palette ---
		{id: "up", group: "Navigate", keys: "↑ / k", title: "Previous request", motion: true},
		{id: "down", group: "Navigate", keys: "↓ / j", title: "Next request", motion: true},
		{id: "top", group: "Navigate", keys: "g", title: "Jump to the newest", motion: true},
		{id: "bottom", group: "Navigate", keys: "G", title: "Jump to the oldest", motion: true},
		{id: "page", group: "Navigate", keys: "ctrl+u / ctrl+d", title: "Half page up / down", motion: true},
		{id: "focus", group: "Navigate", keys: "tab", title: "Move between sidebar and list", motion: true},
		{id: "back", group: "Navigate", keys: "esc", title: "Back, or close what is open", motion: true},
		{id: "palette", group: "Navigate", keys: "ctrl+k", title: "Command palette", motion: true},
	}
}

// paletteItems returns the commands the palette offers, most useful first.
func (m *Model) paletteItems() []command {
	var out []command
	for _, c := range m.commands() {
		if !c.motion {
			out = append(out, c)
		}
	}
	return out
}

// --- fuzzy matching -------------------------------------------------------

// fuzzyScore reports how well text matches query, and whether it matches at all.
//
// Subsequence matching with a bonus for runs and for word starts: typing "cop"
// should find "Copy…" before "Compare with…", and "rep" should find "Replay"
// even though "response" also contains those letters.
func fuzzyScore(text, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	lowerText, lowerQuery := strings.ToLower(text), strings.ToLower(query)

	score, ti, run := 0, 0, 0
	for qi := 0; qi < len(lowerQuery); qi++ {
		q := lowerQuery[qi]
		if q == ' ' {
			run = 0
			continue
		}
		found := -1
		for ; ti < len(lowerText); ti++ {
			if lowerText[ti] == q {
				found = ti
				break
			}
		}
		if found < 0 {
			return 0, false
		}

		score += 10
		if found == 0 || lowerText[found-1] == ' ' || lowerText[found-1] == '-' {
			score += 20 // start of a word
		}
		if run > 0 {
			score += 15 // contiguous with the previous match
		}
		run++
		ti = found + 1
	}
	// Shorter titles that match are usually the ones meant.
	score -= len(text) / 8
	return score, true
}

// filterCommands ranks the palette against a query.
func (m *Model) filterCommands(query string) []command {
	type scored struct {
		cmd   command
		score int
	}
	var hits []scored

	for _, c := range m.paletteItems() {
		best, ok := fuzzyScore(c.title, query)
		if !ok {
			// Fall back to the description, so "token" finds "Reveal secrets".
			if s, ok2 := fuzzyScore(c.desc, query); ok2 {
				best, ok = s/2, true
			}
		}
		if !ok {
			continue
		}
		if !c.available(m) {
			best -= 200 // still offered, but after everything usable
		}
		hits = append(hits, scored{c, best})
	}

	// Stable insertion sort: the registry order is meaningful, and ties should
	// keep it rather than shuffle between keystrokes.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].score > hits[j-1].score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}

	out := make([]command, len(hits))
	for i, h := range hits {
		out[i] = h.cmd
	}
	return out
}
