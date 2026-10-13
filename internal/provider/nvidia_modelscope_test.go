package provider

import (
	"testing"
)

// NVIDIA NIM and 魔搭 ModelScope are presets (#197): chat completions (and
// Responses at ModelScope), no Anthropic endpoint, their models.dev
// catalogs, and an entry imported at their base URLs is the preset.
func TestNvidiaModelScopePresets(t *testing.T) {
	for _, c := range []struct {
		id, name, icon, catalog, chat, responses, keys string
	}{
		{"nvidia", "NVIDIA NIM", "nvidia-color", "nvidia", "https://integrate.api.nvidia.com/v1", "", "https://build.nvidia.com/settings/api-keys"},
		{"modelscope", "ModelScope", "modelscope-color", "modelscope", "https://api-inference.modelscope.cn/v1", "https://api-inference.modelscope.cn/v1", "https://modelscope.cn/my/myaccesstoken"},
	} {
		p, err := FromPreset(c.id)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name != c.name || p.Icon != c.icon || p.Catalog != c.catalog || p.KeysURL != c.keys {
			t.Fatalf("%s: %+v", c.id, p)
		}
		if p.Chat != c.chat || p.Responses != c.responses || p.Anthropic != "" {
			t.Fatalf("%s endpoints: %q %q %q", c.id, p.Chat, p.Responses, p.Anthropic)
		}
		if pr := Preset(c.id); pr.Kind != KindRelay || pr.NoKey {
			t.Fatalf("%s: %+v", c.id, pr)
		}
		im, _ := imported("x", "k", endpoints{chat: c.chat + "/"}, nil)
		if im.Preset != c.id || im.Icon != c.icon {
			t.Fatalf("%s imported: %+v", c.id, im)
		}
	}
	// ModelScope is not DashScope: the Qwen presets stay where they were
	if Preset("qwen-cn").Chat != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatal("qwen-cn moved")
	}
}
