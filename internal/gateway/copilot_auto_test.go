package gateway

import (
	"encoding/json"
)

func mustString(n int64) string { b, _ := json.Marshal(n); return string(b) }
