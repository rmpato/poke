// Package harimport turns a browser's HAR export into pogo history.
//
// This is the shortest path from "the request works in the browser but not in
// my terminal" to a curl command you can edit and replay: open devtools, save
// all as HAR, import, and every request is in pogo with its headers and body,
// ready to diff against the one you are trying to fix.
//
// Imported entries are marked so they are never mistaken for something pogo
// ran, and they carry no timing breakdown, because pogo did not measure one.
package harimport

import (
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/rmpato/poke/internal/curlargs"
	"github.com/rmpato/poke/internal/history"
)

// har is the subset of the HAR 1.2 format pogo reads.
type har struct {
	Log struct {
		Entries []harEntry `json:"entries"`
	} `json:"log"`
}

type harEntry struct {
	StartedDateTime time.Time `json:"startedDateTime"`
	Time            float64   `json:"time"` // milliseconds
	Request         struct {
		Method      string      `json:"method"`
		URL         string      `json:"url"`
		HTTPVersion string      `json:"httpVersion"`
		Headers     []harHeader `json:"headers"`
		PostData    struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"postData"`
	} `json:"request"`
	Response struct {
		Status      int         `json:"status"`
		StatusText  string      `json:"statusText"`
		HTTPVersion string      `json:"httpVersion"`
		Headers     []harHeader `json:"headers"`
		Content     struct {
			Size     int64  `json:"size"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"content"`
	} `json:"response"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Options tune an import.
type Options struct {
	// Collection files every imported request under one name, so an import can
	// be found — and deleted — as a unit.
	Collection string

	// SkipHeaders drops header names the browser adds that would be noise or
	// nonsense in a curl command.
	SkipHeaders []string
}

// DefaultSkipHeaders are pseudo-headers and hop-by-hop fields that curl sets
// itself. Replaying them verbatim would at best be redundant and at worst make
// curl send a malformed request.
var DefaultSkipHeaders = []string{
	":authority", ":method", ":path", ":scheme",
	"host", "content-length", "connection", "keep-alive",
	"transfer-encoding", "upgrade", "proxy-connection",
}

// Result reports what an import produced.
type Result struct {
	Entries []*history.Entry
	Skipped int // entries the file described but pogo could not use
}

// Parse reads a HAR file and builds history entries. Bodies are returned on the
// entries' Inline field so the caller can decide how to store them.
func Parse(r io.Reader, opts Options) (Result, error) {
	var doc har
	dec := json.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return Result{}, fmt.Errorf("read HAR: %w", err)
	}
	if len(doc.Log.Entries) == 0 {
		return Result{}, fmt.Errorf("no entries in HAR file")
	}

	skip := skipSet(opts.SkipHeaders)
	var res Result

	for _, he := range doc.Log.Entries {
		if he.Request.URL == "" {
			res.Skipped++
			continue
		}
		res.Entries = append(res.Entries, convert(he, skip, opts.Collection))
	}
	if len(res.Entries) == 0 {
		return res, fmt.Errorf("no usable entries in HAR file")
	}
	return res, nil
}

func skipSet(extra []string) map[string]bool {
	out := map[string]bool{}
	for _, h := range DefaultSkipHeaders {
		out[h] = true
	}
	for _, h := range extra {
		out[strings.ToLower(strings.TrimSpace(h))] = true
	}
	return out
}

func convert(he harEntry, skip map[string]bool, collection string) *history.Entry {
	method := strings.ToUpper(he.Request.Method)
	if method == "" {
		method = "GET"
	}

	var headers []curlargs.Header
	for _, h := range he.Request.Headers {
		if skip[strings.ToLower(h.Name)] {
			continue
		}
		// HTTP/2 puts every header in lowercase. Restoring the conventional
		// casing costs nothing and makes an imported command look like one a
		// person wrote, which matters when the whole point is to hand it to
		// someone in a bug report.
		headers = append(headers, curlargs.Header{
			Name:  textproto.CanonicalMIMEHeaderKey(h.Name),
			Value: h.Value,
		})
	}
	// A browser's header order is an implementation detail; sorting makes two
	// imports of the same request comparable.
	sort.SliceStable(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })

	body := he.Request.PostData.Text

	created := he.StartedDateTime
	if created.IsZero() {
		created = time.Now().UTC()
	}

	e := &history.Entry{
		ID:        history.NewID(),
		CreatedAt: created.UTC(),
		Source:    history.SourceImport,
		Command:   history.Command{Args: buildArgs(method, he.Request.URL, headers, body)},
		Request: history.Request{
			Method:  method,
			URL:     he.Request.URL,
			Headers: headers,
		},
		Duration:   history.Duration(time.Duration(he.Time * float64(time.Millisecond))),
		Collection: collection,
	}

	if body != "" {
		e.Request.Body = &history.BodyRef{
			Size: int64(len(body)), Stored: int64(len(body)),
			Inline: body, Origin: "har",
		}
	}

	if he.Response.Status > 0 {
		var respHeaders []curlargs.Header
		for _, h := range he.Response.Headers {
			respHeaders = append(respHeaders, curlargs.Header{
				Name:  textproto.CanonicalMIMEHeaderKey(h.Name),
				Value: h.Value,
			})
		}
		resp := &history.Response{
			Blocks: []history.Block{{
				Proto:   he.Response.HTTPVersion,
				Status:  he.Response.Status,
				Reason:  he.Response.StatusText,
				Headers: respHeaders,
			}},
			ContentType: he.Response.Content.MimeType,
		}
		// base64 content is left out rather than decoded blindly: it is usually
		// an image, and pogo does not render binary payloads anyway.
		if text := he.Response.Content.Text; text != "" && he.Response.Content.Encoding != "base64" {
			resp.Body = &history.BodyRef{
				Size: int64(len(text)), Stored: int64(len(text)),
				Inline: text, Origin: "har",
			}
		}
		e.Response = resp
	}

	return e
}

// buildArgs renders the curl command that would reproduce the request, which is
// what makes an imported entry replayable and editable like any other.
func buildArgs(method, url string, headers []curlargs.Header, body string) []string {
	var args []string
	if method != "GET" {
		args = append(args, "-X", method)
	}
	for _, h := range headers {
		args = append(args, "-H", h.Name+": "+h.Value)
	}
	if body != "" {
		args = append(args, "--data-raw", body)
	}
	return append(args, url)
}
