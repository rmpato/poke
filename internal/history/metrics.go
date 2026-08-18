package history

import "time"

// Metrics is curl's own accounting of a transfer, captured with --write-out.
//
// Field names mirror curl's %{json} keys so that what poke stores can be
// checked against `curl -w '%{json}'` directly. Nothing here is computed by
// poke: if curl did not report a number, it is absent rather than estimated.
type Metrics struct {
	// Times are seconds since the start of the whole operation, which is how
	// curl reports them, so phase durations come from subtraction.
	TimeNamelookup    float64 `json:"time_namelookup"`
	TimeConnect       float64 `json:"time_connect"`
	TimeAppconnect    float64 `json:"time_appconnect"`
	TimePretransfer   float64 `json:"time_pretransfer"`
	TimeStarttransfer float64 `json:"time_starttransfer"`
	TimeRedirect      float64 `json:"time_redirect"`
	TimeTotal         float64 `json:"time_total"`

	SizeDownload  float64 `json:"size_download"`
	SizeUpload    float64 `json:"size_upload"`
	SizeHeader    float64 `json:"size_header"`
	SizeRequest   float64 `json:"size_request"`
	SpeedDownload float64 `json:"speed_download"`

	HTTPVersion  string `json:"http_version"`
	HTTPCode     int    `json:"http_code"`
	Scheme       string `json:"scheme"`
	RemoteIP     string `json:"remote_ip"`
	RemotePort   int    `json:"remote_port"`
	LocalIP      string `json:"local_ip"`
	NumConnects  int    `json:"num_connects"`
	NumRedirects int    `json:"num_redirects"`
	EffectiveURL string `json:"url_effective"`
	ContentType  string `json:"content_type"`
	ErrorMsg     string `json:"errormsg"`
	ExitCode     int    `json:"exitcode"`
}

// Phase is one span of a request's lifetime.
type Phase struct {
	Name     string
	Duration time.Duration
}

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

// Phases breaks the transfer into the spans a developer reasons about.
//
// For a request without redirects curl's counters are all measured from the
// start of the transfer, so consecutive differences give an exact breakdown
// that sums to time_total.
//
// Redirects are reported honestly rather than precisely. Verified against curl
// 8.6, a redirected request produces counters that belong to different hops --
// time_pretransfer can exceed time_starttransfer, because the former describes
// the connection curl set up on the first hop and the latter the final
// response. Splitting that into DNS/Connect/TLS spans would mean inventing a
// story curl did not tell, so the breakdown collapses to the two spans that are
// unambiguous: the redirect chain, and the final transfer.
func (m *Metrics) Phases() []Phase {
	if m == nil || m.TimeTotal == 0 {
		return nil
	}

	if m.NumRedirects > 0 || m.TimeRedirect > 0 {
		return []Phase{
			{"Redirects", secs(m.TimeRedirect)},
			{"Final transfer", secs(clamp(m.TimeTotal - m.TimeRedirect))},
		}
	}

	// Boundaries in order; each phase runs from the previous boundary to its
	// own. TLS is skipped entirely on a plain HTTP request.
	type boundary struct {
		name string
		at   float64
		skip bool
	}
	// A request that never got a response (connection refused, DNS failure)
	// has no transfer phases at all. Reporting a "Download" span for the
	// microseconds curl spent giving up would be an invented number.
	noTransfer := m.TimeStarttransfer == 0

	bounds := []boundary{
		{"DNS", m.TimeNamelookup, false},
		{"Connect", m.TimeConnect, false},
		{"TLS", m.TimeAppconnect, m.TimeAppconnect == 0},
		{"Setup", m.TimePretransfer, false},
		{"Wait (TTFB)", m.TimeStarttransfer, noTransfer},
		{"Download", m.TimeTotal, noTransfer},
	}

	var (
		phases []Phase
		prev   float64
	)
	for _, b := range bounds {
		if b.skip {
			continue
		}
		d := clamp(b.at - prev)
		prev = b.at
		// Sub-microsecond spans are noise on a local socket and clutter on a
		// remote one; the total below still accounts for them.
		if d == 0 && b.name != "Download" {
			continue
		}
		phases = append(phases, Phase{b.name, secs(d)})
	}
	return phases
}

func clamp(f float64) float64 {
	if f < 0 {
		return 0
	}
	return f
}

// Total is the transfer time curl reported, which is authoritative even when
// the individual phases do not sum to it exactly.
func (m *Metrics) Total() time.Duration {
	if m == nil {
		return 0
	}
	return secs(m.TimeTotal)
}

// HasTiming reports whether curl supplied a usable timing breakdown.
func (m *Metrics) HasTiming() bool { return m != nil && m.TimeTotal > 0 }
