package gateway

import (
	"encoding/json"
)

// codexPlanModels are the ChatGPT backend's /codex/models entries, as it
// lists them (slug, display_name, priority, levels, windows): a Pro 5x
// ("prolite") plan's eight, of which a Free plan lists the three below.
var codexPlanModels = []struct {
	slug, name string
	free       bool
	levels     []string
}{
	{"gpt-6.1-sol", "GPT-6.1-Sol", false, []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
	{"gpt-6-astra", "GPT-6-Astra", false, []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
	{"gpt-6-sol", "GPT-6-Sol", false, []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
	{"gpt-6-luna", "GPT-6-Luna", true, []string{"low", "medium", "high", "xhigh", "max"}},
	{"gpt-5.6-sol", "GPT-5.6-Sol", false, []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
	{"gpt-5.6-terra", "GPT-5.6-Terra", true, []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
	{"gpt-5.6-luna", "GPT-5.6-Luna", true, []string{"low", "medium", "high", "xhigh", "max"}},
	{"gpt-5.5", "GPT-5.5", false, []string{"low", "medium", "high", "xhigh"}},
}

func codexPlanList(free bool) string {
	var out []map[string]any
	for i, m := range codexPlanModels {
		if free && !m.free {
			continue
		}
		var ls []map[string]any
		for _, l := range m.levels {
			ls = append(ls, map[string]any{"effort": l, "description": ""})
		}
		out = append(out, map[string]any{"slug": m.slug, "display_name": m.name, "visibility": "list",
			"priority": i + 1, "supported_reasoning_levels": ls, "default_reasoning_level": "medium",
			"input_modalities": []string{"text", "image"}, "context_window": 272000, "max_context_window": 872000,
			"supported_in_api": true, "upgrade": nil})
	}
	b, _ := json.Marshal(map[string]any{"models": out})
	return string(b)
}
