package runner

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/rmpato/pogo/internal/curlargs"
)

// Version is a curl major/minor pair. Patch releases have never gated a feature
// pogo depends on, so they are not tracked.
type Version struct {
	Major int
	Minor int
}

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// AtLeast reports whether v is at least min.
func (v Version) AtLeast(min Version) bool {
	if v.Major != min.Major {
		return v.Major > min.Major
	}
	return v.Minor >= min.Minor
}

var (
	versionMu    sync.Mutex
	versionCache = map[string]Version{}
)

// BinaryVersion reports the version of a curl binary, running it at most once
// per path per process.
//
// This costs a few milliseconds and is only paid when pogo actually needs to
// know: when it is about to inject --no-progress-meter because stdout is a
// terminal.
func BinaryVersion(path string) (Version, error) {
	versionMu.Lock()
	if v, ok := versionCache[path]; ok {
		versionMu.Unlock()
		return v, nil
	}
	versionMu.Unlock()

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return Version{}, err
	}
	v, err := ParseVersion(string(out))
	if err != nil {
		return Version{}, err
	}

	versionMu.Lock()
	versionCache[path] = v
	versionMu.Unlock()
	return v, nil
}

// ParseVersion extracts the version from `curl --version` output, whose first
// line looks like "curl 8.6.0 (x86_64-apple-darwin23.0) libcurl/8.6.0 ...".
func ParseVersion(out string) (Version, error) {
	line, _, _ := strings.Cut(out, "\n")
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "curl") {
		return Version{}, fmt.Errorf("unrecognized curl version line: %q", line)
	}
	parts := strings.SplitN(fields[1], ".", 3)
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("unrecognized curl version %q", fields[1])
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, err
	}
	minor, err := strconv.Atoi(strings.TrimRight(parts[1], "-rc"))
	if err != nil {
		return Version{}, err
	}
	return Version{major, minor}, nil
}

// ExitMessage maps the curl exit codes people actually hit onto a short
// explanation, so pogo can show "couldn't resolve host" instead of "exit 6".
func ExitMessage(code int) string {
	switch code {
	case 0:
		return ""
	case 1:
		return "unsupported protocol"
	case 2:
		return "failed to initialize"
	case 3:
		return "malformed URL"
	case 5:
		return "couldn't resolve proxy"
	case 6:
		return "couldn't resolve host"
	case 7:
		return "failed to connect"
	case 22:
		return "HTTP error returned (--fail)"
	case 23:
		return "write error"
	case 26:
		return "read error"
	case 28:
		return "operation timed out"
	case 35:
		return "SSL connect error"
	case 47:
		return "too many redirects"
	case 52:
		return "empty reply from server"
	case 56:
		return "failure receiving data"
	case 60:
		return "certificate verification failed"
	case 77:
		return "problem reading CA cert"
	default:
		return "curl exit " + strconv.Itoa(code)
	}
}

// ErrorText derives a one-line explanation of a failure from curl's output.
//
// It only looks for curl's own "curl: (7) ..." diagnostics rather than keeping
// the last line of stderr, because stderr also carries the progress meter and
// -v traces. Treating those as an error message would put transfer statistics
// in the error column of every successful request.
func ErrorText(stderr []byte, exit int) string {
	const max = 300

	for _, raw := range bytes.Split(bytes.TrimRight(stderr, "\x00"), []byte("\n")) {
		line := strings.TrimSpace(string(raw))
		if strings.HasPrefix(line, "curl:") {
			if len(line) > max {
				return line[:max] + "…"
			}
			return line
		}
	}
	if exit != 0 {
		// curl said nothing quotable (it was silenced with -s, most likely), so
		// fall back to what its exit code means.
		return ExitMessage(exit)
	}
	return ""
}

// hasWriteOut reports whether the user supplied their own --write-out, in which
// case pogo leaves it alone and forgoes timing capture.
func hasWriteOut(spec *curlargs.Spec) bool {
	for _, o := range spec.Options {
		if o.Name == "-w" || o.Name == "--write-out" || strings.HasPrefix(o.Name, "--write-out=") {
			return true
		}
	}
	return false
}
