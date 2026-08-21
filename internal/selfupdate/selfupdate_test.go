package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.2.0", "1.10.0", -1}, // numeric, not lexical
		{"2.0.0", "1.9.9", 1},
		{"1.0.0-rc1", "1.0.0", 0}, // pre-release suffixes are ignored
		{"0.1.0", "0.2.0", -1},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// buildArchive produces a release archive the way goreleaser would.
func buildArchive(t *testing.T, contents map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, body := range contents {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// releaseServer serves a plausible GitHub release. corrupt swaps the archive
// after the checksums were computed, simulating tampering in transit.
func releaseServer(t *testing.T, version string, archive []byte, opts ...func(*serverOpts)) *httptest.Server {
	t.Helper()
	so := &serverOpts{}
	for _, o := range opts {
		o(so)
	}

	assetName := fmt.Sprintf("pogo_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	sums := fmt.Sprintf("%s  %s\n%s  other_file.tar.gz\n", sha256hex(archive), assetName, sha256hex([]byte("x")))
	if so.omitChecksum {
		sums = "deadbeef  something-else.tar.gz\n"
	}

	served := archive
	if so.corrupt {
		served = append(append([]byte(nil), archive...), 'x')
	}

	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "tag_name": "v%s",
		  "html_url": "https://example.invalid/release",
		  "assets": [
		    {"name": %q, "browser_download_url": "%s/dl/%s"},
		    {"name": "checksums.txt", "browser_download_url": "%s/dl/checksums.txt"}
		  ]
		}`, version, assetName, base, assetName, base)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sums))
	})
	mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(served)
	})

	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

type serverOpts struct {
	corrupt      bool
	omitChecksum bool
}

func withCorruptArchive(o *serverOpts) { o.corrupt = true }
func withoutChecksum(o *serverOpts)    { o.omitChecksum = true }

// installDirWith creates a directory holding fake installed binaries.
func installDirWith(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("old binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunReplacesInstalledBinaries(t *testing.T) {
	archive := buildArchive(t, map[string]string{
		"pogo": "new pogo binary", "README.md": "docs",
	})
	srv := releaseServer(t, "1.2.0", archive)
	dir := installDirWith(t, "pogo")

	var out bytes.Buffer
	res, err := Run(context.Background(), Options{
		Current: "1.0.0", APIBase: srv.URL, Dir: dir, Client: srv.Client(),
	}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.To != "1.2.0" || res.From != "1.0.0" {
		t.Errorf("result = %+v", res)
	}
	if len(res.Updated) != 1 {
		t.Errorf("updated %v, want the one binary", res.Updated)
	}
	for name, want := range map[string]string{"pogo": "new pogo binary"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if !strings.Contains(out.String(), "checksum verified") {
		t.Error("the user should be told the download was verified")
	}
}

// The executable bit must survive, or the update bricks the install.
func TestRunPreservesPermissions(t *testing.T) {
	archive := buildArchive(t, map[string]string{"pogo": "new"})
	srv := releaseServer(t, "1.2.0", archive)
	dir := installDirWith(t, "pogo")

	if _, err := Run(context.Background(), Options{
		Current: "1.0.0", APIBase: srv.URL, Dir: dir, Client: srv.Client(),
	}, nil); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(filepath.Join(dir, "pogo"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %o; the binary is no longer executable", fi.Mode().Perm())
	}
}

// An update replaces what is installed here and adds nothing. A directory the
// user dropped pogo into is not somewhere to start writing new files.
func TestRunOnlyTouchesInstalledBinaries(t *testing.T) {
	archive := buildArchive(t, map[string]string{"pogo": "new pogo", "extra": "not ours"})
	srv := releaseServer(t, "1.2.0", archive)
	dir := installDirWith(t, "pogo")

	res, err := Run(context.Background(), Options{
		Current: "1.0.0", APIBase: srv.URL, Dir: dir, Client: srv.Client(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "pogo" {
		t.Errorf("updated %v, want only pogo", res.Updated)
	}
	if _, err := os.Stat(filepath.Join(dir, "extra")); !os.IsNotExist(err) {
		t.Error("a file from the archive was installed that was not there before")
	}
}

func TestRunRefusesTamperedDownload(t *testing.T) {
	archive := buildArchive(t, map[string]string{"pogo": "new"})
	srv := releaseServer(t, "1.2.0", archive, withCorruptArchive)
	dir := installDirWith(t, "pogo")

	_, err := Run(context.Background(), Options{
		Current: "1.0.0", APIBase: srv.URL, Dir: dir, Client: srv.Client(),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want a checksum mismatch", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "pogo"))
	if string(got) != "old binary" {
		t.Error("a failed verification must leave the installed binary alone")
	}
}

func TestRunRefusesUnpublishedChecksum(t *testing.T) {
	archive := buildArchive(t, map[string]string{"pogo": "new"})
	srv := releaseServer(t, "1.2.0", archive, withoutChecksum)
	dir := installDirWith(t, "pogo")

	_, err := Run(context.Background(), Options{
		Current: "1.0.0", APIBase: srv.URL, Dir: dir, Client: srv.Client(),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "no checksum published") {
		t.Fatalf("err = %v, want a refusal to install unverified bytes", err)
	}
}

func TestRunReportsUpToDate(t *testing.T) {
	archive := buildArchive(t, map[string]string{"pogo": "new"})
	srv := releaseServer(t, "1.0.0", archive)
	dir := installDirWith(t, "pogo")

	_, err := Run(context.Background(), Options{
		Current: "1.0.0", APIBase: srv.URL, Dir: dir, Client: srv.Client(),
	}, nil)
	if !errors.Is(err, ErrUpToDate) {
		t.Errorf("err = %v, want ErrUpToDate", err)
	}

	// A newer local build must not be downgraded either.
	_, err = Run(context.Background(), Options{
		Current: "2.0.0", APIBase: srv.URL, Dir: dir, Client: srv.Client(),
	}, nil)
	if !errors.Is(err, ErrUpToDate) {
		t.Errorf("err = %v, want ErrUpToDate for a newer local version", err)
	}
}

// A crafted archive must not be able to write outside the install directory.
func TestExtractIgnoresPathsOutsideTheArchive(t *testing.T) {
	archive := buildArchive(t, map[string]string{
		"../../../../tmp/pogo": "evil",
		"pogo":                 "good",
	})
	got, err := extract(archive)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["pogo"]) != "good" && string(got["pogo"]) != "evil" {
		t.Fatalf("unexpected contents: %q", got["pogo"])
	}
	for name := range got {
		if strings.ContainsAny(name, "/\\") {
			t.Errorf("extract kept a path separator in %q; it must use base names only", name)
		}
	}
}

func TestLatestReportsMissingReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Latest(context.Background(), Options{APIBase: srv.URL, Client: srv.Client()})
	if err == nil || !strings.Contains(err.Error(), "no releases published") {
		t.Errorf("err = %v, want a clear 'no releases' message", err)
	}
}

func TestRunFailsClearlyWhenNothingIsInstalled(t *testing.T) {
	archive := buildArchive(t, map[string]string{"pogo": "new"})
	srv := releaseServer(t, "1.2.0", archive)

	_, err := Run(context.Background(), Options{
		Current: "1.0.0", APIBase: srv.URL, Dir: t.TempDir(), Client: srv.Client(),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "no pogo binary") {
		t.Errorf("err = %v, want a clear message about finding nothing to update", err)
	}
}
