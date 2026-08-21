package runner

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/rmpato/poke/internal/history"
)

// ParseMetrics reads the JSON documents curl's --write-out appended.
//
// One document is written per transfer, so a command with several URLs (or
// --next) produces several. The last one describes the transfer whose body pogo
// captured, which is the one the rest of the entry is about.
func ParseMetrics(data []byte) *history.Metrics {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))

	var last *history.Metrics
	for {
		var m history.Metrics
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			// A truncated final document means curl died mid-write; keep
			// whatever complete documents came before it.
			break
		}
		cp := m
		last = &cp
	}
	return last
}
