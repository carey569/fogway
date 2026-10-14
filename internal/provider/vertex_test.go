package provider

import (
	"bytes"
	"cmp"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/testenv"
)

// vertexHome gives a test a home of its own with no Google credentials in
// it, neither GOOGLE_APPLICATION_CREDENTIALS nor CLOUDSDK_CONFIG set, and
// no Vertex AI token kept from another test.
func vertexHome(t *testing.T) string {
	t.Helper()
	isolate(t)
	h := t.TempDir()
	testenv.SetHome(t, h)
	t.Setenv("APPDATA", filepath.Join(h, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(h, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(h, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(h, ".local", "state"))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_CONFIG", "")
	forgetVertexTokens()
	t.Cleanup(forgetVertexTokens)
	return h
}

// gcloudCredentials is where `gcloud auth application-default login`
// writes a person's credentials for home: ~/.config/gcloud, or
// %APPDATA%\gcloud on Windows.
func gcloudCredentials(home string) string {
	dir := filepath.Join(home, ".config", "gcloud")
	if runtime.GOOS == "windows" {
		dir = filepath.Join(os.Getenv("APPDATA"), "gcloud")
	}
	return filepath.Join(dir, "application_default_credentials.json")
}

// vertexGoogle is Google as a Vertex AI provider meets it: the token
// endpoint, IAM Credentials' generateAccessToken and a project's
// generateContent. It keeps every request each was sent.
type vertexGoogle struct {
	url  string
	quit chan struct{} // closed as the test ends, for a request still held
	got  chan struct{} // told of a token request held
	mu   sync.Mutex
	// set before the request it is for
	answers vertexAnswers
	// what each was sent
	tokens, iam, vertex []vertexCall
}

// vertexAnswers are how vertexGoogle answers. The token endpoint mints
// ya29.minted-<n> lasting expiresIn seconds (3599 when 0) after slow, or
// answers status and reply when status is set; a request is held
// unanswered while held is, and answered once gate is closed when that is
// set. IAM Credentials mints ya29.impersonated-<n> lasting iamLife (an hour
// when 0), or answers iamStatus and iamReply.
type vertexAnswers struct {
	expiresIn int
	slow      time.Duration
	status    int
	reply     string
	held      bool
	gate      chan struct{}
	iamLife   time.Duration
	iamStatus int
	iamReply  string
}

// vertexCall is a request vertexGoogle was sent.
type vertexCall struct {
	path   string // as it was sent, escaped
	header http.Header
	body   string
}

// newVertexGoogle serves a vertexGoogle on loopback, Vertex AI's root,
// Google's token endpoint and IAM Credentials pointed at it until the test
// ends.
func newVertexGoogle(t *testing.T) *vertexGoogle {
	t.Helper()
	f := &vertexGoogle{quit: make(chan struct{}), got: make(chan struct{}, 1)}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(f.quit) }) // before Close, which waits for a held request
	f.url = srv.URL
	t.Cleanup(VertexForTest(srv.URL+"/vertex", srv.URL+"/token", srv.URL+"/iam/v1"))
	return f
}

func (f *vertexGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c := vertexCall{r.URL.EscapedPath(), r.Header.Clone(), string(b)}
	token, iam, vertex := r.URL.Path == "/token", strings.HasPrefix(r.URL.Path, "/iam/v1/"), strings.HasPrefix(r.URL.Path, "/vertex/")
	f.mu.Lock()
	n := 0
	switch {
	case token:
		f.tokens = append(f.tokens, c)
		n = len(f.tokens)
	case iam:
		f.iam = append(f.iam, c)
		n = len(f.iam)
	case vertex:
		f.vertex = append(f.vertex, c)
	}
	a := f.answers
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if token && a.gate != nil {
		select {
		case f.got <- struct{}{}:
		default:
		}
		select {
		case <-a.gate:
		case <-r.Context().Done():
			return
		case <-f.quit:
			return
		}
	}
	switch {
	case token && a.held:
		select {
		case f.got <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-f.quit:
		}
	case token && a.status != 0:
		time.Sleep(a.slow)
		w.WriteHeader(a.status)
		io.WriteString(w, a.reply)
	case token:
		time.Sleep(a.slow)
		fmt.Fprintf(w, `{"access_token":"ya29.minted-%d","expires_in":%d,"scope":"https://www.googleapis.com/auth/cloud-platform","token_type":"Bearer"}`, n, cmp.Or(a.expiresIn, 3599))
	case iam && a.iamStatus != 0:
		w.WriteHeader(a.iamStatus)
		io.WriteString(w, a.iamReply)
	case iam:
		fmt.Fprintf(w, `{"accessToken":"ya29.impersonated-%d","expireTime":%q}`, n, time.Now().Add(cmp.Or(a.iamLife, time.Hour)).UTC().Format(time.RFC3339))
	case vertex && strings.Contains(r.URL.Path, "/models/gemini-9-pro:"):
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "{\"error\":{\"code\":404,\"message\":\"Publisher Model `projects/acme-vertex/locations/global/publishers/google/models/gemini-9-pro` was not found or your project does not have access to it. Please ensure you are using a valid model version. For more information, see: https://cloud.google.com/vertex-ai/generative-ai/docs/learn/model-versions\",\"status\":\"NOT_FOUND\"}}")
	case vertex:
		io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi"}]},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2},"modelVersion":"gemini-3.8-flash"}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *vertexGoogle) tokenCalls() []vertexCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.tokens)
}

func (f *vertexGoogle) iamCalls() []vertexCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.iam)
}

