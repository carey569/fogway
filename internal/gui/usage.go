package gui

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fogway/fogway/internal/agent"
	"github.com/fogway/fogway/internal/edit"
	"github.com/fogway/fogway/internal/provider"
	"github.com/fogway/fogway/internal/sessions"
	"github.com/fogway/fogway/internal/usage"
	"github.com/fogway/fogway/internal/usageapi"
)

// The Usage page's JSON is built in usageapi, which the gateway's
// /v1/fogway/usage answers from too.
type (
	usageGroup = usageapi.Group
	usageJSON  = usageapi.Usage
	ledgerRow  = usageapi.Row
	ledgerJSON = usageapi.Ledger
)

var (
	usageState   = usageapi.Page
	ledgerPage   = usageapi.LedgerPage
	periodOf     = usageapi.PeriodOf
	ledgerFilter = usageapi.FilterOf
)

func init() {
	usageapi.Clients = func() []usageapi.Client {
		var out []usageapi.Client
		for _, a := range agent.Clients() {
			out = append(out, usageapi.Client{ID: a.ID, Name: a.Name, Icon: a.Icon})
		}
		return out
	}
}

// csvStamp names the selected day, or, when no day is selected, the period
// and the day of now, the moment its rows were read at.
func csvStamp(p usage.Period, day string, now time.Time) string {
	if _, err := time.Parse(time.DateOnly, day); err == nil {
		return "fogway-requests-day-" + day
	}
	if p.IsRange() { // the days picked, which already say when
		return "fogway-requests-" + strings.Replace(string(p), "..", "-to-", 1)
	}
	return "fogway-requests-" + string(p) + "-" + now.Format(time.DateOnly)
}

// contentJSON is what was said in a request, or why it can't be told.
type contentJSON struct {
	Found bool `json:"found"`
	// Why not: "session" (the request named none), "agent" (fogway reads the session
	// files of Claude Code, Claude Desktop and Codex only), "missing" (the files have no
	// such call: deleted, moved, or not written yet) or "read" (the file wouldn't read)
	Why   string `json:"why,omitempty"`
	Model string `json:"model,omitempty"`
	sessions.Content
}

// requestContent finds the call a request is in its agent's session files and reads it.
func requestContent(agent, session string, from, to, at time.Time) contentJSON {
	out := contentJSON{Content: sessions.Content{Input: []sessions.Part{}, Output: []sessions.Part{}}}
	switch {
	case session == "":
		out.Why = "session"
	case agent != "claude" && agent != "claude-desktop" && agent != "codex":
		out.Why = "agent"
	default:
		c, ok := sessions.FindCall(session, from, to, at)
		if !ok {
			out.Why = "missing"
			break
		}
		content, err := sessions.ContentOf(c)
		if err != nil {
			out.Why = "read"
			break
		}
		out.Found, out.Model, out.Content = true, c.Model, content
	}
	return out
}

func usageRoutes(mux *http.ServeMux, w Windows) {
	mux.HandleFunc("GET /api/usage", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, usageState(periodOf(r.URL.Query().Get("period"))))
	})
	// the ledger: the period's calls, newest first, a page at a time
	mux.HandleFunc("GET /api/usage/requests", func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		writeJSON(rw, ledgerPage(periodOf(q.Get("period")), ledgerFilter(q), offset, limit))
	})
	// the heatmap (#1369): the last 53 weeks a day each, of the requests the
	// page's filters keep, read from the same index as the page
	mux.HandleFunc("GET /api/usage/heatmap", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, usage.HeatmapOf(ledgerFilter(r.URL.Query())))
	})
	// what was said in one request, read from the agent's session file when the
	// row is opened, between two times (the call's own, or a gateway request's
	// span): fogway keeps no copy
	mux.HandleFunc("GET /api/usage/requests/content", func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		from, err1 := time.Parse(time.RFC3339Nano, q.Get("from"))
		to, err2 := time.Parse(time.RFC3339Nano, q.Get("to"))
		if err1 != nil || err2 != nil {
			http.Error(rw, "from and to are times", http.StatusBadRequest)
			return
		}
		// at: when the call should have ended (a gateway request's start and
		// its time); none is the window's end
		at, err := time.Parse(time.RFC3339Nano, q.Get("at"))
		if err != nil {
			at = to
		}
		writeJSON(rw, requestContent(q.Get("agent"), q.Get("session"), from, to, at))
	})
	// the same CSV to a browser (fogway web), which saves it itself
	mux.HandleFunc("GET /api/usage/requests.csv", func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		p := periodOf(q.Get("period"))
		// one moment for the rows and the day in the file's name
		now := usage.Clock()
		rows := usage.LedgerOfAt(p, ledgerFilter(q), now).Rows
		rw.Header().Set("Content-Type", "text/csv; charset=utf-8")
		rw.Header().Set("Content-Disposition", `attachment; filename="`+csvStamp(p, q.Get("day"), now)+`.csv"`)
		usage.WriteCSV(rw, rows)
	})
	// the rows the ledger shows, all its pages, as a CSV in Downloads
	mux.HandleFunc("POST /api/usage/requests/export", func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		p := periodOf(q.Get("period"))
		now := usage.Clock()
		rows := usage.LedgerOfAt(p, ledgerFilter(q), now).Rows
		var b bytes.Buffer
		if err := usage.WriteCSV(&b, rows); err != nil {
			fail(rw, err)
			return
		}
		dir := downloads()
		stamp := csvStamp(p, q.Get("day"), now)
		name := filepath.Join(dir, stamp+".csv")
		for i := 2; ; i++ { // never over an earlier one
			if _, err := os.Stat(name); err != nil {
				break
			}
			name = filepath.Join(dir, fmt.Sprintf("%s-%d.csv", stamp, i))
		}
		if err := edit.WriteAtomic(name, b.Bytes()); err != nil {
			fail(rw, err)
			return
		}
		_ = w.OpenFolder(dir) // saved either way; the path is in the answer
		writeJSON(rw, map[string]any{"path": tilde(name), "rows": len(rows)})
	})
	// The subscriptions' quotas, the plans' bought with a key, and the
	// keys' balances come from the
	// vendors, which can be slow or unreachable, so the page asks for them
	// apart from the local log.
	// ?asked=1 is the user opening or refreshing the page: Claude Code's
	// own /usage is run at once (provider.AskClaudeUsage).
	// Only then are they read while the user has allowances read only
	// when asked (#1518).
	mux.HandleFunc("GET /api/usage/quotas", func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.URL.Query().Get("asked") != "" {
			ctx = provider.Asked(ctx)
		}
		ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		writeJSON(rw, provider.Quotas(ctx))
	})
	// One card read again, from its refresh button (#840): ?provider= and,
	// of a card with several accounts, &user=; the others are left as they
	// were read. The answer is every card, as GET's.
	mux.HandleFunc("POST /api/usage/quotas/refresh", func(rw http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("provider")
		if id == "" {
			http.Error(rw, "provider is required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		provider.RefreshUsage(ctx, id, r.URL.Query().Get("user"))
		writeJSON(rw, provider.Quotas(ctx))
	})
	// what was left of each window over time, for the quota cards' curves
	// (#651); ?days= back
	mux.HandleFunc("GET /api/usage/quotas/history", func(rw http.ResponseWriter, r *http.Request) {
		days := r.URL.Query().Get("days")
		hs := provider.QuotaHistories(provider.QuotaHistorySince(days, time.Now()), "", "")
		writeJSON(rw, hs)
	})
}
