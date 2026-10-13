package agent

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// agentFiles reads every file under home but magpie's own (its settings,
// stash and caches): what an agent reads its config from.
func agentFiles(t *testing.T, home string) map[string]string {
	t.Helper()
	files := map[string]string{}
	filepath.WalkDir(home, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(home, p)
		if e.IsDir() {
			if rel == filepath.Join(".config", "magpie") || rel == ".cache" {
				return filepath.SkipDir
			}
			return nil
		}
		b, _ := os.ReadFile(p)
		files[rel] = string(b)
		return nil
	})
	return files
}

// codexChatGPTAuth is a Codex auth.json signed in to ChatGPT as email.
func codexChatGPTAuth(email, acct string) map[string]any {
	claims := func(m map[string]any) string {
		b, _ := json.Marshal(m)
		return "h." + base64.RawURLEncoding.EncodeToString(b) + ".s"
	}
	return map[string]any{"auth_mode": "chatgpt", "tokens": map[string]any{
		"id_token":      claims(map[string]any{"email": email}),
		"access_token":  claims(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}),
		"refresh_token": "r-" + acct, "account_id": acct}}
}
