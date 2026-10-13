package usage

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// accountRecs are a month of two Codex accounts' calls, one Claude account's
// under the same email, an older Codex call that named its account only in
// its host, one that named none, and a keyed provider's, which has none.
func accountRecs(now time.Time) []Record {
	return []Record{
		{Time: now, Provider: "cx", Model: "gpt-5", Host: "chatgpt.com as a@example.com", ProviderAccount: "a@example.com", Input: 100, Output: 10, CacheRead: 1000, Status: 200},
		{Time: now, Provider: "cx", Model: "gpt-5", Host: "chatgpt.com as a@example.com", ProviderAccount: "a@example.com", Input: 50, Output: 5, Status: 200},
		{Time: now, Provider: "cx", Model: "gpt-5", Host: "chatgpt.com as b@example.com", ProviderAccount: "b@example.com", Input: 20, Output: 2, Status: 429},
		// written before the field: its host says the account it went out as
		{Time: now.Add(-time.Hour), Provider: "cx", Model: "gpt-5", Host: "chatgpt.com as b@example.com", Input: 30, Output: 3, Status: 200},
		// nothing says which account: not recorded, never guessed
		{Time: now.Add(-time.Hour), Provider: "cx", Model: "gpt-5", Host: "chatgpt.com", Input: 7, Status: 200},
		{Time: now, Provider: "claude", Model: "sonnet", Host: "api.anthropic.com as a@example.com", ProviderAccount: "a@example.com", Input: 9, Output: 1, Status: 200},
		{Time: now, Provider: "relay", Model: "m", Host: "relay.example", ProviderKeyID: provider.KeyID("relay-secret"), ProviderKeyName: "Team", Input: 5, Status: 200},
		// out of the period
		{Time: now.Add(-40 * 24 * time.Hour), Provider: "cx", Model: "gpt-5", ProviderAccount: "old@example.com", Input: 9999, Status: 200},
	}
}

// The Requests page packs rows into its cache: the account must survive
// that, its filter keep the account's calls, and the page offer the
// accounts of the period, whatever is picked.
func TestRequestPageByAccount(t *testing.T) {
	pageHome(t)
	now := time.Now()
	for i, r := range accountRecs(now)[:7] {
		r.Time = now.Add(time.Duration(i) * time.Second)
		r.Agent = "codex"
		Append(r)
	}
	for repeat := 0; repeat < 2; repeat++ {
		page := QueryPage(All, Filter{Account: "b@example.com"}, 0, 100)
		if page.Total != 2 || len(page.Rows) != 2 {
			t.Fatalf("account filter: total %d, %+v", page.Total, page.Rows)
		}
		for _, r := range page.Rows {
			if r.Account() != "b@example.com" {
				t.Fatalf("row of another account: %+v", r)
			}
		}
		if p := QueryPage(All, Filter{}, 0, 100); !slices.ContainsFunc(p.Rows, func(r Row) bool { return r.ProviderAccount == "a@example.com" }) {
			t.Fatalf("rows lost their account: %+v", p.Rows)
		}
		var ids []string
		for _, g := range page.Accounts {
			ids = append(ids, g.ID)
		}
		slices.Sort(ids)
		if strings.Join(ids, ",") != "claude@a@example.com,cx@a@example.com,cx@b@example.com" {
			t.Fatalf("account choices: %v", ids)
		}
	}
	s := Summarize(All)
	if len(s.Accounts) != 4 {
		t.Fatalf("indexed summary accounts: %+v", s.Accounts)
	}
}
