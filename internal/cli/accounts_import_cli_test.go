package cli

import (
	"encoding/base64"
	"encoding/json"
)

// importJWT is an unsigned JWT carrying claims, as ChatGPT's tokens are read.
func importJWT(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "e30." + base64.RawURLEncoding.EncodeToString(b) + ".sig"
}
