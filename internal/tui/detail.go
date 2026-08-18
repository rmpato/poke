package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/history"
	"github.com/rmpato/poke/internal/runner"
)

type detailTab int

const (
	tabOverview detailTab = iota
	tabRequest
	tabResponse
	tabTiming
	tabRaw
)

var tabNames = []string{"Overview", "Request", "Response", "Timing", "Raw"}

// bodyMode selects how a payload is displayed. Three modes cover the real
// needs: understand the shape, read the values, see the exact bytes.
type bodyMode int

const (
	bodyTree bodyMode = iota
	bodyPretty
	bodyRaw
)

var bodyModeNames = []string{"tree", "pretty", "raw"}

// treeState is the structural view of one payload. Request and response each
// get their own: sharing a single tree meant whichever body rendered first won
// and the other silently displayed the wrong data.
type treeState struct {
	tree   *jnode
	err    error
	lines  []treeLine
	cursor int
}

func (t *treeState) reset() { *t = treeState{} }

// detailModel holds the inspection state for one entry.
type detailModel struct {
	tab     detailTab
	vp      viewport.Model
	entryID string
	loaded  bool
	reqBody []byte
	resBody []byte
	bodyErr error

	mode bodyMode
	req  treeState
	res  treeState
}

// focused returns the tree the cursor acts on: the request payload on the
// request pane, the response everywhere else, since that is what people are
// usually reading.
func (d *detailModel) focused() *treeState {
	if d.tab == tabRequest {
		return &d.req
	}
	return &d.res
}

func (d *detailModel) resize(w, h int) {
	d.vp.Width = maxInt(10, w)
	d.vp.Height = maxInt(3, h)
}

// setBodies installs payloads fetched from the store and invalidates the tree
// built from the previous entry.
func (d *detailModel) setBodies(msg bodiesMsg) {
	if msg.id != d.entryID {
		return // a stale load for an entry the user has already navigated away from
	}
	d.reqBody, d.resBody, d.bodyErr = msg.request, msg.response, msg.err
	d.loaded = true
	d.req.reset()
	d.res.reset()
}

// loadDetail fetches the selected entry's payloads if they are not in hand.
func (m *Model) loadDetail() tea.Cmd {
	e := m.selected()
	if e == nil {
		return nil
	}
	if m.detail.entryID == e.ID && m.detail.loaded {
		return nil
	}
	m.detail.entryID = e.ID
	m.detail.loaded = false
	m.detail.req.reset()
	m.detail.res.reset()
	return loadBodies(m.st, e)
}

// renderDetail draws the inspection pane for an entry.
func (m *Model) renderDetail(e *history.Entry, width, height int, withTabs bool) string {
	if e == nil {
		return m.centerNotice(width, height, styFaint.Render("no request selected"))
	}

	view := m.cfg.Redact.Masked(e)
	if m.reveal {
		view = e
	}

	var head string
	if withTabs {
		head = m.renderTabs(width) + "\n"
	}

	body := ""
	switch m.detail.tab {
	case tabOverview:
		body = m.renderOverview(e, view, width)
	case tabRequest:
		body = m.renderRequest(e, view, width)
	case tabResponse:
		body = m.renderResponse(e, view, width)
	case tabTiming:
		body = m.renderTiming(e, width)
	case tabRaw:
		body = m.renderRaw(e, view, width)
	}

	m.detail.vp.Width = width
	m.detail.vp.Height = maxInt(1, height-lipgloss.Height(head))
	m.detail.vp.SetContent(clampBlock(body, width))
	return head + m.detail.vp.View()
}

func (m *Model) renderTabs(width int) string {
	parts := make([]string, 0, len(tabNames))
	for i, name := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if detailTab(i) == m.detail.tab {
			parts = append(parts, stySelected.Render(label))
		} else {
			parts = append(parts, styFaint.Render(label))
		}
	}
	line := strings.Join(parts, styFaint.Render("  ·  "))
	if pct := m.detail.vp.ScrollPercent(); m.detail.vp.TotalLineCount() > m.detail.vp.Height {
		meta := styFaint.Render(fmt.Sprintf("%3.0f%%", pct*100))
		if gap := width - lipgloss.Width(line) - lipgloss.Width(meta); gap > 1 {
			line += strings.Repeat(" ", gap) + meta
		}
	}
	return line
}

// --- overview --------------------------------------------------------------

