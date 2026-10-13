package gateway

import (
	"strings"
)

func drawingTurnID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 128 || strings.ContainsAny(id, "\r\n\t") {
		return ""
	}
	return id
}
