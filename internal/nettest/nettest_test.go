package nettest

import "testing"

func TestParseTrace(t *testing.T) {
	trace := parseTrace("ip=203.0.113.1\nloc=SG\ncolo=SIN\n")
	if trace.IP != "203.0.113.1" || trace.Country != "SG" || trace.Colo != "SIN" {
		t.Fatalf("unexpected trace: %#v", trace)
	}
}
