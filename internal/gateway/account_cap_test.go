package gateway

import (
	"testing"
	"time"

	"github.com/fogway/fogway/internal/provider"
)

// capUsage stands in for the accounts' windows as last read: the five
// hours of each, by user, at the share given.
func capUsage(t *testing.T, used map[string]float64) {
	t.Helper()
	old := allowances
	t.Cleanup(func() { allowances = old })
	reset := time.Now().Add(3 * time.Hour)
	allowances = func(string) map[string]provider.Allowance {
		out := map[string]provider.Allowance{}
		for u, n := range used {
			out[u] = provider.Allowance{
				{Used: n, Resets: reset, Span: 5 * time.Hour},
				{Used: 10, Resets: time.Now().Add(4 * 24 * time.Hour), Span: 7 * 24 * time.Hour},
			}
		}
		return out
	}
}

// windowUsage stands in for the accounts' windows as last read, named as
// the vendor's reading names them: each user's five hours and week.
func windowUsage(t *testing.T, used map[string][2]float64) {
	t.Helper()
	old := allowances
	t.Cleanup(func() { allowances = old })
	five, week := time.Now().Add(3*time.Hour), time.Now().Add(4*24*time.Hour)
	allowances = func(string) map[string]provider.Allowance {
		out := map[string]provider.Allowance{}
		for u, n := range used {
			out[u] = provider.Allowance{
				{Name: "5 hours", Used: n[0], Resets: five, Span: 5 * time.Hour},
				{Name: "Weekly", Used: n[1], Resets: week, Span: 7 * 24 * time.Hour},
			}
		}
		return out
	}
}
