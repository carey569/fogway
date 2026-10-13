package gateway

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// WorkBuddy's two refusals, as Natsumeretsu quoted them (#1669).
const (
	wbZh = "WorkBuddy：您的使用量已超出频率限制，将在 2026-10-11 20:16:02 UTC+8重置，您也可以切换其他模型继续使用。"
	wbEn = "WorkBuddy AI: usage exceeds frequency limit, but don't worry, your usage will reset at 2026-10-11 12:37:53 UTC+8, alternatively, you can switch to the other models to continue using it."
)

// escaped is msg as a vendor's JSON error whose encoder escapes what
// isn't ASCII as \uXXXX, as Python's json.dumps does by default.
func escaped(msg, code string) string {
	return `{"error":{"message":` + strconv.QuoteToASCII(msg) + `,"code":"` + code + `"}}`
}

// wbAt is WorkBuddy's refusal in lang, saying it resets at t.
func wbAt(lang string, t time.Time) string {
	at := t.In(time.FixedZone("", 8*3600)).Format("2006-01-02 15:04:05")
	if lang == "zh" {
		return strings.Replace(wbZh, "2026-10-11 20:16:02", at, 1)
	}
	return strings.Replace(wbEn, "2026-10-11 12:37:53", at, 1)
}

// A reset time written in words is read in the offset it is written with,
// in either language; one cut short, garbled, with no offset, past, too far
// off, or a date another vendor mentions in passing is not read at all.
func TestResetsWritten(t *testing.T) {
	cst := time.FixedZone("", 8*3600)
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04:05", s, cst)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct {
		name, body string
		now        time.Time
		want       time.Duration
	}{
		{"zh", wbZh, at("2026-10-11 17:00:00"), 3*time.Hour + 16*time.Minute + 2*time.Second},
		{"en", wbEn, at("2026-10-11 10:00:00"), 2*time.Hour + 37*time.Minute + 53*time.Second},
		// the same instant, written in another offset
		{"en UTC", strings.Replace(wbEn, "12:37:53 UTC+8", "04:37:53 UTC+0", 1), at("2026-10-11 10:00:00"), 2*time.Hour + 37*time.Minute + 53*time.Second},
		{"en +08:00", strings.Replace(wbEn, "UTC+8", "UTC+08:00", 1), at("2026-10-11 10:00:00"), 2*time.Hour + 37*time.Minute + 53*time.Second},
		{"en -5", strings.Replace(wbEn, "2026-10-11 12:37:53 UTC+8", "2026-10-10 23:37:53 UTC-5", 1), at("2026-10-11 10:00:00"), 2*time.Hour + 37*time.Minute + 53*time.Second},
		{"zh JSON", `{"error":{"message":"` + wbZh + `","code":"6004"}}`, at("2026-10-11 17:00:00"), 3*time.Hour + 16*time.Minute + 2*time.Second},
		{"zh escaped", escaped(wbZh, "6004"), at("2026-10-11 17:00:00"), 3*time.Hour + 16*time.Minute + 2*time.Second},
		{"zh escaped, cut short", escaped(wbZh[:strings.Index(wbZh, "+8重置")], "6004"), at("2026-10-11 17:00:00"), 0},
		// cut short: never read as UTC, or as a time without its seconds
		{"zh cut after UTC", wbZh[:strings.Index(wbZh, "+8重置")], at("2026-10-11 17:00:00"), 0},
		{"zh cut after +", wbZh[:strings.Index(wbZh, "8重置")], at("2026-10-11 17:00:00"), 0},
		{"zh cut after +8", wbZh[:strings.Index(wbZh, "重置")], at("2026-10-11 17:00:00"), 0},
		{"en cut after UTC", wbEn[:strings.Index(wbEn, "+8,")], at("2026-10-11 10:00:00"), 0},
		{"en cut after +8", wbEn[:strings.Index(wbEn, ", alternatively")], at("2026-10-11 10:00:00"), 0},
		{"en cut in the time", wbEn[:strings.Index(wbEn, ":53")], at("2026-10-11 10:00:00"), 0},
		{"zh cut in a rune", wbZh[:strings.Index(wbZh, "重置")+1], at("2026-10-11 17:00:00"), 0},
		// garbled
		{"month 13", strings.Replace(wbZh, "2026-10-11", "2026-13-11", 1), at("2026-10-11 17:00:00"), 0},
		{"day 45", strings.Replace(wbEn, "2026-10-11", "2026-10-45", 1), at("2026-10-11 10:00:00"), 0},
		{"hour 25", strings.Replace(wbZh, "20:16:02", "25:16:02", 1), at("2026-10-11 17:00:00"), 0},
		{"offset +99", strings.Replace(wbEn, "UTC+8", "UTC+99", 1), at("2026-10-11 10:00:00"), 0},
		{"no offset", strings.Replace(wbZh, " UTC+8", "", 1), at("2026-10-11 17:00:00"), 0},
		// past, or further off than a week
		{"past", wbZh, at("2026-10-11 20:30:00"), 0},
		{"8 days", wbEn, at("2026-10-03 12:37:52"), 0},
		{"7 days", wbEn, at("2026-10-04 12:37:53"), 7 * 24 * time.Hour},
		// another vendor's date in passing is no reset
		{"deprecated", "This model will be deprecated on 2026-10-12 00:00:00 UTC+8, please switch to the other models.", at("2026-10-11 10:00:00"), 0},
		{"retry after", "Too many requests, retry after 2026-10-11 12:37:53 UTC+8.", at("2026-10-11 10:00:00"), 0},
	} {
		if got := resetsWritten([]byte(c.body), c.now); got != c.want {
			t.Errorf("%s: %v, want %v (%q)", c.name, got, c.want, c.body)
		}
		if got := resetsAt([]byte(c.body), c.now); got != c.want {
			t.Errorf("%s: resetsAt %v, want %v", c.name, got, c.want)
		}
	}
}

// The word lists read a JSON error that escapes its Chinese as \uXXXX the
// same as one that doesn't (#1669): Zhipu's 1113 is out of credit, not a
// rate limit, and a sub2api key's "7天限额已用完" sent as
// rate_limit_exceeded is out of quota.
func TestFailureReadsEscapedWords(t *testing.T) {
	for _, c := range []struct {
		body, want string
	}{
		{escaped("余额不足或无可用资源包，请充值。", "1113"), failCredit},
		{`{"error":{"type":"rate_limit_exceeded","message":` + strconv.QuoteToASCII("api key 7天限额已用完") + `}}`, failQuota},
		{escaped(wbZh, "6004"), failQuota},
		{escaped("请求太频繁，请稍后再试", "1302"), failRate},
	} {
		if !strings.Contains(c.body, `\u`) {
			t.Fatalf("not escaped: %s", c.body)
		}
		if got := failure(429, []byte(c.body)); got != c.want {
			t.Errorf("%s: %s, want %s", c.body, got, c.want)
		}
	}
}
