package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/carey569/fogway/internal/edit"
)

// Candidates are the models a computer is assessed for, smallest first:
// what Fogway recommends installing, or that none be.
var Candidates = []Model{Spark17, Spark4}

// Estimate is how one of the Candidates would run on this computer.
type Estimate struct {
	Model string `json:"model"` // its ID
	Title string `json:"title"`
	Size  uint64 `json:"size"` // its download, bytes
	Need  uint64 `json:"need"` // the memory it takes, bytes
	// Where it would run: "gpu" (Metal, or an NVIDIA card it fits on) or
	// "cpu" (any other computer: llama.cpp's Vulkan build may find a GPU
	// there, which a measurement then shows).
	Where string `json:"where"`
	// Prompt is how many tokens a second it reads a prompt at, Gen how many
	// it writes a second.
	Prompt float64 `json:"prompt"`
	Gen    float64 `json:"gen"`
	// Judge is the seconds it would take to judge a turn for group/auto:
	// read the router's instructions and a user's message, and answer.
	Judge float64 `json:"judge"`
	// Measured: Prompt and Gen were read from the model running here, not
	// estimated. Corrected: estimated, then set right by how a measured
	// model ran against its own estimate.
	Measured  bool  `json:"measured,omitempty"`
	Corrected bool  `json:"corrected,omitempty"`
	Grade     Grade `json:"grade"`
}

// Grade is how well a model would run here.
type Grade string

const (
	Smooth Grade = "smooth" // judges a turn in a few seconds, writes fast
	Usable Grade = "usable" // slower, still worth it for judging and masking
	Slow   Grade = "slow"   // too slow to stand in front of every turn
	Tight  Grade = "tight"  // it runs, but takes too much of the memory
	NoRoom Grade = "memory" // it doesn't fit in memory
	NoDisk Grade = "disk"   // the disk hasn't room for it
	// NoRuntime: llama.cpp's build for this computer won't run on it, as
	// its C library is older than the build's.
	NoRuntime Grade = "runtime"
)

// libcNeed is the glibc llama.cpp's Linux builds need, by architecture:
// the newest version readelf finds among their symbols (b11583's x64
// builds, the CPU's and Vulkan's, are made on Ubuntu 22.04, arm64's on
// 24.04).
var libcNeed = map[string]string{"amd64": "2.34", "arm64": "2.38"}

// libcSystems are the systems that have a glibc that new.
var libcSystems = map[string]string{"2.34": "Ubuntu 22.04, Debian 12 or later", "2.38": "Ubuntu 24.04, Debian 13 or later"}

// libcShort is the glibc llama.cpp needs here and doesn't find, "" when
// it finds it or can't tell.
func libcShort(h Hardware) string {
	need, ok := libcNeed[h.Arch]
	have, found := strings.CutPrefix(h.Libc, "glibc ")
	if h.OS != "linux" || !ok || !found || !older(have, need) {
		return ""
	}
	return need
}

