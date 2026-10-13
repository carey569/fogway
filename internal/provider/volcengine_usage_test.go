package provider

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"
)

// The signature is the one Volcengine's own SDK makes: these were made by
// volc-sdk-golang (06f9ef66) base.GetSignRequest for a POST to
// open.volcengineapi.com/ with Action and Version in the query, body {},
// Content-Type application/json, service ark, at 2026-10-09T08:30:15Z.
func TestVolcSignMatchesSDK(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 30, 15, 0, time.UTC)
	for _, c := range []struct{ region, action, sig string }{
		{"cn-beijing", "GetCodingPlanUsage", "deb80c0cd8bd77768ca40f6d47c7204621c0994428f6ab77e23b76e15dc4c047"},
		{"cn-shanghai", "GetAFPUsage", "8a241be0996c93de4dfcd9e8b1db37494bf63170804fcaebe8380ff7dca2bb23"},
	} {
		auth, xDate, sha := volcSign("AKLTexample", "c2VjcmV0LWV4YW1wbGU=", c.region,
			url.Values{"Action": {c.action}, "Version": {"2024-01-01"}}, []byte("{}"), now)
		want := "HMAC-SHA256 Credential=AKLTexample/20261009/" + c.region + "/ark/request, SignedHeaders=content-type;host;x-content-sha256;x-date, Signature=" + c.sig
		if auth != want {
			t.Errorf("%s:\n got %s\nwant %s", c.action, auth, want)
		}
		if xDate != "20261009T083015Z" || sha != "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a" {
			t.Errorf("%s: X-Date %s, X-Content-Sha256 %s", c.action, xDate, sha)
		}
	}
	// a time in another zone signs as the same instant in UTC
	cst := time.Date(2026, 10, 9, 16, 30, 15, 0, time.FixedZone("CST", 8*3600))
	if _, xDate, _ := volcSign("a", "b", "cn-beijing", url.Values{}, nil, cst); xDate != "20261009T083015Z" {
		t.Errorf("X-Date in CST: %s", xDate)
	}
}

// GetCodingPlanUsage as pi-ark-usage's fixture has it: Percent used,
// ResetTimestamp in seconds; an unknown level is left out.
func TestVolcReadCodingPlan(t *testing.T) {
	ws := volcReadCodingPlan(json.RawMessage(`{"Status":"ok","UpdateTimestamp":1758000000,"QuotaUsage":[
		{"Level":"session","Percent":10.5,"ResetTimestamp":1758100000},
		{"Level":"weekly","Percent":20.25,"ResetTimestamp":1758200000},
		{"Level":"monthly","Percent":"30.75","ResetTimestamp":1758300000},
		{"Level":"daily","Percent":1}]}`))
	if len(ws) != 3 || ws[0].Name != "5 hours" || ws[0].Used != 10.5 || ws[0].Span != 5*time.Hour || ws[0].ResetsAt == nil || ws[0].ResetsAt.Unix() != 1758100000 ||
		ws[1].Name != "Weekly" || ws[1].Used != 20.25 || ws[2].Name != "Month" || ws[2].Used != 30.75 || ws[2].ResetsAt.Unix() != 1758300000 {
		t.Fatalf("%+v", ws)
	}
	if ws := volcReadCodingPlan(json.RawMessage(`{"QuotaUsage":[]}`)); len(ws) != 0 {
		t.Fatalf("none: %+v", ws)
	}
}

// GetAFPUsage in both forms the tools built on it read: Used of Quota with
// a reset in ms (dsh-ark-quota), and Percent or Used of Total with
// ResetTimestamp (arkcli); a bucket with no quota is not on the plan.
func TestVolcReadAFP(t *testing.T) {
	ws, tier := volcReadAFP(json.RawMessage(`{"AFPFiveHour":{"Quota":100,"Used":42,"ResetTime":1790000000000},
		"AFPDaily":{"Quota":500,"Used":400},"AFPWeekly":{"Quota":0,"Used":5},"AFPMonthly":{"Quota":200,"Used":50}}`))
	if tier != "" || len(ws) != 2 || ws[0].Name != "5 hours" || ws[0].Used != 42 || ws[0].Amount != 42 || ws[0].Limit != 100 ||
		ws[0].ResetsAt == nil || ws[0].ResetsAt.Unix() != 1790000000 || ws[1].Name != "Month" || ws[1].Used != 25 || ws[1].ResetsAt != nil {
		t.Fatalf("Quota form: %+v", ws)
	}
	ws, tier = volcReadAFP(json.RawMessage(`{"Tier":"Pro","AFPFiveHour":{"Percent":12.5,"ResetTimestamp":1790000000},
		"AFPWeekly":{"Used":"30","Total":"120","ResetTime":"2026-10-12T00:00:00Z"}}`))
	if tier != "Pro" || len(ws) != 2 || ws[0].Used != 12.5 || ws[0].ResetsAt.Unix() != 1790000000 ||
		ws[1].Name != "Weekly" || ws[1].Used != 25 || ws[1].ResetsAt.Format(time.RFC3339) != "2026-10-12T00:00:00Z" {
		t.Fatalf("Total form: %+v %q", ws, tier)
	}
}
