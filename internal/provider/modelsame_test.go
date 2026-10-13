package provider

import (
	"slices"
	"testing"
)

// The groups fogway finds merge a model however a vendor spells its id:
// Volcengine Ark's dated deepseek-v4-1-flash-260910 is ali's
// deepseek-v4.1-flash (kyzhouxu, #583). Models that differ stay apart.
func TestSameModelVendorSpellings(t *testing.T) {
	for in, want := range map[string]string{
		// Ark's YYMMDD snapshot
		"deepseek-v4-1-flash-260910":      "deepseek-v4-1-flash",
		"deepseek-v4.1-flash":             "deepseek-v4-1-flash",
		"DeepSeek-V4.1-Flash":             "deepseek-v4-1-flash",
		"deepseek-ai/DeepSeek-V4.1-Flash": "deepseek-v4-1-flash",
		"doubao-seed-1-6-250615":          "doubao-seed-1-6",
		"doubao-seed-1.6-thinking-250715": "doubao-seed-1-6-thinking",
		// Fireworks: its path, and "p" for the dot
		"accounts/fireworks/models/deepseek-v4p1-flash": "deepseek-v4-1-flash",
		"accounts/fireworks/models/qwen2p5-72b":         "qwen2-5-72b",
		// "_" for "-", and Vertex's "@" date
		"deepseek_v4_1_flash":      "deepseek-v4-1-flash",
		"claude-opus-5-5@20260801": "claude-opus-5-5",
		// not a date: kept
		"deepseek-v4-1-flash-128000": "deepseek-v4-1-flash-128000",
		"m-221301":                   "m-221301",
		"m-261301":                   "m-261301",
		"m-260100":                   "m-260100",
		"qwen3-235b-a22b-2507":       "qwen3-235b-a22b-2507",
		"kimi-k2-0905":               "kimi-k2-0905",
		"gpt-4o":                     "gpt-4o",
		"o3-pro":                     "o3-pro",
		"llama-3p":                   "llama-3p",
	} {
		if got := sameModel(in); got != want {
			t.Errorf("sameModel(%q) = %q, want %q", in, got, want)
		}
	}
	// genuinely different models: never one group
	for _, pair := range [][2]string{
		{"deepseek-v4-1-flash-260910", "deepseek-v4.1"},
		{"deepseek-v4-1-260910", "deepseek-v4.1-flash"},
		{"deepseek-v4.1-thinking", "deepseek-v4.1"},
		{"qwen3-235b-a22b-2507", "qwen3-235b-a22b"},
		{"claude-opus-5.5:batch", "claude-opus-5.5"},
		{"deepseek-v4.1-flash", "deepseek-v4.11-flash"},
	} {
		if sameModel(pair[0]) == sameModel(pair[1]) {
			t.Errorf("%s and %s merged as %s", pair[0], pair[1], sameModel(pair[0]))
		}
	}
	e := func(p, m string) Entry { return Entry{ID: p + "/" + m, Model: m, Name: m, Provider: Provider{ID: p}} }
	gs := autoGroups([]Entry{e("volcengine", "deepseek-v4-1-flash-260910"), e("ali-tp", "deepseek-v4.1-flash")}, nil)
	if len(gs) != 1 || gs[0].ID != "auto-deepseek-v4-1-flash" || !slices.Equal(gs[0].Members, []string{"volcengine/deepseek-v4-1-flash-260910", "ali-tp/deepseek-v4.1-flash"}) {
		t.Fatalf("groups: %+v", gs)
	}
}
