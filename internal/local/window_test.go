package local

import (
	"strings"
	"testing"
)

// The memory a window takes is what llama-server took with it: the file
// and its footprint once healthy (b11583, Metal, --parallel 2
// --kv-unified, the way serve runs it), with no more than 0.4 GB to spare.
func TestNeedAtMatchesLlamaServer(t *testing.T) {
	const mib = 1 << 20
	for _, c := range []struct {
		m         Model
		n         int
		footprint uint64
	}{
		{Spark17, 32 << 10, 660 * mib},
		{Spark17, 512 << 10, 7405 * mib},
		{Spark4, 32 << 10, 1485 * mib},
		{Spark4, 128 << 10, 4951 * mib},
		{Spark4, 256 << 10, 9540 * mib},
	} {
		took := c.m.Size + c.footprint
		if need := c.m.NeedAt(c.n); need < took || need > took+400_000_000 {
			t.Errorf("%s at %s: need %s, llama-server took %s", c.m.Title, Tokens(c.n), gb(need), gb(took))
		}
	}
	if n := (Model{Repo: "a/b", File: "c.gguf", Size: 1 << 30}).NeedAt(32 << 10); n != 0 {
		t.Errorf("a model whose cache is unknown needs %d", n)
	}
}

// Each model is offered the windows it was trained for, up to 1M, with
// the memory each takes; the ones a computer can't hold can't be picked,
// the way the 1.7B's 1M ran out of the M4 Pro's GPU memory once loaded.
func TestWindows(t *testing.T) {
	m4pro := Hardware{OS: "darwin", Arch: "arm64", RAM: 24 * gib, Unified: true}
	m1 := Hardware{OS: "darwin", Arch: "arm64", RAM: 8 * gib, Unified: true}
	rtx := Hardware{OS: "windows", Arch: "amd64", RAM: 16 * gib, NVIDIA: true, VRAM: 12 * gib}
	qwen2, _ := CatalogModel("qwen3.5-2b")
	custom := Model{Repo: "someone/Their-GGUF", File: "their.gguf"}
	odd := Model{Repo: "someone/Odd-GGUF", File: "odd.gguf", Size: 1 << 30, Window: 40 << 10, KV: 16 << 10}
	room := func(ws []Window) string {
		var out []string
		for _, w := range ws {
			out = append(out, Tokens(w.Tokens)+":"+string(w.Room))
		}
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		name string
		m    Model
		h    Hardware
		want string
	}{
		{"the 4B on an M4 Pro", Spark4, m4pro, "8K: 16K: 32K: 64K: 128K: 256K:tight 512K:memory 1M:memory"},
		{"the 1.7B on an M4 Pro", Spark17, m4pro, "8K: 16K: 32K: 64K: 128K: 256K: 512K:tight 1M:memory"},
		{"the 1.7B on 8 GB", Spark17, m1, "8K: 16K: 32K: 64K: 128K:tight 256K:memory 512K:memory 1M:memory"},
		{"the 4B on a 12 GB card", Spark4, rtx, "8K: 16K: 32K: 64K: 128K: 256K:memory 512K:memory 1M:memory"},
		{"Qwen3.5, trained for 256K", qwen2, m4pro, "8K: 16K: 32K: 64K: 128K: 256K:"},
		{"a model of the user's own", custom, m4pro, "8K: 16K: 32K: 64K: 128K:"},
		{"one trained for 40K", odd, m4pro, "8K: 16K: 32K: 40K:"},
	} {
		ws := Windows(c.m, c.h)
		if got := room(ws); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
		for i, w := range ws {
			if known := c.m.KV > 0 || Canon(c.m).KV > 0; known != (w.Need > 0) || i > 0 && w.Need < ws[i-1].Need {
				t.Errorf("%s: %+v", c.name, ws)
			}
		}
	}
	if err := FitWindow(Spark4, m4pro, 256<<10); err != nil {
		t.Error(err)
	}
	if err := FitWindow(Spark4, m4pro, 1<<20); err == nil || !strings.Contains(err.Error(), "1M") {
		t.Errorf("the 4B at 1M on 24 GB: %v", err)
	}
	if err := FitWindow(qwen2, m4pro, 512<<10); err == nil || !strings.Contains(err.Error(), "256K") {
		t.Errorf("Qwen3.5 past 256K: %v", err)
	}
}

// The window run at is the one set, cut to what the model was trained for
// and to what memory holds with it: one set for the 1.7B and kept when the
// 4B took over runs at the most the 4B holds.
func TestContextOr(t *testing.T) {
	m4pro := Hardware{OS: "darwin", Arch: "arm64", RAM: 24 * gib, Unified: true}
	qwen2, _ := CatalogModel("qwen3.5-2b")
	custom := Model{Repo: "someone/Their-GGUF", File: "their.gguf"}
	for _, c := range []struct {
		name string
		c    Config
		want int
	}{
		{"none set", Config{Model: Spark4}, DefaultContext},
		{"set", Config{Model: Spark4, Context: 128 << 10}, 128 << 10},
		{"past the model's own", Config{Model: qwen2, Context: 1 << 20}, 256 << 10},
		{"past the memory", Config{Model: Spark4, Context: 512 << 10}, 256 << 10},
		{"between two windows, past the memory", Config{Model: Spark4, Context: 400_000}, 256 << 10},
		{"the 1.7B holds it", Config{Model: Spark17, Context: 512 << 10}, 512 << 10},
		{"a model saved before windows were known", Config{Model: Model{Repo: Spark4.Repo, File: Spark4.File}, Context: 1 << 20}, 256 << 10},
		{"a model of the user's own", Config{Model: custom, Context: 1 << 20}, 1 << 20},
	} {
		if got := c.c.contextOn(m4pro); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, Tokens(got), Tokens(c.want))
		}
	}
}

func TestTokens(t *testing.T) {
	for n, want := range map[int]string{8 << 10: "8K", 256 << 10: "256K", 1 << 20: "1M", 40960: "40K", 300_000: "300000"} {
		if got := Tokens(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
}
