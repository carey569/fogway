package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What the sync writes is left as it is where the file says nothing sure of
// another machine: the gateway's address on this machine (another port, or
// localhost) follows the gateway, as does one a WSL distro reached Windows
// at; with two addresses gone, which went where can't be told.
func TestKeptAwayOnlyWhatIsSure(t *testing.T) {
	syncHome(t)
	t.Setenv("FOGWAY_ADDR", "127.0.0.1:3591")
	dir := t.TempDir()
	now := `{"p":{"baseUrl":"http://127.0.0.1:3591/v1","apiKey":"fogway"}}`
	for _, c := range []struct{ name, was, want string }{
		{"nas", `{"p":{"baseUrl":"http://192.168.1.20:3725/v1","apiKey":"nas-key"}}`,
			`{"p":{"baseUrl":"http://192.168.1.20:3725/v1","apiKey":"nas-key"}}`},
		{"nas, fogway's key", `{"p":{"baseUrl":"http://nas.local:3725/v1","apiKey":"fogway"}}`,
			`{"p":{"baseUrl":"http://nas.local:3725/v1","apiKey":"fogway"}}`},
		{"old port", `{"p":{"baseUrl":"http://127.0.0.1:3725/v1","apiKey":"fogway"}}`, now},
		{"localhost", `{"p":{"baseUrl":"http://localhost:3591/v1","apiKey":"other"}}`, now},
		{"two gone", `{"p":{"baseUrl":"http://192.168.1.20:3725/v1","apiKey":"a"},"q":{"baseUrl":"http://192.168.1.21:3725/v1"}}`, now},
		{"wsl", `{"p":{"baseUrl":"http://172.29.0.1:3591/v1","apiKey":"lan"}}`, now},
	} {
		wsl.Lock()
		wsl.seen = map[string]*distro{"Ubuntu": {Gateway: "172.30.0.1", Was: []string{"172.29.0.1"}}}
		wsl.Unlock()
		p := filepath.Join(dir, strings.ReplaceAll(c.name, " ", "-")+".json")
		os.WriteFile(p, []byte(c.was), 0o644)
		if got := string(keptAway(p, []byte(now))); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	// a model of the user's own on this machine (Ollama's) is no sign of
	// fogway here
	p := filepath.Join(dir, "ollama.json")
	os.WriteFile(p, []byte(`{"o":{"baseUrl":"http://localhost:11434/v1"},"p":{"baseUrl":"http://192.168.1.20:3725/v1","apiKey":"nas-key"}}`), 0o644)
	if got, want := string(keptAway(p, []byte(`{"o":{"baseUrl":"http://localhost:11434/v1"},"p":{"baseUrl":"http://127.0.0.1:3591/v1","apiKey":"fogway"}}`))),
		`{"o":{"baseUrl":"http://localhost:11434/v1"},"p":{"baseUrl":"http://192.168.1.20:3725/v1","apiKey":"nas-key"}}`; got != want {
		t.Errorf("beside Ollama: %s, want %s", got, want)
	}
	// Qwen Code writes its key on its own, before the models
	p = filepath.Join(dir, "qwen.json")
	os.WriteFile(p, []byte(`{"env":{"FOGWAY_QWEN_API_KEY":"nas-key"},"m":{"baseUrl":"http://192.168.1.20:3725/v1"}}`), 0o644)
	if got := string(keptAway(p, []byte(`{"env":{"FOGWAY_QWEN_API_KEY":"fogway"},"m":{"baseUrl":"http://192.168.1.20:3725/v1"}}`))); !strings.Contains(got, `"nas-key"`) {
		t.Errorf("the key alone went back to fogway's: %s", got)
	}
	wsl.Lock()
	wsl.seen = nil
	wsl.Unlock()
}

// The keys keptAway carries over are those a file keeps what the agent
// sends the gateway in, in each format fogway writes, and never a model's
// token limit.
func TestKeyValuesFormats(t *testing.T) {
	in := strings.Join([]string{
		`"apiKey": "fogway-qoder",`,
		`api_key = "fogway"`,
		`  apiKey: fogway`,
		`GEMINI_API_KEY=fogway`,
		`"Authorization": "Bearer fogway"`,
		`http_headers = { "x-openai-actor-authorization" = "fogway" }`,
		`"max_tokens": 8192,`,
		`"name": "fogway"`,
		`"token": {"x": 1},`,
	}, "\n")
	var got []string
	for _, kv := range keyValues([]byte(in)) {
		got = append(got, in[kv.start:kv.end])
	}
	want := []string{"fogway-qoder", "fogway", "fogway", "fogway", "Bearer fogway", "fogway"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}