// older: version a comes before b (both major.minor).
func older(a, b string) bool {
	pa, pb := strings.SplitN(a, ".", 3), strings.SplitN(b, ".", 3)
	for i := range 2 {
		x, y := 0, 0
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// Speed is how fast a model ran here, measured.
type Speed struct {
	Prompt  float64   `json:"prompt"` // tokens a second, reading a prompt
	Gen     float64   `json:"gen"`    // tokens a second, writing
	Backend string    `json:"backend,omitempty"`
	Build   string    `json:"build,omitempty"`
	At      time.Time `json:"at"`
}

// Assessment is this computer, measured, and the model recommended for it.
type Assessment struct {
	At       time.Time `json:"at"`
	Hardware Hardware  `json:"hardware"`
	Bench    Bench     `json:"bench"`
	// Quick: no bench has run here yet, and the estimates take a computer
	// of this kind's usual figures.
	Quick bool `json:"quick,omitempty"`
	// Disk is the space free where models go, bytes; 0 when unknown.
	Disk   uint64     `json:"disk"`
	Models []Estimate `json:"models"`
	// Pick is the model recommended (an ID of Candidates), "" when none is
	// worth running here.
	Pick string `json:"pick"`
	// Why is the reason, one of: fast (the 4B runs smoothly), 4b-memory,
	// 4b-disk, 4b-slow (the 1.7B, as the 4B takes too much memory, hasn't
	// room on the disk, or is too slow), memory, disk, slow (none, as even
	// the 1.7B is so), runtime (none, as llama.cpp won't run here).
	Why string `json:"why"`
	// LibcNeed is the glibc llama.cpp needs, when this Linux's is older.
	LibcNeed string `json:"libcNeed,omitempty"`
	// Speeds are what models ran at here, by ID.
	Speeds map[string]Speed `json:"speeds,omitempty"`
}

// Estimate is the one for the model with id.
func (a Assessment) Estimate(id string) (Estimate, bool) {
	for _, e := range a.Models {
		if e.Model == id {
			return e, true
		}
	}
	return Estimate{}, false
}

// What a computer's figures make of llama.cpp's speed, from llama-bench
// (b11583) beside the bench above, on an M4 Pro (20 GPU cores; memory read
// at 224–244 GB/s) with Metal, and in Linux on four of its cores (20–21
// billion multiply-adds a second; memory at 138–152 GB/s): Spark X2.5 1.7B
// (Q4_K_M, 1.11 GB) and Qwen3.5 2B (Q4_K_M, 1.28 GB).
const (
	// Writing a token reads every weight once: tokens a second ≈ a share
	// of the bandwidth over the file's size. With Metal Spark wrote 161
	// tokens a second (178 GB/s), Qwen 128 (164 GB/s); in Linux Spark 89
	// (99 of 152 GB/s), Qwen 65 (83 of 138).
	genShareGPU = 0.65
	genShareCPU = 0.55
	// With few cores the CPU computes slower than it reads: on four, Spark
	// wrote 89 tokens a second, 7 per multiply-add of the bench over its
	// parameters, Qwen 29 to 65 as its cores were slow or fast ones.
	cpuGenPerOp = 6
	// Reading a prompt is arithmetic: tokens a second ≈ parameters·tokens
	// a second over the parameters. With Metal Spark read 2086 tokens a
	// second and Qwen 1971, about 180 billion parameter·tokens per GPU
	// core; in Linux Spark 201 and Qwen 176, 16 per multiply-add.
	appleCorePrompt = 185e9
	cpuPromptPerOp  = 16
	// An NVIDIA card fogway can't measure: a modest one's figures (an RTX
	// 3060 does three times them).
	nvidiaPrompt    = 4e12
	nvidiaBandwidth = 200e9
	// Without a bench: a computer's usual figures.
	quickBandwidthApple = 100 // GB/s
	quickBandwidthPC    = 30
	quickComputePerCore = 3 // billion multiply-adds a second
)

// A turn's judgement for group/auto: the router's instructions and the
// user's message in, a short JSON answer out, and the request's own time.
const (
	judgeIn       = 700
	judgeOut      = 40
	judgeOverhead = 0.15
)

// Grades' bounds: a judgement within Smooth's seconds, writing at least
// its tokens a second, is smooth; within Usable's, usable.
const (
	smoothJudge, smoothGen = 4.0, 15.0
	usableJudge, usableGen = 6.0, 8.0
)

var appleGen = regexp.MustCompile(`Apple M(\d+)`)

// appleClock is an Apple GPU core's speed against an M4's: M1's ran at
// about four fifths of its clock.
func appleClock(cpu string) float64 {
	m := appleGen.FindStringSubmatch(cpu)
	if m == nil {
		return 0.8
	}
	switch n, _ := strconv.Atoi(m[1]); {
	case n <= 1:
		return 0.8
	case n == 2:
		return 0.85
	case n == 3:
		return 0.9
	}
	return 1
}

// simdShare is how much of the calibrated CPU speed llama.cpp gets
// without AVX2 or NEON.
func simdShare(h Hardware) float64 {
	switch h.SIMD {
	case "avx512", "avx2", "neon":
		return 1
	case "avx":
		return 0.5
	}
	return 0.25
}

// estimate is how m would run on h with bench b.
func estimate(h Hardware, b Bench, m Model) Estimate {
	e := Estimate{Model: m.ID(), Title: m.Title, Size: m.Size, Need: m.Need, Where: "cpu"}
	params, size := float64(m.Params), float64(m.Size)
	if params == 0 {
		params = size * 8 / 4.85 // Q4_K_M's bits a weight
	}
	bw, comp := b.Bandwidth*1e9, b.Compute*1e9
	if bw == 0 {
		bw = quickBandwidthPC * 1e9
		if h.Unified {
			bw = quickBandwidthApple * 1e9
		}
	}
	if comp == 0 {
		comp = float64(max(h.Cores, 1)) * quickComputePerCore * 1e9
	}
	switch {
	case h.Unified:
		e.Where = "gpu"
		cores := float64(h.GPUCores)
		if cores == 0 {
			cores = 8
		}
		e.Prompt = appleCorePrompt * appleClock(h.CPU) * cores / params
		e.Gen = genShareGPU * bw / size
	case h.NVIDIA && float64(h.VRAM)*0.85 >= float64(m.Need):
		e.Where = "gpu"
		e.Prompt = nvidiaPrompt / params
		e.Gen = genShareGPU * nvidiaBandwidth / size
	default:
		s := simdShare(h)
		e.Prompt = cpuPromptPerOp * comp / params * s
		e.Gen = min(genShareCPU*bw/size, cpuGenPerOp*comp/params*s)
	}
	return e
}

// judge is the seconds a judgement takes at e's speeds.
func (e Estimate) judge() float64 {
	if e.Prompt <= 0 || e.Gen <= 0 {
		return math.Inf(1)
	}
	return judgeIn/e.Prompt + judgeOut/e.Gen + judgeOverhead
}

// grade is how well e runs on h with disk bytes free: memory first, then
// the disk, then speed.
func (e Estimate) grade(h Hardware, disk uint64, downloaded bool) Grade {
	need := float64(e.Need)
	switch {
	case h.RAM == 0:
		// memory unknown: speed alone decides
	case e.Where == "gpu" && h.NVIDIA:
		if h.RAM < 7<<30 {
			return Tight
		}
	case need*3 <= float64(h.RAM):
		// a third of the memory at most, the rest the system's and the agents'
	case e.Need <= h.Budget():
		return Tight
	default:
		return NoRoom
	}
	if !downloaded && disk > 0 && disk < e.Size+1<<30 {
		return NoDisk
	}
	switch j := e.judge(); {
	case j <= smoothJudge && e.Gen >= smoothGen:
		return Smooth
	case j <= usableJudge && e.Gen >= usableGen:
		return Usable
	}
	return Slow
}

// assess is the assessment of h with bench b, disk bytes free and the
// speeds measured here: each candidate estimated (or measured), graded,
// and the pick.
func assess(h Hardware, b Bench, disk uint64, speeds map[string]Speed, downloaded func(Model) bool) Assessment {
	a := Assessment{Hardware: h, Bench: b, Disk: disk, Speeds: speeds}
	// a model measured here sets the others' estimates right, the one
	// measured last; a measurement on another backend says nothing of them
	var kp, kg float64
	var last time.Time
	for id, s := range speeds {
		m, ok := CatalogModel(id)
		if ok && s.At.After(last) && s.Prompt > 0 && s.Gen > 0 {
			e := estimate(h, b, m)
			kp, kg, last = s.Prompt/e.Prompt, s.Gen/e.Gen, s.At
		}
	}
	for _, m := range Candidates {
		e := estimate(h, b, m)
		if s, ok := speeds[m.ID()]; ok && s.Prompt > 0 && s.Gen > 0 {
			e.Prompt, e.Gen, e.Measured = s.Prompt, s.Gen, true
		} else if kp > 0 {
			e.Prompt, e.Gen, e.Corrected = e.Prompt*kp, e.Gen*kg, true
		}
		e.Judge = round(e.judge(), 1)
		e.Prompt, e.Gen = round(e.Prompt, 0), round(e.Gen, 0)
		e.Grade = e.grade(h, disk, downloaded(m))
		a.Models = append(a.Models, e)
	}
	if need := libcShort(h); need != "" {
		for i := range a.Models {
			a.Models[i].Grade = NoRuntime
		}
		a.Why, a.LibcNeed = "runtime", need
		return a
	}
	small, _ := a.Estimate(Spark17.ID())
	big, _ := a.Estimate(Spark4.ID())
	switch {
	case big.Grade == Smooth:
		a.Pick, a.Why = big.Model, "fast"
	case small.Grade == Smooth || small.Grade == Usable:
		a.Pick = small.Model
		switch big.Grade {
		case NoRoom, Tight:
			a.Why = "4b-memory"
		case NoDisk:
			a.Why = "4b-disk"
		default:
			a.Why = "4b-slow"
		}
	default:
		switch small.Grade {
		case NoRoom, Tight:
			a.Why = "memory"
		case NoDisk:
			a.Why = "disk"
		default:
			a.Why = "slow"
		}
	}
	return a
}

func round(v float64, places int) float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0
	}
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// downloaded: m's file is here already, so the disk needn't have room.
func downloaded(m Model) bool {
	_, err := os.Stat(ModelPath(m))
	return err == nil
}

// assessPath is where the last assessment is kept: beside the models.
func assessPath() string { return filepath.Join(Dir(), "assessment.json") }

// assessMu keeps one assessment written at a time.
var assessMu sync.Mutex

// Assess measures this computer (about a second), assesses it, and keeps
// the result, with the speeds measured on it before.
func Assess() Assessment {
	h := Detect()
	b := measureBench(h.RAM, h.Cores)
	assessMu.Lock()
	defer assessMu.Unlock()
	speeds := map[string]Speed{}
	if old, ok := loadAssessment(); ok && sameHardware(old.Hardware, h) {
		speeds = old.Speeds
	}
	a := assess(h, b, diskFree(Dir()), speeds, downloaded)
	a.At = time.Now()
	saveAssessment(a)
	return a
}

// Assessed is the assessment of this computer: the last one kept, its
// estimates made again (a model downloaded since, the disk as it is now),
// or a quick one from no bench when none was kept on this hardware.
func Assessed() Assessment {
	h := Detect()
	if old, ok := loadAssessment(); ok && sameHardware(old.Hardware, h) {
		a := assess(h, old.Bench, diskFree(Dir()), old.Speeds, downloaded)
		a.At = old.At
		return a
	}
	a := assess(h, Bench{}, diskFree(Dir()), nil, downloaded)
	a.Quick = true
	return a
}

// sameHardware: an assessment of a is good for b.
func sameHardware(a, b Hardware) bool {
	return a.OS == b.OS && a.Arch == b.Arch && a.RAM == b.RAM && a.CPU == b.CPU && a.Cores == b.Cores &&
		a.GPU == b.GPU && a.VRAM == b.VRAM && a.GPUCores == b.GPUCores
}

func loadAssessment() (Assessment, bool) {
	var a Assessment
	b, err := os.ReadFile(assessPath())
	if err != nil || json.Unmarshal(b, &a) != nil || a.Hardware.OS == "" {
		return Assessment{}, false
	}
	return a, true
}

func saveAssessment(a Assessment) {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(Dir(), 0o755) == nil {
		edit.WriteAtomic(assessPath(), append(b, '\n'))
	}
}

// RecordSpeed keeps what model id was measured at on this computer.
func RecordSpeed(id string, s Speed) Assessment {
	assessMu.Lock()
	defer assessMu.Unlock()
	h := Detect()
	old, ok := loadAssessment()
	if !ok || !sameHardware(old.Hardware, h) {
		old = Assessment{Hardware: h, Quick: true}
	}
	speeds := map[string]Speed{}
	for k, v := range old.Speeds {
		speeds[k] = v
	}
	speeds[id] = s
	a := assess(h, old.Bench, diskFree(Dir()), speeds, downloaded)
	a.At, a.Quick = old.At, old.Quick
	if a.At.IsZero() {
		a.At = time.Now()
	}
	saveAssessment(a)
	return a
}

// measuredHere: model id was measured on this computer with this backend
// and build.
func measuredHere(id, backend, build string) bool {
	a, ok := loadAssessment()
	if !ok || !sameHardware(a.Hardware, Detect()) {
		return false
	}
	s, ok := a.Speeds[id]
	return ok && s.Backend == backend && s.Build == build
}

// measurePrompt is what Measure has the model read: about 700 tokens of
// code and prose, as a turn's message and the router's instructions are.
var measurePrompt = strings.Repeat(`// retry calls f until it succeeds or ctx ends, waiting longer each time.
func retry(ctx context.Context, f func() error) error {
	wait := 100 * time.Millisecond
	for {
		if err := f(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		wait = min(wait*2, 10*time.Second)
	}
}
The function above retries an operation with exponential backoff. 请解释它在网络抖动时的行为，以及如何给它加上最大重试次数。
`, 5)

// Measure has the model llama-server serves at url (its OpenAI address,
// …/v1) read a prompt and write, twice, once it is idle, and is the faster
// of the two at each: the first warms it up, and a request of an agent's
// that comes in meanwhile slows one, not both.
func Measure(ctx context.Context, url string) (Speed, error) {
	base := strings.TrimSuffix(strings.TrimSuffix(url, "/"), "/v1")
	body, _ := json.Marshal(map[string]any{
		"prompt": measurePrompt, "n_predict": 48, "cache_prompt": false, "ignore_eos": true, "temperature": 0,
	})
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	waitIdle(ctx, client, base)
	var s Speed
	for range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/completion", bytes.NewReader(body))
		if err != nil {
			return s, err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return s, err
		}
		var out struct {
			Timings struct {
				PromptN   int     `json:"prompt_n"`
				PromptTPS float64 `json:"prompt_per_second"`
				GenN      int     `json:"predicted_n"`
				GenTPS    float64 `json:"predicted_per_second"`
			} `json:"timings"`
		}
		err = json.NewDecoder(res.Body).Decode(&out)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return s, fmt.Errorf("llama-server answered %s", res.Status)
		}
		if err != nil {
			return s, fmt.Errorf("llama-server's answer: %w", err)
		}
		if out.Timings.PromptN == 0 || out.Timings.GenN == 0 {
			return s, errNoTimings
		}
		s.Prompt, s.Gen = max(s.Prompt, round(out.Timings.PromptTPS, 0)), max(s.Gen, round(out.Timings.GenTPS, 0))
	}
	s.At = time.Now()
	return s, nil
}

