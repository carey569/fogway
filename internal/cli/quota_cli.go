package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/carey569/fogway/internal/provider"
	"github.com/carey569/fogway/internal/settings"
)

const quotaUsage = `usage: fogway quota [<provider>…] [--json]
       fogway quota alert [<percent>|off] [--balance <amount>|off]
       fogway quota wait <provider|account> [--timeout <duration>] [--quiet]
       fogway quota history [<provider|account>…] [--days <n>] [--json]
  what is left of every subscription, plan and key fogway has: each window's use and
  when it starts again, and each key's balance, asked of the vendors now (or less than
  a minute ago). --json is for scripts and agents, each entry with lastServedAt, when it
  last answered through the gateway, and last: true on the latest; the gateway answers the same at
  GET http://127.0.0.1:3725/v1/fogway/quotas (on Settings' Gateway port), and to another machine only while fogway is
  shared on the local network, with its key as the API key (Authorization: Bearer or x-api-key)
  quota alert 80 has the fogway app notify when any window of a subscription or plan
  reaches 80% used, once each time the window runs (not windows set aside, such as
  on-demand spending); --balance 5 when a balance falls to 5 or under, in its own
  currency or credits, once until topped up past it. off turns either off, and alone
  it says what is set. Both are off until set.
  quota wait codex returns once any Codex account fogway has on has allowance again
  (a window that stops it no longer used up), and an account's email or login, or
  <provider>/<account>, waits for that one alone: it reads the vendors itself, gateway
  running or not, again shortly after the soonest reset (every 1 to 10 minutes), and
  says on stderr what it waits for. For a script to go on with work it stopped:
  until codex exec …; do fogway quota wait codex || break; done. Exit 0 once there
  is allowance, 1 when --timeout (30m, 6h) passes first, 2 for a name it doesn't
  know, 130 on Ctrl+C.
  quota history is what fogway has read of each subscription's and plan's windows
  over time, kept 45 days (UTC times, a reading only when it changed, each window's
  runs apart by when they reset): the latest readings of each window, or with --json
  every one since --days days ago, as GET /v1/fogway/quotas/history?days=&provider=&user=
  answers. A provider, an account's email or login, or <provider>/<account> narrows it.
  A reading is kept at the time it was read: a Claude account's, which Claude Code
  tells only as it answers or runs /usage, isn't written again each time it is shown.`

