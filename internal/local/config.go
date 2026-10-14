package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/carey569/fogway/internal/appdir"
	"github.com/carey569/fogway/internal/edit"
)

// Model is a GGUF file on Hugging Face: the repository and the file in it.
type Model struct {
	Repo string `json:"repo"` // unsloth/Qwen3.5-4B-GGUF
	File string `json:"file"` // Qwen3.5-4B-Q4_K_M.gguf
	// Name is what agents and the router know it by; the file's name
	// without .gguf when empty.
	Name string `json:"name,omitempty"`
	// Size is the file's size as the catalog has it, for a picker before
	// it is asked; the download goes by what Hugging Face says.
	Size uint64 `json:"size,omitempty"`
	// Need is the memory it wants to run at its context: the file and the
	// cache, roughly.
	Need uint64 `json:"need,omitempty"`
	Note string `json:"note,omitempty"`
}

// ID is the model's name as the gateway serves it: local/<ID>.
func (m Model) ID() string {
	if m.Name != "" {
		return m.Name
	}
	return strings.ToLower(strings.TrimSuffix(filepath.Base(m.File), ".gguf"))
}

// Catalog is the models Fogway offers to set up, smallest first; any other
// repository and file can be given.
var Catalog = []Model{
	{Repo: "unsloth/Qwen3.5-2B-GGUF", File: "Qwen3.5-2B-Q4_K_M.gguf", Name: "qwen3.5-2b", Size: 1_280_000_000, Need: 3 << 30,
		Note: "for 8 GB of memory: routes and masks well, does small tasks"},
	{Repo: "unsloth/Qwen3.5-4B-GGUF", File: "Qwen3.5-4B-Q4_K_M.gguf", Name: "qwen3.5-4b", Size: 2_740_000_000, Need: 5 << 30,
		Note: "for 16 GB: the default, a fair coder for its size"},
	{Repo: "unsloth/Qwen3.5-9B-GGUF", File: "Qwen3.5-9B-Q4_K_M.gguf", Name: "qwen3.5-9b", Size: 5_680_000_000, Need: 9 << 30,
		Note: "for 32 GB, or a GPU with 10 GB and more"},
}

// Recommend is the largest catalog model the hardware runs comfortably,
// the smallest when even that is a squeeze.
func Recommend(h Hardware) Model {
	best := Catalog[0]
	for _, m := range Catalog {
		if h.Budget() >= m.Need {
			best = m
		}
	}
	return best
}

// Config is the local model's setup, kept in local.json beside fogway's
// other settings.
type Config struct {
	// On runs the local model while the gateway does.
	On bool `json:"on"`
	// Model is the GGUF it runs; the recommended one when empty.
	Model Model `json:"model"`
	// Context is the context window it runs at, in tokens: 32K when 0.
	Context int `json:"context,omitempty"`
	// GPULayers is how many layers go on the GPU: all when 0, none when -1.
	GPULayers int `json:"gpuLayers,omitempty"`
	// Port is where llama-server listens on 127.0.0.1: one free from 3731
	// up when 0.
	Port int `json:"port,omitempty"`
	// Backend picks the llama.cpp build: metal, cuda, vulkan or cpu; the
	// one the hardware suits when "".
	Backend string `json:"backend,omitempty"`
	// Build is the llama.cpp release (b11583) to run; the newest when "".
	Build string `json:"build,omitempty"`
	// HFEndpoint is where models are pulled from: https://huggingface.co
	// when "", else a mirror such as https://hf-mirror.com. HF_ENDPOINT
	// in the environment is taken when this is "".
	HFEndpoint string `json:"hfEndpoint,omitempty"`
	// GitHubMirror is put before github.com download addresses, for a
	// network that can't reach GitHub (as fogway update's --mirror).
	GitHubMirror string `json:"githubMirror,omitempty"`
	// Think lets a model that reasons do so before it answers: steadier,
	// and slower. Off, it answers at once.
	Think bool `json:"think,omitempty"`
}

// ContextOr is the context window to run at.
func (c Config) ContextOr() int {
	if c.Context > 0 {
		return c.Context
	}
	return 32 << 10
}

// Path is local.json.
func Path() string { return filepath.Join(appdir.Config(), "local.json") }

// Dir is where the runtime and the models are kept: the cache, as they can
// be downloaded again.
func Dir() string { return filepath.Join(appdir.Cache(), "local") }

// Load reads the setup; none saved is the default, off.
func Load() (Config, error) {
	var c Config
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if len(b) == 0 {
		return c, nil
	}
	return c, json.Unmarshal(b, &c)
}

// Save writes the setup.
func Save(c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		return err
	}
	return edit.WriteAtomic(Path(), append(b, '\n'))
}

// Picked is the model the setup runs: its own, else the one recommended
// for this computer.
func (c Config) Picked(h Hardware) Model {
	if c.Model.Repo != "" && c.Model.File != "" {
		return c.Model
	}
	return Recommend(h)
}
