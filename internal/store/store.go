// Package store persists captured requests.
//
// The format is an append-only JSONL log of operations plus a blob directory
// for payloads. That combination was chosen over SQLite deliberately:
//
//   - The workload is "append one row; read a few thousand at startup". SQLite
//     would buy indexes nobody queries, at the cost of cgo or a very large pure
//     Go port.
//   - Appending is a single locked write, so several poke invocations can run
//     concurrently — including while pogo has the history open — without a
//     read-modify-write race.
//   - Mutations are operations, not edits. Favouriting or deleting appends a
//     record; the original capture is never rewritten. History stays immutable
//     by construction rather than by discipline.
//   - The result is greppable with jq and portable with cp, which matters for a
//     local-first tool people are asked to trust with their traffic.
//
// This package knows nothing about terminals or Bubble Tea.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/history"
)

// ErrNotFound is returned when an id is absent from the folded history.
var ErrNotFound = errors.New("entry not found")

// op names in the log. They are short because they are repeated on every line.
const (
	opPut    = "put"
	opPatch  = "patch"
	opDelete = "del"
)

// record is one line of history.jsonl.
type record struct {
	Op    string         `json:"op"`
	At    time.Time      `json:"at"`
	Entry *history.Entry `json:"entry,omitempty"`
	ID    string         `json:"id,omitempty"`
	Patch *patch         `json:"patch,omitempty"`
}

// patch carries partial updates. Pointer fields distinguish "not set" from
// "set to the zero value".
type patch struct {
	Favorite *bool   `json:"favorite,omitempty"`
	Note     *string `json:"note,omitempty"`
}

// Store is a handle on a history directory. It is safe for concurrent use by
// multiple processes; within a process, callers should not share a Store across
// goroutines without external synchronization.
type Store struct {
	dir        string
	logPath    string
	lockPath   string
	blobDir    string
	maxEntries int
}

// Open prepares the history directory, creating it if needed.
//
// Permissions are deliberately tight: this directory holds request headers, and
// those routinely contain bearer tokens.
func Open(cfg config.Config) (*Store, error) {
	s := &Store{
		dir:        cfg.Dir(),
		logPath:    cfg.HistoryFile(),
		lockPath:   filepath.Join(cfg.Dir(), ".lock"),
		blobDir:    cfg.BlobDir(),
		maxEntries: cfg.Capture.MaxEntries,
	}
	if err := os.MkdirAll(s.blobDir, 0o700); err != nil {
		return nil, fmt.Errorf("create history dir: %w", err)
	}
	// Repair permissions on a directory created by an older or sloppier tool.
	_ = os.Chmod(s.dir, 0o700)
	return s, nil
}

// Dir returns the history directory.
func (s *Store) Dir() string { return s.dir }

// Path returns the history log path.
func (s *Store) Path() string { return s.logPath }

// Append records a new entry. The entry is not modified.
func (s *Store) Append(e *history.Entry) error {
	if e.ID == "" {
		e.ID = history.NewID()
	}
	return s.write(record{Op: opPut, At: time.Now().UTC(), Entry: e})
}

// SetFavorite marks or unmarks an entry.
func (s *Store) SetFavorite(id string, fav bool) error {
	return s.write(record{Op: opPatch, At: time.Now().UTC(), ID: id, Patch: &patch{Favorite: &fav}})
}

// SetNote attaches a note to an entry.
func (s *Store) SetNote(id, note string) error {
	return s.write(record{Op: opPatch, At: time.Now().UTC(), ID: id, Patch: &patch{Note: &note}})
}

// Delete tombstones an entry and removes its payload blobs. The log line
// remains, which is what makes deletion safe under concurrent writers.
func (s *Store) Delete(id string) error {
	if err := s.write(record{Op: opDelete, At: time.Now().UTC(), ID: id}); err != nil {
		return err
	}
	for _, ext := range []string{".req", ".res"} {
		_ = os.Remove(filepath.Join(s.blobDir, id+ext))
	}
	return nil
}

// write appends one record under an exclusive lock.
func (s *Store) write(r record) error {
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	f, err := os.OpenFile(s.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	defer func() { _ = f.Close() }()

	// One Write for the whole line: even if the lock were somehow bypassed, a
	// reader never sees a half-written record.
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}

// lock takes an exclusive advisory lock on a dedicated lock file.
//
// The lock lives in its own file rather than on history.jsonl because
// compaction replaces the log by rename; a lock held on the old inode would not
// exclude a writer that had already opened the new one.
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock history: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// LoadResult carries the folded history plus what was skipped, so callers can
// tell the user "3 damaged records ignored" instead of silently losing data.
type LoadResult struct {
	Entries []*history.Entry // newest first
	Records int              // log lines read
	Skipped int              // lines that failed to parse
}

// Load reads the log and folds its operations into the current history,
// ordered newest first.
func (s *Store) Load() (LoadResult, error) {
	var res LoadResult

	f, err := os.Open(s.logPath)
	if errors.Is(err, os.ErrNotExist) {
		return res, nil // no history yet is not an error
	}
	if err != nil {
		return res, fmt.Errorf("open history: %w", err)
	}
	defer func() { _ = f.Close() }()

	byID := make(map[string]*history.Entry)
	order := make([]string, 0, 256)

	sc := bufio.NewScanner(f)
	// Entries embed headers and small inline bodies; the default 64 KiB token
	// limit is not enough for a request with a large header set.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		res.Records++

		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			res.Skipped++
			continue
		}

		switch r.Op {
		case opPut:
			if r.Entry == nil || r.Entry.ID == "" {
				res.Skipped++
				continue
			}
			if _, seen := byID[r.Entry.ID]; !seen {
				order = append(order, r.Entry.ID)
			}
			byID[r.Entry.ID] = r.Entry
		case opPatch:
			e, ok := byID[r.ID]
			if !ok || r.Patch == nil {
				continue // a patch for a deleted entry is a no-op, not damage
			}
			if r.Patch.Favorite != nil {
				e.Favorite = *r.Patch.Favorite
			}
			if r.Patch.Note != nil {
				e.Note = *r.Patch.Note
			}
		case opDelete:
			delete(byID, r.ID)
		default:
			res.Skipped++
		}
	}
	if err := sc.Err(); err != nil {
		return res, fmt.Errorf("read history: %w", err)
	}

	entries := make([]*history.Entry, 0, len(byID))
	for _, id := range order {
		if e, ok := byID[id]; ok {
			entries = append(entries, e)
		}
	}
	// Log order is close to chronological already; sorting makes it exact even
	// when several poke processes interleave their appends.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})

	res.Entries = entries
	return res, nil
}

// Get returns a single entry by id.
func (s *Store) Get(id string) (*history.Entry, error) {
	res, err := s.Load()
	if err != nil {
		return nil, err
	}
	for _, e := range res.Entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, ErrNotFound
}
