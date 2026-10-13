package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// The gateway draws: /v1/images/generations and /v1/images/edits take
// OpenAI's images request and answer in its shape, whichever vendor draws.
// A model on an images API (gpt-image, dall-e, imagen, flux, seedream,
// qwen-image…) is asked there. OpenRouter uses its /images endpoint for
// those models; its chat image models (gemini-*-image, gpt-5-image) stay on
// chat completions with modalities. Gemini at Google is asked on its own
// generateContent. A request that names no model draws with the Settings'
// image generation model, or one magpie picks (AutoDrawer).

// DrawAgent is the User-Agent magpie's generate_image tool adds to its own.
const DrawAgent = "magpie-image/1"

// drawTimeout is how long a vendor is given to draw: to answer, and to send
// all of its answer.
var drawTimeout = 5 * time.Minute

// maxDrawings is how many images one request may ask for.
const maxDrawings = 4

// drawer is the model a request that names none draws with: the Settings'
// ImageGen while it resolves, else AutoDrawer's. None when it is off or no
// provider draws.
func drawer() (string, bool) {
	switch v := settings.Load().ImageGen; v {
	case "off":
		return "", false
	case "":
	default:
		if pickedMissing(v) == "" {
			return v, true
		}
	}
	m := AutoDrawer()
	return m, m != ""
}

// Drawer is the model a request that names none draws with (drawer), as
// provider/model: "" when image generation is off or no provider draws.
func Drawer() string {
	m, _ := drawer()
	return m
}

// DrawerMissing is the model Settings › Models › Image generation names
// when magpie can't find it any more, so that AutoDrawer's draws in its
// place, as VisionMissing is Image recognition's. "" when none is named,
// it is off, or it resolves.
func DrawerMissing() string { return pickedMissing(settings.Load().ImageGen) }

// The families of the image models a ChatGPT account and a Grok
// subscription draw with (drawsFamily).
const (
	codexDrawerFamily = "gpt-image-"
	grokDrawerFamily  = "grok-imagine-image"
)

// codexDrawers is the image model a ChatGPT account draws with, at its
// Codex backend's images API, as Codex CLI does (gpt-image-2 is the one it
// asks for). It is one: the backend picks the model itself, and answers
// gpt-image-2, gpt-image-2.5 and a made-up name alike with the same image
// (#1865), so listing 2.5 apart said there was a choice where there is
// none. A gpt-image-2.5 saved when it was listed still draws there
// (drawsFamily) and shows as this one (ListedDrawer).
var codexDrawers = []catalog.Model{
	{ID: "gpt-image-2", Name: "GPT Image", Released: "2026-01-01"},
}

// VendorPicksDrawer is whether p's vendor picks the image model itself,
// whichever is named: a ChatGPT account's images backend (#1865).
func VendorPicksDrawer(p provider.Provider) bool { return drawsCodex(p) }

// ListedDrawer is the listed image model a picked one (provider/model)
// stands for: a ChatGPT account's gpt-image-2.5, or any other GPT Image
// name, is its one GPT Image, as its backend draws the same for any name
// (#1865). ref itself otherwise, and when its provider isn't found. The
// pick itself is kept as it was saved, and draws as before.
func ListedDrawer(ref string) string {
	pid, id, ok := strings.Cut(ref, "/")
	if !ok || !strings.HasPrefix(id, codexDrawerFamily) || id == codexDrawer {
		return ref
	}
	p, err := provider.Find(pid)
	if err != nil || !drawsCodex(*p) {
		return ref
	}
	return pid + "/" + codexDrawer
}

// codexDrawer is the image model Codex CLI asks a ChatGPT account for, the
// one Automatic picks there.
const codexDrawer = "gpt-image-2"

// grokDrawers are the image models a Grok subscription (SuperGrok, X Premium+)
// draws with, at the Imagine API of the backend Grok Build's image_gen tool
// calls: cli-chat-proxy.grok.com/v1/images/generations.
var grokDrawers = []catalog.Model{
	{ID: "grok-imagine-image", Name: "Grok Imagine"},
	{ID: "grok-imagine-image-quality", Name: "Grok Imagine Quality"},
}

