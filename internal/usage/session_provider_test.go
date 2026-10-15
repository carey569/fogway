package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/sessions"
)

func TestDesktopSessionAccountMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	root := sessions.DesktopDataDirs()[0]
	root3p := sessions.DesktopDataDirs()[1]
	account, org := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "local-agent-mode-sessions", account, org, "local_cowork.json"), `{"cliSessionId":"cowork","emailAddress":"historical@example.com"}`)
	write(filepath.Join(root, "claude-code-sessions", account, org, "local_code.json"), `{"cliSessionId":"desktop-code"}`)
	write(filepath.Join(root3p, "local-agent-mode-sessions", account, org, "local_third.json"), `{"cliSessionId":"third","emailAddress":"third@example.com"}`)
	// Today's unrelated login must not replace the session's recorded email.
	write(filepath.Join(home, ".claude", ".claude.json"), `{"oauthAccount":{"accountUuid":"now","emailAddress":"now@example.com"}}`)
	call := sessions.Call{Time: time.Now(), Agent: "claude-desktop", Session: "cowork", File: filepath.Join(root, "local-agent-mode-sessions", account, org, "local_cowork", ".claude", "projects", "p", "s.jsonl")}
	r := newSessionResolver([]sessions.Call{call})
	if got := r.resolve(call); got != "historical@example.com" {
		t.Fatalf("Cowork identity %+v", got)
	}
	call.File, call.Session = "", "desktop-code"
	if got := r.resolve(call); got != "historical@example.com" {
		t.Fatalf("Code tab account mapping %+v", got)
	}
	call.Session = "third"
	if got := r.resolve(call); got != "third@example.com" {
		t.Fatalf("third-party Desktop misattributed %+v", got)
	}
	call.Agent, call.Session = "claude", "unrelated-cli"
	if got := r.resolve(call); got != "" {
		t.Fatalf("CLI history assigned current login %+v", got)
	}
}

func TestLocalSessionAccountsDoNotEstablishProviders(t *testing.T) {
	account, org := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	r := &sessionResolver{
		desktop: map[string]desktopSessionIdentity{},
		bySession: map[string][]desktopSessionIdentity{
			"mine":     {{account: account, org: org, email: "claude@example.com", linkable: true}},
			"other":    {{account: account, org: "33333333-3333-4333-8333-333333333333", email: "team@example.com", linkable: true}},
			"third-3p": {{account: account, org: org, email: "claude@example.com"}},
		},
	}
	for _, tc := range []struct {
		name string
		call sessions.Call
		want string
	}{
		{"Codex with an account recorded", sessions.Call{Agent: "codex", Upstream: "openai", AccountID: "a-mine", UserID: "u-mine"}, ""},
		{"Codex with no account recorded", sessions.Call{Agent: "codex", Upstream: "openai"}, ""},
		{"Claude Desktop on fogway's account", sessions.Call{Agent: "claude-desktop", Session: "mine"}, "claude@example.com"},
		{"Claude Desktop in another organization", sessions.Call{Agent: "claude-desktop", Session: "other"}, "team@example.com"},
		{"Claude-3p on fogway's account's ids", sessions.Call{Agent: "claude-desktop", Session: "third-3p"}, "claude@example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.resolve(tc.call); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
