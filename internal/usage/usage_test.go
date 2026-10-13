package usage

import (
	"testing"
	"time"
)

// FormatCost stays in dollars unless cny is asked for and a usable rate is
// given; it keeps the same 0/2/3-decimal rule either currency, and a rate
// that's missing or non-positive falls back to USD rather than hiding the
// number or dividing by zero.
func TestFormatCost(t *testing.T) {
	for _, c := range []struct {
		amount   float64
		currency string
		rate     float64
		want     string
	}{
		{0.019, "usd", 0, "$0.019"},
		{1.23, "usd", 0, "$1.23"},
		{123, "usd", 0, "$123"},
		{0.019, "cny", 7.2, "¥0.137"},
		{1.23, "cny", 7.2, "¥8.86"},
		{123, "cny", 7.2, "¥886"},
		{1.23, "cny", 0, "$1.23"},   // no rate: stays USD
		{1.23, "cny", -1, "$1.23"},  // a bad rate: stays USD
		{1.23, "eur", 7.2, "$1.23"}, // any other currency: USD
	} {
		if got := FormatCost(c.amount, c.currency, c.rate); got != c.want {
			t.Errorf("FormatCost(%v, %q, %v) = %q, want %q", c.amount, c.currency, c.rate, got, c.want)
		}
	}
}

// A request Saw note is LastSeen's answer at once, before any record of it
// is written.
func TestSawIsLastSeen(t *testing.T) {
	const agent = "saw-is-last-seen"
	t.Cleanup(func() { seen.Delete(agent) }) // the record lasts the process
	if at := LastSeen(agent); !at.IsZero() {
		t.Fatalf("before any request: %v", at)
	}
	before := time.Now()
	Saw(agent)
	if at := LastSeen(agent); at.Before(before) || time.Since(at) > time.Minute {
		t.Fatalf("after a request: %v", at)
	}
}