// drawsGrok is whether p is a Grok subscription, which draws at the Imagine
// API of the backend Grok Build talks to: the built-in's, or the Grok
// plugin's, which signs what it is sent there as the built-in does.
func drawsGrok(p provider.Provider) bool {
	return p.Account != nil && (p.Account.Agent == "grok" || p.PluginProvider() == "grok") && p.Base(provider.Responses) != ""
}

// drawsCodex is whether p is a ChatGPT account, which draws at its Codex
// backend's images API.
func drawsCodex(p provider.Provider) bool {
	return p.Account != nil && p.Account.Agent == "codex" && p.Base(provider.Responses) != ""
}

// Drawers are the models a provider can draw with: its catalogs' models
// that make images, those its own model list names that do, those of its
// own picks named for images, and those the user said draw (#1399), less
// those they said make videos; a ChatGPT account's GPT Image, a Grok
// subscription's Grok Imagine, a WorkBuddy plan's those its config lists.
func Drawers(p provider.Provider) []catalog.Model {
	if drawsWorkBuddy(p) {
		return wbDrawers(p)
	}
	if drawsCodex(p) || drawsGrok(p) {
		out := slices.Clone(codexDrawers)
		if drawsGrok(p) {
			out = slices.Clone(grokDrawers)
		}
		for i := range out {
			out[i].Provider = p.ID
		}
		return out
	}
	if googleAccount(p) {
		// the image models a Google sign-in lists (Antigravity's
		// gemini-3.1-flash-image), which draw on Code Assist (#757)
		var out []catalog.Model
		for _, m := range p.Available() {
			if catalog.DrawsID(m.ID) {
				m.Provider = p.ID
				out = append(out, m)
			}
		}
		return out
	}
	// any other subscription is asked through its agent's own API, which
	// draws nothing magpie can ask for yet. A Responses-only relay (a Codex
	// backend such as sub2api) has no chat base but serves the images API
	// under the same root as its Responses endpoint, so it draws all the same.
	if p.Account != nil || (p.Base(provider.Chat) == "" && p.Base(provider.Responses) == "") {
		return nil
	}
	out := p.PlanDrawers()
	have := make(map[string]bool, len(out))
	for _, m := range out {
		have[m.ID] = true
	}
	for _, c := range p.Catalogs() {
		for _, m := range catalog.Drawers(c) {
			if !have[m.ID] {
				have[m.ID] = true
				out = append(out, m)
			}
		}
	}
	for _, m := range catalog.LiveDrawers(p.ID) {
		if !have[m.ID] {
			have[m.ID] = true
			m.Provider = p.ID
			out = append(out, m)
		}
	}
	for _, id := range p.Models {
		if !have[id] && catalog.DrawsID(id) {
			have[id] = true
			out = append(out, catalog.Model{ID: id, Provider: p.ID})
		}
	}
	// those the user said draw, whatever their list or name says, and
	// none they said makes videos (#1399)
	for _, id := range p.KindModels(provider.KindImage) {
		if !have[id] {
			have[id] = true
			out = append(out, kindModel(p, id))
		}
	}
	return slices.DeleteFunc(out, func(m catalog.Model) bool { return p.ModelKind(m.ID) == provider.KindVideo })
}

// kindModel is p's model id, one the user said makes images or videos
// (provider.SetModelKind), as its list names it.
func kindModel(p provider.Provider, id string) catalog.Model {
	all, _ := catalog.LiveAll(p.ID)
	for _, m := range append(p.Available(), all...) {
		if m.ID == id {
			m.Provider = p.ID
			return m
		}
	}
	return catalog.Model{ID: id, Provider: p.ID}
}