func (f *vertexGoogle) vertexCalls() []vertexCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.vertex)
}

// vertexUser is a person's credentials as `gcloud auth application-default
// login` writes them.
func vertexUser(refresh string) map[string]any {
	return map[string]any{
		"account":          "",
		"client_id":        "1234567890-vertextest.apps.googleusercontent.com",
		"client_secret":    "vertex-test-secret",
		"quota_project_id": "acme-vertex",
		"refresh_token":    refresh,
		"type":             "authorized_user",
		"universe_domain":  "googleapis.com",
	}
}

// vertexServiceKey is a service account's key file as the Cloud console
// gives it.
func vertexServiceKey(email, keyID, key string) map[string]any {
	return map[string]any{
		"type":                        "service_account",
		"project_id":                  "acme-vertex",
		"private_key_id":              keyID,
		"private_key":                 key,
		"client_email":                email,
		"client_id":                   "104857600123456789012",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/" + url.PathEscape(email),
		"universe_domain":             "googleapis.com",
	}
}

// vertexImpersonation is the file `gcloud auth application-default login
// --impersonate-service-account` writes: source's token is traded for one
// of the service account the URL names.
func vertexImpersonation(at string, delegates []string, source any) map[string]any {
	return map[string]any{
		"delegates":                         delegates,
		"service_account_impersonation_url": at,
		"source_credentials":                source,
		"type":                              "impersonated_service_account",
	}
}

// vertexRSA is a service account's RSA key, made once for the tests.
var vertexRSA = sync.OnceValues(func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 2048) })

// vertexAt is a Vertex AI provider at the project acme-vertex in loc,
// signed with the credentials in file ("" finds them as Google's libraries
// do).
func vertexAt(loc, file string) Provider {
	return Provider{ID: "vertex-prod", Name: "Vertex prod", Preset: VertexPreset,
		Vertex: &Vertex{Project: "acme-vertex", Location: loc, Credentials: file}}
}

// signVertex signs a request for model at p's address, as the gateway
// sends one, and gives the Authorization it carries.
func signVertex(ctx context.Context, p Provider, model string) (string, error) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Base(Gemini)+VertexPath(model), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	err = p.Sign(ctx, req, Gemini, body)
	return req.Header.Get("Authorization"), err
}

// jwtOf is a JWT's header and claims, once its signature verifies with pub.
func jwtOf(t *testing.T, jwt string, pub *rsa.PublicKey) (head, claims map[string]any) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("no JWT: %q", jwt)
	}
	var seg [3][]byte
	for i, s := range parts {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("the JWT's part %d isn't unpadded base64url: %v", i, err)
		}
		seg[i] = b
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], seg[2]); err != nil {
		t.Fatalf("the JWT's signature doesn't verify with the key's public half: %v", err)
	}
	if json.Unmarshal(seg[0], &head) != nil || json.Unmarshal(seg[1], &claims) != nil {
		t.Fatalf("the JWT's header or claims aren't JSON: %s %s", seg[0], seg[1])
	}
	return head, claims
}