func (m *Model) renderOverview(e, view *history.Entry, width int) string {
	var b strings.Builder

	b.WriteString(m.requestLine(view, width))
	b.WriteString("\n\n")
	b.WriteString(m.statusLine(e))
	b.WriteString("\n")

	if e.Error != "" {
		b.WriteString("\n" + styErr.Render(truncate(e.Error, width)) + "\n")
	}
	if e.Request.Incomplete {
		b.WriteString("\n" + styMuted.Render("⚠ poke could not fully parse this command; the summary below may be incomplete.") + "\n")
	}
	if e.Redacted {
		b.WriteString("\n" + styMuted.Render("⚠ secrets were stripped before this was stored; replaying it will not authenticate.") + "\n")
	}

	b.WriteString("\n" + section("REQUEST HEADERS"))
	b.WriteString(renderHeaders(view.Request.Headers, width))

	if view.Request.Body != nil {
		b.WriteString("\n" + section("REQUEST BODY"))
		b.WriteString(m.renderBodyBlock(m.detail.reqBody, view.Request.Body, "", width, &m.detail.req, false))
	}

	if fb := view.FinalBlock(); fb != nil {
		b.WriteString("\n" + section("RESPONSE HEADERS"))
		b.WriteString(renderHeaders(fb.Headers, width))
	}
	if view.Response != nil && view.Response.Body != nil {
		b.WriteString("\n" + section("RESPONSE BODY"))
		b.WriteString(m.renderBodyBlock(m.detail.resBody, view.Response.Body, view.Response.ContentType, width, &m.detail.res, true))
	}

	return b.String()
}

// requestLine is the headline: what was sent, where.
func (m *Model) requestLine(view *history.Entry, width int) string {
	method := methodStyle(view.Request.Method).Bold(true).Render(view.Request.Method)
	url := view.Request.URL
	if url == "" {
		url = styFaint.Render("(no URL detected)")
	}
	return method + " " + styText.Render(wrap(url, maxInt(20, width-len(view.Request.Method)-1), len(view.Request.Method)+1))
}

// statusLine summarizes the outcome in the one line people look for first.
func (m *Model) statusLine(e *history.Entry) string {
	var parts []string

	if fb := e.FinalBlock(); fb != nil && fb.Status > 0 {
		text := fmt.Sprintf("%d", fb.Status)
		if fb.Reason != "" {
			text += " " + fb.Reason
		}
		parts = append(parts, statusStyle(fb.Status).Bold(true).Render(text))
	} else if e.Exit != 0 {
		parts = append(parts, styErr.Bold(true).Render(runner.ExitMessage(e.Exit)))
	} else {
		parts = append(parts, styFaint.Render("no response captured"))
	}

	parts = append(parts, styMuted.Render(e.Duration.String()))
	if e.Response != nil && e.Response.Body != nil {
		parts = append(parts, styMuted.Render(bytesHuman(e.Response.Body.Size)))
	}
	if n := e.Redirects(); n > 0 {
		parts = append(parts, styMuted.Render(pluralize(n, "redirect")))
	}
	if proto := protoOf(e); proto != "" {
		parts = append(parts, styFaint.Render(proto))
	}
	parts = append(parts, styFaint.Render(e.CreatedAt.Local().Format("Mon 15:04:05")))
	if e.Source != history.SourcePoke {
		parts = append(parts, styBadge.Render(string(e.Source)))
	}
	return strings.Join(parts, styFaint.Render(" · "))
}

// --- request ---------------------------------------------------------------

