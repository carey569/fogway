package gateway

import (
	"testing"
)

// factoryStandardOut is Factory's refusal for Standard's week used up, as
// #1979 quotes it, with the 402 it comes with.
const factoryStandardOut = `{"error":{"message":"Factory: You've reached your weekly standard usage limit (resets in 0h 19min). Switch to Droid Core or enable Extra Usage to continue."}}`

// A 402 is out of quota when it speaks of a usage limit and nothing of
// money, and out of credit otherwise.
func TestPaymentRequiredReadByItsWords(t *testing.T) {
	for _, c := range []struct {
		body, want string
	}{
		{factoryStandardOut, failQuota},
		{`{}`, failCredit},
		{`{"error":{"message":"Insufficient Balance"}}`, failCredit},
		{`{"error":{"message":"You have reached the limit.","reason":"MONTHLY_REQUEST_COUNT"}}`, failCredit},
		{`{"error":{"message":"usage limit reached, please top up your credit"}}`, failCredit},
		{`{"error":{"code":"insufficient_quota","message":"You exceeded your current quota"}}`, failCredit},
		{`Command Code: you have used this month's credits`, failCredit},
	} {
		if got := failure(402, []byte(c.body)); got != c.want {
			t.Errorf("402 %s: %s, want %s", c.body, got, c.want)
		}
	}
}
