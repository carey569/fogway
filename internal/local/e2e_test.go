package local

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fogway/fogway/internal/provider"
)

// TestE2E downloads llama.cpp for this computer and a tiny model from
// Hugging Face, runs llama-server, and asks it, as the gateway would. It
// goes to the network and takes a minute: run with FOGWAY_E2E=1, and
// FOGWAY_E2E_MODEL=<repo>:<file> for a model that chats (the default, a
// 1 MB model, only proves the plumbing).
func TestE2E(t *testing.T) {
	if os.Getenv("FOGWAY_E2E") == "" {
		t.Skip("FOGWAY_E2E=1 runs it: it downloads llama.cpp and a model")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if d := os.Getenv("FOGWAY_E2E_CACHE"); d != "" {
		t.Setenv("XDG_CACHE_HOME", d) // downloads kept between runs
	}
	m := Model{Repo: "ggml-org/test-model-stories260K", File: "stories260K-f32.gguf", Name: "stories"}
	if v := os.Getenv("FOGWAY_E2E_MODEL"); v != "" {
		repo, file, _ := strings.Cut(v, ":")
		m = Model{Repo: repo, File: file}
	}
	c := Config{On: true, Model: m, Context: 2048, HFEndpoint: os.Getenv("HF_ENDPOINT")}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour) // a real model at a slow line
	defer cancel()
	path, err := Prepare(ctx, c, func(p Progress) { t.Logf("%s %d/%d", p.What, p.Done, p.Total) })
	if err != nil {
		t.Fatal(err)
	}
	t.Log("model at", path)

	Start(ctx, c)
	defer Stop()
	deadline := time.Now().Add(3 * time.Minute)
	for {
		s := Now()
		if s.Phase == Ready {
			t.Logf("ready at %s (%s %s)", s.URL, s.Build, s.Backend)
			break
		}
		if s.Phase == Failed || time.Now().After(deadline) {
			b, _ := os.ReadFile(filepath.Join(Dir(), "llama-server.log"))
			t.Fatalf("not ready: %+v\n%s", s, tail(string(b), 3000))
		}
		time.Sleep(300 * time.Millisecond)
	}
	s := Now()

	// the provider follows it
	var p *provider.Provider
	for range 20 {
		if p, err = provider.Find(ProviderID); err == nil && p.Chat == s.URL {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if p == nil || p.Chat != s.URL || len(p.Models) != 1 || p.Models[0] != m.ID() || p.Contexts[m.ID()] != 2048 {
		t.Fatalf("provider %+v, want local at %s serving %s at 2048", p, s.URL, m.ID())
	}

	// and it answers
	body := `{"model":"` + m.ID() + `","messages":[{"role":"user","content":"Once upon a time"}],"max_tokens":16}`
	res, err := http.Post(s.URL+"/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if res.StatusCode != 200 || json.Unmarshal(b, &out) != nil || len(out.Choices) == 0 {
		t.Fatalf("%d %s", res.StatusCode, b)
	}
	t.Logf("it said %q", out.Choices[0].Message.Content)
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
