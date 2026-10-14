package provider

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/carey569/fogway/internal/catalog"
	"github.com/carey569/fogway/internal/codexcat"
	"github.com/carey569/fogway/internal/edit"
	"github.com/carey569/fogway/internal/filememo"
	"github.com/carey569/fogway/internal/settings"
	"github.com/carey569/fogway/internal/steady"
)

// Quota cards, readings and file helpers that the key-based providers
// share; they lived beside the subscription accounts Fogway no longer has.

// reading is an account's windows as Allowances last read them, and when.
type reading struct {
	a  Allowance
	at time.Time
}

// keyReading is a key's windows as routing read them, with the plan or
// balance read with them, for its card.
type keyReading struct {
	ws      []QuotaWindow
	plan    string
	balance string
	parts   []BalancePart
	at      time.Time
	seq     uint64 // quotaReading's, begun as it was asked
}

// SpentShareOf is the share past which routing counts an account spent:
// SpentShare, but In order sends requests to the first until a window of it
// is used up or the vendor refuses it, so there an account at 98% is still
// tried in its turn (#530).
func SpentShareOf(routing string) float64 {
	if routing == Ordered {
		return 100
	}
	return SpentShare
}

// usedUp reports whether an account's allowance is used up at now: a
// window that stops the account, for every model, at 100%.
func usedUp(q SubscriptionQuota, now time.Time) bool {
	return usedPast(q, 100, now)
}

var googleTokenURL = "https://oauth2.googleapis.com/token"

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func writeFileAtomic(path string, b []byte) error {
	path, err := edit.Target(path) // a symlink stays, its target written
	if err != nil {
		return err
	}
	return steady.WriteFile(path, b, 0o600) // on the disk before it is renamed in (#1505)
}

// writePrivate replaces a file readable by the user alone, atomically, so
// an agent reading it at that moment sees either version, never half; the
// new one is on the disk before it is renamed over the old (#1505).
func writePrivate(path string, b []byte) error {
	defer filememo.Forget()        // read again, where a request holds it
	path, err := edit.Target(path) // a symlink stays, its target written
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return steady.WriteFile(path, b, 0o600)
}

// postTokenAs is postToken asked with bearer, a token already had, when it
// isn't "" (Google's IAM Credentials, which trades one for another).
func postTokenAs(ctx context.Context, tokenURL, ctype, bearer string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error            any    `json:"error"`
			ErrorDescription string `json:"error_description"`
			Detail           string `json:"detail"`
			Message          string `json:"message"` // Connect RPC errors
		}
		_ = json.Unmarshal(b, &e)
		msg := e.ErrorDescription
		if msg == "" {
			msg = e.Detail
		}
		if msg == "" {
			msg = e.Message
		}
		if msg == "" {
			if m, ok := e.Error.(map[string]any); ok {
				msg, _ = m["message"].(string)
			} else if s, ok := e.Error.(string); ok {
				msg = s
			}
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &tokenRefused{resp.StatusCode, msg}
	}
	return json.Unmarshal(b, out)
}

// tokenRefused is a token endpoint's refusal, as postToken answers it.
type tokenRefused struct {
	status int
	msg    string
}

