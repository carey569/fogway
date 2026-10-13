package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/yetone/magpie/internal/redact"
	"github.com/yetone/magpie/internal/settings"
)

const mediaSecret = "sk-proj-abcdEFGH1234ijklMNOP5678qrst"
const mediaPrompt = "Draw \"Nightjar\"\non a phone: 13812345678, key: " + mediaSecret + ", gateway: acme-Zx9ab12cdEF"

func mediaRedactionOn(t *testing.T) {
	t.Helper()
	if err := settings.Save(settings.Settings{Redact: true, RedactPersonal: true, RedactWords: []string{"Nightjar"},
		RedactRules: []redact.Rule{{Kind: "GW_KEY", Prefix: "acme-"}}}); err != nil {
		t.Fatal(err)
	}
}

type mediaSent struct {
	path, prompt string
	image        []byte
}

// The vendor echoes its prompt in every supported reply shape. The
// captured request is inspected after the handler has completed.
func mediaEchoClient(t *testing.T, sent chan<- mediaSent, refuse bool) *http.Client {
	t.Helper()
	return &http.Client{Transport: countTransport(func(r *http.Request) (*http.Response, error) {
		got := mediaSent{path: r.URL.Path}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				return nil, err
			}
			defer r.MultipartForm.RemoveAll()
			got.prompt = r.FormValue("prompt")
			f, _, err := r.FormFile("image")
			if err != nil {
				return nil, err
			}
			defer f.Close()
			got.image, err = io.ReadAll(f)
			if err != nil {
				return nil, err
			}
		} else {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			got.prompt = gjson.GetBytes(b, "prompt").String()
			if strings.HasSuffix(r.URL.Path, "/chat/completions") {
				got.prompt = gjson.GetBytes(b, "messages.0.content.0.text").String()
			} else if strings.HasSuffix(r.URL.Path, ":generateContent") {
				got.prompt = gjson.GetBytes(b, "contents.0.parts.0.text").String()
			}
			for _, path := range []string{"messages.0.content.1.image_url.url", "image.url", "images.0.image_url", "input_reference"} {
				if src := gjson.GetBytes(b, path).String(); strings.HasPrefix(src, "data:") {
					_, data, _ := strings.Cut(src, ",")
					got.image, err = base64.StdEncoding.DecodeString(data)
					if err != nil {
						return nil, err
					}
				}
			}
			if data := gjson.GetBytes(b, "contents.0.parts.1.inlineData.data").String(); data != "" {
				got.image, err = base64.StdEncoding.DecodeString(data)
				if err != nil {
					return nil, err
				}
			}
		}
		sent <- got
		png := base64.StdEncoding.EncodeToString(pngBytes)
		body := map[string]any{
			"data": []any{map[string]any{"b64_json": png, "revised_prompt": got.prompt}},
			"choices": []any{map[string]any{"message": map[string]any{"content": got.prompt,
				"images": []any{map[string]any{"image_url": map[string]string{"url": "data:image/png;base64," + png}}}}}},
			"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{
				map[string]string{"text": got.prompt}, map[string]any{"inlineData": map[string]string{"mimeType": "image/png", "data": png}}}}}},
			"request_id": "req-1", "id": "video-1", "prompt": got.prompt,
		}
		code := 200
		if refuse {
			code, body = 400, map[string]any{"error": map[string]string{"message": "rejected: " + got.prompt}}
		}
		b, err := json.Marshal(body)
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(bytes.NewReader(b))}, err
	})}
}

func mediaInput(t *testing.T, model, field string, form bool) (string, string, []byte) {
	t.Helper()
	// Even bytes that look like secrets must reach the vendor unchanged.
	image := append([]byte(nil), pngBytes...)
	image = append(image, []byte(" Nightjar "+mediaSecret)...)
	if form {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		if err := mw.WriteField("model", model); err != nil {
			t.Fatal(err)
		}
		if err := mw.WriteField("prompt", mediaPrompt); err != nil {
			t.Fatal(err)
		}
		part, err := mw.CreateFormFile(field, "first.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(image); err != nil {
			t.Fatal(err)
		}
		if err := mw.Close(); err != nil {
			t.Fatal(err)
		}
		return mw.FormDataContentType(), buf.String(), image
	}
	obj := map[string]any{"model": model, "prompt": mediaPrompt}
	if field != "" {
		obj[field] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	} else {
		image = nil
	}
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return "application/json", string(b), image
}

func assertMediaMasked(t *testing.T, prompt string) {
	t.Helper()
	for _, value := range []string{mediaSecret, "13812345678", "Nightjar", "acme-Zx9ab12cdEF"} {
		if strings.Contains(prompt, value) {
			t.Errorf("vendor received %q in prompt %q", value, prompt)
		}
	}
	for _, prefix := range []string{"{{API_KEY_", "{{PHONE_", "{{TERM_", "{{GW_KEY_"} {
		if !strings.Contains(prompt, prefix) {
			t.Errorf("vendor prompt %q has no %s placeholder", prompt, prefix)
		}
	}
}