func (m *Model) renderRequest(e, view *history.Entry, width int) string {
	var b strings.Builder

	b.WriteString(m.requestLine(view, width) + "\n\n")

	if q := queryOf(view.Request.URL); q != "" {
		b.WriteString(section("QUERY"))
		for _, pair := range strings.Split(q, "&") {
			k, v, _ := strings.Cut(pair, "=")
			b.WriteString("  " + styJSONKey.Render(pad(k, 22)) + " " + styText.Render(v) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(section("HEADERS"))
	b.WriteString(renderHeaders(view.Request.Headers, width))
	b.WriteString("\n")

	if len(view.Request.Options) > 0 {
		b.WriteString(section("CURL OPTIONS"))
		for _, o := range view.Request.Options {
			line := "  " + styKey.Render(pad(o.Name, 20))
			if o.HasValue {
				line += " " + styText.Render(truncate(o.Value, maxInt(10, width-24)))
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(section("BODY"))
	if view.Request.Body == nil {
		b.WriteString(styFaint.Render("  —") + "\n")
	} else {
		b.WriteString(m.renderBodyBlock(m.detail.reqBody, view.Request.Body, "", width, &m.detail.req, true))
	}

	if len(e.Request.Unparsed) > 0 {
		b.WriteString("\n" + section("NOT UNDERSTOOD BY POKE"))
		for _, u := range e.Request.Unparsed {
			b.WriteString("  " + styMuted.Render(u) + "\n")
		}
	}
	return b.String()
}

// --- response --------------------------------------------------------------

func (m *Model) renderResponse(e, view *history.Entry, width int) string {
	var b strings.Builder

	if view.Response == nil || len(view.Response.Blocks) == 0 {
		b.WriteString(m.statusLine(e) + "\n\n")
		if e.Error != "" {
			b.WriteString(styErr.Render(e.Error) + "\n")
		}
		b.WriteString(styFaint.Render("No response was captured.") + "\n")
		if e.Request.Flags.Head {
			b.WriteString(styFaint.Render("This was a HEAD request, so there is no body by design.") + "\n")
		}
		return b.String()
	}

	// Show the redirect chain when there was one: the hops are usually the
	// answer to "why did this end up there".
	if len(view.Response.Blocks) > 1 {
		b.WriteString(section("CHAIN"))
		for i, blk := range view.Response.Blocks {
			line := "  " + statusStyle(blk.Status).Render(fmt.Sprintf("%d", blk.Status)) + " " + styMuted.Render(blk.Reason)
			if loc, ok := blk.Header("Location"); ok {
				line += styFaint.Render("  → ") + styText.Render(truncateMiddle(loc, maxInt(20, width-24)))
			}
			if i == len(view.Response.Blocks)-1 {
				line += styFaint.Render("  (final)")
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(m.statusLine(e) + "\n\n")
	b.WriteString(section("HEADERS"))
	if fb := view.FinalBlock(); fb != nil {
		b.WriteString(renderHeaders(fb.Headers, width))
	}
	b.WriteString("\n")

	ct := ""
	if view.Response != nil {
		ct = view.Response.ContentType
	}
	b.WriteString(section("BODY " + styFaint.Render("("+bodyModeNames[m.detail.mode]+" · v to switch)")))
	if view.Response.Body == nil {
		b.WriteString(styFaint.Render("  —") + "\n")
	} else {
		b.WriteString(m.renderBodyBlock(m.detail.resBody, view.Response.Body, ct, width, &m.detail.res, true))
	}
	return b.String()
}

// --- timing ----------------------------------------------------------------

func (m *Model) renderTiming(e *history.Entry, width int) string {
	var b strings.Builder
	b.WriteString(section("TIMING"))

	if e.Metrics == nil || !e.Metrics.HasTiming() {
		b.WriteString(styFaint.Render("  curl did not report a timing breakdown for this request.") + "\n\n")
		b.WriteString(styMuted.Render("  Timings come from curl's --write-out, which can only be") + "\n")
		b.WriteString(styMuted.Render("  captured to a file by curl 8.3 and later. poke records") + "\n")
		b.WriteString(styMuted.Render("  wall-clock duration regardless:") + "\n\n")
		b.WriteString("  " + styKey.Render(pad("Total", 16)) + styText.Render(e.Duration.String()) + "\n")
		return b.String()
	}

	mt := e.Metrics
	phases := mt.Phases()
	total := mt.Total()

	// A proportional bar makes the dominant phase obvious at a glance, which is
	// the entire reason to look at this screen.
	barWidth := clampInt(width-34, 10, 40)
	for _, p := range phases {
		frac := 0.0
		if total > 0 {
			frac = float64(p.Duration) / float64(total)
		}
		filled := int(frac*float64(barWidth) + 0.5)
		bar := styAccentBar.Render(repeat("█", filled)) + styFaint.Render(repeat("·", barWidth-filled))
		fmt.Fprintf(&b, "  %s %s %s\n",
			styKey.Render(pad(p.Name, 14)),
			styMuted.Render(padLeft(history.Duration(p.Duration).String(), 8)),
			bar)
	}

	b.WriteString("  " + styFaint.Render(repeat("─", 14+1+8+1+barWidth)) + "\n")
	fmt.Fprintf(&b, "  %s %s\n",
		stySelected.Render(pad("Total", 14)),
		stySelected.Render(padLeft(history.Duration(total).String(), 8)))

	b.WriteString("\n" + section("TRANSFER"))
	// Only report figures curl actually measured. A row of zeroes and dashes
	// reads as data when it is really an absence of data.
	rows := make([][2]string, 0, 7)
	add := func(label, value string) { rows = append(rows, [2]string{label, value}) }

	if mt.SizeDownload > 0 {
		add("Downloaded", bytesHuman(int64(mt.SizeDownload)))
	}
	if mt.SizeUpload > 0 {
		add("Uploaded", bytesHuman(int64(mt.SizeUpload)))
	}
	if mt.SpeedDownload > 0 {
		add("Speed", bytesHuman(int64(mt.SpeedDownload))+"/s")
	}
	if p := protoOf(e); p != "" {
		add("Protocol", p)
	}
	if addr := remoteAddr(mt); addr != "" {
		add("Remote", addr)
	}
	if mt.NumConnects > 0 {
		add("Connections", fmt.Sprintf("%d", mt.NumConnects))
	}
	if mt.NumRedirects > 0 {
		add("Redirects", fmt.Sprintf("%d", mt.NumRedirects))
	}

	if len(rows) == 0 {
		b.WriteString(styFaint.Render("  nothing was transferred") + "\n")
	}
	for _, r := range rows {
		b.WriteString("  " + styKey.Render(pad(r[0], 16)) + styText.Render(r[1]) + "\n")
	}

	if e.Exit != 0 {
		b.WriteString("\n" + styErr.Render("  "+runner.ExitMessage(e.Exit)) + "\n")
	}

	b.WriteString("\n" + styFaint.Render("  Wall-clock as measured by poke, including curl startup: "+e.Duration.String()) + "\n")
	return b.String()
}

// protoOf prefers the protocol curl wrote in the response status line, which is
// exact, over the version field in the metrics, which reports "1" for both
// HTTP/1.0 and HTTP/1.1.
func protoOf(e *history.Entry) string {
	if b := e.FinalBlock(); b != nil && b.Proto != "" {
		return b.Proto
	}
	// curl reports version "0" when no HTTP exchange took place.
	if e.Metrics != nil && e.Metrics.HTTPVersion != "" && e.Metrics.HTTPVersion != "0" {
		return "HTTP/" + e.Metrics.HTTPVersion
	}
	return ""
}

func remoteAddr(m *history.Metrics) string {
	if m.RemoteIP == "" {
		return ""
	}
	if m.RemotePort > 0 {
		return fmt.Sprintf("%s:%d", m.RemoteIP, m.RemotePort)
	}
	return m.RemoteIP
}

// --- raw -------------------------------------------------------------------

func (m *Model) renderRaw(e, view *history.Entry, width int) string {
	var b strings.Builder

	b.WriteString(section("COMMAND"))
	b.WriteString(indentBlock(highlightCommand(view.Command.Multiline()), "  ") + "\n\n")

	if e.Command.Dir != "" {
		b.WriteString(section("WORKING DIRECTORY"))
		b.WriteString("  " + styMuted.Render(e.Command.Dir) + "\n\n")
	}

	b.WriteString(section("ENTRY"))
	meta := [][2]string{
		{"id", e.ID},
		{"captured", e.CreatedAt.Local().Format("2006-01-02 15:04:05 MST")},
		{"source", string(e.Source)},
		{"exit", fmt.Sprintf("%d %s", e.Exit, runner.ExitMessage(e.Exit))},
		{"duration", e.Duration.String()},
	}
	if e.ParentID != "" {
		meta = append(meta, [2]string{"replay of", e.ParentID})
	}
	if e.Note != "" {
		meta = append(meta, [2]string{"note", e.Note})
	}
	for _, kv := range meta {
		b.WriteString("  " + styKey.Render(pad(kv[0], 12)) + styText.Render(kv[1]) + "\n")
	}

	if len(m.detail.resBody) > 0 {
		b.WriteString("\n" + section("RESPONSE BODY (raw)"))
		b.WriteString(indentBlock(rawText(m.detail.resBody, width), "  "))
	}
	return b.String()
}

// --- shared pieces ---------------------------------------------------------

func section(title string) string {
	return styHeading.Render(title) + "\n"
}

func renderHeaders(headers []curlargs.Header, width int) string {
	if len(headers) == 0 {
		return styFaint.Render("  —") + "\n"
	}
	nameWidth := 0
	for _, h := range headers {
		if n := len(h.Name); n > nameWidth {
			nameWidth = n
		}
	}
	nameWidth = clampInt(nameWidth, 4, 28)

	var b strings.Builder
	for _, h := range headers {
		value := h.Value
		if value == "" {
			value = styFaint.Render("(empty)")
		}
		b.WriteString("  " + styJSONKey.Render(pad(h.Name, nameWidth)) + "  " +
			styText.Render(wrap(value, maxInt(10, width-nameWidth-4), nameWidth+4)) + "\n")
	}
	return b.String()
}

// renderBodyBlock renders a payload according to the current body mode,
// refusing structural views for payloads big enough to stall the UI.
func (m *Model) renderBodyBlock(data []byte, ref *history.BodyRef, contentType string, width int, ts *treeState, interactive bool) string {
	if ref == nil {
		return styFaint.Render("  —") + "\n"
	}
	if !m.detail.loaded {
		return styFaint.Render("  " + m.spinner.View() + " loading…\n")
	}
	if m.detail.bodyErr != nil {
		return styErr.Render("  could not read payload: "+m.detail.bodyErr.Error()) + "\n"
	}
	if len(data) == 0 {
		if ref.Size > 0 {
			return styFaint.Render(fmt.Sprintf("  payload not stored (%s)", bytesHuman(ref.Size))) + "\n"
		}
		return styFaint.Render("  (empty)") + "\n"
	}

	var b strings.Builder
	if ref.Truncated {
		b.WriteString(styMuted.Render(fmt.Sprintf("  showing first %s of %s",
			bytesHuman(ref.Stored), bytesHuman(ref.Size))) + "\n")
	}
	if ref.Binary {
		b.WriteString(styFaint.Render(fmt.Sprintf("  binary payload, %s — not rendered", bytesHuman(ref.Size))) + "\n")
		return b.String()
	}

	if looksJSON(contentType, data) && m.detail.mode != bodyRaw {
		if m.detail.mode == bodyTree {
			b.WriteString(m.renderTree(data, width, ts, interactive))
			return b.String()
		}
		pretty, ok := prettyJSON(data)
		if ok {
			b.WriteString(indentBlock(highlightJSON(string(pretty)), "  "))
			return b.String()
		}
	}

	b.WriteString(indentBlock(rawText(data, width), "  "))
	return b.String()
}

// renderTree lazily builds the structural view for one payload and renders the
// nodes that are currently unfolded.
func (m *Model) renderTree(data []byte, width int, ts *treeState, interactive bool) string {
	if ts.tree == nil && ts.err == nil {
		ts.tree, ts.err = parseJSONTree(data)
	}
	if ts.err != nil {
		return styMuted.Render("  "+ts.err.Error()+"; showing raw text\n") +
			indentBlock(rawText(data, width), "  ")
	}

	ts.lines = flattenTree(ts.tree, width-2)
	if ts.cursor >= len(ts.lines) {
		ts.cursor = maxInt(0, len(ts.lines)-1)
	}

	var b strings.Builder
	for i, line := range ts.lines {
		prefix := "  "
		if interactive && i == ts.cursor {
			prefix = styCursor.Render("▌ ")
		}
		b.WriteString(prefix + line.text + "\n")
	}
	return b.String()
}

// rawText prepares arbitrary bytes for display: control characters that would
// scramble the terminal are neutralized, and very long lines are wrapped.
func rawText(data []byte, width int) string {
	const maxRender = 512 << 10
	truncatedNote := ""
	if len(data) > maxRender {
		data = data[:maxRender]
		truncatedNote = "\n" + styMuted.Render(fmt.Sprintf("… truncated for display at %s", bytesHuman(maxRender)))
	}

	s := sanitizeControl(string(data))
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = wrap(line, maxInt(20, width-2), 0)
	}
	return strings.Join(lines, "\n") + truncatedNote
}

// sanitizeControl replaces escape and control bytes so a hostile or binary
// response cannot repaint the screen or move the cursor.
func sanitizeControl(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == '\r':
			// dropped: a lone CR would rewind the line
		case r < 0x20 || r == 0x7f:
			b.WriteRune('·')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// wrap hard-wraps text at width, indenting continuation lines so wrapped values
// stay visually attached to their label.
func wrap(s string, width, indent int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	pad := strings.Repeat(" ", indent)
	var out []string
	runes := []rune(s)
	for len(runes) > width {
		out = append(out, string(runes[:width]))
		runes = runes[width:]
		width = maxInt(10, width-0)
	}
	out = append(out, string(runes))
	return strings.Join(out, "\n"+pad)
}

func indentBlock(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// queryOf returns the query string of a URL, without the leading "?".
func queryOf(u string) string {
	_, q, ok := strings.Cut(u, "?")
	if !ok {
		return ""
	}
	q, _, _ = strings.Cut(q, "#")
	return q
}

// highlightCommand colors a rendered curl command so options and values are
// distinguishable at a glance.
func highlightCommand(cmd string) string {
	var out []string
	for _, line := range strings.Split(cmd, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		indent := line[:len(line)-len(trimmed)]
		switch {
		case strings.HasPrefix(trimmed, "curl"):
			out = append(out, indent+styKey.Render("curl")+trimmed[4:])
		case strings.HasPrefix(trimmed, "-"):
			opt, rest, _ := strings.Cut(trimmed, " ")
			out = append(out, indent+styJSONKey.Render(opt)+" "+styText.Render(rest))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
