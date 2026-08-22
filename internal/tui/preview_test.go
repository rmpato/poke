package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rmpato/pogo/internal/curlargs"
	"github.com/rmpato/pogo/internal/history"
)

// previewEntries are two calls to one endpoint and one failure, which is
// enough to exercise every insight the panel can draw.
func previewEntries() []*history.Entry {
	return []*history.Entry{
		testEntry("GET", "https://api.staging.acme.com/orders/9021", 200, func(e *history.Entry) {
			e.CreatedAt = time.Now().Add(-1 * time.Minute)
			e.Duration = history.Duration(44 * time.Millisecond)
			e.Command.Args = []string{"-H", "Authorization: Bearer sk-live-4f9c", "{{base}}/orders/9021"}
			e.Request.Headers = []curlargs.Header{{Name: "Authorization", Value: "Bearer sk-live-4f9c"}}
			e.Response.Body = &history.BodyRef{Size: 42, Inline: `{"id":9021,"status":"pending"}`}
		}),
		testEntry("GET", "https://api.staging.acme.com/orders/9021", 200, func(e *history.Entry) {
			e.CreatedAt = time.Now().Add(-3 * time.Minute)
			e.Duration = history.Duration(61 * time.Millisecond)
		}),
		testEntry("GET", "https://api.acme.com/billing/invoices", 403, func(e *history.Entry) {
			e.CreatedAt = time.Now().Add(-5 * time.Minute)
			e.Response.Body = &history.BodyRef{
				Size: 60, Inline: `{"error":"forbidden","message":"missing scope: billing.read"}`}
		}),
	}
}

func previewModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t, previewEntries()...)
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	return m
}

// The panel answers "is this the one?" — so it has to carry where the request
// went, what came back, and what is worth knowing before opening it.
func TestPreviewShowsTheRequestAtAGlance(t *testing.T) {
	m := previewModel(t)
	m.detail.entryID = m.selectedID()
	m.detail.loaded = true
	m.detail.resBody = []byte(`{"id":9021,"status":"pending"}`)

	view := m.View()
	for _, want := range []string{
		"api.staging.acme.com", // where it went
		"/orders/9021",         // what it asked for
		"acme.com", "staging",  // which API, which environment
		"44ms",          // how long it took
		"Authorization", // what it carried
		`"status"`,      // and what came back
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the preview does not mention %q", want)
		}
	}
}

// A masked secret must stay masked in the preview: it is on screen for every
// row you move the cursor past, which is more exposure than the inspector.
func TestPreviewMasksSecrets(t *testing.T) {
	m := previewModel(t)
	if view := m.View(); strings.Contains(view, "sk-live-4f9c") {
		t.Error("the preview revealed a bearer token")
	}
}

// The failure insight is the one that changes what you do next, so it comes
// from the API's own words when it gave any.
func TestPreviewExplainsAFailure(t *testing.T) {
	m := previewModel(t)
	m.move(2) // the 403
	m.detail.entryID = m.selectedID()
	m.detail.loaded = true
	m.detail.resBody = []byte(`{"error":"forbidden","message":"missing scope: billing.read"}`)

	if view := m.View(); !strings.Contains(view, "missing scope: billing.read") {
		t.Error("the preview should surface why the request failed")
	}
}

// Two calls to the same endpoint are a history; the panel says so, and offers
// the comparison.
func TestPreviewFindsSiblings(t *testing.T) {
	m := previewModel(t)

	if got := len(m.siblings(m.selected())); got != 2 {
		t.Errorf("found %d calls to this endpoint, want 2", got)
	}
	if prev := m.previousSibling(m.selected()); prev == nil {
		t.Fatal("the earlier call to this endpoint was not found")
	}
	if view := m.View(); !strings.Contains(view, "2 calls") {
		t.Error("the preview should say how often this endpoint has been called")
	}
}

// p turns it off, and the list gets the columns back.
func TestPreviewToggles(t *testing.T) {
	m := previewModel(t)
	before := m.listWidth()
	if m.previewWidth() == 0 {
		t.Fatal("the preview should be on at this width")
	}

	press(m, "p")
	if m.previewWidth() != 0 {
		t.Error("p should hide the preview")
	}
	if m.listWidth() <= before {
		t.Error("hiding the preview should give its columns to the list")
	}

	press(m, "p")
	if m.previewWidth() == 0 {
		t.Error("p should bring it back")
	}
}