// QuotaWindow is one rolling allowance reported by a subscription provider.
type QuotaWindow struct {
	Unlimited bool       `json:"unlimited,omitempty"`
	Name      string     `json:"name"`
	Used      float64    `json:"used"`
	ResetsAt  *time.Time `json:"resetsAt,omitempty"`
	ResetSecs int64      `json:"resetSecs,omitempty"`
	Display   string     `json:"display,omitempty"`
	// Amount of Limit is the window's own count in Unit, when the vendor
	// counts it so (WorkBuddy's credits): Used is Amount as a share of
	// Limit. The GUI says it as used or left, as it says the share (#659).
	Amount float64 `json:"amount,omitempty"`
	Limit  float64 `json:"limit,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	// RunsOutAt is when the soonest of what is left is lost, and RunsOut
	// how much (in Unit), when it runs out apart from the window's reset:
	// WorkBuddy's credits come in packages that each end on their own
	// day. Smart routing takes it for the window's renewal (cdredfox on X).
	RunsOutAt *time.Time `json:"runsOutAt,omitempty"`
	RunsOut   float64    `json:"runsOut,omitempty"`

	// For routing (see Allowances): how long the window runs, zero when
	// not known; the only models it counts, by a word in their ids
	// ("opus"), when it doesn't count them all; and Aside when using it up
	// doesn't stop the account — on-demand spending past the allowance,
	// or Copilot's code completions, which no request here makes.
	Span  time.Duration `json:"-"`
	Model string        `json:"-"`
	Aside bool          `json:"-"`
}

// SubscriptionQuota is provider-reported allowance usage. This is separate
// from Dial's local token log: vendors expose percentages, not token totals.
type SubscriptionQuota struct {
	Provider  string        `json:"provider"`
	Name      string        `json:"name"`
	Icon      string        `json:"icon"`
	Plan      string        `json:"plan,omitempty"`
	AccessSKU string        `json:"accessSku,omitempty"`
	User      string        `json:"user,omitempty"` // the account, so two of one vendor tell apart
	Windows   []QuotaWindow `json:"windows"`
	Balance   string        `json:"balance,omitempty"` // what is left on an API key, instead of windows
	// BalanceParts are the Balance's amounts each apart, when the balance
	// field the user wrote has several or a percent (cardParts)
	BalanceParts []BalancePart `json:"balanceParts,omitempty"`
	// BalanceTrend is the Balance over time, as fogway read it, and when
	// it runs out at that pace (balance_history.go)
	BalanceTrend *BalanceTrend `json:"balanceTrend,omitempty"`
	// Until is when the plan's paid time ends: it renews then when Renew
	// is "auto", is over when "off", and either when "" (the vendor
	// doesn't say which).
	Until *time.Time `json:"until,omitempty"`
	Renew string     `json:"renew,omitempty"`
	Error string     `json:"error,omitempty"`
	// NeedsAccessKey is a plan read with the account's access key, which
	// its provider hasn't (accessKeyCard, #1821): the Usage page offers
	// the provider's settings to give it in.
	NeedsAccessKey bool `json:"needsAccessKey,omitempty"`
	// AsOf is when an allowance shown in place of one that couldn't be
	// read was read (see keepLast); nil for a reading just made.
	AsOf *time.Time `json:"asOf,omitempty"`
	// ReadAt is when what is shown was read: a key's balance, kept a
	// minute (KeyBalances), or an account's allowance, which a cache (a
	// minute's, or a Claude account's until Claude Code tells it again)
	// can hand back long after; nil when not known.
	ReadAt *time.Time `json:"readAt,omitempty"`
	// Kind is the card's kind as another fogway is told it (CachedCards):
	// subscription, plan or balance; "" here.
	Kind string `json:"kind,omitempty"`
	// LastServedAt is when the account, plan or key last answered a
	// request through this computer's gateway, nil when it hasn't in the
	// last 30 days (served.go); set by Quotas, never cached.
	LastServedAt *time.Time `json:"lastServedAt,omitempty"`
	// In-process read order, separate from the vendor's ReadAt and never
	// persisted: restarting starts a new sequence.
	readSeq uint64
	// glmPlan marks a key's quota read from the GLM Coding Plan endpoint,
	// including custom providers. Only these can share ZCode's allowance.
	glmPlan bool
}

// elapsed is ws as of now: a window that has reset since it was read
// starts again from nothing.
func elapsed(ws []QuotaWindow, now time.Time) []QuotaWindow {
	out := make([]QuotaWindow, len(ws))
	for i, w := range ws {
		if w.ResetsAt != nil && !now.Before(*w.ResetsAt) {
			w.Used, w.ResetsAt = 0, nil
		}
		out[i] = w
	}
	return out
}

// SpentShare is the share of an allowance past which an account is all
// but used up: Smart routing keeps it for when no other can take a
// request, and the agent signed in to it is signed in to another.
const SpentShare = 98

// usedPast reports whether one of q's windows that stop the account for
// every model is at share or past at now. A window whose reset has passed
// has started again, whatever share it was read at: a reading kept from
// before, or one the vendor gave late (Claude Code's /usage tells a reset
// to the minute), says nothing of now.
func usedPast(q SubscriptionQuota, share float64, now time.Time) bool {
	for _, w := range q.Windows {
		if !w.Aside && w.Model == "" && w.Used >= share && !resetPassed(w, now) {
			return true
		}
	}
	return false
}

func (e *tokenRefused) Error() string {
	return fmt.Sprintf("the sign-in was refused (%d): %s", e.status, e.msg)
}

// resetPassed says whether w has started again by now: its reset time has
// passed, whatever share it was read at. One that gives no reset time
// can't say.
func resetPassed(w QuotaWindow, now time.Time) bool {
	return w.ResetsAt != nil && !now.Before(*w.ResetsAt)
}

// AccountRefs are the provider's keys as a key's own limits know them:
// each key's KeyID.
func (p Provider) AccountRefs() []string {
	var out []string
	for _, k := range p.KeyList() {
		out = append(out, k.ID)
	}
	return out
}

// AccountRef is one of a provider's keys as a gateway key's account list
// knows it (#905): ID its fingerprint, User what it is shown as.
type AccountRef struct {
	ID   string
	User string
	Plan string
	Key  bool
}

// AccountIDs are the provider's keys as a gateway key's account list
// knows them (#905): by fingerprint, with the name each is shown by.
func (p Provider) AccountIDs() []AccountRef {
	var out []AccountRef
	for _, k := range p.KeyList() {
		out = append(out, AccountRef{ID: k.ID, User: cmp.Or(k.Name, k.Masked), Key: true})
	}
	return out
}

// AccountNames maps every key a gateway key may name (#905),
// "<provider>/<key id>", to how it is shown, "<provider>/<name>"; what no
// provider has now maps to itself.
func AccountNames() map[string]string {
	out := map[string]string{}
	for _, p := range All() {
		for _, a := range p.AccountIDs() {
			out[p.ID+"/"+a.ID] = p.ID + "/" + a.User
		}
	}
	return out
}

// accountRef is the key of p that ref names: by its KeyID or its name, in
// any case.
func (p Provider) accountRef(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	for _, k := range p.KeyList() {
		if strings.EqualFold(k.ID, ref) || k.Name != "" && strings.EqualFold(k.Name, ref) {
			return k.ID, true
		}
	}
	return "", false
}
func cmdPlanName(id string) string {
	name, _ := cmdPlanOf(id)
	return name
}

type cmdCredits struct {
	Credits struct {
		PlanID    string `json:"planId"`
		Monthly   any    `json:"monthlyCredits"`
		Purchased any    `json:"purchasedCredits"`
		Free      any    `json:"freeCredits"`
	} `json:"credits"`
	Windows *struct {
		FiveHour *cmdWindow `json:"fiveHour"`
		Weekly   *cmdWindow `json:"weekly"`
	} `json:"windowLimits"`
}

// cmdWindows is the plan's 5-hour and weekly windows in a credits reply,
// those it gives a cap.
func cmdWindows(c cmdCredits) []QuotaWindow {
	out := []QuotaWindow{}
	if c.Windows == nil {
		return out
	}
	for _, w := range []struct {
		name string
		span time.Duration
		w    *cmdWindow
	}{{"5 hours", 5 * time.Hour, c.Windows.FiveHour}, {"Weekly", 7 * 24 * time.Hour, c.Windows.Weekly}} {
		if w.w == nil {
			continue
		}
		used, ok1 := number(w.w.Used)
		limit, ok2 := number(w.w.Cap)
		if !ok1 || !ok2 || limit <= 0 {
			continue
		}
		out = append(out, QuotaWindow{Name: w.name, Used: min(100, 100*max(0, used)/limit), Span: w.span,
			ResetsAt: cmdTime(w.w.ResetAt)})
	}
	return out
}

// cmdLeft is the dollars left on the account, the month's credits and the
// bought and free ones together (the CLI's totalRemaining), and the
// month's alone; ok is false when the reply tells none of them.
func cmdLeft(c cmdCredits) (monthly, left float64, ok bool) {
	for i, v := range []any{c.Credits.Monthly, c.Credits.Purchased, c.Credits.Free} {
		if n, is := number(v); is {
			n = max(0, n)
			if i == 0 {
				monthly = n
			}
			left += n
			ok = true
		}
	}
	return monthly, left, ok
}

// normalModels is a list of model ids as it is kept: trimmed, each once,
// in the order given.
func normalModels(ms []string) []string {
	var out []string
	for _, m := range ms {
		if m = strings.TrimSpace(m); m != "" && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// normalAccountModels is m as it is kept: keys in lower case, lists
// normalized, accounts serving all the provider's models left out; nil
// for none.
func normalAccountModels(m map[string][]string) map[string][]string {
	var out map[string][]string
	for u, ms := range m {
		u, ms = accountKey(u), normalModels(ms)
		if u == "" || len(ms) == 0 {
			continue
		}
		if out == nil {
			out = map[string][]string{}
		}
		out[u] = ms
	}
	return out
}
func cmdPlanOf(id string) (name string, monthly float64) {
	id = strings.ReplaceAll(strings.ToLower(id), "_", "-")
	for _, p := range cmdPlans {
		if strings.HasPrefix(id, p.id) {
			return p.name, p.monthly
		}
	}
	return "", 0
}

// cmdCredits is /alpha/billing/credits as the CLI's /usage reads it
// (projectUsageView): the dollars left of the month's credits, and of the
// bought and free ones beside them, under "credits"; the plan's windows
// next to it, not in it —
//
//	{"credits":{"planId":"individual-goat-monthly","monthlyCredits":41.2,
//	            "purchasedCredits":5,"freeCredits":0},
//	 "windowLimits":{"limited":true,
//	   "fiveHour":{"used":3.1,"cap":10,"resetAt":1790000000000},
//	   "weekly":{"used":12,"cap":40,"resetAt":1790400000000}},
//	 "sandboxMinutes":{…},"sandboxAccess":false}
//
// resetAt is in milliseconds (the CLI compares it with Date.now()); used
// and cap are only ever divided, one by the other.
type cmdWindow struct {
	Used    any `json:"used"`
	Cap     any `json:"cap"`
	ResetAt any `json:"resetAt"` // unix ms (seconds or a time taken too)
}

// cmdTime reads a reset time: unix seconds or milliseconds, or RFC 3339.
func cmdTime(v any) *time.Time {
	if s, ok := v.(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return &t
		}
	}
	n, ok := number(v)
	if !ok || n <= 0 {
		return nil
	}
	if n < 1e12 {
		n *= 1000
	}
	t := time.UnixMilli(int64(n))
	return &t
}

// zcodeBizRoot is the business API of the site a plan is served from.
func zcodeBizRoot(base string) string {
	if h := hostOf(base); h == hostOf(ZCodeBigModelBase) || strings.HasSuffix(h, "bigmodel.cn") {
		return zcodeBigModelAPI
	}
	return zcodeRoot(base)
}

// zcodeTeamHeaders are what the team's business and usage endpoints are
// asked with besides the Authorization.
func zcodeTeamHeaders(base, org, project string) map[string]string {
	lang := "en"
	if zcodeBizRoot(base) == zcodeBigModelAPI {
		lang = "zh"
	}
	return map[string]string{
		"Bigmodel-Organization": org,
		"Bigmodel-Project":      project,
		"Set-Language":          lang,
		"Accept-Language":       "en-US,en",
	}
}

// zhipuTeamOf is the team project a GLM key belongs to, when that key is a
// ZCode account's team key fogway holds: never, in Fogway.
func zhipuTeamOf(key string) (org, project string) {
	return "", ""
}

// zhipuTerm is how long a GLM Coding plan is paid for, from the
// account's subscriptions as ZCode reads them: the valid one's next
// renewal, a charge when it renews itself, else the end of its time — or
// the last date its "valid" span names.
func zhipuTerm(ctx context.Context, root, key string) (*time.Time, string) {
	var subs []zhipuSubscription
	if zcodeGet(ctx, root+"/api/biz/subscription/list", key, &subs) != nil {
		return nil, ""
	}
	return zhipuTermOf(subs)
}

// zcodeRoot is the site a plan's endpoint is on: https://api.z.ai.
func zcodeRoot(base string) string {
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return zcodeZaiAPI
}

// Do sends req to p with client, without the header fogway names a
// conversation by, which is the gateway's own.
func (p Provider) Do(client *http.Client, req *http.Request) (*http.Response, error) {
	req.Header.Del(ConversationHeader)
	return client.Do(req)
}

// cmdPlans names the plans as the CLI does, by their id's start, longest
// first, with the dollars of credits each gives a month (its getPlanInfo;
// "individual-pro-v1" is the old Pro, $80 of them).
var cmdPlans = []struct {
	id, name string
	monthly  float64
}{
	{"individual-provider", "Provider", 15}, {"individual-pro-v1", "Pro", 80}, {"individual-goat", "GOAT", 70},
	{"individual-ultra", "Ultra", 300}, {"individual-max", "Max", 150}, {"individual-pro", "Pro", 30},
	{"individual-go", "Go", 10}, {"teams-pro", "Teams Pro", 40},
}

// ConversationHeader carries the conversation a request belongs to, to a
// plugin's chat.headers hook as its session; it goes no further.
const ConversationHeader = "X-Fogway-Conversation"

var ZCodeBigModelBase = "https://open.bigmodel.cn/api/anthropic"

var zcodeZaiAPI = "https://api.z.ai"

var zcodeBigModelAPI = "https://bigmodel.cn"

type zhipuSubscription struct {
	Status    string `json:"status"`
	Valid     string `json:"valid"`
	AutoRenew any    `json:"autoRenew"` // true or 1
	NextRenew string `json:"nextRenewTime"`
}

func zhipuTermOf(subs []zhipuSubscription) (*time.Time, string) {
	for _, s := range subs {
		if !strings.EqualFold(s.Status, "VALID") {
			continue
		}
		auto := s.AutoRenew == true || s.AutoRenew == float64(1)
		if t := zhipuTime(s.NextRenew); t != nil {
			if auto {
				return t, "auto"
			}
			return t, "off"
		}
		if ds := zhipuDate.FindAllString(s.Valid, -1); len(ds) > 0 && !auto {
			if t := zhipuTime(ds[len(ds)-1]); t != nil {
				return t, "off"
			}
		}
		return nil, ""
	}
	return nil, ""
}

// zcodeGet asks with a coding plan's key, which goes bare in Authorization.
func zcodeGet(ctx context.Context, u, key string, dst any) error {
	return zcodeCall(ctx, http.MethodGet, u, key, nil, dst)
}

// zhipuDate finds the dates in a span like "2026-09-18 12:00:00-2026-10-18 12:00:00".
var zhipuDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[ T]\d{2}:\d{2}:\d{2})?`)

