package gateway

// ModelScope's API-Inference draws on an images API of its own: a POST to
// /images/generations with X-ModelScope-Async-Mode starts a task and
// answers its task_id, which GET /tasks/<id> (X-ModelScope-Task-Type:
// image_generation) reports on until task_status is SUCCEED, with
// output_images, or FAILED. It takes one task of a user's at a time
// ("user queue is full" otherwise) and limits bursts with 429s, so magpie
// asks it one image at a time, and again after a short wait when it is busy.
// Its answers carry no OpenAI "data", which read as an empty drawing (#645).

import (
	"sync"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// isModelScope is a provider at ModelScope's API-Inference, a preset's or
// one added by hand.
var isModelScope = func(p provider.Provider) bool {
	return provider.HostOf(p.Base(provider.Chat)) == "api-inference.modelscope.cn"
}

var (
	// modelScopePoll is how often a task is asked after
	modelScopePoll = 2 * time.Second
	// modelScopeBusy are the waits before asking again when busy
	modelScopeBusy = []time.Duration{3 * time.Second, 8 * time.Second, 15 * time.Second}
	// modelScopeQueues hold one task at a time per key
	modelScopeQueues sync.Map
)

// modelScopeResult is where ModelScope puts its images: output_images on a
// task, images on a sync answer.
type modelScopeResult struct {
	Output []string `json:"output_images"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}
