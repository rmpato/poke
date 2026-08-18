package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Rendering limits. A response body can be tens of megabytes, and a TUI that
// stalls while formatting one is worse than a TUI that admits it will not.
const (
	maxHighlightBytes = 512 << 10 // beyond this, show raw text
	maxTreeBytes      = 2 << 20   // beyond this, no structural view
	maxTreeNodes      = 200_000
)

var (
	styJSONKey    = lipgloss.NewStyle().Foreground(colBlue)
	styJSONString = lipgloss.NewStyle().Foreground(colGreen)
	styJSONNumber = lipgloss.NewStyle().Foreground(colYellow)
	styJSONBool   = lipgloss.NewStyle().Foreground(colPurple)
	styJSONNull   = lipgloss.NewStyle().Foreground(colMuted)
	styJSONPunct  = lipgloss.NewStyle().Foreground(colFaint)
)

// looksJSON decides whether to offer structural views for a payload. The
// content type is a hint, not proof: plenty of APIs serve JSON as text/plain,
// and plenty of endpoints labelled JSON return an HTML error page.
func looksJSON(contentType string, body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	if c := trimmed[0]; c != '{' && c != '[' {
		return false
	}
	return json.Valid(trimmed)
}

// prettyJSON re-indents a payload while preserving key order, which json.Indent
// does because it works on the text rather than decoding into a map.
func prettyJSON(body []byte) ([]byte, bool) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, bytes.TrimSpace(body), "", "  "); err != nil {
		return body, false
	}
	return buf.Bytes(), true
}

// highlightJSON colours JSON text. It scans rather than parses so that it can
// colour text json.Indent has already laid out, and so malformed input still
// renders instead of vanishing.
func highlightJSON(src string) string {
	if len(src) > maxHighlightBytes {
		return src
	}
	var b strings.Builder
	b.Grow(len(src) * 2)

	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '"':
			end := scanString(src, i)
			text := src[i:end]
			// A string followed by a colon is a key, and keys are the thing
			// people scan for.
			if isKey(src, end) {
				b.WriteString(styJSONKey.Render(text))
			} else {
				b.WriteString(styJSONString.Render(text))
			}
			i = end

		case c == '-' || (c >= '0' && c <= '9'):
			end := i
			for end < len(src) && strings.IndexByte("-+.eE0123456789", src[end]) >= 0 {
				end++
			}
			b.WriteString(styJSONNumber.Render(src[i:end]))
			i = end

		case strings.HasPrefix(src[i:], "true"):
			b.WriteString(styJSONBool.Render("true"))
			i += 4
		case strings.HasPrefix(src[i:], "false"):
			b.WriteString(styJSONBool.Render("false"))
			i += 5
		case strings.HasPrefix(src[i:], "null"):
			b.WriteString(styJSONNull.Render("null"))
			i += 4

		case strings.IndexByte("{}[],:", c) >= 0:
			b.WriteString(styJSONPunct.Render(string(c)))
			i++

		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// scanString returns the index just past the string literal starting at i.
func scanString(src string, i int) int {
	j := i + 1
	for j < len(src) {
		switch src[j] {
		case '\\':
			j += 2
			continue
		case '"':
			return j + 1
		}
		j++
	}
	return len(src)
}

func isKey(src string, end int) bool {
	for j := end; j < len(src); j++ {
		switch src[j] {
		case ' ', '\t', '\n', '\r':
			continue
		case ':':
			return true
		default:
			return false
		}
	}
	return false
}

// --- structural view -------------------------------------------------------

type jkind uint8

const (
	jObject jkind = iota
	jArray
	jString
	jNumber
	jBool
	jNull
)

// jnode is a JSON value with its key order preserved.
//
// encoding/json decodes objects into maps, which loses ordering; APIs return
// fields in a meaningful order and developers navigate by it, so the tree is
// built from the token stream instead.
type jnode struct {
	key      string
	hasKey   bool
	index    int
	kind     jkind
	scalar   string
	children []*jnode
	expanded bool
}

// parseJSONTree builds an ordered tree, refusing payloads large enough to make
// the UI unresponsive.
func parseJSONTree(body []byte) (*jnode, error) {
	if len(body) > maxTreeBytes {
		return nil, fmt.Errorf("payload too large for the structural view (%s)", bytesHuman(int64(len(body))))
	}
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(body)))
	dec.UseNumber()

	count := 0
	root, err := parseValue(dec, &count)
	if err != nil {
		return nil, err
	}
	// Two levels open is the sweet spot: the shape is visible immediately
	// without a wall of leaves.
	expandTo(root, 2)
	return root, nil
}

func parseValue(dec *json.Decoder, count *int) (*jnode, error) {
	*count++
	if *count > maxTreeNodes {
		return nil, fmt.Errorf("payload has too many nodes for the structural view")
	}

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}

	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			n := &jnode{kind: jObject}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := keyTok.(string)
				child, err := parseValue(dec, count)
				if err != nil {
					return nil, err
				}
				child.key, child.hasKey = key, true
				n.children = append(n.children, child)
			}
			if _, err := dec.Token(); err != nil { // consume '}'
				return nil, err
			}
			return n, nil

		case '[':
			n := &jnode{kind: jArray}
			for dec.More() {
				child, err := parseValue(dec, count)
				if err != nil {
					return nil, err
				}
				child.index = len(n.children)
				n.children = append(n.children, child)
			}
			if _, err := dec.Token(); err != nil { // consume ']'
				return nil, err
			}
			return n, nil
		}
		return nil, fmt.Errorf("unexpected %v", t)

	case string:
		return &jnode{kind: jString, scalar: t}, nil
	case json.Number:
		return &jnode{kind: jNumber, scalar: t.String()}, nil
	case bool:
		return &jnode{kind: jBool, scalar: fmt.Sprintf("%t", t)}, nil
	case nil:
		return &jnode{kind: jNull, scalar: "null"}, nil
	}
	return nil, fmt.Errorf("unsupported JSON token")
}

