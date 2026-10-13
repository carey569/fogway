package provider

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func fakeJWT(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "h." + base64.RawURLEncoding.EncodeToString(b) + ".sig"
}

// isolate leaves a test none of the plans' and keys' cards another test
// read: PlanQuotas and KeyBalances keep them a minute.
func isolate(t *testing.T) {
	t.Helper()
	forgetPlanQuotas()
	ForgetBalances()
}
