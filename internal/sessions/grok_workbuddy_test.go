package sessions

import (
	"encoding/json"
	"testing"
)

const (
	grokMain = "0190aaaa-0000-7000-8000-00000000000a"
	grokFork = "0190aaaa-0000-7000-8000-00000000000c"
	wbID     = "22222222-aaaa-4bbb-8ccc-000000000001"
	wbOldID  = "0123456789abcdef0123456789abcdef"
)

func TestWorkBuddyPrompt(t *testing.T) {
	for in, want := range map[string]string{
		`[{"type":"input_text","text":"<system-reminder>x</system-reminder>"},{"type":"input_text","text":"@scene:4"},{"type":"input_text","text":"Fix   it"}]`: "Fix it",
		`"plain words"`:                                "plain words",
		`"<command>x</command>"`:                       "",
		`[{"type":"input_image","image_url":"data:"}]`: "",
	} {
		if got := wbPrompt(json.RawMessage(in)); got != want {
			t.Errorf("wbPrompt(%s) = %q, want %q", in, got, want)
		}
	}
}
