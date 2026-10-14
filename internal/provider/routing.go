package provider

// Routing: how the gateway spreads requests over the keys or accounts a
// provider has on (see Provider.Routing). Choosing by use needs to know the
// use without waiting for it, so a subscription's allowance is read from
// what was fetched last, and fetched again behind the request when it has
// gone stale — or when an account says it has run out.

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// The routings besides the default, smart, in order.
const (
	Ordered   = "order"
	Rotate    = "rotate"
	LeastUsed = "usage"
	Pace      = "pace"
	// Weighted spreads a provider's requests over its keys by each key's
	// weight (KeyAccount.Weight, #841), smoothly: 3 and 1 go a, a, b, a…
	Weighted = "weight"
)

// SetRouting changes how a provider's requests spread over its keys or
// accounts.
func SetRouting(id, routing string) error {
	p, err := Find(id)
	if err != nil {
		return err
	}
	p.Routing = routing
	return Save(*p)
}

// SetSink sends a provider's keys or accounts rate limited with quota
// left to the back of its order, or lets them keep their place
// (Provider.Sink).
func SetSink(id string, sink bool) error {
	p, err := Find(id)
	if err != nil {
		return err
	}
	p.Sink = sink
	return Save(*p)
}

// SetAffinity changes how long a provider's conversations stay with the key
// or account that answered them.
func SetAffinity(id, affinity string) error {
	p, err := Find(id)
	if err != nil {
		return err
	}
	p.Affinity = affinity
	return Save(*p)
}

var usedCache struct {
	sync.Mutex
	m       map[string]map[string]Allowance // agent → user → allowance
	at      map[string]time.Time
	loading map[string]chan struct{} // closed when the fetch in flight is done
	// renewed: when an account's windows were last started again (a
	// Codex reset spent), by agent/user: a reading begun before says
	// nothing of them
	renewed map[string]time.Time
	// seen: each account's windows as last read, and when, by
	// agent/user, kept through a reading that failed, for the next to be
	// told renewed by (renewedFrom)
	seen map[string]reading
	// stale: made stale (StaleAllowance) while a reading was out, by
	// agent; that reading was asked before an account said it was out,
	// so what it tells is kept only until the next ask reads again
	stale map[string]bool
}

// firstWait is how long a request waits for an agent's allowances the
// first time they are asked for, so the first requests after fogway
// starts are routed by them too; later ones never wait.
var firstWait = 3 * time.Second

// Allowance is an account's allowance windows as last known.
type Allowance []Limit

// Limit is one window of an allowance.
type Limit struct {
	Used   float64       // share used, 0–100
	Resets time.Time     // zero when not known
	Span   time.Duration // how long the window runs; zero when not known
	Model  string        // the only models it counts, by a word in their ids
	// Amount of Of in Unit: the window's own count, used, when the vendor
	// counts it so (QuotaWindow.Amount)
	Amount, Of float64
	Unit       string
	Name       string // QuotaWindow.Name: which window it is, one reading to the next
	// RunsOut is when the soonest of what the window has left is lost,
	// apart from its reset (QuotaWindow.RunsOutAt): zero when not known.
	RunsOut time.Time
}

func (l Limit) applies(model string) bool {
	return l.Model == "" || strings.Contains(model, l.Model)
}

// For is what an allowance leaves a request for model at now: the share
// used of the fullest window that counts it, and when those windows renew,
// the biggest first — the longest, the week before the five hours in it.
// A window whose reset has passed is empty again, its next reset not known.
func (a Allowance) For(model string, now time.Time) (used float64, renews []time.Time) {
	model = strings.ToLower(model)
	var ls []Limit
	for _, l := range a {
		if !l.applies(model) {
			continue
		}
		if !l.Resets.IsZero() && !l.Resets.After(now) {
			l.Used, l.Resets = 0, time.Time{}
		}
		used = max(used, l.Used)
		ls = append(ls, l)
	}
	sort.SliceStable(ls, func(i, j int) bool {
		if ls[i].Span != ls[j].Span {
			return ls[i].Span > ls[j].Span
		}
		return ls[i].Resets.After(ls[j].Resets)
	})
	for _, l := range ls {
		renews = append(renews, l.Resets)
	}
	return used, renews
}