// idleWait is how long Measure waits for llama-server to be idle, and how
// often it looks.
var idleWait, idleEvery = 2 * time.Minute, 3 * time.Second

// slot is one of llama-server's /slots.
type slot struct {
	Processing bool `json:"is_processing"`
}

func (s slot) busy() bool { return s.Processing }

// waitIdle waits until none of llama-server's slots is busy with a
// request, as its /slots says, or idleWait has gone by; at once when it
// doesn't say.
func waitIdle(ctx context.Context, client *http.Client, base string) {
	deadline := time.Now().Add(idleWait)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/slots", nil)
		if err != nil {
			return
		}
		res, err := client.Do(req)
		if err != nil {
			return
		}
		var slots []slot
		err = json.NewDecoder(res.Body).Decode(&slots)
		res.Body.Close()
		if err != nil || res.StatusCode != http.StatusOK || !slices.ContainsFunc(slots, slot.busy) {
			return
		}
		select {
		case <-time.After(idleEvery):
		case <-ctx.Done():
			return
		}
	}
}

var errNoTimings = errors.New("llama-server said nothing of how fast it went")

// MeasureServing measures the model running now and keeps the speed.
func MeasureServing(ctx context.Context) (Assessment, error) {
	st := Now()
	if st.Phase != Ready || st.URL == "" {
		return Assessment{}, ErrOff
	}
	s, err := Measure(ctx, st.URL)
	if err != nil {
		return Assessment{}, err
	}
	s.Backend, s.Build = st.Backend, st.Build
	return RecordSpeed(Canon(st.Model).ID(), s), nil
}

