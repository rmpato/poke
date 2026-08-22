package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/pogo/internal/environment"
	"github.com/rmpato/pogo/internal/history"
	"github.com/rmpato/pogo/internal/ui"
)

// The preview is the panel down the right of the list: enough of a request to
// decide about it without leaving the list.
//
// It is deliberately not the inspector in miniature. The inspector answers
// "what exactly happened here", and it has five panes and a scrollbar to do
// it. The preview answers the question you actually have while moving the
// cursor — is this the one? — which is a different question, and mostly needs
// three things: what you sent, what came back, and anything about it that
// would change what you do next.
//
// Sections are rendered in priority order and stop when the height runs out,
// so a short terminal loses the least useful part rather than half of every
// part.

// renderPreview draws the panel for one entry.
func (m *Model) renderPreview(e *history.Entry, width, height int) string {
	if width < 24 || height < 6 {
		return ui.ClampBlock("", maxInt(0, width), maxInt(0, height))
	}
	if e == nil {
		return ui.Panel("preview", ui.Border,
			ui.EmptyState("·", "Nothing selected", "", width-4, height-2),
			width, height)
	}

	view := m.cfg.Redact.Masked(e)
	if m.reveal {
		view = e
	}

	inner := width - 4
	body := m.previewBody(e, view, inner, height-2)
	return ui.Panel(e.Request.Method, ui.MethodColor(e.Request.Method), body, width, height)
}

// previewBody fills the panel, section by section, until the height is gone.
func (m *Model) previewBody(e, view *history.Entry, width, height int) string {
	lines := m.previewHead(e, view, width)

	// Every section below is optional, and each is asked for the space that is
	// actually left rather than for what it would like.
	add := func(title string, block []string) {
		left := height - len(lines) - 2 // the rule, and a blank line before it
		if left < 2 || len(block) == 0 {
			return
		}
		if len(block) > left {
			block = block[:left]
		}
		lines = append(lines, "", ui.Rule(title, width))
		lines = append(lines, block...)
	}

	add("insights", m.previewInsights(e, view, width))
	add("request", m.previewRequest(e, view, width))
	add("response", m.previewResponse(e, view, width, height-len(lines)))

	return strings.Join(lines, "\n")
}

// previewHead is the identity of the request: how it went, where it went, and
// which API and environment that was.
func (m *Model) previewHead(e, view *history.Entry, width int) []string {
	kind := ui.StatusKind(e.Status())
	status := statusText(e.Status(), e.Exit)
	if b := e.FinalBlock(); b != nil && b.Reason != "" {
		status += " " + b.Reason
	}

	facts := []string{e.Duration.String()}
	if e.Response != nil && e.Response.Body != nil {
		facts = append(facts, bytesHuman(e.Response.Body.Size))
	}
	if n := e.Redirects(); n > 0 {
		facts = append(facts, pluralize(n, "hop"))
	}

	head := ui.StatusBar(
		lipgloss.NewStyle().Foreground(kind.Color()).Bold(true).Render(kind.Glyph()+" "+status),
		ui.SubtitleStyle.Render(strings.Join(facts, " · ")),
		width)

	out := []string{
		head,
		ui.SubtitleStyle.Render(ui.FitLine(m.displayHost(view), width)),
		styText.Render(ui.FitLine(truncateMiddle(m.displayPath(view), width), width)),
	}

	// Which API, and which environment of it — the two facts the list groups
	// by, repeated here because the group heading may have scrolled away.
	ref := m.apiRef(e)
	if ref.Domain != "" {
		tags := ui.Tag(ui.Fallback(ref.Name, ref.Domain), ui.Primary)
		if ref.Env != "" {
			tags += " " + ui.Tag(ref.Env, envColor(ref.Env))
		}
		if e.Collection != "" {
			tags += " " + ui.Tag(e.Collection, ui.Alt)
		}
		out = append(out, ui.FitLine(tags, width))
	}
	return out
}

// previewRequest is what was sent: the headers that carry meaning, and the
// first of the body.
func (m *Model) previewRequest(e, view *history.Entry, width int) []string {
	var pairs [][2]string
	for _, h := range view.Request.Headers {
		if len(pairs) >= 3 {
			break
		}
		if notableHeader(h.Name) {
			pairs = append(pairs, [2]string{h.Name, h.Value})
		}
	}

	var out []string
	if len(pairs) > 0 {
		out = append(out, strings.Split(ui.DefinitionList(pairs, width, 15), "\n")...)
	}
	if body := m.previewBodyText(e, m.detail.reqBody, view.Request.Body, width, 3); len(body) > 0 {
		out = append(out, body...)
	}
	if len(out) == 0 {
		out = append(out, ui.SubtitleStyle.Render(ui.FitLine("no headers or body", width)))
	}
	return out
}

