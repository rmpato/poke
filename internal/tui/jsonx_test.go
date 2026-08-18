package tui

import (
	"strings"
	"testing"
)

func TestLooksJSON(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{`{"a":1}`, true},
		{`[1,2,3]`, true},
		{"  \n {\"a\":1} ", true},
		{`{"a":}`, false}, // invalid, even though it starts like JSON
		{`<html></html>`, false},
		{`plain text`, false},
		{``, false},
	}
	for _, tt := range tests {
		if got := looksJSON("application/json", []byte(tt.body)); got != tt.want {
			t.Errorf("looksJSON(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
}

// json.Indent works on the text, so key order survives. Order is meaningful to
// the person reading a response and must not be rearranged.
func TestPrettyJSONPreservesKeyOrder(t *testing.T) {
	out, ok := prettyJSON([]byte(`{"zebra":1,"apple":2,"mango":3}`))
	if !ok {
		t.Fatal("valid JSON should pretty-print")
	}
	got := string(out)
	if strings.Index(got, "zebra") > strings.Index(got, "apple") {
		t.Errorf("key order was changed:\n%s", got)
	}
}

func TestPrettyJSONLeavesInvalidInputAlone(t *testing.T) {
	in := []byte(`not json`)
	out, ok := prettyJSON(in)
	if ok {
		t.Error("invalid JSON should report failure")
	}
	if string(out) != string(in) {
		t.Errorf("invalid input should be returned unchanged, got %q", out)
	}
}

func TestHighlightJSONKeepsTextIntact(t *testing.T) {
	src := `{
  "name": "Pato",
  "n": 42,
  "ok": true,
  "nothing": null
}`
	out := highlightJSON(src)

	// Styling adds escape sequences; the visible text must be unchanged.
	if stripped := stripANSI(out); stripped != src {
		t.Errorf("highlighting altered the text:\ngot  %q\nwant %q", stripped, src)
	}
}

func TestHighlightJSONHandlesEscapedQuotes(t *testing.T) {
	src := `{"say": "he said \"hi\"", "next": 1}`
	if stripped := stripANSI(highlightJSON(src)); stripped != src {
		t.Errorf("escaped quotes broke the scanner: %q", stripped)
	}
}

func TestParseJSONTreePreservesOrderAndTypes(t *testing.T) {
	tree, err := parseJSONTree([]byte(`{"zebra":1,"apple":"two","list":[1,2],"flag":false,"none":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if tree.kind != jObject || len(tree.children) != 5 {
		t.Fatalf("tree = %+v", tree)
	}

	wantKeys := []string{"zebra", "apple", "list", "flag", "none"}
	for i, want := range wantKeys {
		if tree.children[i].key != want {
			t.Errorf("child %d is %q, want %q — key order must be preserved", i, tree.children[i].key, want)
		}
	}
	if tree.children[0].kind != jNumber || tree.children[1].kind != jString {
		t.Error("scalar kinds were not detected")
	}
	if tree.children[2].kind != jArray || len(tree.children[2].children) != 2 {
		t.Error("array was not parsed")
	}
	if tree.children[3].kind != jBool || tree.children[4].kind != jNull {
		t.Error("bool/null were not detected")
	}
}

// Numbers must not be reformatted: 9.50 and 1e3 mean something to whoever is
// reading the response.
func TestParseJSONTreeKeepsNumberFormatting(t *testing.T) {
	tree, err := parseJSONTree([]byte(`{"a":9.50,"b":1e3,"c":10000000000000000001}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"9.50", "1e3", "10000000000000000001"}
	for i, w := range want {
		if got := tree.children[i].scalar; got != w {
			t.Errorf("number %d rendered as %q, want %q", i, got, w)
		}
	}
}

func TestParseJSONTreeRefusesHugePayloads(t *testing.T) {
	huge := make([]byte, maxTreeBytes+1)
	for i := range huge {
		huge[i] = ' '
	}
	huge[0], huge[len(huge)-1] = '[', ']'

	if _, err := parseJSONTree(huge); err == nil {
		t.Error("a payload past the limit should be refused rather than stall the UI")
	}
}

func TestFlattenTreeRespectsFolding(t *testing.T) {
	tree, err := parseJSONTree([]byte(`{"a":{"b":{"c":1}}}`))
	if err != nil {
		t.Fatal(err)
	}

	// Two levels are open by default, so the deepest value stays hidden.
	lines := flattenTree(tree, 80)
	if len(lines) != 3 {
		t.Errorf("got %d visible rows, want 3 with two levels expanded", len(lines))
	}

	tree.children[0].children[0].expanded = true
	if got := len(flattenTree(tree, 80)); got != 4 {
		t.Errorf("after unfolding, got %d rows, want 4", got)
	}

	tree.expanded = false
	if got := len(flattenTree(tree, 80)); got != 1 {
		t.Errorf("a folded root should render one row, got %d", got)
	}
}

func TestCanonicalJSONSortsKeysAndNormalisesFormat(t *testing.T) {
	a, okA := canonicalJSON([]byte(`{"b":2,"a":1}`))
	b, okB := canonicalJSON([]byte("{\n  \"a\":   1,\n  \"b\": 2\n}"))

	if !okA || !okB {
		t.Fatal("both inputs are valid JSON")
	}
	if string(a) != string(b) {
		t.Errorf("the same data in different shapes should canonicalize identically:\n%s\n---\n%s", a, b)
	}
}

// Array order is data, not formatting, so it must survive canonicalization.
func TestCanonicalJSONKeepsArrayOrder(t *testing.T) {
	out, _ := canonicalJSON([]byte(`[3,1,2]`))
	if i3, i1 := strings.Index(string(out), "3"), strings.Index(string(out), "1"); i3 > i1 {
		t.Errorf("array order was changed:\n%s", out)
	}
}

func TestCanonicalJSONLeavesNonJSONAlone(t *testing.T) {
	in := []byte("<html>hello</html>")
	out, ok := canonicalJSON(in)
	if ok {
		t.Error("non-JSON should report failure")
	}
	if string(out) != string(in) {
		t.Error("non-JSON should be returned unchanged for a plain text diff")
	}
}

// stripANSI removes styling so tests can assert on the visible text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
