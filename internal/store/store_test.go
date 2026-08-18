package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/history"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(config.ForDir(t.TempDir()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return st
}

func entry(url string) *history.Entry {
	return &history.Entry{
		ID:        history.NewID(),
		CreatedAt: time.Now().UTC(),
		Source:    history.SourcePoke,
		Command:   history.Command{Args: []string{url}},
		Request:   history.Request{Method: "GET", URL: url},
	}
}

func TestAppendAndLoadRoundTrip(t *testing.T) {
	st := newStore(t)

	e := entry("https://example.com/users")
	e.Duration = history.Duration(42 * time.Millisecond)
	e.Response = &history.Response{
		Blocks: []history.Block{{Proto: "HTTP/2", Status: 200, Reason: "OK"}},
	}
	if err := st.Append(e); err != nil {
		t.Fatalf("append: %v", err)
	}

	res, err := st.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(res.Entries))
	}

	got := res.Entries[0]
	if got.ID != e.ID || got.Request.URL != e.Request.URL {
		t.Errorf("entry did not survive the round trip: %+v", got)
	}
	if got.Status() != 200 {
		t.Errorf("status = %d, want 200", got.Status())
	}
	if got.Duration != e.Duration {
		t.Errorf("duration = %v, want %v", got.Duration, e.Duration)
	}
}