// previewResponse is what came back. It gets whatever height is left, because
// it is usually the reason you are looking.
func (m *Model) previewResponse(e, view *history.Entry, width, left int) []string {
	if view.Response == nil {
		if e.Error != "" {
			return wrapLines(styErr.Render(e.Error), width, 3)
		}
		return []string{ui.SubtitleStyle.Render(ui.FitLine("no response captured", width))}
	}

	var out []string
	if b := view.FinalBlock(); b != nil {
		if ct, ok := b.Header("Content-Type"); ok {
			out = append(out, strings.Split(
				ui.DefinitionList([][2]string{{"Content-Type", ct}}, width, 15), "\n")...)
		}
	}
	if body := m.previewBodyText(e, m.detail.resBody, view.Response.Body, width, maxInt(2, left-len(out)-3)); len(body) > 0 {
		out = append(out, body...)
	}
	if len(out) == 0 {
		out = append(out, ui.SubtitleStyle.Render(ui.FitLine("empty", width)))
	}
	return out
}

// previewBodyText renders the first lines of a payload, pretty-printed when it
// is JSON, and says how much was left out.
func (m *Model) previewBodyText(e *history.Entry, loaded []byte, ref *history.BodyRef, width, max int) []string {
	if ref == nil || max < 1 {
		return nil
	}

	// Bodies arrive asynchronously; say so rather than showing an empty box
	// that looks like an empty body.
	body := loaded
	if m.detail.entryID != e.ID || !m.detail.loaded {
		body = nil
		if ref.Inline == "" {
			return []string{ui.SubtitleStyle.Render(ui.FitLine("…", width))}
		}
	}
	if len(body) == 0 {
		body = []byte(ref.Inline)
	}
	if len(body) == 0 {
		return []string{ui.SubtitleStyle.Render(ui.FitLine(bytesHuman(ref.Size)+" not shown", width))}
	}
	if ref.Binary {
		return []string{ui.SubtitleStyle.Render(ui.FitLine(bytesHuman(ref.Size)+" of binary", width))}
	}

	text := strings.TrimRight(string(body), "\n")
	if looksJSON("", body) {
		if pretty, ok := prettyJSON(body); ok {
			text = strings.TrimRight(string(pretty), "\n")
		}
	}

	all := strings.Split(text, "\n")
	shown := all
	if len(shown) > max {
		shown = shown[:max]
	}

	out := make([]string, 0, len(shown)+1)
	for _, line := range shown {
		out = append(out, styMuted.Render(ui.FitLine("  "+expandTabs(line), width)))
	}
	if rest := len(all) - len(shown); rest > 0 {
		out = append(out, ui.SubtitleStyle.Render(ui.FitLine("  … "+pluralize(rest, "more line"), width)))
	}
	return out
}

// --- insights --------------------------------------------------------------

// previewInsights is the part that is not just a smaller inspector.
//
// Each line has to earn its place by changing what you would do next: why this
// failed, whether it always fails, where the time went, what it is carrying.
// They are ordered by that, and cut from the bottom when space runs out.
func (m *Model) previewInsights(e, view *history.Entry, width int) []string {
	var out []string
	line := func(glyph string, style lipgloss.Style, text string) {
		if len(out) >= 5 || text == "" {
			return
		}
		out = append(out, ui.FitLine(style.Render(glyph)+" "+styText.Render(text), width))
	}

	// Why it failed, in the server's own words where it gave any.
	if !e.OK() {
		if reason := failureReason(e, m.responseBody(e)); reason != "" {
			line("✗", styErr, reason)
		}
	}

	// Whether this endpoint is reliable, and how it usually performs. Two
	// calls is not a trend, so the sparkline only appears once there are more.
	if siblings := m.siblings(e); len(siblings) > 1 {
		durations := make([]int, 0, len(siblings))
		failed := 0
		for i := len(siblings) - 1; i >= 0; i-- { // oldest first, so it reads left to right
			durations = append(durations, int(time.Duration(siblings[i].Duration).Milliseconds()))
			if !siblings[i].OK() {
				failed++
			}
		}
		spark := ui.SparklineBaseline(durations, minInt(12, width/3))
		summary := pluralize(len(siblings), "call")
		if failed > 0 {
			summary += fmt.Sprintf(", %d failed", failed)
		}
		out = append(out, ui.FitLine(spark+" "+styText.Render(summary), width))

		if prev := m.previousSibling(e); prev != nil {
			line("↔", styBadge, "d compares with the call "+age(prev.CreatedAt, m.now)+" ago")
		}
	}

	// Where the time actually went, when curl told us. A phase that is
	// essentially the whole request is worth saying differently: "wait took
	// 30ms of 30ms" reads like an arithmetic error rather than a finding.
	if e.Metrics != nil && e.Metrics.HasTiming() {
		if name, d, total := slowestPhase(e); total > 0 && d > 0 {
			if float64(d)/float64(total) > 0.9 {
				line("◷", styMuted, fmt.Sprintf("almost all %s — %s", name, history.Duration(d)))
			} else {
				line("◷", styMuted, fmt.Sprintf("%s took %s of %s",
					name, history.Duration(d), history.Duration(total)))
			}
		}
	}

	if n := e.Redirects(); n > 0 {
		line("→", styMuted, pluralize(n, "redirect")+" before this one")
	}

	// What it is carrying, and what it depends on.
	if refs := environment.References(e.Command.Args); len(refs) > 0 {
		text := "{{" + strings.Join(refs, "}} {{") + "}}"
		if e.Env != "" {
			text += " · " + e.Env
		}
		line("◇", styBadge, text)
	} else if name, ok := authHeader(e); ok {
		line("⚿", styMuted, name+" — stored, masked here")
	}

	if e.Source != history.SourceRun {
		line("↺", styMuted, string(e.Source)+" of an earlier request")
	}
	if b := view.Response; b != nil && b.Body != nil && b.Body.Truncated {
		line("!", styErr, "stored copy truncated at "+bytesHuman(b.Body.Stored))
	}
	if e.Note != "" {
		line("✎", styMuted, e.Note)
	}

	return out
}