// AutoDrawer is the model magpie draws with when the Settings name none:
// a ChatGPT account's, which its plan pays for, else the cheapest of the
// models the API keys set up can draw with, the newest of those whose price
// isn't known. "" when none can.
func AutoDrawer() string {
	best, bestCost, bestDate := "", 0.0, ""
	for _, p := range provider.All() {
		if !p.On() || p.DecideOnly() || drawsWorkBuddy(p) {
			// WorkBuddy's images cost the plan's credits: it draws only
			// with its model picked or named
			continue
		}
		for _, m := range Drawers(p) {
			if drawsCodex(p) && m.ID != codexDrawer {
				// Codex CLI draws with gpt-image-2 only; a plan may be
				// refused the others (chatgpt.com: 403, #545)
				continue
			}
			cost := 1e9
			switch {
			case drawsCodex(p), drawsGrok(p):
				cost = 0 // a ChatGPT or Grok plan's images come with it
			case m.Price != nil:
				cost = m.Price.Input + m.Price.Output
			}
			if best == "" || cost < bestCost || cost == bestCost && m.Released > bestDate {
				best, bestCost, bestDate = p.ID+"/"+m.ID, cost, m.Released
			}
		}
	}
	return best
}

// picture is an image sent along to draw from, or one drawn.
type picture struct {
	Mime string
	Data []byte
	URL  string // an image the vendor keeps, when it gave no bytes
}

// drawing is one images request, whichever shape it came in.
type drawing struct {
	Model, Prompt, Size, Quality, Background, Format string
	// Want is the response_format the caller asked for: with "b64_json"
	// an image a vendor gave only a URL for is fetched (inline)
	Want   string
	N      int
	Images []picture // to edit, or draw from
	Mask   *picture
}

// drawn is what a vendor gave back.
type drawn struct {
	Images  []picture
	Text    string // what a chat model said besides
	Revised string // the prompt as an images API rewrote it
	Input   int
	Output  int
}

func vendorSaid(text string) string {
	if text = strings.TrimSpace(text); text == "" {
		return ""
	}
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return ": " + text
}

// drawVia is the API a model draws on at a provider.
type drawVia int

const (
	viaImages drawVia = iota // /images/generations, /images/edits
	viaChat                  // chat completions with modalities
	viaGemini                // Google's own generateContent
)

func (v drawVia) String() string {
	return [...]string{"images API", "chat completions", "generateContent"}[v]
}

// tracked is a body that notes when its last bytes came.
type tracked struct {
	r    io.Reader
	last time.Time
}

func (t *tracked) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		t.last = time.Now()
	}
	return n, err
}

// vendorMessage is the message of a vendor's error, or its body.
func vendorMessage(b []byte) string {
	var e struct {
		Error  json.RawMessage `json:"error"`
		Errors struct {
			Message string `json:"message"`
		} `json:"errors"` // ModelScope's
		Code json.RawMessage `json:"code"` // WorkBuddy's {code, msg}
		Msg  string          `json:"msg"`
	}
	if json.Unmarshal(b, &e) == nil && e.Errors.Message != "" {
		return e.Errors.Message
	}
	if json.Unmarshal(b, &e) == nil && len(e.Error) == 0 && e.Msg != "" {
		if code := strings.Trim(string(e.Code), `"`); code != "" && code != "0" && code != "null" {
			return e.Msg + " (" + code + ")"
		}
	}
	if json.Unmarshal(b, &e) == nil && len(e.Error) > 0 {
		var m struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e.Error, &m) == nil && m.Message != "" {
			return m.Message
		}
		var s string
		if json.Unmarshal(e.Error, &s) == nil {
			return s
		}
	}
	return string(b)
}

// ImageStall is how long an image being downloaded may send nothing before
// the download is given up, and ImageLongest how long it may take in all.
// A slow download goes on while it moves: a 4K PNG is 10 MB and more, and
// a cap on the whole of it (generate_image's was 2 minutes) lost one that
// was still coming (#1576).
var (
	ImageStall   = time.Minute
	ImageLongest = 5 * time.Minute
)

// maxImageBytes is the most of an image a download reads.
const maxImageBytes = 64 << 20

var errImageStalled = errors.New("the download stalled")

// grokAspects are the aspect ratios Grok's Imagine API takes.
var grokAspects = []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "2:1", "1:2", "19.5:9", "9:19.5", "20:9", "9:20"}