// zhipuTime reads the plan's times, "2026-10-18 12:00:00" in Beijing.
func zhipuTime(s string) *time.Time {
	s = strings.Replace(strings.TrimSpace(s), " ", "T", 1)
	cst := time.FixedZone("CST", 8*3600)
	for _, f := range []string{"2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, s, cst); err == nil {
			return &t
		}
	}
	return nil
}

// zcodeCall asks one of Z.ai's JSON endpoints, which wrap what they say in
// {code, msg, data}: code 0 or 200 is a success.
func zcodeCall(ctx context.Context, method, u, auth string, body, dst any) error {
	return zcodeCallH(ctx, method, u, auth, nil, body, dst)
}

// zcodeCallH is zcodeCall with more headers: a team's organization and
// project.
func zcodeCallH(ctx context.Context, method, u, auth string, hdr map[string]string, body, dst any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var env struct {
		Code json.RawMessage `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if json.Unmarshal(b, &env) == nil && env.Msg != "" {
			if c := strings.Trim(string(env.Code), `"`); c != "" && c != "null" && c != "0" {
				return fmt.Errorf("%s (%d, code %s)", env.Msg, res.StatusCode, c)
			}
			return fmt.Errorf("%s (%d)", env.Msg, res.StatusCode)
		}
		return fmt.Errorf("%s", http.StatusText(res.StatusCode))
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}
	if c := strings.Trim(string(env.Code), `"`); c != "" && c != "0" && c != "200" && c != "null" {
		return errors.New(firstNonEmpty(env.Msg, "error "+c))
	}
	if dst == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, dst)
}

