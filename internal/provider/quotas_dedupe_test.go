package provider

import (
	"strings"
	"testing"
	"time"
)

// fogway quota, its --json and the gateway show a pool's own windows in
// place of the models' drawing on it (the shapes TestAntigravityQuotaPools
// records): one span per pool window, the pool in its name.
func TestQuotaReportPoolsAntigravity(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	reset := now.Add(48 * time.Hour)
	five, week := 5*time.Hour, 7*24*time.Hour
	antigravity := SubscriptionQuota{Provider: "antigravity", Name: "Antigravity", User: "u@x.com",
		Windows: []QuotaWindow{
			{Name: "Gemini 3 Flash", Model: "gemini-3-flash", Family: "Gemini", Pool: "Gemini", Used: 85, ResetsAt: &reset},
			{Name: "Gemini 3.1 Pro (High)", Model: "gemini-3.1-pro-high", Family: "Gemini", Pool: "Gemini", Used: 85, ResetsAt: &reset},
			{Name: "Claude Opus 4.6 (Thinking)", Model: "claude-opus-4-6-thinking", Family: "Claude", Pool: "Claude & GPT", Used: 90},
			{Name: "GPT-OSS 120B (Medium)", Model: "gpt-oss-120b-medium", Family: "GPT-OSS", Pool: "Claude & GPT", Used: 90},
			{Name: "7 days", Pool: "Gemini", Span: week, Aside: true, Used: 85, ResetsAt: &reset},
			{Name: "5 hours", Pool: "Gemini", Span: five, Aside: true, Used: 95, ResetsAt: &reset},
			{Name: "7 days", Pool: "Claude & GPT", Span: week, Aside: true, Used: 90, ResetsAt: &reset},
			{Name: "5 hours", Pool: "Claude & GPT", Span: five, Aside: true, Used: 90, ResetsAt: &reset},
		}}
	got := quotaReport([]SubscriptionQuota{antigravity}, nil, nil, now)
	var names []string
	for _, w := range got[0].Windows {
		names = append(names, w.Name)
	}
	if strings.Join(names, ",") != "Gemini · 7 days,Gemini · 5 hours,Claude & GPT · 7 days,Claude & GPT · 5 hours" {
		t.Fatalf("windows %v", names)
	}
	// the pool's own use, label and reset reach the span
	w := got[0].Windows[0]
	if w.Used != 85 || w.Pool != "Gemini" || w.Remaining != 15 || w.ResetsAt == nil || !w.ResetsAt.Equal(reset) {
		t.Fatalf("span %+v", w)
	}
}
