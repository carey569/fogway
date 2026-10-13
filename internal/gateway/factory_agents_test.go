package gateway

// openingOf is the text a Factory body opens with, on whichever wire it took.
func openingOf(m map[string]any) string {
	if s, ok := m["instructions"].(string); ok {
		return s
	}
	switch s := m["system"].(type) {
	case string:
		return s
	case []any:
		if len(s) > 0 {
			b, _ := s[0].(map[string]any)
			t, _ := b["text"].(string)
			return t
		}
	}
	if si, ok := m["systemInstruction"].(map[string]any); ok {
		parts, _ := si["parts"].([]any)
		if len(parts) > 0 {
			b, _ := parts[0].(map[string]any)
			t, _ := b["text"].(string)
			return t
		}
	}
	msgs, _ := m["messages"].([]any)
	if len(msgs) > 0 {
		msg, _ := msgs[0].(map[string]any)
		if msg["role"] == "system" {
			switch c := msg["content"].(type) {
			case string:
				return c
			case []any:
				if len(c) > 0 {
					b, _ := c[0].(map[string]any)
					t, _ := b["text"].(string)
					return t
				}
			}
		}
	}
	return ""
}