type accountStatusError struct {
	status     int
	retryAfter time.Duration // what the vendor asked to wait, when it said
}

// zcodeAppVersion is the ZCode whose sign-in fogway makes.
const zcodeAppVersion = "3.14.3"

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

var zcodeAPI = "https://zcode.z.ai"
var zcodeDevices = struct {
	sync.Mutex
	m map[string]string // by the file it is kept in
}{m: map[string]string{}}

var zcodeUUIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// splitAntigravityID is an id's family and the level its effort word
// says, "" when it has none.
func splitAntigravityID(id string) (family, effort string) {
	for _, e := range antigravityEffortWords {
		if b, ok := strings.CutSuffix(id, "-"+e.word); ok && b != "" {
			return b, e.level
		}
	}
	return id, ""
}

// EffortFamily is the model an id with an effort word after it is a level
// of ("gemini-3.8-flash-medium" is gemini-3.8-flash), and the id itself
// when it has none.
func EffortFamily(id string) string {
	f, _ := splitAntigravityID(id)
	return f
}

// antigravityEffortWords are the effort words of Antigravity's ids and the
// level each is, longest first.
var antigravityEffortWords = []struct{ word, level string }{
	{"extra-high", "xhigh"}, {"extra-low", "minimal"}, {"xhigh", "xhigh"}, {"minimal", "minimal"},
	{"low", "low"}, {"medium", "medium"}, {"high", "high"},
}

