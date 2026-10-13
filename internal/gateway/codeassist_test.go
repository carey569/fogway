package gateway

import (
	"encoding/json"
	"reflect"
	"testing"
)

func codeAssistRequest(t *testing.T) *Request {
	t.Helper()
	r, err := parseAnthropic([]byte(`{"model":"x","max_tokens":2000,"system":"be brief","stream":true,
		"thinking":{"type":"enabled","budget_tokens":4000},
		"tools":[{"name":"read","description":"read a file","input_schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object",
			"properties":{"path":{"type":["string","null"],"format":"uri"},"mode":{"const":"r"},"opts":{"$ref":"#/$defs/Opts"},
			"n":{"anyOf":[{"type":"integer"},{"type":"null"}]}},"required":["path","gone"],"additionalProperties":false,
			"$defs":{"Opts":{"type":"object","properties":{"deep":{"type":"boolean","default":false}}}}}}],
		"messages":[
			{"role":"user","content":"read a"},
			{"role":"assistant","content":[{"type":"thinking","thinking":"hm","signature":"sig-from-claude"},
				{"type":"text","text":"ok"},{"type":"tool_use","id":"toolu_01:x","name":"read","input":{"path":"a"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01:x","content":"A!"}]},
			{"role":"user","content":"and?"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCodeAssistDecoder(t *testing.T) {
	var got []Event
	d := &codeAssistDecoder{}
	for _, line := range []string{
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"let me see","thought":true}]}}],"modelVersion":"gemini-2.5-pro","responseId":"r1"},"traceId":"t"}`,
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Reading."}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":3}}}`,
		`{"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read","args":{"path":"a"}},"thoughtSignature":"abc"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":10,"thoughtsTokenCount":20,"cachedContentTokenCount":40}}}`,
	} {
		d.decode(line, func(ev Event) { got = append(got, ev) })
	}
	kinds := []EventKind{KStart, KThink, KText, KToolStart, KToolArgs, KStop, KUsage}
	if len(got) != len(kinds) {
		t.Fatalf("events = %+v", got)
	}
	for i, k := range kinds {
		if got[i].Kind != k {
			t.Fatalf("event %d = %+v, want kind %d", i, got[i], k)
		}
	}
	if got[0].Model != "gemini-2.5-pro" || got[3].Name != "read" || got[3].ID == "" || got[4].Text != `{"path":"a"}` {
		t.Errorf("events = %+v", got)
	}
	if got[5].Stop != "tool" {
		t.Errorf("stop = %q", got[5].Stop)
	}
	if u := got[6].Usage; u != (Usage{Input: 60, CacheRead: 40, Output: 30, Reasoning: 20}) {
		t.Errorf("usage = %+v", u)
	}

	var errs []Event
	(&codeAssistDecoder{}).decode(`{"error":{"code":429,"message":"quota"}}`, func(ev Event) { errs = append(errs, ev) })
	if len(errs) != 1 || errs[0].Kind != KError || errs[0].Text != "quota" {
		t.Errorf("error events = %+v", errs)
	}
}

// A call Antigravity's model writes out in its reply's text,
// call:default_api:<tool>{...}, is the call it meant, however the stream
// cuts it up, and a call named with Gemini's namespace is the client's
// tool (#636). Text that only looks like one stays text.
func TestCodeAssistTextCalls(t *testing.T) {
	chunk := func(parts string, finish string) string {
		f := ""
		if finish != "" {
			f = `,"finishReason":"` + finish + `"`
		}
		return `{"response":{"candidates":[{"content":{"role":"model","parts":[` + parts + `]}` + f + `}]}}`
	}
	text := func(s string) string { b, _ := json.Marshal(map[string]string{"text": s}); return string(b) }
	type call struct{ name, args string }
	for _, c := range []struct {
		name   string
		chunks []string
		text   string
		calls  []call
		stop   string
	}{
		{"whole", []string{chunk(text("Let's see what `shims/three.ts` imports!call:default_api:bash{command:cat /tmp/w/shims/three.ts}"), "STOP")},
			"Let's see what `shims/three.ts` imports!", []call{{"bash", `{"command":"cat /tmp/w/shims/three.ts"}`}}, "tool"},
		{"cut up", []string{chunk(text("And lines 240-270:cal"), ""), chunk(text("l:default_api:ba"), ""),
			chunk(text(`sh{command:sed -n '241,275p' a.ts}`), ""), chunk(text(""), "STOP")},
			"And lines 240-270:", []call{{"bash", `{"command":"sed -n '241,275p' a.ts"}`}}, "tool"},
		{"quoted", []string{chunk(text(`call:default_api:edit{path:<ctrl46>a.go<ctrl46>,new:<ctrl46>func f() {<ctrl46>,n:2}`), "STOP")},
			"", []call{{"edit", `{"path":"a.go","new":"func f() {","n":2}`}}, "tool"},
		{"json", []string{chunk(text(`ok.call:default_api:read{"path":"a","limit":3}`), "STOP")},
			"ok.", []call{{"read", `{"path":"a","limit":3}`}}, "tool"},
		{"named", []string{chunk(`{"functionCall":{"name":"default_api:bash","args":{"command":"ls"}}}`, "STOP")},
			"", []call{{"bash", `{"command":"ls"}`}}, "tool"},
		{"unfinished", []string{chunk(text("look call:default_api:bash{command:ls"), "MAX_TOKENS")},
			"look call:default_api:bash{command:ls", nil, "length"},
		{"not a call", []string{chunk(text("say call:default_api: or recall"), ""), chunk(text(" ca"), "STOP")},
			"say call:default_api: or recall ca", nil, "stop"},
	} {
		var col collector
		dec := (&codeAssistDecoder{}).decode
		for _, ch := range c.chunks {
			dec(ch, col.add)
		}
		res := col.finish()
		var got string
		var calls []call
		for _, p := range res.Parts {
			switch p.Kind {
			case Text:
				got += p.Text
			case ToolCall:
				calls = append(calls, call{p.Name, string(p.Args)})
			}
		}
		if got != c.text || res.Stop != c.stop || len(calls) != len(c.calls) {
			t.Errorf("%s: text %q, calls %v, stop %q; want %q, %v, %q", c.name, got, calls, res.Stop, c.text, c.calls, c.stop)
			continue
		}
		for i, cl := range calls {
			var a, b any
			json.Unmarshal([]byte(cl.args), &a)
			json.Unmarshal([]byte(c.calls[i].args), &b)
			if cl.name != c.calls[i].name || !reflect.DeepEqual(a, b) {
				t.Errorf("%s: call %v, want %v", c.name, cl, c.calls[i])
			}
		}
	}
}
