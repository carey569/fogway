package provider

import (
	"testing"
	"time"
)

// What stops an account, for fogway quota wait and the Codex resets, is a
// full window still to reset: one whose reset has passed doesn't stop it,
// nor say when it is back.
func TestUsedUpLeavesOutARenewedWindow(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	renewed := SubscriptionQuota{Provider: "codex", Windows: []QuotaWindow{{Name: "5 hours", Used: 100, ResetsAt: &past}}}
	if UsedUp(renewed, now) || !BackAt(renewed, now).IsZero() {
		t.Fatalf("a window reset a minute ago stops the account: back %v", BackAt(renewed, now))
	}
	full := SubscriptionQuota{Provider: "codex", Windows: []QuotaWindow{
		{Name: "5 hours", Used: 100, ResetsAt: &past}, {Name: "7 days", Used: 100, ResetsAt: &future},
	}}
	if !UsedUp(full, now) || !BackAt(full, now).Equal(future) {
		t.Fatalf("a full week doesn't stop it: back %v", BackAt(full, now))
	}
	// read as it was, before it reset, it was used up (quota_last's holds)
	if !usedUp(renewed, time.Time{}) {
		t.Fatal("the reading as read isn't used up")
	}
}
