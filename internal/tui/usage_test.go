package tui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/usage"
)

func TestUnlimitedQuotaCell(t *testing.T) {
	for _, left := range []bool{false, true} {
		got := quotaCell(provider.QuotaWindow{Name: "Chat", Unlimited: true}, left, time.Now())
		if !strings.Contains(got, "Unlimited") || strings.Contains(got, "%") || strings.Contains(got, "░") {
			t.Fatalf("unlimited: %q", got)
		}
	}
}

// The Usage page shows, under the gateway's calls, those the agents made
// on their own, read from their session files, as the app's Usage page
// counts them (Kumo31 on Discord: Codex used outside magpie wasn't in the
// TUI).
func TestUsagePageShowsCallsNotThroughMagpie(t *testing.T) {
	home(t)
	// the usage clock held at noon, so that the calls a minute or two
	// before it are today's whenever the test runs
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
	oldClock := usage.Clock
	usage.Clock = func() time.Time { return now }
	t.Cleanup(func() { usage.Clock = oldClock })
	usage.Append(usage.Record{Time: now.Add(-time.Minute), Agent: "claude", Provider: "relay", Model: "m", Input: 5, Status: 200})
	old := usage.LogCalls
	usage.LogCalls = func(time.Time) []sessions.Call {
		return []sessions.Call{{Time: now.Add(-2 * time.Minute), Agent: "codex", Session: "c1", Model: "gpt-6-luna", Tokens: sessions.Tokens{Input: 1200, Output: 300}}}
	}
	t.Cleanup(func() { usage.LogCalls = old })
	m := press(t, model{w: 160, h: 60, page: pageUsage}, "t")
	v := m.viewUsage()
	gw, own, ok := strings.Cut(v, "not through magpie")
	if !ok || !strings.Contains(gw, "relay/m") || strings.Contains(gw, "gpt-6-luna") || !strings.Contains(own, "gpt-6-luna") {
		t.Fatal(v)
	}
	if strings.Contains(own, usage.UnknownProvider) {
		t.Fatalf("the unknown provider's id is shown:\n%s", own)
	}
}

// The Usage page asked for as midnight falls shows the calls through magpie
// and those not through it of one day, the day it was asked on, when a
// period is picked and when the page is read again.
func TestUsagePageAtMidnight(t *testing.T) {
	home(t)
	midnight := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	usage.Append(usage.Record{Time: midnight.Add(-time.Minute), Agent: "claude", Provider: "relay", Model: "m", Input: 5, Status: 200})
	old := usage.LogCalls
	usage.LogCalls = func(time.Time) []sessions.Call {
		return []sessions.Call{{Time: midnight.Add(-2 * time.Minute), Agent: "codex", Session: "c1", Model: "gpt-6-luna", Tokens: sessions.Tokens{Input: 1200, Output: 300}}}
	}
	t.Cleanup(func() { usage.LogCalls = old })
	// the usage clock reads a tenth of a second before midnight the first
	// time and a tenth of a second after it from then on, as when midnight
	// falls while the page is worked out
	var read atomic.Bool
	oldClock := usage.Clock
	usage.Clock = func() time.Time {
		if read.Swap(true) {
			return midnight.Add(100 * time.Millisecond)
		}
		return midnight.Add(-100 * time.Millisecond)
	}
	t.Cleanup(func() { usage.Clock = oldClock })
	both := func(how string, m model) {
		t.Helper()
		v := m.viewUsage()
		gw, own, ok := strings.Cut(v, "not through magpie")
		if !ok || !strings.Contains(gw, "relay/m") || !strings.Contains(own, "gpt-6-luna") {
			t.Errorf("%s:\n%s", how, v)
		}
	}
	m := press(t, model{w: 160, h: 60, page: pageUsage}, "t")
	both("today picked", m)
	read.Store(false)
	m.reload()
	both("read again", m)
}

// The Usage page's headline says how many calls its cost leaves out for
// having no known price (#1856), not only a "+" after the sum.
func TestUsagePageSaysCallsUnpriced(t *testing.T) {
	home(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
	oldClock := usage.Clock
	usage.Clock = func() time.Time { return now }
	t.Cleanup(func() { usage.Clock = oldClock })
	old := usage.LogCalls
	usage.LogCalls = func(time.Time) []sessions.Call { return nil }
	t.Cleanup(func() { usage.LogCalls = old })
	for i := range 3 {
		usage.Append(usage.Record{Time: now.Add(-time.Duration(i+1) * time.Minute), Agent: "claude", Provider: "relay", Model: "kimi-k3-auto", Input: 500, Output: 50, Status: 200})
	}
	m := press(t, model{w: 160, h: 60, page: pageUsage}, "t")
	if v := m.viewUsage(); !strings.Contains(v, "(3 calls unpriced)") {
		t.Fatal(v)
	}
}
