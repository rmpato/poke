package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rmpato/pogo/internal/history"
)

// Stats describes how much of the log is still meaningful, which is what
// decides whether compaction is worth doing.
type Stats struct {
	Records int   // lines in the log
	Live    int   // entries that survive folding
	Bytes   int64 // log size
}

// Waste is the fraction of log records that no longer contribute an entry.
func (st Stats) Waste() float64 {
	if st.Records == 0 {
		return 0
	}
	return float64(st.Records-st.Live) / float64(st.Records)
}

// Stat summarizes the log without materializing the entries twice.
func (s *Store) Stat() (Stats, error) {
	res, err := s.Load()
	if err != nil {
		return Stats{}, err
	}
	st := Stats{Records: res.Records, Live: len(res.Entries)}
	if fi, err := os.Stat(s.logPath); err == nil {
		st.Bytes = fi.Size()
	}
	return st, nil
}

// ShouldCompact reports whether the log has accumulated enough dead weight to
// be worth rewriting. Compaction is never required for correctness.
func (s *Store) ShouldCompact() bool {
	st, err := s.Stat()
	if err != nil {
		return false
	}
	if s.maxEntries > 0 && st.Live > s.maxEntries {
		return true
	}
	return st.Records > 200 && st.Waste() > 0.4
}

// Compact rewrites the log with one put record per surviving entry, applies the
// configured entry cap, and removes blobs no entry references any more.
//
// It holds the lock for the whole rewrite and swaps the file in by rename, so a
// concurrent pogo either lands in the old log (and is picked up by the next
// compaction) or blocks until the swap is done. A crash mid-compaction leaves
// the original log untouched.
func (s *Store) Compact() (Stats, error) {
	unlock, err := s.lock()
	if err != nil {
		return Stats{}, err
	}
	defer unlock()

	res, err := s.Load()
	if err != nil {
		return Stats{}, err
	}

	entries := res.Entries // newest first
	dropped := []*history.Entry{}
	if s.maxEntries > 0 && len(entries) > s.maxEntries {
		kept := make([]*history.Entry, 0, s.maxEntries)
		for _, e := range entries {
			// Favorites are exempt from the cap: the user said these matter.
			if len(kept) < s.maxEntries || e.Favorite {
				kept = append(kept, e)
			} else {
				dropped = append(dropped, e)
			}
		}
		entries = kept
	}

	tmp, err := os.CreateTemp(s.dir, "history-*.jsonl")
	if err != nil {
		return Stats{}, fmt.Errorf("create temp log: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeds

	w := bufio.NewWriter(tmp)
	// Oldest first, so the compacted log keeps reading chronologically.
	for i := len(entries) - 1; i >= 0; i-- {
		line, err := json.Marshal(record{Op: opPut, At: time.Now().UTC(), Entry: entries[i]})
		if err != nil {
			_ = tmp.Close()
			return Stats{}, err
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			_ = tmp.Close()
			return Stats{}, err
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return Stats{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return Stats{}, err
	}
	if err := tmp.Close(); err != nil {
		return Stats{}, err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return Stats{}, err
	}
	if err := os.Rename(tmpName, s.logPath); err != nil {
		return Stats{}, fmt.Errorf("replace history: %w", err)
	}

	for _, e := range dropped {
		for _, ext := range []string{".req", ".res"} {
			_ = os.Remove(filepath.Join(s.blobDir, e.ID+ext))
		}
	}
	s.sweepBlobs(entries)

	return Stats{Records: len(entries), Live: len(entries)}, nil
}

// sweepBlobs removes payload files with no surviving entry. Orphans appear when
// a process dies between writing a blob and appending its log record.
func (s *Store) sweepBlobs(entries []*history.Entry) {
	live := make(map[string]struct{}, len(entries)*2)
	for _, e := range entries {
		live[e.ID] = struct{}{}
	}
	files, err := os.ReadDir(s.blobDir)
	if err != nil {
		return
	}
	for _, f := range files {
		name := f.Name()
		id := strings.TrimSuffix(strings.TrimSuffix(name, ".req"), ".res")
		if _, ok := live[id]; !ok {
			_ = os.Remove(filepath.Join(s.blobDir, name))
		}
	}
}
