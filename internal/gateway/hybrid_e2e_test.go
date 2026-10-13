package gateway

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fogway/fogway/internal/catalog"
	lm "github.com/fogway/fogway/internal/local"
	"github.com/fogway/fogway/internal/provider"
)

// TestEdgeCloudE2E runs group/auto with a real model on this computer as
// its judge and its masker, in front of a cloud that says back what it was
// sent: FOGWAY_E2E=1, FOGWAY_E2E_MODEL=<repo>:<file> (a model that chats),
// FOGWAY_E2E_CACHE=<folder> to keep the downloads. The decisions are a
// small model's and are logged; what is held to is what masking promises:
// the cloud never sees the secrets, and the agent gets them back.
func TestEdgeCloudE2E(t *testing.T) {
	model := os.Getenv("FOGWAY_E2E_MODEL")
	if os.Getenv("FOGWAY_E2E") == "" || model == "" {
		t.Skip("FOGWAY_E2E=1 FOGWAY_E2E_MODEL=<repo>:<file> runs it")
	}
	fresh(t)
	if d := os.Getenv("FOGWAY_E2E_CACHE"); d != "" {
		t.Setenv("XDG_CACHE_HOME", d)
	}
	repo, file, _ := strings.Cut(model, ":")
	c := lm.Config{On: true, Model: lm.Model{Repo: repo, File: file}, Context: 16384, HFEndpoint: os.Getenv("HF_ENDPOINT")}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	if _, err := lm.Prepare(ctx, c, func(lm.Progress) {}); err != nil {
		t.Fatal(err)
	}

	cloud := &edgeUp{answer: func(b string) string { return "you said: " + lastUser(b) }}
	srv := httptest.NewServer(cloud)
	defer srv.Close()
	if err := provider.Save(provider.Provider{ID: "cloud", Name: "Cloud", Key: "k", Models: []string{"big"}, Chat: srv.URL + "/v1"}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("cloud", srv.URL+"/v1", []catalog.Model{{ID: "big", Context: 200000}}); err != nil {
		t.Fatal(err)
	}

	lm.Start(ctx, c)
	defer lm.Stop()
	for deadline := time.Now().Add(5 * time.Minute); lm.Now().Phase != lm.Ready; time.Sleep(300 * time.Millisecond) {
		if s := lm.Now(); s.Phase == lm.Failed || time.Now().After(deadline) {
			b, _ := os.ReadFile(filepath.Join(lm.Dir(), "llama-server.log"))
			t.Fatalf("not ready: %+v\n%s", s, b)
		}
	}
	// the gateway makes group/auto as the model serves: over it and the cloud
	var g provider.Group
	for range 50 {
		var ok bool
		if g, _, ok = provider.FindGroup("group/auto"); ok && g.Hybrid != nil && len(g.Members) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if g.Hybrid == nil || g.Members[1] != "cloud/big" {
		t.Fatalf("group/auto: %+v", g)
	}
	g.Hybrid.Mask = true
	if err := provider.SaveGroup(g); err != nil {
		t.Fatal(err)
	}
	t.Logf("local model %s at %s; group/auto %v", g.Hybrid.Local, lm.Now().URL, g.Members)

	s := New()
	secrets := []string{"Tr0ub4dor&3", "sk-proj-" + strings.Repeat("Zx9", 16)}
	for i, turn := range []string{
		"hi! what's 2 + 2?",
		"Rename the variable `cnt` to `count` in this line: for cnt := 0; cnt < n; cnt++ {",
		"Design a multi-region, event-sourced billing system for 50 microservices: the architecture, the data model, the migration plan from our monolith, and a full implementation of the ledger service in Go with tests.",
		"Our production database is db-prod-07.corp.acme.cn, user svc_billing, password " + secrets[0] + ", and the OpenAI key is " + secrets[1] + ". Redesign the whole billing architecture of our company around event sourcing, with a migration plan.",
		"my password is " + secrets[0] + " — fix the typo in this comment: // retun the value",
	} {
		before := len(cloud.turns())
		out, r := postOK(t, s, "e2e-"+string(rune('a'+i)), edgeAsk(turn))
		h := r.Rule.Hybrid
		where := "local"
		if len(cloud.turns()) > before {
			where = "cloud"
		}
		t.Logf("turn %d → %s: local=%v sensitive=%v masked=%v only=%v terms=%d %dms why=%q err=%q", i+1, where, h.Local, h.Sensitive, h.Masked, h.Only, h.Terms, h.Ms, h.Why, h.Error)
		if where == "cloud" {
			sent := cloud.turns()[len(cloud.turns())-1]
			for _, sec := range secrets {
				if strings.Contains(turn, sec) && strings.Contains(sent, sec) {
					t.Errorf("turn %d: the cloud saw %q: %s", i+1, sec, sent)
				}
				if strings.Contains(turn, sec) && !strings.Contains(out, sec) {
					t.Errorf("turn %d: %q not restored: %s", i+1, sec, out)
				}
			}
			if strings.Contains(turn, "db-prod-07") {
				t.Logf("turn %d: the cloud saw: %s", i+1, lastUser(sent))
			}
		}
	}
}