// claudeDashedish is an id that could be a dotted one respelled.
var claudeDashedish = regexp.MustCompile(`(?i)claude-(?:opus|sonnet|haiku|fable|mythos)-\d+-\d`)

// claudeSplit splits a model as an agent is given it into the id and what
// follows it: an effort fixed on it (":high", MemberEfforts) and Claude
// Code's [1m] mark.
func claudeSplit(ref string) (id, rest string) {
	const mark = "[1m]"
	id, marked := strings.CutSuffix(ref, mark)
	if i := strings.LastIndex(id, ":"); i > 0 && slices.Contains(MemberEfforts, strings.ToLower(id[i+1:])) {
		id, rest = id[:i], id[i:]
	}
	if marked {
		rest += mark
	}
	return id, rest
}

// ClaudeUndashed is the served id (a provider's model or a routing group)
// whose dashed spelling (ClaudeDashed) ref is, its effort and [1m] mark
// kept, when ref is no served id as it is and exactly one served id is so
// spelled. An id served as it is spelled always wins.
func ClaudeUndashed(ref string) (string, bool) {
	id, rest := claudeSplit(ref)
	if !claudeDashedish.MatchString(id) {
		return "", false
	}
	if real, ok := undashIn(Served(), id); ok {
		return real + rest, true
	}
	return "", false
}

func undashIn(entries []Entry, id string) (string, bool) {
	found := ""
	for _, e := range entries {
		if e.ID == id {
			return "", false // served as it is spelled
		}
		if e.ID == found || ClaudeDashed(e.ID) != id {
			continue
		}
		if found != "" {
			return "", false // two ids dash alike: neither is meant
		}
		found = e.ID
	}
	return found, found != ""
}

// CodeAssist is Google's Code Assist API, which fogway only speaks
// upstream, to a Gemini CLI or Antigravity account. Not in Protocols: no
// other provider serves it.
const CodeAssist Protocol = "codeassist"

// claudeDotted is a Claude model's version written with a dot.
var claudeDotted = regexp.MustCompile(`(?i)claude-(?:opus|sonnet|haiku|fable|mythos)-\d+(?:\.\d+)+`)

// ClaudeDashed is id with each dot of a Claude model's version a dash:
// relay/claude-opus-4.6-us[1m] is relay/claude-opus-4-6-us[1m]. Any other
// id is as it is.
func ClaudeDashed(id string) string {
	if !strings.Contains(id, ".") {
		return id
	}
	return claudeDotted.ReplaceAllStringFunc(id, func(s string) string { return strings.ReplaceAll(s, ".", "-") })
}

