package usage

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"slices"
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

func TestModelsNeverEstablishProviderOrAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	at := time.Now()
	logs := []sessions.Call{
		{Time: at, Agent: "claude", Model: "claude-opus-5", Requested: "claude-opus-5[1m]"},
		{Time: at, Agent: "codex", Model: "gpt-6-astra", Upstream: "relay"},
		{Time: at, Agent: "claude", Model: "deepseek-review", Requested: "claude-sonnet-5"},
	}
	rows, _, _, providers := ledgerWith(time.Time{}, Filter{}, nil, logs)
	if len(rows) != len(logs) || len(providers) != 1 || providers[0] != UnknownProvider {
		t.Fatalf("models changed actual provider groups: %v", providers)
	}
	for _, row := range rows {
		if row.Provider != UnknownProvider || row.Host != "" || row.SessionAccount != "" {
			t.Fatalf("model falsely attributed route/account: %+v", row)
		}
	}
	if rows[1].SessionProvider != "relay" || rows[2].Served != "deepseek-review" {
		t.Fatal("recorded upstream or returned model lost")
	}
	for _, official := range []string{"session-openai", "session-anthropic", "codex", "claude"} {
		if known, _, _, _ := ledgerWith(time.Time{}, Filter{Provider: official}, nil, logs); len(known) != 0 {
			t.Fatalf("model-only calls counted as official %s calls", official)
		}
	}
	var out bytes.Buffer
	if err := WriteCSV(&out, rows[1:2]); err != nil {
		t.Fatal(err)
	}
	data, err := csv.NewReader(&out).ReadAll()
	if err != nil || data[1][3] != UnknownProvider || data[1][slices.Index(CSVHeader, "session_provider")] != "relay" || data[1][slices.Index(CSVHeader, "session_account")] != "" || data[1][slices.Index(CSVHeader, "session_official_login")] != "false" {
		t.Fatalf("unconfirmed CSV route misattributed %+v: %v", data, err)
	}
	gateway := []Record{
		{Time: at, Agent: "opencode", Provider: "relay", Host: "relay.example", Model: "gpt-6-astra", Input: 1, Status: 200},
		{Time: at, Agent: "claude", Provider: "third-api", Host: "third.example", Model: "claude-opus-5", Input: 1, Status: 200},
	}
	for _, provider := range []string{"relay", "third-api"} {
		actual, _, _, _ := ledgerWith(time.Time{}, Filter{Provider: provider}, gateway, logs)
		if len(actual) != 1 || actual[0].Provider != provider || actual[0].Host == "" || actual[0].Source != "" {
			t.Fatalf("third-party gateway route changed: %+v", actual)
		}
	}
}

// An OpenCode reply names only the model it was sent to, and the effort
// picked for its prompt (#680): the row's requested model and effort, not a
// served model the file never saw.
func TestOpenCodeSessionCallIsWhatItAskedFor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	logs := []sessions.Call{{Time: time.Now(), Agent: "opencode", Model: "space-bunny-free", Upstream: "opencode", Effort: "high",
		Tokens: sessions.Tokens{Input: 10, Output: 2}}}
	rows, _, _, _ := ledgerWith(time.Time{}, Filter{}, nil, logs)
	if len(rows) != 1 {
		t.Fatalf("rows %+v", rows)
	}
	if r := rows[0]; r.Requested != "space-bunny-free" || r.Model != "space-bunny-free" || r.Served != "" || r.Effort != "high" {
		t.Fatalf("an OpenCode call: requested %q model %q served %q effort %q", r.Requested, r.Model, r.Served, r.Effort)
	}
}
