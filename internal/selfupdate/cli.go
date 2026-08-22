package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// CLI runs an update on behalf of a command and prints a human report.
// It returns the process exit code.
func CLI(name, current string, out, errOut io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if current == "dev" {
		fmt.Fprintf(errOut, "%s: this is a development build; updating will replace it with the latest release\n", name)
	}

	res, err := Run(ctx, Options{Current: current}, out)
	switch {
	case errors.Is(err, ErrUpToDate):
		fmt.Fprintf(out, "%s %s is already the latest release\n", name, res.From)
		return 0
	case err != nil:
		fmt.Fprintf(errOut, "%s: %v\n", name, err)
		return 1
	}

	for _, bin := range res.Updated {
		fmt.Fprintf(out, "updated %s → %s (%s)\n", bin, res.To, res.Dir)
	}
	return 0
}

// CheckCLI reports whether a newer release exists, without installing it.
func CheckCLI(name, current string, out, errOut io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rel, err := Latest(ctx, Options{Current: current})
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", name, err)
		return 1
	}

	cur := current
	if cur == "dev" || CompareVersions(cur, rel.Version()) < 0 {
		fmt.Fprintf(out, "%s %s is available (you have %s)\nrun: %s update\n",
			name, rel.Version(), current, name)
		return 0
	}
	fmt.Fprintf(out, "%s %s is the latest release\n", name, current)
	return 0
}
