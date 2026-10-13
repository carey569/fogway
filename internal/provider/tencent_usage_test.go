package provider

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The signature is the one Tencent Cloud's signing doc (api/213/30654)
// works through: DescribeInstances on cvm.tencentcloudapi.com at
// 1551113065, its payload byte for byte.
func TestTC3SignMatchesDoc(t *testing.T) {
	// the doc sends the name as JSON escapes (a backslash and u672a…), not UTF-8
	esc := strings.ReplaceAll("#u672a#u547d#u540d", "#", `\`)
	payload := []byte(`{"Limit": 1, "Filters": [{"Values": ["` + esc + `"], "Name": "instance-name"}]}`)
	if got := volcHex(payload); got != "35e9c5b0e3ae67532d3c9f17ead6c90222632e5b1ff7f6e89887f1398934f064" {
		t.Fatalf("payload hash %s", got)
	}
	id, key := "AKID"+strings.Repeat("*", 32), strings.Repeat("*", 32)
	got := tc3Sign(id, key, "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", payload, time.Unix(1551113065, 0))
	want := "TC3-HMAC-SHA256 Credential=" + id + "/2019-02-25/cvm/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=10b1a37a7301a02ca19a647ad722d5e43b4b3cff309d421d85b46093f6ab6c4f"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	// a time in another zone signs with the UTC date
	cst := time.Unix(1551113065, 0).In(time.FixedZone("CST", 8*3600))
	if tc3Sign(id, key, "cvm", "cvm.tencentcloudapi.com", "DescribeInstances", payload, cst) != want {
		t.Error("CST time signed differently")
	}
}

// The docs' PackageInfo (quotas as strings) and TokenSummary: used of the
// cycle's quota, credits for 专业套餐 and tokens for 轻享套餐, resetting at
// the cycle's end; with no TokenSummary the cycle is counted from the
// package's start, never past its expiry; no quota is no window.
func TestTencentPlanWindow(t *testing.T) {
	var p tencentPlan
	if err := json.Unmarshal([]byte(`{"TeamId":"team-a1b2c3d4","ProductType":"enterprise",
		"PackageInfo":{"TotalQuota":"1000000","TotalUsed":"100000","TotalCycles":1,"CycleUnit":"month","StartTime":"2026-04-01T00:00:00Z","ExpireTime":"2027-04-01T00:00:00Z",
			"ExclusiveAllocated":"500000","ExclusiveUsed":"100000","SharedPool":"500000","SharedUsed":"50000","CycleQuota":"1000000","CurrentCycle":1,"RemainCycles":0},
		"TokenSummary":{"CycleSeq":1,"CycleStartTime":"2026-04-01T00:00:00+08:00","CycleEndTime":"2026-05-01T00:00:00+08:00",
			"BillingItems":[{"BillingItem":"input","TotalQty":"60000"},{"BillingItem":"output","TotalQty":"40000"}]}}`), &p); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	w, ok := p.window(now)
	if !ok || w.Name != "Month" || w.Used != 10 || w.Amount != 100000 || w.Limit != 1000000 || w.Unit != "credits" ||
		w.ResetsAt == nil || !w.ResetsAt.Equal(time.Date(2026, 4, 30, 16, 0, 0, 0, time.UTC)) || w.Span != 30*24*time.Hour {
		t.Fatalf("%+v %v", w, ok)
	}
	p.ProductType = "enterprise-auto"
	if w, _ := p.window(now); w.Unit != "tokens" {
		t.Errorf("轻享套餐 unit %q", w.Unit)
	}
	// no TokenSummary: month 1 from StartTime, and on past it as time goes
	p.TokenSummary = nil
	if w, _ := p.window(now); w.ResetsAt == nil || !w.ResetsAt.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("from StartTime: %+v", w.ResetsAt)
	}
	if w, _ := p.window(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)); w.ResetsAt == nil || !w.ResetsAt.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("July: %+v", w.ResetsAt)
	}
	p.PackageInfo.ExpireTime = "2026-07-20T00:00:00Z"
	if w, _ := p.window(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)); w.ResetsAt == nil || !w.ResetsAt.Equal(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("capped at expiry: %+v", w.ResetsAt)
	}
	// no cycle quota: the total's; used past it shows full
	p.PackageInfo.CycleQuota, p.PackageInfo.TotalUsed = nil, 2000000.0
	if w, ok := p.window(now); !ok || w.Limit != 1000000 || w.Used != 100 {
		t.Errorf("TotalQuota: %+v", w)
	}
	for _, pk := range []string{`{}`, `{"TotalQuota":"0","TotalUsed":"1"}`, `{"TotalQuota":"10"}`} {
		var q tencentPlan
		json.Unmarshal([]byte(`{"PackageInfo":`+pk+`}`), &q)
		if _, ok := q.window(now); ok {
			t.Errorf("%s gave a window", pk)
		}
	}
	if _, ok := (tencentPlan{}).window(now); ok {
		t.Error("no PackageInfo gave a window")
	}
}
