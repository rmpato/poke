// Package version carries build metadata, injected at link time by the
// Makefile and by goreleaser.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Version is the release version. It is overwritten with -ldflags at build
// time; "dev" is what a plain `go build` produces.
var Version = "dev"

// Commit is the git revision, filled in at build time or read from the module's
// build info when installed with `go install`.
var Commit = ""

// Line renders a one-line version banner for the named command.
func Line(name string) string {
	v, c := Version, Commit
	if c == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					c = s.Value[:7]
				}
			}
		}
	}
	if c != "" {
		v += " (" + c + ")"
	}
	return fmt.Sprintf("%s %s %s/%s %s", name, v, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
