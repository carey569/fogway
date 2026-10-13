package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/local"
	"github.com/yetone/magpie/internal/provider"
)

func localCall(t *testing.T, mux *http.ServeMux, method, path, body string) (int, localJSON, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out localJSON
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

// The Local page reads the setup and this computer, and its saves land in
// local.json, where the gateway picks them up; the edge-cloud group's
// masking and cloud members are set on the group itself.
func TestLocalPage(t *testing.T) {
	sandboxHome(t)
	mux := http.NewServeMux()
	localRoutes(mux)

	code, st, raw := localCall(t, mux, "GET", "/api/local", "")
	if code != 200 || len(st.Catalog) != len(local.Catalog) || st.Recommended == "" || st.Config.On || st.Hybrid != nil {
		t.Fatalf("%d %s", code, raw)
	}

	// a model of the user's own: any repository and .gguf file in it
	code, st, raw = localCall(t, mux, "POST", "/api/local/save", `{"model":{"repo":"Qwen/Qwen3-0.6B-GGUF","file":"Qwen3-0.6B-Q8_0.gguf"},"hfEndpoint":"https://hf-mirror.com"}`)
	if code != 200 || !st.Config.On || st.Ref != "local/qwen3-0.6b-q8_0" {
		t.Fatalf("%d %s", code, raw)
	}
	if c, _ := local.Load(); !c.On || c.Model.Repo != "Qwen/Qwen3-0.6B-GGUF" || c.HFEndpoint != "https://hf-mirror.com" {
		t.Fatalf("local.json: %+v", c)
	}
	if code, _, raw = localCall(t, mux, "POST", "/api/local/save", `{"model":{"repo":"no-owner","file":"x.bin"}}`); code != 400 {
		t.Fatalf("a bad model: %d %s", code, raw)
	}
	if code, st, _ = localCall(t, mux, "POST", "/api/local/off", ``); code != 200 || st.Config.On {
		t.Fatalf("off: %d %+v", code, st.Config)
	}

	// no group yet: masking waits for it
	if code, _, raw = localCall(t, mux, "POST", "/api/local/mask", `{"on":true}`); code != 400 || !strings.Contains(raw, "edge-cloud group") {
		t.Fatalf("mask without a group: %d %s", code, raw)
	}
	if err := provider.Save(provider.Provider{ID: "cloud", Name: "Cloud", Key: "k", Chat: "https://cloud.example/v1", Models: []string{"big", "huge"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(provider.Group{ID: provider.HybridGroupID, Name: "Edge-cloud auto", Members: []string{"local/q", "cloud/big"}, Hybrid: &provider.Hybrid{Local: "local/q"}}); err != nil {
		t.Fatal(err)
	}
	if code, st, raw = localCall(t, mux, "POST", "/api/local/mask", `{"on":true}`); code != 200 || st.Hybrid == nil || !st.Hybrid.Mask {
		t.Fatalf("mask: %d %s", code, raw)
	}
	code, st, raw = localCall(t, mux, "POST", "/api/local/cloud", `{"members":["cloud/huge","local/q","cloud/big"]}`)
	if code != 200 || strings.Join(st.Hybrid.Cloud, ",") != "cloud/huge,cloud/big" || !st.Hybrid.Mask {
		t.Fatalf("cloud: %d %s", code, raw)
	}
	g, _, _ := provider.FindGroup(provider.GroupPrefix + provider.HybridGroupID)
	if strings.Join(g.Members, ",") != "local/q,cloud/huge,cloud/big" || g.Hybrid.Local != "local/q" {
		t.Fatalf("group %+v", g)
	}
}
