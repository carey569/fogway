package gateway

// AlphaSearchPath is where Codex's own web search, its web.run tool, goes:
// "alpha/search" under the model provider's base URL, so /v1/alpha/search
// on magpie's provider and CodexPath+"/alpha/search" when Codex is signed in
// to ChatGPT. Codex offers web.run since 0.143 for a provider that sends
// x-openai-actor-authorization, as magpie's does, on a model whose catalog
// entry says use_responses_lite, as a ChatGPT account's do in magpie
// (codexcat copies Codex's own entry). Unanswered, the 404 ended the turn:
// Codex takes any failure of web.run as fatal (anton on Discord).
const AlphaSearchPath = "/alpha/search"

// alphaSearchRequest is the part of Codex's SearchRequest magpie reads; the
// rest goes on to the ChatGPT backend as it came.
type alphaSearchRequest struct {
	Model    string `json:"model"`
	Commands struct {
		SearchQuery []alphaQuery `json:"search_query"`
		ImageQuery  []alphaQuery `json:"image_query"`
	} `json:"commands"`
}

type alphaQuery struct {
	Q       string   `json:"q"`
	Domains []string `json:"domains"`
}

// alphaSearchReply is Codex's SearchResponse: output is what the model is
// shown as the tool's result.
type alphaSearchReply struct {
	EncryptedOutput *string `json:"encrypted_output"`
	Output          string  `json:"output"`
}
