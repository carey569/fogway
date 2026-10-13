package gateway

import (
	"testing"
)

func TestLateRestsEmptyReply(t *testing.T) {
	if lateRests("Antigravity: an empty reply (the model answered nothing; try again)") {
		t.Errorf("lateRests should be false for empty reply")
	}
	if !lateRests("some generic provider failure") {
		t.Errorf("lateRests should be true for generic provider failure")
	}
}
