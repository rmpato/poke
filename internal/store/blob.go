package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rmpato/pogo/internal/history"
)

// Blob kinds. The extension is the kind, which keeps the directory readable
// with plain ls.
const (
	KindRequest  = "req"
	KindResponse = "res"
)

// InlineLimit is the payload size below which a body is stored directly in the
// log line. Small JSON responses are the common case, and keeping them inline
// means the whole exchange is visible in one `tail -1 history.jsonl`.
const InlineLimit = 4 << 10

// PutBody stores a payload and returns the reference to record on the entry.
//
// data is what was captured, which may already be shorter than total when the
// configured cap kicked in; total is the real size that crossed the wire.
func (s *Store) PutBody(id, kind string, data []byte, total int64, origin string) (*history.BodyRef, error) {
	if len(data) == 0 && total == 0 {
		return nil, nil
	}
	ref := &history.BodyRef{
		Size:      total,
		Stored:    int64(len(data)),
		Truncated: total > int64(len(data)),
		Binary:    looksBinary(data),
		Origin:    origin,
	}

	if len(data) <= InlineLimit && !ref.Binary {
		ref.Inline = string(data)
		return ref, nil
	}

	name := id + "." + kind
	path := filepath.Join(s.blobDir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write body: %w", err)
	}
	ref.Blob = name
	return ref, nil
}

// Body returns the stored payload for a reference, reading the blob only when
// the payload was too large to inline.
func (s *Store) Body(ref *history.BodyRef) ([]byte, error) {
	if ref == nil {
		return nil, nil
	}
	if ref.Blob == "" {
		return []byte(ref.Inline), nil
	}
	// Guard against a doctored history file steering reads out of the blob dir.
	if strings.ContainsAny(ref.Blob, "/\\") || strings.Contains(ref.Blob, "..") {
		return nil, fmt.Errorf("invalid blob reference %q", ref.Blob)
	}
	data, err := os.ReadFile(filepath.Join(s.blobDir, ref.Blob))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // blob pruned; the entry itself is still valid
	}
	return data, err
}

// looksBinary reports whether a payload should be treated as non-text. A NUL
// byte in the first few KiB is the same heuristic grep and git use, and it is
// what keeps the TUI from spraying control codes across the terminal.
func looksBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
