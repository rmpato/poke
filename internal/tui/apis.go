package tui

import (
	"github.com/rmpato/poke/internal/apis"
	"github.com/rmpato/poke/internal/config"
	"github.com/rmpato/poke/internal/history"
)

// The list is organised by API, and an API is a thing pogo works out rather
// than something the user files by hand. Classification happens live, off the
// URL, so that correcting a grouping applies to everything already recorded —
// pin a host to "staging" and the six weeks of history that used it move too.
//
// Live classification cannot work for a request whose host was a {{variable}},
// because the stored command still says {{base}}. Those fall back to the API
// they ran under, which capture wrote down at the time for exactly this reason.

// apiRef classifies one entry, memoised by URL: the same handful of URLs
// repeat down the whole list, and a public-suffix lookup per row per frame is
// work nobody asked for.
func (m *Model) apiRef(e *history.Entry) apis.Ref {
	if e == nil {
		return apis.Ref{}
	}
	if ref, ok := m.refCache[e.Request.URL]; ok {
		return m.withStored(ref, e)
	}
	ref := apis.Classify(e.Request.URL, m.cfg.APIs)
	if m.refCache == nil {
		m.refCache = map[string]apis.Ref{}
	}
	m.refCache[e.Request.URL] = ref
	return m.withStored(ref, e)
}

// withStored fills in what a templated URL could not say.
func (m *Model) withStored(ref apis.Ref, e *history.Entry) apis.Ref {
	if ref.Domain == "" && e.API != "" {
		ref.Domain = e.API
		ref.Name = m.cfg.APIs.Name(e.API)
		if ref.Env == "" {
			ref.Env = e.Env
		}
	}
	return ref
}

// forgetAPIs drops the classification cache. Anything that changes the
// registry has to call this, or a correction would not show until restart.
func (m *Model) forgetAPIs() { m.refCache = nil }

// domainOf is the API key an entry belongs to.
func (m *Model) domainOf(e *history.Entry) string { return m.apiRef(e).Domain }

// apiLabel is what an API is called on screen: its name if it has been given
// one, its domain otherwise.
func (m *Model) apiLabel(e *history.Entry) string {
	ref := m.apiRef(e)
	if ref.Name != "" {
		return ref.Name
	}
	return ref.Domain
}

// envOf is the environment an entry reached.
func (m *Model) envOf(e *history.Entry) string { return m.apiRef(e).Env }

// apiSummary folds the loaded history into the API list the sidebar draws.
func (m *Model) apiSummary() []apis.API {
	refs := make([]apis.Ref, 0, len(m.entries))
	for _, e := range m.entries {
		refs = append(refs, m.apiRef(e))
	}
	return apis.Summarize(refs, m.cfg.APIs)
}

// groupLabel is the header a row is filed under for the current grouping.
func (m *Model) groupLabel(e *history.Entry) string {
	switch m.group {
	case groupAPI:
		label := m.apiLabel(e)
		if label == "" {
			return "unknown"
		}
		if env := m.envOf(e); env != "" {
			return label + " · " + env
		}
		return label
	case groupHost:
		if h := m.displayHost(e); h != "" {
			return h
		}
		return "unknown"
	case groupCollection:
		if e.Collection != "" {
			return e.Collection
		}
		return "unfiled"
	}
	return ""
}

// setAPIOverride records a correction and makes it visible immediately.
// Writing on the same keypress is the whis rule (§11): there is no "unsaved
// grouping" state, because there is no such state to get wrong.
func (m *Model) setAPIOverride(mutate func(*config.Config)) error {
	if err := m.cfgStore.Update(mutate); err != nil {
		return err
	}
	m.cfg = m.cfgStore.Current()
	m.forgetAPIs()
	m.rebuildRows()
	m.buildRail()
	return nil
}
