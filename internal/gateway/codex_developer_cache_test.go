package gateway

import (
	"strconv"
)

// codexItem is a Responses input message, as Codex sends one.
func codexItem(role, text string) string {
	kind := "input_text"
	if role == "assistant" {
		kind = "output_text"
	}
	return `{"type":"message","role":"` + role + `","content":[{"type":"` + kind + `","text":` + strconv.Quote(text) + `}]}`
}
