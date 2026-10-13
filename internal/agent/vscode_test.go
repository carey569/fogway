package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
)

// its requests come with GitHubCopilotChat/<version>
func TestVSCodeUA(t *testing.T) {
	if got := usage.AgentOf("GitHubCopilotChat/0.69.0"); got != "vscode" {
		t.Fatalf("%q", got)
	}
}

// VS Code Insiders is its own row (wani on Discord): its User folder is
// "Code - Insiders" beside VS Code's "Code", connected it writes only there,
// and its models send a token of its own, since its chat's User-Agent is
// Stable's; VS Code's files are left alone
func TestVSCodeInsiders(t *testing.T) {
	home := syncHome(t)
	ins, err := Find("vscode-insiders")
	if err != nil {
		t.Fatal(err)
	}
	st, err := Find("vscode")
	if err != nil {
		t.Fatal(err)
	}
	if ins.Name != "VS Code Insiders" || ins.Bin != "code-insiders" || filepath.Base(filepath.Dir(ins.Dir)) != "Code - Insiders" || filepath.Base(ins.Dir) != "User" ||
		filepath.Dir(filepath.Dir(ins.Dir)) != filepath.Dir(filepath.Dir(st.Dir)) || ins.UA != nil {
		t.Fatalf("insiders: %+v\nstable dir %s", ins, st.Dir)
	}
	if !strings.HasPrefix(ins.Dir, home) {
		t.Fatalf("insiders dir %s outside %s", ins.Dir, home)
	}
	for _, n := range []string{"code-insiders", "vs-code-insiders"} {
		if a, err := Find(n); err != nil || a.ID != "vscode-insiders" {
			t.Fatalf("Find(%q): %v %v", n, a, err)
		}
	}
	writeFile(t, ins.Path, "{}\n")
	writeFile(t, st.Path, `{"chat.defaultModel": "gpt-5"}`+"\n")
	if !ins.Detected() {
		t.Fatal("not detected")
	}
	if err := ins.Connect(); err != nil {
		t.Fatal(err)
	}
	g, ok := edit.GetJSONItem(filepath.Join(ins.Dir, "chatLanguageModels.json"), vscodeGroup)
	if !ok || gjson.Get(g, "models.0.requestHeaders.Authorization").String() != "Bearer magpie-vscode-insiders" || !ins.Wired() || ins.Check() != "" {
		t.Fatalf("connected: %s %v %q", g, ins.Wired(), ins.Check())
	}
	if readFile(st.Path) != `{"chat.defaultModel": "gpt-5"}`+"\n" || isFile(filepath.Join(st.Dir, "chatLanguageModels.json")) || st.Wired() {
		t.Fatalf("VS Code touched:\n%s", readFile(st.Path))
	}
	// its requests are its own, Stable's still Stable's
	if got := usage.AgentOf("vscode-insiders"); got != "vscode-insiders" {
		t.Fatalf("token: %q", got)
	}
	if got := usage.AgentOf("GitHubCopilotChat/0.69.0"); got != "vscode" {
		t.Fatalf("UA: %q", got)
	}
	if err := ins.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if _, ok := edit.GetJSONItem(filepath.Join(ins.Dir, "chatLanguageModels.json"), vscodeGroup); ok || ins.Wired() {
		t.Fatal("disconnected, magpie's group kept")
	}
}

