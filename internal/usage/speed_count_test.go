package usage

import (
	"testing"
)

// Codex rows tell a floor too (Fallmonkey on Discord): a call in a turn of
// several, timed from what asked for it, by the same rule as Claude Code's;
// a turn of one call that reasoned, whose first text isn't in the file, by
// its answer over the time after its first token, never its whole output
// over it, which counts reasoning written before the first token, when that
// is a second or more and 100 tokens or more; else over the whole turn, by
// the same 1,000-token rule as Claude Code's whole-time floor.
func TestCodexRowsTellAFloor(t *testing.T) {
	row := func(out, reasoning int, ms, ttft int64) Row {
		return Row{Source: "log", Record: Record{Agent: "codex", Model: "gpt-6-luna", Output: out, Reasoning: reasoning, Millis: ms, TTFT: ttft}}
	}
	for _, c := range []struct {
		name string
		r    Row
		n    int
		ms   int64
	}{
		// 272 answer tokens 827 ms after a 10.2 s first token could be one
		// push, and the whole turn, 496 tokens, is under 1,000: none
		{"one call that reasoned, its answer under a second", row(496, 224, 11055, 10228), 0, 0},
		// 1,100 answer tokens 500 ms after an 11 s first token: the whole
		// turn, 1,300 over 11.5 s
		{"one call that reasoned, a long answer in one push", row(1300, 200, 11500, 11000), 1300, 11500},
		{"one call that reasoned, all of it after its first token", row(1200, 200, 12000, 2000), 1000, 10000},
		{"one call that reasoned, an answer of 60 over 3 s", row(260, 200, 5000, 2000), 0, 0},
		{"one call that didn't reason tells its own speed", row(500, 0, 5000, 1000), 0, 0},
		{"one call whose answer came too fast to tell", row(500, 100, 2050, 2000), 0, 0},
		{"one call that only reasoned", row(300, 300, 9000, 2000), 0, 0},
		{"one call that only reasoned, 1,000 tokens", row(1000, 1000, 9000, 2000), 1000, 9000},
		{"a call of several, 1,500 over 30 s", row(1500, 600, 30000, 0), 1500, 30000},
		{"a call of several too short to tell", row(800, 100, 20000, 0), 0, 0},
	} {
		n, ms := c.r.Floor()
		if n != c.n || ms != c.ms {
			t.Errorf("%s: floor %d/%d ms, want %d/%d", c.name, n, ms, c.n, c.ms)
		}
	}
	// through the gateway a Codex row is measured, never a floor
	gw := row(496, 224, 11055, 10228)
	gw.Source = ""
	if n, ms := gw.Floor(); n != 0 || ms != 0 {
		t.Errorf("gateway row: floor %d/%d", n, ms)
	}
}

// Fallmonkey on Discord: an 11-token astra answer over 2.75 s, its window
// after the first token too short, read ≥4 tok/s over the whole turn, while
// an 815-token Claude Code reply, the same formula, told nothing. A Codex
// whole-turn floor has the same 1,000-token gate.
func TestCodexWholeTurnFloorNeedsAThousandTokens(t *testing.T) {
	astra := Row{Source: "log", Record: Record{Agent: "codex", Model: "gpt-6-astra", Output: 11, Millis: 2750, TTFT: 2600}}
	if n, ms := astra.Floor(); n != 0 || ms != 0 {
		t.Errorf("astra's 11 tokens over 2.75 s: floor %d/%d ms, want none", n, ms)
	}
	claude := Row{Source: "log", Record: Record{Agent: "claude", Model: "claude-opus-5-5", Output: 815, Millis: 18000}}
	if n, ms := claude.Floor(); n != 0 || ms != 0 {
		t.Errorf("Claude Code's 815 tokens over 18 s: floor %d/%d ms, want none", n, ms)
	}
	for _, r := range []Row{
		{Source: "log", Record: Record{Agent: "codex", Model: "gpt-6-astra", Output: 1000, Millis: 12000, TTFT: 11800}},
		{Source: "log", Record: Record{Agent: "claude", Model: "claude-opus-5-5", Output: 1000, Millis: 12000}},
	} {
		if n, ms := r.Floor(); n != 1000 || ms != 12000 {
			t.Errorf("%s's 1,000 tokens over 12 s: floor %d/%d ms, want 1000/12000", r.Agent, n, ms)
		}
	}
}