// quotaCmd: fogway quota [<provider>…] [--json]
func quotaCmd(args []string) error {
	if len(args) > 1 && args[1] == "alert" {
		return quotaAlertCmd(args[2:])
	}
	if len(args) > 1 && args[1] == "wait" {
		return quotaWaitCmd(args[2:])
	}
	if len(args) > 1 && args[1] == "history" {
		return quotaHistoryCmd(args[2:])
	}
	asJSON := false
	var only []string
	for _, a := range args[1:] {
		switch a {
		case "--json", "-j":
			asJSON = true
		case "help", "-h", "--help":
			fmt.Println(quotaUsage)
			return nil
		default:
			only = append(only, strings.ToLower(a))
		}
	}
	// fogway quota is the user asking: read even when allowances are read
	// only when asked (#1518)
	ctx, cancel := context.WithTimeout(provider.Asked(context.Background()), 15*time.Second)
	defer cancel()
	qs := []provider.Quota{}
	for _, q := range provider.QuotaReport(ctx, time.Now()) {
		if len(only) == 0 || quotaMatches(q, only) {
			qs = append(qs, q)
		}
	}
	qs, err := withUntold(qs, only)
	if err != nil {
		return err
	}
	if asJSON {
		b, _ := json.MarshalIndent(qs, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if len(qs) == 0 {
		if len(only) > 0 {
			return fmt.Errorf("no subscription, plan or key balance of %s", strings.Join(only, ", "))
		}
		fmt.Println(muted.Render("nothing to tell ·"), "a signed-in subscription, a coding plan or a key whose vendor tells its balance shows up here")
		return nil
	}
	width := 0
	for _, q := range qs {
		width = max(width, len([]rune(quotaTitle(q))))
	}
	now := time.Now() // one instant for every window's reset time
	for _, q := range qs {
		line := fmt.Sprintf("%-*s  %s", width, quotaTitle(q), muted.Render(fmt.Sprintf("%-12s", q.Kind)))
		for _, w := range q.Windows {
			line += "  " + quotaCell(w, now)
		}
		if q.Balance != "" {
			line += "  " + balanceCell(q.Balance, q.Provider, q.User)
		}
		if q.Error != "" {
			line += "  " + muted.Render(q.Error)
		}
		line += quotaReadingCell(q.AsOf, q.ReadAt, q.Windows)
		fmt.Println(line)
	}
	fmt.Println(faint.Render("  % is how much of a window is used · ↻ when it starts again · --json for scripts, or GET /v1/fogway/quotas on the gateway"))
	return nil
}

// balanceCell is what an account holds in a line: "1.2K credits left",
// and "· not spent" for a Codex account set not to spend its credits.
func balanceCell(balance, id, user string) string {
	cell := bold.Render(balance) + muted.Render(" left")
	return cell
}

// quotaAlertCmd: fogway quota alert [<percent>|off] [--balance <amount>|off]
// — the usage alerts Settings sets (#368), the notifications the app shows.
func quotaAlertCmd(args []string) error {
	s := settings.Load()
	changed := false
	for i := 0; i < len(args); i++ {
		a := strings.ToLower(strings.TrimSpace(args[i]))
		switch {
		case a == "help" || a == "-h" || a == "--help":
			fmt.Println(quotaUsage)
			return nil
		case a == "--balance" || strings.HasPrefix(a, "--balance="):
			v, ok := strings.CutPrefix(a, "--balance=")
			if !ok {
				if i+1 >= len(args) {
					return fmt.Errorf("--balance needs an amount, or off")
				}
				i++
				v = strings.ToLower(strings.TrimSpace(args[i]))
			}
			if v == "off" {
				s.BalanceAlert = 0
			} else {
				n, err := strconv.ParseFloat(v, 64)
				if err != nil || n <= 0 {
					return fmt.Errorf("a balance alert is at an amount over 0, or off, not %q", v)
				}
				s.BalanceAlert = n
			}
			changed = true
		case a == "off":
			s.UsageAlert = 0
			changed = true
		default:
			n, err := strconv.Atoi(strings.TrimSuffix(a, "%"))
			if err != nil || n < 1 || n > 100 {
				return fmt.Errorf("a usage alert is at a percentage from 1 to 100, or off, not %q", args[i])
			}
			s.UsageAlert = n
			changed = true
		}
	}
	if changed {
		if err := settings.Save(s); err != nil {
			return err
		}
		fmt.Print(green.Render("✓") + " ")
	}
	if s.UsageAlert > 0 {
		fmt.Printf("notifies when a window reaches %d%% used, once each time it runs", s.UsageAlert)
	} else {
		fmt.Print("no usage alert")
	}
	if s.BalanceAlert > 0 {
		fmt.Printf("; and when a balance falls to %s or under, once until topped up\n", strconv.FormatFloat(s.BalanceAlert, 'f', -1, 64))
	} else {
		fmt.Println("; no balance alert")
	}
	if s.UsageAlert > 0 || s.BalanceAlert > 0 {
		fmt.Println(muted.Render("  the notifications come from the fogway app, while it runs"))
	}
	return nil
}

// quotaTitle is a quota's line head: its provider, plan and account.
func quotaTitle(q provider.Quota) string {
	t := q.Provider
	if q.Plan != "" {
		t += " · " + q.Plan
	}
	if s := provider.PlanTerm(q.Until, q.Renew); s != "" {
		t += " · " + s
	}
	if q.User != "" {
		t += " · " + q.User
	}
	return t
}

func quotaMatches(q provider.Quota, only []string) bool {
	for _, o := range only {
		if strings.EqualFold(q.Provider, o) || strings.EqualFold(q.Name, o) || strings.EqualFold(q.Kind, o) {
			return true
		}
	}
	return false
}

// withUntold adds a line for each provider named that told nothing, saying
// why, rather than leaving it out as if it weren't there; a name that is no
// provider at all is an error.
func withUntold(qs []provider.Quota, only []string) ([]provider.Quota, error) {
	for _, o := range only {
		if o == "subscription" || o == "plan" || o == "balance" || slices.ContainsFunc(qs, func(q provider.Quota) bool { return quotaMatches(q, []string{o}) }) {
			continue
		}
		p, err := provider.Find(o)
		if err != nil {
			return nil, err
		}
		qs = append(qs, provider.Quota{Provider: p.ID, Name: p.Name, Kind: "balance", Windows: []provider.QuotaSpan{},
			Error: "not configured · no balance endpoint known for it; set Balance URL and Balance field in its editor"})
	}
	return qs, nil
}

// quotaHistoryCmd: fogway quota history [<provider|account>…] [--days <n>] [--json]
func quotaHistoryCmd(args []string) error {
	asJSON, days := false, ""
	var only []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--json" || a == "-j":
			asJSON = true
		case a == "--days" && i+1 < len(args):
			i++
			days = args[i]
		case strings.HasPrefix(a, "--days="):
			days = strings.TrimPrefix(a, "--days=")
		case a == "help" || a == "-h" || a == "--help":
			fmt.Println(quotaUsage)
			return nil
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s\n%s", a, quotaUsage)
		default:
			only = append(only, strings.ToLower(a))
		}
	}
	if days != "" {
		if n, err := strconv.ParseFloat(days, 64); err != nil || n <= 0 {
			return fmt.Errorf("--days takes a number of days, such as 1 or 7, not %q", days)
		}
	}
	now := time.Now()
	hs := []provider.QuotaHistory{}
	all := provider.QuotaHistories(provider.QuotaHistorySince(days, now), "", "")
	for _, h := range all {
		rid, there, _ := strings.Cut(h.Provider, "/")
		if len(only) == 0 || slices.ContainsFunc(only, func(o string) bool {
			return o == h.Provider || o == h.User || o == h.Provider+"/"+h.User || there != "" && (o == rid || o == there || o == there+"/"+h.User)
		}) {
			hs = append(hs, h)
		}
	}
	if asJSON {
		b, _ := json.MarshalIndent(hs, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if len(hs) == 0 {
		if len(only) > 0 {
			return fmt.Errorf("no readings kept of %s", strings.Join(only, ", "))
		}
		fmt.Println(muted.Render("nothing kept yet ·"), "a subscription's or plan's windows are kept here as fogway reads them")
		return nil
	}
	for _, h := range hs {
		title := h.Provider
		if h.User != "" {
			title += " · " + h.User
		}
		fmt.Println(bold.Render(title))
		for _, l := range h.Lines {
			fmt.Println("  " + l.Name + "  " + quotaPointsCell(l.Points, 8))
		}
	}
	fmt.Println(faint.Render("  % is how much was left · ↻ a new run of the window · --json for every reading, or GET /v1/fogway/quotas/history on the gateway"))
	return nil
}

// quotaPointsCell is a window's last n readings, oldest first: "Oct 4
// 09:12 80% · 10:40 61% · ↻ 14:02 100%", a day only where it changes.
func quotaPointsCell(pts []provider.QuotaPoint, n int) string {
	if len(pts) > n {
		pts = pts[len(pts)-n:]
	}
	cells := []string{}
	day := ""
	for i, p := range pts {
		at := p.At.Local()
		cell := ""
		if i > 0 && quotaNewRun(pts[i-1], p) {
			cell = "↻ "
		}
		if d := at.Format("Jan 2"); d != day {
			day = d
			cell += muted.Render(d+" ") + at.Format("15:04")
		} else {
			cell += at.Format("15:04")
		}
		cells = append(cells, cell+" "+bold.Render(strconv.FormatFloat(p.Left, 'f', -1, 64)+"%"))
	}
	return strings.Join(cells, muted.Render(" · "))
}

// quotaNewRun is whether b is in a run of its window after a's: it resets
// at another time, or has more left with no reset to go by.
func quotaNewRun(a, b provider.QuotaPoint) bool {
	if a.ResetsAt != nil && b.ResetsAt != nil {
		d := b.ResetsAt.Sub(*a.ResetsAt)
		return d > 10*time.Minute || d < -10*time.Minute
	}
	return b.Left > a.Left+5
}
