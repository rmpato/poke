package runner

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/history"
)

// ParseHeaderDump reads the file produced by curl's -D/--dump-header.
//
// The dump contains one block per response head, so a redirect chain arrives as
// several blocks in order, as do 1xx interim responses. All of them are kept:
// "302 then 200" is a materially different story from "200", and collapsing it
// would hide the redirect that people are usually trying to debug.
func ParseHeaderDump(data []byte) []history.Block {
	if len(data) == 0 {
		return nil
	}

	var (
		blocks  []history.Block
		current *history.Block
	)

	for _, raw := range bytes.Split(data, []byte("\n")) {
		line := strings.TrimRight(string(raw), "\r")

		if line == "" {
			current = nil // blank line closes the current head
			continue
		}

		if isStatusLine(line) {
			blocks = append(blocks, parseStatusLine(line))
			current = &blocks[len(blocks)-1]
			continue
		}

		if current == nil {
			// Headers without a status line: non-HTTP protocols such as file://
			// dump metadata this way. Give them a home rather than dropping it.
			blocks = append(blocks, history.Block{})
			current = &blocks[len(blocks)-1]
		}
		if name, value, ok := strings.Cut(line, ":"); ok {
			current.Headers = append(current.Headers, curlargs.Header{
				Name:  strings.TrimSpace(name),
				Value: strings.TrimSpace(value),
			})
		}
	}
	return blocks
}

func isStatusLine(line string) bool {
	return strings.HasPrefix(line, "HTTP/")
}

// parseStatusLine handles both "HTTP/1.1 200 OK" and HTTP/2's reasonless
// "HTTP/2 200".
func parseStatusLine(line string) history.Block {
	b := history.Block{}
	proto, rest, ok := strings.Cut(line, " ")
	b.Proto = proto
	if !ok {
		return b
	}
	code, reason, _ := strings.Cut(strings.TrimSpace(rest), " ")
	if n, err := strconv.Atoi(code); err == nil {
		b.Status = n
	}
	b.Reason = strings.TrimSpace(reason)
	return b
}