// Canon is the catalog's model for m's repository and file, so that a
// model given by them is known by the catalog's name; m itself when the
// catalog hasn't it.
func Canon(m Model) Model {
	for _, c := range Catalog {
		if strings.EqualFold(c.Repo, m.Repo) && strings.EqualFold(c.File, m.File) {
			return c
		}
	}
	return m
}

// Reason is Why in a sentence, with the figures it rests on.
func (a Assessment) Reason() string {
	small, _ := a.Estimate(Spark17.ID())
	big, _ := a.Estimate(Spark4.ID())
	gb := func(n uint64) string { return fmt.Sprintf("%.1f GB", float64(n)/(1<<30)) }
	switch a.Why {
	case "fast":
		return fmt.Sprintf("it judges a turn in about %.1f s here and writes %.0f tokens a second", big.Judge, big.Gen)
	case "4b-memory":
		return fmt.Sprintf("%s would take %s, too much of this computer's %s; %s judges a turn in about %.1f s", big.Title, gb(big.Need), gb(a.Hardware.RAM), small.Title, small.Judge)
	case "4b-disk":
		return fmt.Sprintf("the disk has %s free, too little for %s (%s); %s judges a turn in about %.1f s", gb(a.Disk), big.Title, gb(big.Size), small.Title, small.Judge)
	case "4b-slow":
		return fmt.Sprintf("%s would take about %.1f s to judge each turn, %s about %.1f s", big.Title, big.Judge, small.Title, small.Judge)
	case "memory":
		return fmt.Sprintf("even %s would take %s, too much of this computer's %s", small.Title, gb(small.Need), gb(a.Hardware.RAM))
	case "disk":
		return fmt.Sprintf("the disk has %s free, too little for %s (%s)", gb(a.Disk), small.Title, gb(small.Size))
	case "slow":
		return fmt.Sprintf("even %s would take about %.1f s to judge each turn and write %.0f tokens a second", small.Title, small.Judge, small.Gen)
	case "runtime":
		return fmt.Sprintf("llama.cpp's builds for this computer need glibc %s or newer, and this Linux has %s: a newer system runs them (%s)", a.LibcNeed, strings.TrimPrefix(a.Hardware.Libc, "glibc "), libcSystems[a.LibcNeed])
	}
	return ""
}
