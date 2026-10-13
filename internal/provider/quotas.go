package provider

import (
	"context"
	"math"
	"time"
)

// Quotas is what is left everywhere fogway can ask: the plans bought with
// a key, and the keys' balances, as the Usage page shows them. The vendors are asked at once;
// what doesn't answer before ctx ends comes back with an error.
func Quotas(ctx context.Context) []SubscriptionQuota {
	subs, plans, balances := quotas(ctx)
	return cardsServed(subs, plans, balances, LastServed())
}

// Allotted is Quotas without the keys' balances: the plans whose
// allowance runs out and starts again, which is what fogway quota wait
// waits on; no key's balance is asked for.
func Allotted(ctx context.Context) []SubscriptionQuota {
	return PlanQuotas(ctx)
}

// quotas reads the plans and the balances at once. Fogway has no
// subscriptions: subs is always empty.
func quotas(ctx context.Context) (subs, plans, balances []SubscriptionQuota) {
	b := make(chan []SubscriptionQuota, 1)
	go func() { b <- KeyBalances(ctx) }()
	plans = PlanQuotas(ctx)
	return nil, plans, <-b
}

// Quota is one account's or key's allowance as fogway quota --json and
// the gateway's GET /v1/fogway/quotas tell it, for an agent choosing
// where to send its work: Provider is the first part of the models'
// ids (provider/model) for a key's plan or balance, the agent for a
// subscription.
type Quota struct {
	Provider string      `json:"provider"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"` // subscription, plan (bought with a key) or balance (a key's money)
	Plan     string      `json:"plan,omitempty"`
	User     string      `json:"user,omitempty"`
	Windows  []QuotaSpan `json:"windows"`
	Balance  string      `json:"balance,omitempty"`
	Error    string      `json:"error,omitempty"`
	AsOf     *time.Time  `json:"asOf,omitempty"` // the cached reading's time, nil for a new one
	// ReadAt is when the windows or balance shown were read, nil when not
	// known; a Claude account's can be well before now (claudeReadAt).
	ReadAt *time.Time `json:"readAt,omitempty"`
	// Until is when the plan's paid time ends, renewed then when Renew is
	// "auto", over when "off", either when "".
	Until *time.Time `json:"until,omitempty"`
	Renew string     `json:"renew,omitempty"`
	// LastServedAt is when the account, plan or key last answered a
	// request through the gateway, nil when it hasn't in the last 30 days
	// (served.go); Last is on the latest of them (#570).
	LastServedAt *time.Time `json:"lastServedAt,omitempty"`
	Last         bool       `json:"last,omitempty"`
}

// QuotaSpan is one window of an allowance: how much of it is used and
// left, in percent, and when it starts again.
type QuotaSpan struct {
	Unlimited bool       `json:"unlimited,omitempty"`
	Name      string     `json:"name"`
	Used      float64    `json:"used"`
	Remaining float64    `json:"remaining"`
	ResetsAt  *time.Time `json:"resetsAt,omitempty"`
	Display   string     `json:"display,omitempty"` // the vendor's own count, "1.2k / 3k"
	// Amount of Limit in Unit: the window's count, used, when the vendor
	// counts it so ("credits")
	Amount float64 `json:"amount,omitempty"`
	Limit  float64 `json:"limit,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	Pool   string  `json:"pool,omitempty"` // the shared pool the window draws on ("Gemini")
}

// QuotaReport is Quotas as Quota, the reset times made absolute from now.
func QuotaReport(ctx context.Context, now time.Time) []Quota {
	subs, plans, balances := quotas(ctx)
	return withServed(quotaReport(subs, plans, balances, now), LastServed())
}

// quotaReport assembles the cards from readings the tests feed directly.
func quotaReport(subs, plans, balances []SubscriptionQuota, now time.Time) []Quota {
	out := []Quota{}
	for _, g := range []struct {
		kind string
		qs   []SubscriptionQuota
	}{{"subscription", subs}, {"plan", plans}, {"balance", balances}} {
		for _, q := range g.qs {
			r := Quota{Provider: q.Provider, Name: q.Name, Kind: g.kind, Plan: q.Plan, User: q.User, AsOf: q.AsOf, ReadAt: q.ReadAt,
				Windows: []QuotaSpan{}, Balance: q.Balance, Error: q.Error, Until: q.Until, Renew: q.Renew}
			// a pool's own windows stand in for the models' drawing on it,
			// as the usage page shows them
			for _, w := range PooledWindows(q.Windows) {
				s := QuotaSpan{Unlimited: w.Unlimited, Name: w.Name, Used: w.Used, Remaining: max(0, 100-w.Used), ResetsAt: w.ResetsAt, Display: w.Display,
					Amount: w.Amount, Limit: w.Limit, Unit: w.Unit, Pool: w.Pool}
				if s.ResetsAt == nil && w.ResetSecs > 0 {
					t := now.Add(time.Duration(w.ResetSecs) * time.Second)
					s.ResetsAt = &t
				}
				r.Windows = append(r.Windows, s)
			}
			out = append(out, r)
		}
	}
	return out
}

// ResetClock is when a window starts again, on the clock: "14:30" today,
// "tomorrow 09:00", "Wed 14:30" within the week, else "Oct 3 14:30".
func ResetClock(at, now time.Time) string {
	at, now = at.Local(), now.Local()
	day := func(t time.Time) time.Time { y, m, d := t.Date(); return time.Date(y, m, d, 0, 0, 0, 0, time.Local) }
	switch days := int(math.Round(day(at).Sub(day(now)).Hours() / 24)); {
	case days <= 0:
		return at.Format("15:04")
	case days == 1:
		return "tomorrow " + at.Format("15:04")
	case days < 7:
		return at.Format("Mon 15:04")
	}
	return at.Format("Jan 2 15:04")
}

// PlanTerm says when a plan's paid time ends, "renews Oct 18", "expires
// Oct 18" or "until Oct 18" when the vendor doesn't say which; "" when
// it isn't known.
func PlanTerm(until *time.Time, renew string) string {
	if until == nil {
		return ""
	}
	d := until.Local().Format("Jan 2")
	switch renew {
	case "auto":
		return "renews " + d
	case "off":
		return "expires " + d
	}
	return "until " + d
}