func TestVSCodiumModelsUseTheirOwnVisibility(t *testing.T) {
	syncHome(t)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"visible", "hidden"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetHiddenModels("vscodium", []string{"relay/hidden"}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetHiddenModels("vscode", []string{"relay/visible"}); err != nil {
		t.Fatal(err)
	}
	has := func(k vscodeKind, id string) bool {
		b, err := json.Marshal(vscodeGroupJSON(k, vscodeEndpoint))
		if err != nil {
			t.Fatal(err)
		}
		return gjson.GetBytes(b, "models.#(id==\""+id+"\")").Exists()
	}
	if has(vscodiumKind, "relay/hidden") || !has(vscodiumKind, "relay/visible") {
		t.Fatal("VSCodium did not use its own hidden-model list")
	}
	if !has(vscodeStable, "relay/hidden") || has(vscodeStable, "relay/visible") {
		t.Fatal("VS Code did not retain its own hidden-model list")
	}
}

// copilotChat0481Providers is contributes.languageModelChatProviders of
// the Marketplace's GitHub Copilot Chat 0.48.1, the last one VSCodium can
// install (its configuration schemas left out): an OpenAI Compatible
// provider, no Custom Endpoint.
const copilotChat0481Providers = `[{"vendor": "copilot", "displayName": "Copilot"}, {"vendor": "copilotcli", "displayName": "Copilot CLI", "when": "false"}, {"vendor": "claude-code", "displayName": "Claude Code", "when": "false"}, {"vendor": "anthropic", "displayName": "Anthropic"}, {"vendor": "xai", "displayName": "xAI"}, {"vendor": "gemini", "displayName": "Google"}, {"vendor": "openrouter", "displayName": "OpenRouter"}, {"vendor": "openai", "displayName": "OpenAI"}, {"vendor": "ollama", "displayName": "Ollama"}, {"vendor": "customoai", "when": "productQualityType != 'stable'", "displayName": "OpenAI Compatible"}, {"vendor": "azure", "displayName": "Azure"}]`

// installCopilotChat installs a Copilot Chat of a version declaring
// providers in VSCodium's extensions folder under home, listed in
// extensions.json as VSCodium lists it.
func installCopilotChat(t *testing.T, home, version, providers string) {
	t.Helper()
	exts := filepath.Join(home, ".vscode-oss", "extensions")
	folder := "github.copilot-chat-" + version
	writeFile(t, filepath.Join(exts, folder, "package.json"), `{"name":"copilot-chat","publisher":"GitHub","version":"`+version+`","contributes":{"languageModelChatProviders":`+providers+`}}`)
	writeFile(t, filepath.Join(exts, "extensions.json"), `[{"identifier":{"id":"github.copilot-chat","uuid":"7ec7d6e6-b89e-4cc5-a59b-d6c4d238246f"},"version":"`+version+`","location":{"$mid":1,"path":"/x","scheme":"file"},"relativeLocation":"`+folder+`","metadata":{"source":"gallery"}}]`)
}

func TestVSCodeChatInstalledWithoutExtensionsJSON(t *testing.T) {
	dir := t.TempDir()
	if c := vscodeChatOf(filepath.Join(dir, "missing")); !c.none || c.vendor != vscodeEndpoint {
		t.Fatalf("missing: %+v", c)
	}
	writeFile(t, filepath.Join(dir, "github.copilot-chat-0.9.0", "package.json"), `{"contributes":{"languageModelChatProviders":[{"vendor":"customendpoint"}]}}`)
	writeFile(t, filepath.Join(dir, "github.copilot-chat-0.48.1", "package.json"), `{"contributes":{"languageModelChatProviders":`+copilotChat0481Providers+`}}`)
	writeFile(t, filepath.Join(dir, "github.copilot-chat-0.50.0", "package.json"), `{"contributes":{"languageModelChatProviders":[{"vendor":"customendpoint"}]}}`)
	writeFile(t, filepath.Join(dir, ".obsolete"), `{"github.copilot-chat-0.50.0":true}`)
	if c := vscodeChatOf(dir); c.vendor != vscodeOAI || c.version != "0.48.1" {
		t.Fatalf("got %+v", c)
	}
	writeFile(t, filepath.Join(dir, "github.copilot-chat-0.48.1", "package.json"), `{not json`)
	if c := vscodeChatOf(dir); c.vendor != vscodeEndpoint || c.none || c.unsupported {
		t.Fatalf("unreadable: %+v", c)
	}
}