// Each location is asked at a host of its own: global at
// aiplatform.googleapis.com, the us and eu multi-regions at their rep
// hosts, a region at its own. A project or location no address can be made
// of gives none, so nothing is sent anywhere, and a model typed in by hand
// stays one segment of the path.
func TestVertexAddress(t *testing.T) {
	vertexHome(t)
	for loc, want := range map[string]string{
		"":             "https://aiplatform.googleapis.com/v1/projects/acme-vertex/locations/global",
		"global":       "https://aiplatform.googleapis.com/v1/projects/acme-vertex/locations/global",
		"us":           "https://aiplatform.us.rep.googleapis.com/v1/projects/acme-vertex/locations/us",
		"eu":           "https://aiplatform.eu.rep.googleapis.com/v1/projects/acme-vertex/locations/eu",
		"us-central1":  "https://us-central1-aiplatform.googleapis.com/v1/projects/acme-vertex/locations/us-central1",
		"europe-west4": "https://europe-west4-aiplatform.googleapis.com/v1/projects/acme-vertex/locations/europe-west4",
	} {
		p := vertexAt(loc, "")
		if got := p.vertexBase(); got != want {
			t.Errorf("at %q: %s, want %s", loc, got, want)
		}
		if got := p.Base(Gemini); got != want {
			t.Errorf("at %q generateContent's base is %s, want %s", loc, got, want)
		}
	}
	// a domain-scoped project's id has a dot and a colon in it
	p := vertexAt("global", "")
	p.Vertex.Project = "example.com:acme-vertex"
	if got := p.vertexBase(); got != "https://aiplatform.googleapis.com/v1/projects/example.com:acme-vertex/locations/global" {
		t.Errorf("a domain-scoped project: %s", got)
	}
	for _, v := range []*Vertex{
		nil, {}, {Location: "us-central1"},
		{Project: "Acme-Vertex"}, {Project: "acme-vertex/../other-project"}, {Project: "acme-vertex?alt=sse"},
		{Project: "acme-vertex", Location: "us central1"}, {Project: "acme-vertex", Location: "../global"}, {Project: "acme-vertex", Location: "us-central1/x"},
	} {
		p := Provider{Name: "Vertex prod", Preset: VertexPreset, Vertex: v}
		if got := p.vertexBase(); got != "" || p.Base(Gemini) != "" || len(p.Speaks()) > 0 || p.Ready() {
			t.Errorf("%+v has an address: %q, speaks %v, ready %v", v, got, p.Speaks(), p.Ready())
		}
	}
	if got := VertexPath("gemini-3.1-pro-preview-customtools"); got != "/publishers/google/models/gemini-3.1-pro-preview-customtools:streamGenerateContent?alt=sse" {
		t.Errorf("a model's path: %s", got)
	}
	if got := VertexPath("tuned/gemini 1?x#y"); got != "/publishers/google/models/tuned%2Fgemini%201%3Fx%23y:streamGenerateContent?alt=sse" {
		t.Errorf("a model with a slash, a space, ? and # in it: %s", got)
	}
}