// Fullest is when the window For's share is of renews: the fullest that
// counts model, the later of those as full. A display pairs the two, so
// it names one window: a Factory account 32% into its week and 16% into
// its month is 32% used, renewing with the week, not the month (#1778).
// Zero when that window's reset isn't known, or has passed (it is empty
// again then, its next reset not known).
func (a Allowance) Fullest(model string, now time.Time) time.Time {
	model = strings.ToLower(model)
	used, at := -1.0, time.Time{}
	for _, l := range a {
		if !l.applies(model) {
			continue
		}
		if !l.Resets.IsZero() && !l.Resets.After(now) {
			l.Used, l.Resets = 0, time.Time{}
		}
		if l.Used > used || l.Used == used && l.Resets.After(at) {
			used, at = l.Used, l.Resets
		}
	}
	return at
}

// Count is the count of the window For's share is of — the fullest that
// counts model — when the vendor counts it (WorkBuddy's credits): how much
// of of is used, in unit; of is zero when it isn't counted so. A window
// whose reset has passed is empty again.
func (a Allowance) Count(model string, now time.Time) (amount, of float64, unit string) {
	model = strings.ToLower(model)
	used := -1.0
	for _, l := range a {
		if !l.applies(model) {
			continue
		}
		if !l.Resets.IsZero() && !l.Resets.After(now) {
			l.Used, l.Amount = 0, 0
		}
		if l.Used > used {
			used, amount, of, unit = l.Used, l.Amount, l.Of, l.Unit
		}
	}
	return amount, of, unit
}

// Renewal is For's renews as routing ranks them: a window not started,
// or whose reset has passed, that says how long it runs is taken to
// renew that long from now — a five hours not started renews within five
// hours, the soonest of an account that has nothing longer (#576). One
// whose credits left run out before it renews (Limit.RunsOut) renews
// then, as what is left is lost then all the same: WorkBuddy's accounts,
// whose credits have no reset, go by the package that ends first
// (cdredfox on X).
func (a Allowance) Renewal(model string, now time.Time) []time.Time {
	model = strings.ToLower(model)
	var ls []Limit
	for _, l := range a {
		if !l.applies(model) {
			continue
		}
		if !l.Resets.After(now) {
			l.Resets = time.Time{}
			if l.Span > 0 {
				l.Resets = now.Add(l.Span)
			}
		}
		if l.RunsOut.After(now) && (l.Resets.IsZero() || l.RunsOut.Before(l.Resets)) {
			l.Resets = l.RunsOut
		}
		ls = append(ls, l)
	}
	sort.SliceStable(ls, func(i, j int) bool {
		if ls[i].Span != ls[j].Span {
			return ls[i].Span > ls[j].Span
		}
		return ls[i].Resets.After(ls[j].Resets)
	})
	out := make([]time.Time, len(ls))
	for i, l := range ls {
		out[i] = l.Resets
	}
	return out
}

// Full is when an account used up for model can take it again: the last
// reset of its windows that are full, zero when none is or it isn't known.
func (a Allowance) Full(model string, share float64, now time.Time) time.Time {
	model = strings.ToLower(model)
	var t time.Time
	for _, l := range a {
		if l.applies(model) && l.Used >= share && l.Resets.After(now) && l.Resets.After(t) {
			t = l.Resets
		}
	}
	return t
}

// budgetSpan is the shortest window that is a budget rather than a rate
// cap: the week (Kiro's month too), not the five hours in it — what the
// five hours leave at their reset is nothing lost.
const budgetSpan = 24 * time.Hour

// weekSpan is the budget window an account is taken to have when its
// vendor tells none a day or longer, or tells one without saying how long
// it runs or when it renews.
const weekSpan = 7 * 24 * time.Hour

// FreshPace is the pace of an account with a whole week ahead of it:
// what one not known counts as.
const FreshPace = 100 / (7 * 24.0)

