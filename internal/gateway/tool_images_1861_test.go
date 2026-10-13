package gateway

import (
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// codexViewImage is a Codex turn whose view_image call came back with an
// image, as codex-rs 0.162 sends it (FunctionCallOutputContentItem in
// protocol/src/models.rs; view_image.rs answers with the image alone, its
// detail "high"): the function_call_output's output is a list holding one
// input_image, a data: URL (#1861).
const codexViewImage = `{"model":"m","stream":true,"input":[
{"type":"message","role":"user","content":[{"type":"input_text","text":"使用computer use截图当前屏幕看下"}]},
{"type":"function_call","name":"view_image","arguments":"{\"path\":\"/tmp/magpie_icon.png\"}","call_id":"call_view1"},
{"type":"function_call_output","call_id":"call_view1","output":[{"type":"input_image","image_url":"data:image/png;base64,` + viewedPNG + `","detail":"high"}]}
]}`

// a 1x1 PNG
const viewedPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func codexViewImageRequest(t *testing.T) *Request {
	t.Helper()
	r, err := parse(provider.Responses, []byte(codexViewImage))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
