package selfupdate

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Notice prints a one-line note when a newer release is known, and arranges for
// the cached answer to be refreshed later.
//
// The check never delays the command the user actually ran. Reading the cache is
// a file read; refreshing it happens in a detached child process that outlives
// this one, so the next invocation has a fresh answer and this one pays nothing.
//
// Nothing is installed. This only ever prints a sentence.
func Notice(dir, current string, interval time.Duration, enabled bool, w io.Writer) {
	if !enabled || dir == "" {
		return
	}

	cache := LoadCache(dir)
	if v := cache.Available(current); v != "" {
		fmt.Fprintf(w, "\npoke %s is available (you have %s) — run: poke --update\n", v, current)
		fmt.Fprintf(w, "silence this with POKE_NO_UPDATE_CHECK=1\n")
	}

	if cache.Stale(interval) {
		spawnRefresh()
	}
}

// spawnRefresh starts a detached copy of this binary to refresh the cache.
//
// Detaching matters: poke exits as soon as curl does, so an in-process check
// would either block the exit or be killed before it finished.
func spawnRefresh() {
	exe, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(exe, refreshFlag)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return
	}
	// Release the child so it is not left as a zombie when this process exits.
	_ = cmd.Process.Release()
}

// refreshFlag is the internal flag the detached child is invoked with. It is
// deliberately verbose and namespaced: it is not part of the user interface.
const refreshFlag = "--poke-refresh-update-cache"

// RefreshFlag reports the internal refresh flag so main can intercept it.
func RefreshFlag() string { return refreshFlag }

// RefreshCache performs the network check. It is what the detached child runs,
// and it stays silent whatever happens: nobody is watching its output.
func RefreshCache(dir, current string) int {
	if _, err := Refresh(dir, Options{Current: current}); err != nil {
		return 1
	}
	return 0
}
