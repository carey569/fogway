package local

import (
	"fmt"
	"strconv"
)

// Window is a context window offered for a model: the memory it takes
// with it and how that sits in this computer's.
type Window struct {
	Tokens int    `json:"tokens"`
	Need   uint64 `json:"need,omitempty"` // bytes; 0 when unknown
	// Room is "" when it fits well, Tight when it fits but leaves little
	// to the rest, NoRoom when it doesn't fit: llama-server may come up
	// and then fail every request.
	Room Grade `json:"room,omitempty"`
}

// windowSizes are the context windows offered, up to the model's own.
var windowSizes = []int{8 << 10, 16 << 10, 32 << 10, 64 << 10, 128 << 10, 256 << 10, 512 << 10, 1 << 20}

// unknownWindow is the largest window offered for a model whose own isn't
// known: one not in the catalog.
const unknownWindow = 128 << 10

// Windows are the context windows m can run at on h: up to the most it was
// trained for, each with the memory it takes.
func Windows(m Model, h Hardware) []Window {
	m = Canon(m)
	top := m.Window
	if top <= 0 {
		top = unknownWindow
	}
	var out []Window
	for _, n := range windowSizes {
		if n > top {
			break
		}
		out = append(out, h.window(m, n))
	}
	if len(out) == 0 || out[len(out)-1].Tokens < top && top < windowSizes[len(windowSizes)-1] {
		out = append(out, h.window(m, top))
	}
	return out
}

// window is m with n tokens of context on h.
func (h Hardware) window(m Model, n int) Window {
	need := m.NeedAt(n)
	return Window{Tokens: n, Need: need, Room: h.room(need)}
}

// FitWindow says why m can't run with n tokens of context on h, nil when
// it can: more than m was trained for, or more memory than h has for it.
func FitWindow(m Model, h Hardware, n int) error {
	m = Canon(m)
	name := m.Title
	if name == "" {
		name = m.ID()
	}
	if m.Window > 0 && n > m.Window {
		return fmt.Errorf("%s was trained for a window of %s tokens at most", name, Tokens(m.Window))
	}
	if w := h.window(m, n); w.Room == NoRoom {
		return fmt.Errorf("a window of %s tokens takes about %s with %s, more than this computer's %s can give it", Tokens(n), gb(w.Need), name, gb(h.RAM))
	}
	return nil
}

// Tokens is n as the windows are named: 32K, 1M, else the number.
func Tokens(n int) string {
	switch {
	case n >= 1<<20 && n%(1<<20) == 0:
		return strconv.Itoa(n>>20) + "M"
	case n >= 1<<10 && n%(1<<10) == 0:
		return strconv.Itoa(n>>10) + "K"
	}
	return strconv.Itoa(n)
}
