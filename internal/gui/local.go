package gui

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yetone/magpie/internal/local"
	"github.com/yetone/magpie/internal/provider"
)

// localJSON is the Local page: the setup, what runs, this computer, the
// catalog and the edge-cloud group.
type localJSON struct {
	Config      local.Config   `json:"config"`
	Status      local.Status   `json:"status"`
	Hardware    local.Hardware `json:"hardware"`
	Model       local.Model    `json:"model"` // the one the setup runs
	Ref         string         `json:"ref"`   // its id: local/<model>
	Catalog     []catalogModel `json:"catalog"`
	Recommended string         `json:"recommended"`
	Log         string         `json:"log"`
	Hybrid      *hybridJSON    `json:"hybrid"`
}

type catalogModel struct {
	local.Model
	Name string `json:"name"`
}

type hybridJSON struct {
	Group   string   `json:"group"`
	Local   string   `json:"local"`
	Cloud   []string `json:"cloud"`
	Choices []string `json:"choices"` // the models the cloud side can have
	Mask    bool     `json:"mask"`
}

func localState() localJSON {
	c, _ := local.Load()
	h := local.Detect()
	m := c.Picked(h)
	s := local.Now()
	if s.Hardware.OS == "" {
		s.Hardware = h
	}
	out := localJSON{Config: c, Status: s, Hardware: h, Model: m, Ref: local.Ref(m), Recommended: local.Recommend(h).ID(),
		Log: filepath.Join(local.Dir(), "llama-server.log")}
	for _, cm := range local.Catalog {
		out.Catalog = append(out.Catalog, catalogModel{Model: cm, Name: cm.ID()})
	}
	if g, _, ok := provider.FindGroup(provider.GroupPrefix + provider.HybridGroupID); ok && g.Hybrid != nil {
		hj := &hybridJSON{Group: provider.GroupPrefix + g.ID, Local: g.Hybrid.Local, Mask: g.Hybrid.Mask, Cloud: []string{}, Choices: []string{}}
		for _, id := range g.Members {
			if id != g.Hybrid.Local {
				hj.Cloud = append(hj.Cloud, id)
			}
		}
		for _, e := range provider.Served() {
			if e.Group == "" && e.Provider.ID != local.ProviderID && e.Provider.Ready() {
				hj.Choices = append(hj.Choices, e.ID)
			}
		}
		out.Hybrid = hj
	}
	return out
}

func localRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/local", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, localState())
	})
	mux.HandleFunc("POST /api/local/{action}", func(rw http.ResponseWriter, r *http.Request) {
		var in struct {
			Model      *local.Model `json:"model"`
			HFEndpoint *string      `json:"hfEndpoint"`
			Context    int          `json:"context"`
			On         bool         `json:"on"`
			Members    []string     `json:"members"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		c, err := local.Load()
		if err != nil {
			fail(rw, err)
			return
		}
		switch r.PathValue("action") {
		case "on", "off":
			c.On = r.PathValue("action") == "on"
		case "save":
			if in.Model != nil {
				m := local.Model{Repo: strings.TrimSpace(in.Model.Repo), File: strings.TrimSpace(in.Model.File)}
				if m.Repo == "" || m.File == "" || !strings.Contains(m.Repo, "/") || !strings.HasSuffix(strings.ToLower(m.File), ".gguf") {
					fail(rw, errBadModel)
					return
				}
				for _, cm := range local.Catalog {
					if cm.Repo == m.Repo && cm.File == m.File {
						m = cm
					}
				}
				c.Model = m
			}
			if in.HFEndpoint != nil {
				c.HFEndpoint = strings.TrimSpace(*in.HFEndpoint)
			}
			if in.Context > 0 {
				c.Context = in.Context
			}
			c.On = true
		case "mask", "cloud":
			g, _, ok := provider.FindGroup(provider.GroupPrefix + provider.HybridGroupID)
			if !ok || g.Hybrid == nil {
				fail(rw, errNoHybrid)
				return
			}
			h := *g.Hybrid
			if r.PathValue("action") == "mask" {
				h.Mask = in.On
			} else {
				members := []string{h.Local}
				for _, id := range in.Members {
					if id != h.Local && !slices.Contains(members, id) {
						members = append(members, id)
					}
				}
				g.Members = members
			}
			g.Hybrid = &h
			if err := provider.SaveGroup(g); err != nil {
				fail(rw, err)
				return
			}
			writeJSON(rw, localState())
			return
		default:
			http.NotFound(rw, r)
			return
		}
		if err := local.Save(c); err != nil {
			fail(rw, err)
			return
		}
		writeJSON(rw, localState())
	})
}

type localError string

func (e localError) Error() string { return string(e) }

const (
	errBadModel = localError("give a Hugging Face repository (owner/name) and a .gguf file in it")
	errNoHybrid = localError("there is no edge-cloud group yet: it is made once the local model serves")
)