// Save turns away a Vertex AI provider whose requests it couldn't address
// or sign, saying what to give instead, and keeps nothing of it; and one
// given an API key, since Vertex AI is asked with the user's Google
// credentials.
func TestVertexSaveRefuses(t *testing.T) {
	home := vertexHome(t)
	for _, c := range []struct {
		v    *Vertex
		key  string
		keys []KeyAccount
		want string
	}{
		{v: &Vertex{Project: "acme-vertex"}, key: "AIzaSyD-not-a-real-key", want: "Google Vertex AI is asked with your Google credentials, not an API key"},
		{v: &Vertex{Project: "acme-vertex"}, keys: []KeyAccount{{Name: "express", Key: "AIzaSyD-not-a-real-key"}}, want: "Google Vertex AI is asked with your Google credentials, not an API key"},
		{want: "Google Vertex AI needs the id of your Google Cloud project"},
		{v: &Vertex{Location: "us-central1"}, want: "Google Vertex AI needs the id of your Google Cloud project"},
		{v: &Vertex{Project: "acme_vertex"}, want: `"acme_vertex" is no Google Cloud project id`},
		{v: &Vertex{Project: "-acme-vertex"}, want: `"-acme-vertex" is no Google Cloud project id`},
		{v: &Vertex{Project: "acme-vertex/locations"}, want: `"acme-vertex/locations" is no Google Cloud project id`},
		{v: &Vertex{Project: "acme-vertex", Location: "us central1"}, want: `"us central1" is no Vertex AI location: global, us, eu or a region such as us-central1`},
		{v: &Vertex{Project: "acme-vertex", Location: "us-central1/"}, want: `"us-central1/" is no Vertex AI location: global, us, eu or a region such as us-central1`},
		{v: &Vertex{Project: "acme-vertex", Credentials: "keys/acme-vertex.json"}, want: `give the credentials file's full path, not "keys/acme-vertex.json"`},
		// another user's home, which nothing expands: it would be read
		// under whatever folder fogway was started in
		{v: &Vertex{Project: "acme-vertex", Credentials: "~other/acme-vertex.json"}, want: `give the credentials file's full path, not "~other/acme-vertex.json"`},
		{v: &Vertex{Project: "acme-vertex", Impersonate: "vertex-runner"}, want: `"vertex-runner" is no service account's email`},
		{v: &Vertex{Project: "acme-vertex", Impersonate: "vertex-runner@acme vertex.iam.gserviceaccount.com"}, want: `"vertex-runner@acme vertex.iam.gserviceaccount.com" is no service account's email`},
	} {
		err := Save(Provider{ID: "vertex-prod", Name: "Vertex prod", Preset: VertexPreset, Vertex: c.v, Key: c.key, Keys: c.keys})
		if err == nil || err.Error() != c.want {
			t.Errorf("%+v, key %q %v: %v, want %q", c.v, c.key, c.keys, err, c.want)
		}
	}
	if _, err := os.Stat(Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a provider refused was kept: %v", err)
	}
	// a file's full path, or one under the home, is taken
	for _, cred := range []string{filepath.Join(home, "keys", "acme-vertex.json"), "~/keys/acme-vertex.json"} {
		if err := Save(Provider{ID: "vertex-prod", Name: "Vertex prod", Preset: VertexPreset, Vertex: &Vertex{Project: "acme-vertex", Credentials: cred}}); err != nil {
			t.Errorf("credentials %s: %v", cred, err)
		}
	}
}

// A Vertex AI provider's project is each copy's own: a caller changing the
// one it found leaves the list a request holds as it was, a copy the user
// makes is at the same project and location until they say otherwise, and
// a renamed one keeps them.
func TestVertexCopies(t *testing.T) {
	vertexHome(t)
	if err := Save(Provider{ID: "vertex-prod", Name: "Vertex prod", Preset: VertexPreset, Vertex: &Vertex{Project: "acme-vertex", Location: "us"}}); err != nil {
		t.Fatal(err)
	}
	release := Hold()
	a, err := Find("vertex-prod")
	if err != nil {
		t.Fatal(err)
	}
	a.Vertex.Project = "changed"
	b, err := Find("vertex-prod")
	release()
	if err != nil || b.Vertex.Project != "acme-vertex" {
		t.Fatalf("a caller's change reached the held list: %+v (%v)", b, err)
	}
	id, err := AddCopy(Provider{Name: "Vertex prod copy", Preset: VertexPreset}, "vertex-prod")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Find(id)
	if err != nil || c.Vertex == nil || *c.Vertex != (Vertex{Project: "acme-vertex", Location: "us"}) {
		t.Fatalf("the copy %s: %+v (%v)", id, c, err)
	}
	if err := Rename("vertex-prod", "vertex-main"); err != nil {
		t.Fatal(err)
	}
	r, err := Find("vertex-main")
	if err != nil || r.Base(Gemini) != "https://aiplatform.us.rep.googleapis.com/v1/projects/acme-vertex/locations/us" {
		t.Fatalf("renamed: %+v (%v)", r, err)
	}
}

// waitCtx tells waiting when a request begins to wait on another's mint:
// the first thing to ask for its Done.
type waitCtx struct {
	context.Context
	waiting chan<- struct{}
	once    sync.Once
}

func (c *waitCtx) Done() <-chan struct{} {
	c.once.Do(func() { c.waiting <- struct{}{} })
	return c.Context.Done()
}