func TestLoadOrdersNewestFirst(t *testing.T) {
	st := newStore(t)

	for i := 0; i < 5; i++ {
		e := entry(fmt.Sprintf("https://example.com/%d", i))
		e.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		if err := st.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	res, _ := st.Load()
	for i := 1; i < len(res.Entries); i++ {
		if res.Entries[i-1].CreatedAt.Before(res.Entries[i].CreatedAt) {
			t.Fatalf("entries are not newest-first: %v before %v",
				res.Entries[i-1].CreatedAt, res.Entries[i].CreatedAt)
		}
	}
}

func TestPatchAndDeleteFold(t *testing.T) {
	st := newStore(t)
	a, b := entry("https://example.com/a"), entry("https://example.com/b")
	if err := st.Append(a); err != nil {
		t.Fatal(err)
	}
	if err := st.Append(b); err != nil {
		t.Fatal(err)
	}

	if err := st.SetFavorite(a.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetNote(a.ID, "check this"); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(b.ID); err != nil {
		t.Fatal(err)
	}

	res, _ := st.Load()
	if len(res.Entries) != 1 {
		t.Fatalf("got %d entries after delete, want 1", len(res.Entries))
	}
	if !res.Entries[0].Favorite || res.Entries[0].Note != "check this" {
		t.Errorf("patches did not fold onto the entry: %+v", res.Entries[0])
	}

	// The original capture record is still on disk: history is append-only, and
	// a delete is a tombstone rather than a rewrite.
	data, err := os.ReadFile(st.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), b.ID) {
		t.Error("deleting rewrote history instead of appending a tombstone")
	}
}

// Unfavouriting must win over the earlier favorite, which is the whole point
// of folding operations in order.
func TestPatchesApplyInOrder(t *testing.T) {
	st := newStore(t)
	e := entry("https://example.com")
	st.Append(e)
	st.SetFavorite(e.ID, true)
	st.SetFavorite(e.ID, false)

	res, _ := st.Load()
	if res.Entries[0].Favorite {
		t.Error("the last patch should win")
	}
}

func TestCorruptLinesAreSkippedNotFatal(t *testing.T) {
	st := newStore(t)
	good := entry("https://example.com/good")
	st.Append(good)

	f, err := os.OpenFile(st.Path(), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{ this is not json\n")
	f.WriteString(`{"op":"put"}` + "\n") // put with no entry
	f.WriteString(`{"op":"nonsense","id":"x"}` + "\n")
	f.Close()

	res, err := st.Load()
	if err != nil {
		t.Fatalf("a damaged line should not fail the load: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Errorf("got %d entries, want the one good entry", len(res.Entries))
	}
	if res.Skipped != 3 {
		t.Errorf("Skipped = %d, want 3 so the UI can report the damage", res.Skipped)
	}
}

// Several poke processes append at once in real use. The lock plus single-write
// append must keep every record intact and parseable.
func TestConcurrentAppends(t *testing.T) {
	st := newStore(t)

	const writers, each = 8, 25
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				e := entry(fmt.Sprintf("https://example.com/%d/%d", w, i))
				// A long header makes each record big enough to matter.
				e.Request.Headers = append(e.Request.Headers,
					curlargs.Header{Name: "X-Filler", Value: strings.Repeat("x", 512)})
				if err := st.Append(e); err != nil {
					t.Errorf("append: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	res, err := st.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(res.Entries) != writers*each {
		t.Errorf("got %d entries, want %d", len(res.Entries), writers*each)
	}
	if res.Skipped != 0 {
		t.Errorf("%d records were damaged by concurrent writes", res.Skipped)
	}
}

func TestBlobInlineAndFile(t *testing.T) {
	st := newStore(t)
	id := history.NewID()

	small := []byte(`{"ok":true}`)
	ref, err := st.PutBody(id, KindResponse, small, int64(len(small)), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Blob != "" || ref.Inline == "" {
		t.Errorf("small payloads should stay inline, got %+v", ref)
	}

	big := []byte(strings.Repeat("a", InlineLimit+1))
	ref2, err := st.PutBody(id, KindRequest, big, int64(len(big)), "-d")
	if err != nil {
		t.Fatal(err)
	}
	if ref2.Blob == "" {
		t.Errorf("large payloads should go to a blob file, got %+v", ref2)
	}

	for _, r := range []*history.BodyRef{ref, ref2} {
		got, err := st.Body(r)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if len(got) == 0 {
			t.Errorf("body came back empty for %+v", r)
		}
	}
}

func TestBodyMarksTruncationAndBinary(t *testing.T) {
	st := newStore(t)
	id := history.NewID()

	data := []byte("only the first part")
	ref, _ := st.PutBody(id, KindResponse, data, 10_000, "stdout")
	if !ref.Truncated || ref.Size != 10_000 {
		t.Errorf("truncation must be recorded honestly: %+v", ref)
	}

	binary := append([]byte("PNG"), 0x00, 0x01, 0x02)
	ref2, _ := st.PutBody(id, KindRequest, binary, int64(len(binary)), "-d")
	if !ref2.Binary {
		t.Error("payload with NUL bytes should be marked binary")
	}
}

// A history file is a plain text file a user might edit or receive. A doctored
// blob reference must not be able to read outside the blob directory.
func TestBodyRejectsPathTraversal(t *testing.T) {
	st := newStore(t)
	for _, bad := range []string{"../../etc/passwd", "sub/dir", "..\\x"} {
		if _, err := st.Body(&history.BodyRef{Blob: bad}); err == nil {
			t.Errorf("Body(%q) should be rejected", bad)
		}
	}
}

func TestMissingBlobIsNotAnError(t *testing.T) {
	st := newStore(t)
	got, err := st.Body(&history.BodyRef{Blob: "NOSUCHBLOB.res"})
	if err != nil {
		t.Errorf("a pruned blob should not fail the read: %v", err)
	}
	if got != nil {
		t.Errorf("got %q, want nil", got)
	}
}

func TestCompactDropsDeadRecordsAndOrphanBlobs(t *testing.T) {
	st := newStore(t)

	kept := entry("https://example.com/kept")
	st.Append(kept)
	for i := 0; i < 10; i++ {
		e := entry(fmt.Sprintf("https://example.com/gone/%d", i))
		st.Append(e)
		st.PutBody(e.ID, KindResponse, []byte(strings.Repeat("x", InlineLimit+1)), 1, "stdout")
		st.SetFavorite(e.ID, true)
		st.Delete(e.ID)
	}

	before, _ := st.Stat()
	if before.Waste() == 0 {
		t.Fatal("test setup produced no dead records")
	}

	if _, err := st.Compact(); err != nil {
		t.Fatalf("compact: %v", err)
	}

	res, _ := st.Load()
	if len(res.Entries) != 1 || res.Entries[0].ID != kept.ID {
		t.Fatalf("compaction lost or kept the wrong entries: %+v", res.Entries)
	}
	if res.Records != 1 {
		t.Errorf("log still holds %d records after compaction, want 1", res.Records)
	}

	blobs, _ := os.ReadDir(filepath.Join(st.Dir(), "blobs"))
	if len(blobs) != 0 {
		t.Errorf("orphaned blobs survived compaction: %d left", len(blobs))
	}
}

func TestCompactAppliesEntryCapButKeepsFavorites(t *testing.T) {
	cfg := config.ForDir(t.TempDir())
	cfg.Capture.MaxEntries = 5
	st, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var starred string
	for i := 0; i < 20; i++ {
		e := entry(fmt.Sprintf("https://example.com/%d", i))
		e.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		st.Append(e)
		if i == 0 { // the oldest entry, which the cap would otherwise drop
			starred = e.ID
			st.SetFavorite(e.ID, true)
		}
	}

	if _, err := st.Compact(); err != nil {
		t.Fatal(err)
	}
	res, _ := st.Load()

	if len(res.Entries) != 6 {
		t.Errorf("got %d entries, want 5 recent plus 1 starred", len(res.Entries))
	}
	found := false
	for _, e := range res.Entries {
		if e.ID == starred {
			found = true
		}
	}
	if !found {
		t.Error("a starred entry was dropped by the cap; the user said it mattered")
	}
}

func TestGetReportsMissing(t *testing.T) {
	st := newStore(t)
	if _, err := st.Get("NOPE"); err != ErrNotFound {
		t.Errorf("Get on a missing id = %v, want ErrNotFound", err)
	}
}

func TestOpenCreatesPrivateDirectory(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(config.ForDir(filepath.Join(dir, "poke")))
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(st.Dir())
	if err != nil {
		t.Fatal(err)
	}
	// History holds Authorization headers; it must not be world readable.
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("history dir mode = %o, want 700", perm)
	}
}