// Pace is the share of its week an account has left for model, per hour
// until that week renews: the rate it would have to be used at to spend
// the week just in time, so the account with the highest has the most to
// lose at its reset. Of several budget windows that count the model
// (Opus's own beside the general), the tightest; due is when that one
// renews, zero when it isn't known. A window not started, or whose reset
// isn't known, is taken to run its whole span from now; one that doesn't
// say how long it runs (a plugin may not) is a budget, for as long as its
// reset says, else a week. The hours until a reset are never taken as
// fewer than one, or a window a minute from renewing would outweigh all
// the rest. An account whose vendor tells no budget window, only a short
// one (Claude Enterprise's five hours), has nothing longer to keep for:
// what its five hours have left is lost at their reset, so it goes by
// that, per hour until then — the whole span when not started (#576):
// ahead of nearly every week, behind only one with more of it to lose
// sooner.
func (a Allowance) Pace(model string, now time.Time) (pace float64, due time.Time) {
	model = strings.ToLower(model)
	any, used := false, 0.0
	short, shortPace, shortDue := false, 0.0, time.Time{}
	for _, l := range a {
		if !l.applies(model) {
			continue
		}
		u := min(100, max(0, l.Used))
		if !l.Resets.IsZero() && !l.Resets.After(now) {
			u = 0 // its reset has passed: empty again
		}
		if l.Span != 0 && l.Span < budgetSpan {
			used = max(used, u)
			until, renews := l.Span, time.Time{}
			if l.Resets.After(now) {
				until, renews = l.Resets.Sub(now), l.Resets
			}
			if p := (100 - u) / max(until, time.Hour).Hours(); !short || p < shortPace {
				shortPace, shortDue, short = p, renews, true
			}
			continue
		}
		until, renews := l.Span, time.Time{}
		if until == 0 {
			until = weekSpan
		}
		if l.Resets.After(now) {
			until, renews = l.Resets.Sub(now), l.Resets
		}
		if p := (100 - u) / max(until, time.Hour).Hours(); !any || p < pace {
			pace, due, any = p, renews, true
		}
	}
	if !any {
		if short {
			return shortPace, shortDue
		}
		return (100 - used) / weekSpan.Hours(), time.Time{}
	}
	return pace, due
}

// OnRenewed has f told, as agent "" and the key's KeyAllowanceID, when a
// reading of a key's own windows finds one it was full in full no more:
// its limit raised in its panel, or its usage reset. What sat out waiting
// for it can come back.
func OnRenewed(f func(agent, user string)) {
	renewedHooks.Lock()
	renewedHooks.fs = append(renewedHooks.fs, f)
	renewedHooks.Unlock()
}

var renewedHooks struct {
	sync.Mutex
	fs []func(agent, user string)
}

// tellRenewed tells those OnRenewed asked.
func tellRenewed(agent, user string) {
	renewedHooks.Lock()
	fs := renewedHooks.fs
	renewedHooks.Unlock()
	for _, f := range fs {
		f(agent, user)
	}
}

// StaleAllowance makes the next Allowances ask the vendor again for user's
// allowance rather than trust what it last said, or what a reading out
// now, asked before, comes back with: the account just answered that it
// has run out.
func StaleAllowance(agent, user string) {
	if readsAsked() {
		markStaleRead(agent, user) // read though allowances are read only when asked (#1518)
	}
	usedCache.Lock()
	if usedCache.at != nil {
		usedCache.at[agent] = time.Time{}
	}
	// a reading out now was asked before: read again once it is back
	if usedCache.loading[agent] != nil {
		if usedCache.stale == nil {
			usedCache.stale = map[string]bool{}
		}
		usedCache.stale[agent] = true
	}
	usedCache.Unlock()
}

// allowanceOf keeps the windows that can stop an account.
func allowanceOf(ws []QuotaWindow, now time.Time) Allowance {
	var a Allowance
	for _, w := range ws {
		if w.Aside {
			continue
		}
		l := Limit{Used: w.Used, Span: w.Span, Model: w.Model, Amount: w.Amount, Of: w.Limit, Unit: w.Unit, Name: w.Name}
		switch {
		case w.ResetsAt != nil:
			l.Resets = *w.ResetsAt
		case w.ResetSecs > 0:
			l.Resets = now.Add(time.Duration(w.ResetSecs) * time.Second)
		}
		if w.RunsOutAt != nil {
			l.RunsOut = *w.RunsOutAt
		}
		a = append(a, l)
	}
	return a
}
