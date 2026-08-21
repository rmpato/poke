package curlargs

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestOptionTableMatchesLocalCurl checks the generated arity table against the
// curl actually installed here.
//
// The table decides whether an option consumes the next argument, which is how
// pogo tells a flag's value from a URL. A wrong entry produces a misleading
// history record, so drift is worth catching — but only where the table makes a
// claim. Options this curl knows and the table does not are reported and not
// failed: newer curl releases add options all the time, and the parser already
// treats unknown options safely.
func TestOptionTableMatchesLocalCurl(t *testing.T) {
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl is not installed")
	}

	help, err := exec.Command(curl, "--help", "all").Output()
	if err != nil {
		t.Skipf("curl --help all failed: %v", err)
	}

	optionLine := regexp.MustCompile(`^\s+(-[a-zA-Z0-9#:*], )?(--[a-zA-Z0-9.-]+)`)
	seen := map[string]bool{}
	var unknown []string

	for _, line := range strings.Split(string(help), "\n") {
		m := optionLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimPrefix(m[2], "--")
		if seen[name] {
			continue
		}
		seen[name] = true

		_, inValue := longWithValue[name]
		_, inBool := longBoolean[name]
		if !inValue && !inBool {
			unknown = append(unknown, "--"+name)
			continue
		}

		// Ask curl itself: invoking an option bare makes it say whether the
		// option needs an argument.
		out, _ := exec.Command(curl, "--"+name).CombinedOutput()
		wantsValue := strings.Contains(string(out), "requires parameter")

		// --help takes an optional category; pogo intercepts it either way.
		if name == "help" {
			continue
		}
		if wantsValue != inValue {
			t.Errorf("--%s: curl says takes-value=%v, table says %v — regenerate with scripts/gen-curl-options.sh",
				name, wantsValue, inValue)
		}
	}

	if len(unknown) > 0 {
		t.Logf("this curl knows %d options the table does not (harmless; parser treats them as switches): %s",
			len(unknown), strings.Join(unknown, " "))
	}
	if len(seen) == 0 {
		t.Error("no options were parsed from curl --help all; the scraper regex may be stale")
	}
}