// AccountServes says whether the key ref (its KeyID) is let serve model:
// true when it has no list of its own.
func (p Provider) AccountServes(ref, model string) bool {
	ms, ok := p.AccountModels[strings.ToLower(strings.TrimSpace(ref))]
	return !ok || len(ms) == 0 || slices.Contains(ms, model)
}

// AccountID is the stable id of p's key as a gateway key's account list
// knows it (#905): the key's fingerprint.
func (p Provider) AccountID() string { return KeyID(p.Key) }

// ClaudeSpelled is ref (a served id, with an effort and [1m] mark or not)
// as Claude Code is given it: dashed (ClaudeDashed) when that spelling
// comes back to this id (ClaudeUndashed), else as it is.
func ClaudeSpelled(ref string) string {
	id, rest := claudeSplit(ref)
	d := ClaudeDashed(id)
	if d == id {
		return ref
	}
	if real, ok := undashIn(Served(), d); ok && real == id {
		return d + rest
	}
	return ref
}

// CodexCatalog is shown as a Codex that names fogway its model_provider is
// handed it from GET /v1/codex/models (#1281): every model, a ChatGPT
// account's own among them, since that Codex reaches them through fogway's
// /v1 by fogway's id, not through the ChatGPT backend's list.
func CodexCatalog(shown []Entry) []catalog.Model {
	find := GroupFinder()
	return codexListed(shown, func(id string) []Member {
		_, ms, _ := find(id)
		return ms
	}, true)
}

// FlatName is the name a namespaced tool is offered to a model under, which
// takes one flat name: namespace__name, as Codex names an MCP server's tools.
// A name longer than the 64 characters APIs allow is cut and made unique by
// a hash of the whole.
func FlatName(namespace, name string) string {
	flat := namespace + "__" + name
	if len(flat) <= 64 {
		return flat
	}
	sum := sha256.Sum256([]byte(namespace + "\x00" + name))
	return flat[:55] + "_" + hex.EncodeToString(sum[:4])
}

// ObjectRoot makes a tool's input schema a plain object at the root, with
// no anyOf, oneOf or allOf there: Grok's backend wants one, and so do
// Anthropic's models behind Factory ("input_schema does not support
// oneOf, allOf, or anyOf at the top level", #646). allOf's branches are
// all merged, properties and required alike. Of anyOf's and oneOf's object
// branches the properties are merged, a field each of them requires stays
// required, and the other branches go. A branch that is a union itself, as
// zod writes a discriminated union inside another (Codex desktop's
// automation_update, #1271), is folded the same way first, so its fields
// are kept; a field the branches give different schemas takes any of them.
// It reports whether it changed anything.
func ObjectRoot(ps map[string]any) bool {
	return objectRootIn(ps, ps, 0)
}