func expandTo(n *jnode, depth int) {
	if n == nil {
		return
	}
	n.expanded = depth > 0
	for _, c := range n.children {
		expandTo(c, depth-1)
	}
}

func (n *jnode) container() bool { return n.kind == jObject || n.kind == jArray }

// summary describes a collapsed container by its size, so a folded node still
// tells you whether it is worth opening.
func (n *jnode) summary() string {
	switch n.kind {
	case jObject:
		return fmt.Sprintf("{%d}", len(n.children))
	case jArray:
		return fmt.Sprintf("[%d]", len(n.children))
	}
	return ""
}

// treeLine is one rendered row of the structural view, kept alongside its node
// so the cursor can act on what it is pointing at.
type treeLine struct {
	node  *jnode
	depth int
	text  string
}

// flattenTree renders the currently expanded nodes into display rows.
func flattenTree(n *jnode, width int) []treeLine {
	var out []treeLine
	var walk func(n *jnode, depth int)

	walk = func(n *jnode, depth int) {
		out = append(out, treeLine{node: n, depth: depth, text: renderTreeRow(n, depth, width)})
		if n.container() && n.expanded {
			for _, c := range n.children {
				walk(c, depth+1)
			}
		}
	}
	walk(n, 0)
	return out
}

func renderTreeRow(n *jnode, depth, width int) string {
	indent := strings.Repeat("  ", depth)

	toggle := "  "
	if n.container() {
		if n.expanded {
			toggle = styFaint.Render("▾") + " "
		} else {
			toggle = styFaint.Render("▸") + " "
		}
	}

	label := ""
	switch {
	case n.hasKey:
		label = styJSONKey.Render(n.key)
	case depth > 0:
		label = styFaint.Render(fmt.Sprintf("%d", n.index))
	}

	var value string
	switch {
	case n.container() && n.expanded:
		value = styFaint.Render(n.summary())
	case n.container():
		value = styFaint.Render(n.summary()) + " " + styMuted.Render(preview(n))
	case n.kind == jString:
		value = styJSONString.Render(quoteScalar(n.scalar, width))
	case n.kind == jNumber:
		value = styJSONNumber.Render(n.scalar)
	case n.kind == jBool:
		value = styJSONBool.Render(n.scalar)
	default:
		value = styJSONNull.Render(n.scalar)
	}

	row := indent + toggle
	if label != "" {
		// A fixed gutter aligns values into a column the eye can run down.
		labelWidth := lipgloss.Width(label)
		gap := 14 - labelWidth
		if gap < 1 {
			gap = 1
		}
		row += label + strings.Repeat(" ", gap)
	}
	return row + value
}

// preview summarises a collapsed container's contents in one line.
func preview(n *jnode) string {
	if len(n.children) == 0 {
		return ""
	}
	var parts []string
	for i, c := range n.children {
		if i == 3 {
			parts = append(parts, "…")
			break
		}
		switch {
		case c.hasKey:
			parts = append(parts, c.key)
		case c.container():
			parts = append(parts, c.summary())
		default:
			parts = append(parts, truncate(c.scalar, 12))
		}
	}
	return strings.Join(parts, ", ")
}

func quoteScalar(s string, width int) string {
	if width > 20 {
		s = truncate(s, width-24)
	}
	return `"` + s + `"`
}

// --- canonical form for diffing --------------------------------------------

// canonicalJSON renders a payload with sorted keys and stable indentation.
//
// Diffing raw bodies would flag every reordered key and every whitespace change
// as a difference. Canonicalising first means a diff shows what actually
// changed in the data, which is the whole point of comparing two responses.
func canonicalJSON(body []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return body, false
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()

	count := 0
	root, err := parseValue(dec, &count)
	if err != nil {
		return body, false
	}
	var buf bytes.Buffer
	writeCanonical(&buf, root, 0)
	buf.WriteByte('\n')
	return buf.Bytes(), true
}

func writeCanonical(b *bytes.Buffer, n *jnode, depth int) {
	indent := strings.Repeat("  ", depth)
	inner := strings.Repeat("  ", depth+1)

	switch n.kind {
	case jObject:
		if len(n.children) == 0 {
			b.WriteString("{}")
			return
		}
		kids := append([]*jnode(nil), n.children...)
		sort.SliceStable(kids, func(i, j int) bool { return kids[i].key < kids[j].key })
		b.WriteString("{\n")
		for i, c := range kids {
			b.WriteString(inner)
			key, _ := json.Marshal(c.key)
			b.Write(key)
			b.WriteString(": ")
			writeCanonical(b, c, depth+1)
			if i < len(kids)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(indent + "}")

	case jArray:
		if len(n.children) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, c := range n.children {
			b.WriteString(inner)
			writeCanonical(b, c, depth+1)
			if i < len(n.children)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(indent + "]")

	case jString:
		s, _ := json.Marshal(n.scalar)
		b.Write(s)
	default:
		b.WriteString(n.scalar)
	}
}
