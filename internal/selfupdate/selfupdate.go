// Package selfupdate replaces the installed binary with the latest release.
//
// It is deliberately unexciting: it runs only when the user asks, it talks to
// GitHub over HTTPS and nowhere else, and it refuses to install anything whose
// SHA-256 does not match the published checksums. There is no background check,
// no telemetry, and no phoning home on startup — pogo works offline, and an
// update mechanism that quietly contacted a server would break that promise.
package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultRepo is the project this binary updates from.
const DefaultRepo = "rmpato/pogo"

// binaries are the executables a release ships. pogo is one binary now; the
// list survives because the archive layout and the "only replace what is
// actually installed here" rule below are worth keeping either way.
// sibling too when they live in the same directory, because they are two halves
// of one tool and a version skew between them is confusing.
var binaries = []string{"pogo"}

// ErrUpToDate is returned when the installed version is already current.
var ErrUpToDate = errors.New("already up to date")

// Options configures an update.
type Options struct {
	Repo    string // owner/name; defaults to DefaultRepo
	Current string // the running version, e.g. "v0.1.0" or "dev"
	Client  *http.Client

	// APIBase and DownloadBase exist so tests can point at a local server.
	APIBase      string
	DownloadBase string

	// Dir is where the binaries live. Empty means "next to the running one".
	Dir string
}

// Release describes a published version.
type Release struct {
	Tag     string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Version is the release version without its leading "v".
func (r Release) Version() string { return strings.TrimPrefix(r.Tag, "v") }

func (o *Options) fill() {
	if o.Repo == "" {
		o.Repo = DefaultRepo
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 60 * time.Second}
	}
	if o.APIBase == "" {
		o.APIBase = "https://api.github.com"
	}
}

// Latest reports the most recent published release.
func Latest(ctx context.Context, opts Options) (Release, error) {
	opts.fill()

	url := fmt.Sprintf("%s/repos/%s/releases/latest", opts.APIBase, opts.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := opts.Client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("contact GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return Release{}, fmt.Errorf("no releases published for %s yet", opts.Repo)
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub returned %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("read release info: %w", err)
	}
	if rel.Tag == "" {
		return Release{}, errors.New("GitHub returned a release with no tag")
	}
	return rel, nil
}

// Result describes what an update did.
type Result struct {
	From    string
	To      string
	Dir     string
	Updated []string
}

// Run downloads the latest release and replaces the installed binaries.
//
// Progress is written to out so the user can see what is happening to their
// executables; nothing about this should be invisible.
func Run(ctx context.Context, opts Options, out io.Writer) (Result, error) {
	opts.fill()
	if out == nil {
		out = io.Discard
	}

	rel, err := Latest(ctx, opts)
	if err != nil {
		return Result{}, err
	}

	current := strings.TrimPrefix(opts.Current, "v")
	if current != "dev" && current != "" && CompareVersions(current, rel.Version()) >= 0 {
		return Result{From: current, To: rel.Version()}, ErrUpToDate
	}

	dir := opts.Dir
	if dir == "" {
		if dir, err = installDir(); err != nil {
			return Result{}, err
		}
	}
	if err := writable(dir); err != nil {
		return Result{}, err
	}

	assetName := fmt.Sprintf("pogo_%s_%s_%s.tar.gz", rel.Version(), runtime.GOOS, runtime.GOARCH)
	assetURL := findAsset(rel, assetName, opts.DownloadBase)
	sumsURL := findAsset(rel, "checksums.txt", opts.DownloadBase)
	if assetURL == "" {
		return Result{}, fmt.Errorf("release %s has no build for %s/%s", rel.Tag, runtime.GOOS, runtime.GOARCH)
	}
	if sumsURL == "" {
		return Result{}, errors.New("release has no checksums.txt; refusing to install unverified binaries")
	}

	fmt.Fprintf(out, "downloading %s\n", assetName)
	archive, err := download(ctx, opts.Client, assetURL)
	if err != nil {
		return Result{}, err
	}
	sums, err := download(ctx, opts.Client, sumsURL)
	if err != nil {
		return Result{}, err
	}

	if err := verify(archive, sums, assetName); err != nil {
		return Result{}, err
	}
	fmt.Fprintln(out, "checksum verified")

	extracted, err := extract(archive)
	if err != nil {
		return Result{}, err
	}

	res := Result{From: current, To: rel.Version(), Dir: dir}
	for _, name := range binaries {
		data, ok := extracted[name]
		if !ok {
			continue
		}
		target := filepath.Join(dir, name)
		// Only replace binaries that are actually installed here, so an update
		// never adds a file to a directory the user did not put pogo in.
		if _, err := os.Stat(target); err != nil {
			continue
		}
		if err := replace(target, data); err != nil {
			return res, fmt.Errorf("replace %s: %w", target, err)
		}
		res.Updated = append(res.Updated, name)
	}

	if len(res.Updated) == 0 {
		return res, fmt.Errorf("found no pogo binary to update in %s", dir)
	}
	return res, nil
}

// findAsset resolves an asset URL, preferring what the release advertises.
func findAsset(rel Release, name, downloadBase string) string {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.URL
		}
	}
	if downloadBase != "" {
		return strings.TrimSuffix(downloadBase, "/") + "/" + name
	}
	return ""
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	// 100 MiB is far beyond any plausible release and bounds a hostile server.
	return io.ReadAll(io.LimitReader(resp.Body, 100<<20))
}

// verify checks the archive against the published checksums. A missing entry is
// a failure, not something to shrug at.
func verify(archive, sums []byte, name string) error {
	sum := sha256.Sum256(archive)
	actual := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if fields[0] != actual {
			return fmt.Errorf("checksum mismatch for %s:\n  expected %s\n  actual   %s", name, fields[0], actual)
		}
		return nil
	}
	return fmt.Errorf("no checksum published for %s; refusing to install", name)
}

func extract(archive []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	out := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		// Only the binaries are of interest, and only by exact base name, so a
		// crafted archive cannot write outside the install directory.
		name := filepath.Base(hdr.Name)
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		for _, want := range binaries {
			if name == want {
				data, err := io.ReadAll(io.LimitReader(tr, 200<<20))
				if err != nil {
					return nil, err
				}
				out[name] = data
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("archive contained no pogo binary")
	}
	return out, nil
}

// replace swaps a binary atomically.
//
// The new file is written beside the target and renamed over it, so a running
// process keeps its open file and no one ever observes a half-written
// executable. Unlinking-by-rename is why this works while pogo is running.
func replace(target string, data []byte) error {
	dir := filepath.Dir(target)

	mode := os.FileMode(0o755)
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".pogo-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func installDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate the running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

// writable reports a clear, actionable error when the install directory needs
// elevated permissions, which is the common case for /usr/local/bin.
func writable(dir string) error {
	f, err := os.CreateTemp(dir, ".pogo-write-test-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w\n  try: sudo pogo update", dir, err)
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

// CompareVersions orders two dotted versions, ignoring any pre-release suffix.
// It returns -1, 0 or 1 in the manner of strings.Compare.
func CompareVersions(a, b string) int {
	pa, pb := splitVersion(a), splitVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func splitVersion(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// Drop a pre-release or build suffix: 1.2.3-rc1 compares as 1.2.3.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return out
		}
		out[i] = n
	}
	return out
}
