package gateway

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// The gateway makes videos: POST /v1/videos, GET /v1/videos/{id} and
// GET /v1/videos/{id}/content take OpenAI's Videos API and answer in its
// shape, so its SDKs work unchanged. A video takes seconds to minutes, so a
// request starts one and answers at once with its id; the id is what asks
// for it again. It says who is making the video, video_<provider>.<the
// vendor's id>.<when it started>, so the gateway keeps nothing.
//
// A Grok subscription uses the Imagine API of the backend Grok Build talks
// to: <cli-chat-proxy.grok.com/v1>/videos/generations starts one and answers
// with its request_id. Volcengine Agent Plan uses Ark's asynchronous content
// generation tasks under its /api/plan/v3 base.
//
// Another magpie's videos (a Remote magpie's, #545) are asked of it, at its
// own videos API: its id for one goes, encoded, in the id given here, and
// what it answers comes back with this magpie's id and name for the model.
// A provider set up with a base URL whose own model list marks a model
// "kind": "video" (#1446) is asked the same way, at the OpenAI-shaped
// videos API under its base, on the key that started the video.

// grokVideoModels are the video models a Grok subscription makes videos with.
var grokVideoModels = []catalog.Model{
	{ID: "grok-imagine-video", Name: "Grok Imagine Video"},
	{ID: "grok-imagine-video-1.5", Name: "Grok Imagine Video 1.5"},
}

// grokVideoAspects are the aspect ratios Grok's video API takes.
var grokVideoAspects = []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"}

// maxVideoBytes is the most of a video the gateway passes on.
const maxVideoBytes = 512 << 20

// Videomakers are the models a provider can make videos with: a Grok
// subscription's, another magpie's, a preset plan's models when magpie
// implements that vendor's video API, and those a provider's own model list
// marks as video models (customVideomakers).
func Videomakers(p provider.Provider) []catalog.Model {
	if p.IsRemoteMagpie() {
		out := catalog.LiveVideomakers(p.ID)
		for i := range out {
			out[i].Provider = p.ID
		}
		return out
	}
	if out := p.PlanVideos(); len(out) > 0 {
		return out
	}
	if !drawsGrok(p) {
		return customVideomakers(p)
	}
	out := slices.Clone(grokVideoModels)
	for i := range out {
		out[i].Provider = p.ID
	}
	return out
}

// customVideomakers are the video models of a provider set up with a base
// URL — not a subscription's account, not another magpie, not a preset plan
// whose own video API magpie speaks — that its fetched model list marks
// "kind": "video" (catalog.LiveVideomakers, #1446), or the user said make
// videos (provider.SetModelKind, #1399). They are asked at the
// OpenAI-shaped videos API under its base, as another magpie's are. A
// model's name alone makes none of them one: a list that doesn't say so,
// and a user who doesn't, leave the provider making no videos.
func customVideomakers(p provider.Provider) []catalog.Model {
	if p.Account != nil || p.IsRemoteMagpie() || len(p.PlanVideos()) > 0 || videoBase(p) == "" {
		return nil
	}
	out := catalog.LiveVideomakers(p.ID)
	for i := range out {
		out[i].Provider = p.ID
	}
	// those the user said make videos, whatever the list says, and none
	// they said draws (shen-joe, #1399: a relay whose /models marks none)
	for _, id := range p.KindModels(provider.KindVideo) {
		if !slices.ContainsFunc(out, func(m catalog.Model) bool { return m.ID == id }) {
			out = append(out, kindModel(p, id))
		}
	}
	return slices.DeleteFunc(out, func(m catalog.Model) bool { return p.ModelKind(m.ID) == provider.KindImage })
}

// videoBase is the root of the OpenAI-shaped videos API of another magpie
// or a provider set up with a base URL: its Chat base, or a Responses-only
// relay's Responses base, under which it serves the images API too.
func videoBase(p provider.Provider) string {
	return strings.TrimRight(cmp.Or(p.Base(provider.Chat), p.Base(provider.Responses)), "/")
}

// grokVideoFamily is the family of a Grok subscription's video models.
const grokVideoFamily = "grok-imagine-video"

// filming is one videos request, whichever shape it came in.
type filming struct {
	Model, Prompt, Seconds, Size string
	Start                        *picture  // the first frame to animate
	References                   []picture // images the video draws its subjects from
}

// decimalNumber is a number as JSON writes it, not as Go's ParseFloat also
// takes it (hex, inf, nan, underscores).
var decimalNumber = regexp.MustCompile(`^[0-9]+(\.[0-9]*)?([eE][+-]?[0-9]+)?$`)

// idPart is what a provider id or a vendor's request id is made of; an id
// is put in a URL, so nothing that would change its path gets through.
var idPart = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// videoState is how the vendor says a video is going.
type videoState struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Model    string `json:"model"`
	Video    struct {
		URL      string  `json:"url"`
		Duration float64 `json:"duration"`
	} `json:"video"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Duration float64 `json:"duration"`
	Error    struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