// responseBody returns the loaded response payload for an entry, if it is the
// one currently in hand.
func (m *Model) responseBody(e *history.Entry) []byte {
	if m.detail.entryID != e.ID || !m.detail.loaded {
		return nil
	}
	return m.detail.resBody
}

// failureReason explains a failure in one line: curl's own diagnosis when the
// request never completed, and the API's when it did.
func failureReason(e *history.Entry, body []byte) string {
	if e.Exit != 0 {
		if e.Error != "" {
			return strings.TrimSpace(firstLine(e.Error))
		}
		return fmt.Sprintf("curl exited %d", e.Exit)
	}
	if msg := jsonMessage(body); msg != "" {
		return msg
	}
	if b := e.FinalBlock(); b != nil && b.Reason != "" {
		return b.Reason
	}
	return ""
}

// jsonMessage digs the human-readable part out of an error payload. Every API
// spells it differently and all of them mean the same thing.
func jsonMessage(body []byte) string {
	if len(body) == 0 || !looksJSON("", body) {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return ""
	}
	for _, key := range []string{"message", "error_description", "detail", "error", "reason", "title"} {
		switch value := doc[key].(type) {
		case string:
			if value != "" {
				return value
			}
		case map[string]any:
			if nested, ok := value["message"].(string); ok && nested != "" {
				return nested
			}
		}
	}
	return ""
}

// slowestPhase reports which part of the exchange took the longest.
func slowestPhase(e *history.Entry) (string, time.Duration, time.Duration) {
	if e.Metrics == nil {
		return "", 0, 0
	}
	var (
		name  string
		worst time.Duration
	)
	for _, p := range e.Metrics.Phases() {
		if p.Duration > worst {
			// "wait (ttfb)" is precise and too long for this panel; the
			// timing pane spells it out in full.
			label, _, _ := strings.Cut(strings.ToLower(p.Name), " (")
			name, worst = label, p.Duration
		}
	}
	return name, worst, e.Metrics.Total()
}

// siblings are the other calls to this same endpoint: same method, same API,
// same path. It is what turns one row into a history.
func (m *Model) siblings(e *history.Entry) []*history.Entry {
	if e == nil {
		return nil
	}
	domain, path := m.domainOf(e), history.PathOf(e.Request.URL)

	out := make([]*history.Entry, 0, 8)
	for _, other := range m.entries {
		if other.Request.Method != e.Request.Method ||
			history.PathOf(other.Request.URL) != path ||
			m.domainOf(other) != domain {
			continue
		}
		out = append(out, other)
		if len(out) == 12 {
			break
		}
	}
	return out
}

// previousSibling is the call to this endpoint before this one — the thing d
// would compare against.
func (m *Model) previousSibling(e *history.Entry) *history.Entry {
	seen := false
	for _, other := range m.siblings(e) {
		if seen {
			return other
		}
		if other.ID == e.ID {
			seen = true
		}
	}
	return nil
}

// notableHeader reports whether a request header is worth a line in a preview.
// Most requests carry a dozen; three of them ever matter at a glance.
func notableHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "content-type", "accept", "x-api-key", "cookie",
		"idempotency-key", "x-request-id":
		return true
	}
	return false
}

// authHeader names the credential a request carries, if any.
func authHeader(e *history.Entry) (string, bool) {
	for _, h := range e.Request.Headers {
		switch strings.ToLower(h.Name) {
		case "authorization", "cookie", "x-api-key":
			return h.Name, true
		}
	}
	return "", false
}

// envColor keeps an environment the same color it is everywhere else.
func envColor(env string) lipgloss.TerminalColor { return envStyle(env).GetForeground() }

// wrapLines breaks text to a width, capped at a number of lines.
func wrapLines(text string, width, max int) []string {
	wrapped := strings.Split(lipgloss.NewStyle().Width(width).Render(text), "\n")
	if len(wrapped) > max {
		wrapped = wrapped[:max]
	}
	for i, line := range wrapped {
		wrapped[i] = ui.FitLine(line, width)
	}
	return wrapped
}

// expandTabs keeps a tab from rendering as a variable-width hole, which would
// break the exact-rectangle contract the moment a payload contained one.
func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "  ") }
