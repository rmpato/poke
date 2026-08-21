package runner

import (
	"bytes"
	"io"
)

// binaryWarning reproduces, verbatim, what curl prints when it declines to
// write binary data to a terminal. Matching the wording matters: people grep
// their scrollback for it, and a pogo-flavored paraphrase would be a lie about
// which tool made the decision.
const binaryWarning = `Warning: Binary output can mess up your terminal. Use "--output -" to tell 
Warning: curl to output it to your terminal anyway, or consider "--output 
Warning: <FILE>" to save to a file.
`

// teeWriter forwards everything it receives while keeping a bounded copy.
//
// Forwarding is unbounded and always happens first: the user's output is the
// product, and capture is the side effect. A write error to the capture buffer
// can never be allowed to disturb the stream.
type teeWriter struct {
	dst   io.Writer
	buf   bytes.Buffer
	limit int64
	total int64

	truncated bool

	// guardTTY enables curl's binary-output-to-terminal check, which curl
	// itself cannot perform once pogo has put a pipe in the way.
	guardTTY     bool
	guardTripped bool
	warn         io.Writer
}

func (w *teeWriter) Write(p []byte) (int, error) {
	if w.guardTTY && !w.guardTripped && bytes.IndexByte(p, 0) >= 0 {
		w.guardTripped = true
		if w.warn != nil {
			io.WriteString(w.warn, binaryWarning)
		}
	}

	if w.dst != nil && !w.guardTripped {
		if _, err := w.dst.Write(p); err != nil {
			return 0, err
		}
	}

	w.total += int64(len(p))
	if room := w.limit - int64(w.buf.Len()); room > 0 {
		if int64(len(p)) > room {
			w.buf.Write(p[:room])
			w.truncated = true
		} else {
			w.buf.Write(p)
		}
	} else if w.limit > 0 {
		w.truncated = true
	}

	// Always report a full write: the bytes did reach their destination, and
	// reporting a short write would make curl think the transfer failed.
	return len(p), nil
}

func (w *teeWriter) captured() []byte {
	if w.buf.Len() == 0 {
		return nil
	}
	return w.buf.Bytes()
}

// teeReader captures a bounded copy of a request payload arriving on stdin
// while passing every byte through to curl.
type teeReader struct {
	src   io.Reader
	buf   bytes.Buffer
	limit int64
	total int64
}

func (r *teeReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.total += int64(n)
		if room := r.limit - int64(r.buf.Len()); room > 0 {
			if int64(n) > room {
				r.buf.Write(p[:room])
			} else {
				r.buf.Write(p[:n])
			}
		}
	}
	return n, err
}

func (r *teeReader) captured() []byte {
	if r.buf.Len() == 0 {
		return nil
	}
	return r.buf.Bytes()
}

// sanitizeStderr collapses carriage-return progress redraws into their final
// state. When pogo is not attached to a terminal curl keeps its progress meter,
// and storing dozens of overwritten redraws would bury the actual error.
func sanitizeStderr(b []byte) []byte {
	if len(b) == 0 || bytes.IndexByte(b, '\r') < 0 {
		return b
	}
	lines := bytes.Split(b, []byte("\n"))
	for i, line := range lines {
		if j := bytes.LastIndexByte(line, '\r'); j >= 0 {
			lines[i] = line[j+1:]
		}
	}
	out := bytes.Join(lines, []byte("\n"))
	return bytes.TrimRight(out, "\n\r \t")
}
