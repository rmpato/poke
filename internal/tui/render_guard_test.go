package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// lipgloss pads every line of a rendered block to the block's width, so a
// newline inside Render appends a run of trailing spaces:
//
//	styFaint.Render("hello\n")  =>  "hello\n     "
//
// Those spaces shift whatever is written next, which silently wrecks the
// alignment of anything built by appending to a strings.Builder. It has bitten
// this package twice, so the pattern is banned rather than remembered.
func TestNoNewlinesInsideRender(t *testing.T) {
	// Confirm the underlying behaviour still exists; if lipgloss ever changes
	// it, this test should be revisited rather than silently kept.
	if got := stripANSI(styFaint.Render("hello\n")); got == "hello\n" {
		t.Skip("lipgloss no longer pads trailing lines; this guard can be removed")
	}

	pattern := regexp.MustCompile(`\.Render\([^)]*\\n`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if pattern.MatchString(line) {
				t.Errorf("%s:%d: newline inside Render(); put it outside:\n\t%s",
					file, i+1, strings.TrimSpace(line))
			}
		}
	}
}
