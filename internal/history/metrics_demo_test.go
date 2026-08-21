package history

import (
	"testing"
	"time"
)

// The numbers below are real curl 8.6 output, so these tests pin pogo's reading
// of curl's timing model rather than a model pogo made up.

func TestPhasesSumToTotalWithoutRedirect(t *testing.T) {
	m := &Metrics{
		TimeNamelookup: 0.000171, TimeConnect: 0.000579, TimePretransfer: 0.000606,
		TimeStarttransfer: 0.001147, TimeTotal: 0.001153,
	}
	var sum time.Duration
	for _, p := range m.Phases() {
		sum += p.Duration
	}
	if d := sum - m.Total(); d > time.Microsecond || d < -time.Microsecond {
		t.Errorf("phases sum to %v, curl reported %v", sum, m.Total())
	}
}

func TestPhasesWithRedirectStayHonest(t *testing.T) {
	m := &Metrics{
		TimeNamelookup: 0.000145, TimeConnect: 0.000618, TimePretransfer: 0.00066,
		TimeStarttransfer: 0.000383, TimeRedirect: 0.000949, TimeTotal: 0.001335,
		NumRedirects: 1,
	}
	phases := m.Phases()
	if len(phases) != 2 || phases[0].Name != "Redirects" || phases[1].Name != "Final transfer" {
		t.Fatalf("redirected request should collapse to two honest spans, got %+v", phases)
	}
	var sum time.Duration
	for _, p := range phases {
		sum += p.Duration
	}
	if sum != m.Total() {
		t.Errorf("phases sum to %v, curl reported %v", sum, m.Total())
	}
}

func TestPhasesSkipTLSWhenPlainHTTP(t *testing.T) {
	m := &Metrics{TimeNamelookup: 0.001, TimeConnect: 0.002, TimePretransfer: 0.002,
		TimeStarttransfer: 0.010, TimeTotal: 0.012}
	for _, p := range m.Phases() {
		if p.Name == "TLS" {
			t.Fatal("TLS phase reported for a request that never negotiated TLS")
		}
	}
}