// objectRootIn is ObjectRoot on ps, a schema whose local $refs name root's
// $defs; depth bounds how far unions inside unions are followed.
func objectRootIn(root, ps map[string]any, depth int) bool {
	_, any1 := ps["anyOf"]
	_, one := ps["oneOf"]
	_, all := ps["allOf"]
	if ps["type"] == "object" && !any1 && !one && !all {
		return false
	}
	props, _ := ps["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	var required []any
	if r, ok := ps["required"].([]any); ok {
		required = r
	}
	list, _ := ps["allOf"].([]any)
	for _, b := range list {
		bm, _ := b.(map[string]any)
		if bm = grokRef(root, bm); bm == nil {
			continue
		}
		bp, _ := bm["properties"].(map[string]any)
		for k, v := range bp {
			if _, ok := props[k]; !ok {
				props[k] = v
			}
		}
		br, _ := bm["required"].([]any)
		required = append(required, br...)
	}
	delete(ps, "allOf")
	var branches []map[string]any
	for _, k := range []string{"anyOf", "oneOf"} {
		list, _ := ps[k].([]any)
		for _, b := range list {
			bm, _ := b.(map[string]any)
			if bm = grokRef(root, bm); bm == nil {
				continue
			}
			if nestedUnion(bm) && depth < 8 {
				// folded on a copy: the branch may be a $def others name
				bm = maps.Clone(bm)
				objectRootIn(root, bm, depth+1)
			}
			if _, has := bm["properties"]; bm["type"] != "object" && !has {
				continue
			}
			branches = append(branches, bm)
		}
		delete(ps, k)
	}
	// a field every branch requires stays required, beside the root's and
	// allOf's own, which no branch can drop
	var common []any
	// the schemas the branches give a field the root doesn't define
	from := map[string][]any{}
	var order []string
	for i, b := range branches {
		bp, _ := b["properties"].(map[string]any)
		for _, k := range slices.Sorted(maps.Keys(bp)) {
			if _, own := props[k]; own {
				continue
			}
			if _, seen := from[k]; !seen {
				order = append(order, k)
			}
			from[k] = appendDistinct(from[k], bp[k])
		}
		br, _ := b["required"].([]any)
		if i == 0 {
			common = append(common, br...)
			continue
		}
		in := map[any]bool{}
		for _, r := range br {
			in[r] = true
		}
		kept := common[:0]
		for _, r := range common {
			if in[r] {
				kept = append(kept, r)
			}
		}
		common = kept
	}
	for _, k := range order {
		if vs := from[k]; len(vs) == 1 {
			props[k] = vs[0]
		} else {
			props[k] = map[string]any{"anyOf": vs}
		}
	}
	seen := map[string]bool{}
	var merged []any
	for _, r := range append(required, common...) {
		if name, ok := r.(string); ok {
			if seen[name] {
				continue
			}
			seen[name] = true
		}
		merged = append(merged, r)
	}
	required = merged
	ps["type"] = "object"
	ps["properties"] = props
	if len(required) > 0 {
		ps["required"] = required
	} else {
		delete(ps, "required")
	}
	return true
}

// codexListed marks a group Fast when a ChatGPT account's GPT model is in
// it, so Codex offers /fast there too; the tier goes out only to that
// account (buildResponses). own keeps a ChatGPT account's own models in,
// which a signed-in Codex has from the backend already.
func codexListed(shown []Entry, members func(id string) []Member, own bool) []catalog.Model {
	var ms []catalog.Model
	// named among all shown: the account's own, which the backend lists,
	// are in Codex's picker beside these
	labels := Labels(shown)
	seen := described()
	s := settings.Load()
	find := func(id string) (Group, []Member, bool) { return Group{}, members(id), true }
	for i, e := range shown {
		if CodexOwn(e) && !own {
			continue
		}
		m := catalog.Model{ID: e.ID, Name: labels[i], Efforts: e.Efforts, Images: e.Images || seen, Context: e.Context, AgentsV2: e.AgentsV2}
		m.Compact = CompactFor(s, e, find)
		CodexTiers(&m, e, members)
		ms = append(ms, m)
	}
	return ms
}

// nestedUnion reports whether a branch of a union is a union itself.
func nestedUnion(b map[string]any) bool {
	for _, k := range []string{"anyOf", "oneOf", "allOf"} {
		if _, ok := b[k]; ok {
			return true
		}
	}
	return false
}

// appendDistinct adds v to vs unless one there is the same schema.
func appendDistinct(vs []any, v any) []any {
	vb, _ := json.Marshal(v)
	for _, w := range vs {
		if wb, _ := json.Marshal(w); bytes.Equal(vb, wb) {
			return vs
		}
	}
	return append(vs, v)
}

// grokRef is a branch of a schema, or what its local $ref names in the
// schema's $defs or definitions.
func grokRef(root, b map[string]any) map[string]any {
	ref, _ := b["$ref"].(string)
	if ref == "" {
		return b
	}
	for _, k := range []string{"$defs", "definitions"} {
		if name, ok := strings.CutPrefix(ref, "#/"+k+"/"); ok {
			defs, _ := root[k].(map[string]any)
			d, _ := defs[name].(map[string]any)
			return d
		}
	}
	return nil
}

// CodexTiers sets on m what Codex is offered on e's service tiers
// (codexcat.Entries turns them into the entry's service_tiers): one source
// for every list Codex is handed, the gateway's and the models.json file
// fogway writes for a Codex CLI that names fogway its provider (#1530).
func CodexTiers(m *catalog.Model, e Entry, members func(id string) []Member) {
	// a provider the user added by its address is sent the tier Codex
	// asks for as it is (hsiangron on X)
	m.OwnTier = e.Group == "" && e.Provider.Preset == "" && e.Provider.ID != ""
}

// ClaudeRead is a model read back from Claude Code's settings as fogway
// serves it: a dashed spelling ClaudeSpelled wrote taken back to the
// served id, anything else as it is. A model of a provider switched off
// since is taken back too, so fogway still knows it for its own and moves
// the agent off it (Reseat).
func ClaudeRead(ref string) string {
	if real, ok := ClaudeUndashed(ref); ok {
		return real
	}
	id, rest := claudeSplit(ref)
	if !claudeDashedish.MatchString(id) {
		return ref
	}
	if real, ok := undashIn(offEntries(), id); ok {
		return real + rest
	}
	return ref
}

// offEntries are the models of the providers switched off, by id alone.
func offEntries() []Entry {
	var out []Entry
	for _, p := range All() {
		if p.On() {
			continue
		}
		for _, m := range p.Exposed() {
			out = append(out, Entry{ID: p.ID + "/" + m.ID})
		}
	}
	return out
}

// CodexListTag names the list Codex is handed, for its ETag: fogway's
// models, the order they are in, the windows set on them, the auto-review
// model, and whether its OpenAI models say multi-agent V1
// (settings.CodexAgentsV1), so any of them changing has Codex ask for the
// list again.
func CodexListTag() string {
	ms := CodexListed()
	// and the models the user disabled (#1867)
	for _, pat := range DisabledModels() {
		ms = append(ms, catalog.Model{ID: "!" + pat})
	}
	// and the order the user put them in
	for _, id := range ModelOrder("codex") {
		ms = append(ms, catalog.Model{ID: "^" + id})
	}
	// and where a model with no threshold of its own is compacted, when
	// that isn't the working window
	if n := settings.Load().Compact(); n != settings.WorkingWindow {
		ms = append(ms, catalog.Model{ID: "~compact", Context: n})
	}
	// and the model Codex's auto-review runs on (#938)
	if v := settings.Load().CodexAutoReview; v != "" {
		ms = append(ms, catalog.Model{ID: "~autoreview:" + v})
	}
	// and the effort new sessions start at, which fogway's models take as
	// their default (codexcat.ModelDefault): another one set, Codex asks
	// for the list again
	if v := codexcat.NewSessionEffort(); v != "" {
		ms = append(ms, catalog.Model{ID: "~effort:" + v})
	}
	return codexcat.PolicyTag(codexcat.Tag(ms))
}

// CodexListed is the catalog as a Codex signed in to ChatGPT is handed it,
// after the backend's own models: all but a ChatGPT account's in fogway,
// which the backend lists already. A group answers for its first member
// but is not that provider's.
func CodexListed() []catalog.Model {
	shown, _ := CatalogFor("codex")
	find := GroupFinder()
	return codexListed(shown, func(id string) []Member {
		_, ms, _ := find(id)
		return ms
	}, false)
}

// Count is the window's own count as the GUI says it, beside its share:
// "355 / 500 credits" used, or what is left of it when left (#659); the
// vendor's Display when it doesn't count in amounts.
func (w QuotaWindow) Count(left bool) string {
	if w.Limit <= 0 {
		return w.Display
	}
	n := w.Amount
	if left {
		n = max(0, w.Limit-w.Amount)
	}
	return strings.TrimSpace(groupedNumber(n) + " / " + groupedNumber(w.Limit) + " " + w.Unit)
}

// groupedNumber is n with its thousands apart, "12,345.5", to two places.
func groupedNumber(n float64) string {
	s := compactNumber(math.Round(n*100) / 100)
	whole, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	if neg {
		whole = "-" + whole
	}
	if frac != "" {
		return whole + "." + frac
	}
	return whole
}
func compactNumber(n float64) string {
	if n == float64(int64(n)) {
		return fmt.Sprintf("%d", int64(n))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", n), "0"), ".")
}

// SetAccountModels gives the key ref of the provider id the models it
// alone serves; none gives it every model the provider has again.
func SetAccountModels(id, ref string, models []string) error {
	p, err := Find(id)
	if err != nil {
		return err
	}
	r, ok := p.accountRef(ref)
	if !ok {
		return fmt.Errorf("%s has no key %q", p.Name, ref)
	}
	key := strings.ToLower(r)
	ms := normalModels(models)
	if len(ms) == 0 && p.AccountModels[key] == nil {
		return nil
	}
	am := map[string][]string{}
	for k, v := range p.AccountModels {
		am[k] = v
	}
	if len(ms) == 0 {
		delete(am, key)
	} else {
		am[key] = ms
	}
	p.AccountModels = am
	return Save(*p)
}

// AccountModelsOf is the key ref of the provider id names, and the models
// it alone serves (none: every model the provider has).
func AccountModelsOf(id, ref string) (string, []string, error) {
	p, err := Find(id)
	if err != nil {
		return "", nil, err
	}
	r, ok := p.accountRef(ref)
	if !ok {
		return "", nil, fmt.Errorf("%s has no key %q", p.Name, ref)
	}
	return r, p.AccountModels[strings.ToLower(r)], nil
}

// UsedUp is usedUp for the CLI's fogway quota wait, so what it waits out
// is what stops an account here.
func UsedUp(q SubscriptionQuota, now time.Time) bool { return usedUp(q, now) }

// BackAt is when a used-up account has room again: when the last of the
// windows that stop it (usedUp's) starts again, a window's seconds to go
// counted from now. Zero when one of them doesn't say.
func BackAt(q SubscriptionQuota, now time.Time) time.Time {
	var back time.Time
	for _, w := range q.Windows {
		if w.Aside || w.Model != "" || w.Used < 100 || resetPassed(w, now) {
			continue
		}
		var at time.Time
		switch {
		case w.ResetsAt != nil:
			at = *w.ResetsAt
		case w.ResetSecs > 0:
			at = now.Add(time.Duration(w.ResetSecs) * time.Second)
		default:
			return time.Time{}
		}
		if at.After(back) {
			back = at
		}
	}
	return back
}

// BoundCallID is a call's id as the ChatGPT backend takes one: as it is
// when it fits its 64 characters, else the sha256 of it (trimmed, as calls
// and their outputs are paired), 64 hex characters. Foreign providers can
// join two ids into one longer than that (#732, congee949: 86 and 87); the
// same id gives the same hash on every request, so a call and its output
// stay paired, and two ids that differ are never cut down alike.
func BoundCallID(id string) string {
	if len(id) <= 64 {
		return id
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(id)))
	return hex.EncodeToString(sum[:])
}
