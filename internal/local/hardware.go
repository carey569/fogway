// Package local runs a model on this computer: llama.cpp's llama-server,
// downloaded for the platform, serving a GGUF model pulled from Hugging
// Face, which the gateway serves as local/<model>.
package local

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fogway/fogway/internal/proc"
)

// Hardware is what this computer offers a local model: its memory, and a
// graphics card's when one with its own memory is found (NVIDIA's, read
// through nvidia-smi). A Mac with Apple silicon shares its memory with the
// GPU, which Metal uses: Unified says so.
type Hardware struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	RAM     uint64 `json:"ram"`            // bytes, 0 when unknown
	VRAM    uint64 `json:"vram,omitempty"` // a discrete GPU's, bytes
	GPU     string `json:"gpu,omitempty"`  // its name
	NVIDIA  bool   `json:"nvidia,omitempty"`
	Unified bool   `json:"unified,omitempty"`
}

// Detect reads the hardware. It never fails: what it can't read is zero.
func Detect() Hardware {
	h := Hardware{OS: runtime.GOOS, Arch: runtime.GOARCH, RAM: totalRAM()}
	h.Unified = runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	if runtime.GOOS != "darwin" {
		h.GPU, h.VRAM = nvidiaGPU()
		h.NVIDIA = h.GPU != ""
	}
	return h
}

// nvidiaGPU is the first NVIDIA card's name and memory, as nvidia-smi
// tells them; "" when there is none or no driver.
func nvidiaGPU() (string, uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := proc.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return "", 0
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	name, mem, ok := strings.Cut(line, ",")
	if !ok {
		return "", 0
	}
	mib, err := strconv.ParseUint(strings.TrimSpace(mem), 10, 64)
	if err != nil {
		return strings.TrimSpace(name), 0
	}
	return strings.TrimSpace(name), mib << 20
}

// Budget is the memory a model may take: a discrete GPU's when it has one
// big enough to matter, else a share of the RAM, leaving the rest to the
// system and the agents.
func (h Hardware) Budget() uint64 {
	if h.VRAM >= 6<<30 {
		return h.VRAM * 85 / 100
	}
	if h.Unified {
		return h.RAM * 60 / 100 // Metal's working set is about two thirds
	}
	return h.RAM * 50 / 100
}

// Backend is the llama.cpp build that suits the hardware: Metal on a Mac,
// CUDA with an NVIDIA card, else Vulkan on Windows and Linux (which takes
// any GPU, and falls back to the CPU), CPU where there is no other.
func (h Hardware) Backend() string {
	switch {
	case h.OS == "darwin":
		return "metal"
	case h.NVIDIA && h.Arch == "amd64":
		return "cuda"
	case h.OS == "windows" || h.OS == "linux":
		return "vulkan"
	}
	return "cpu"
}
