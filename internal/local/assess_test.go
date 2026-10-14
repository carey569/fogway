package local

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const gib = uint64(1) << 30

// Computers of each kind get the model they run well, the 1.7B when the
// 4B would take too much memory or be slow, and none when even the 1.7B
// would: their figures are what the bench reads on such computers.
func TestAssessPicks(t *testing.T) {
	m4pro := Hardware{OS: "darwin", Arch: "arm64", RAM: 24 * gib, Unified: true, CPU: "Apple M4 Pro", Cores: 10, GPUCores: 20, SIMD: "neon"}
	m1 := Hardware{OS: "darwin", Arch: "arm64", RAM: 8 * gib, Unified: true, CPU: "Apple M1", Cores: 4, GPUCores: 8, SIMD: "neon"}
	m1x16 := m1
	m1x16.RAM = 16 * gib
	laptop := Hardware{OS: "windows", Arch: "amd64", RAM: 16 * gib, CPU: "11th Gen Intel(R) Core(TM) i5-1135G7", Cores: 4, SIMD: "avx512"}
	desktop := Hardware{OS: "linux", Arch: "amd64", RAM: 32 * gib, CPU: "AMD Ryzen 7 5800X", Cores: 8, SIMD: "avx2"}
	rtx := Hardware{OS: "windows", Arch: "amd64", RAM: 16 * gib, NVIDIA: true, GPU: "NVIDIA GeForce RTX 3060", VRAM: 12 * gib, Cores: 6, SIMD: "avx2"}
	gtx := Hardware{OS: "linux", Arch: "amd64", RAM: 8 * gib, NVIDIA: true, GPU: "NVIDIA GeForce GTX 1650", VRAM: 4 * gib, Cores: 4, SIMD: "avx2"}
	small := Hardware{OS: "linux", Arch: "amd64", RAM: 4 * gib, Cores: 4, SIMD: "avx2"}
	five, six := small, small
	five.RAM, six.RAM = 5*gib, 6*gib
	old := desktop
	old.SIMD = ""

	fast := Bench{Bandwidth: 224, Compute: 52}
	none := func(Model) bool { return false }
	for _, c := range []struct {
		name      string
		h         Hardware
		b         Bench
		disk      uint64
		got       func(Model) bool
		pick, why string
	}{
		{"M4 Pro", m4pro, fast, 200 * gib, none, Spark4.ID(), "fast"},
		{"M1, 8 GB", m1, Bench{60, 20}, 100 * gib, none, Spark17.ID(), "4b-memory"},
		{"M1, 16 GB", m1x16, Bench{60, 20}, 100 * gib, none, Spark17.ID(), "4b-slow"},
		{"a laptop's CPU", laptop, Bench{30, 14}, 100 * gib, none, "", "slow"},
		{"a desktop's CPU", desktop, Bench{45, 36}, 100 * gib, none, Spark17.ID(), "4b-slow"},
		{"an RTX 3060", rtx, Bench{40, 30}, 100 * gib, none, Spark4.ID(), "fast"},
		{"a 4 GB card", gtx, Bench{30, 20}, 100 * gib, none, Spark17.ID(), "4b-memory"},
		{"4 GB of memory", small, Bench{20, 12}, 100 * gib, none, "", "memory"},
		{"5 GB of memory", five, Bench{20, 12}, 100 * gib, none, "", "memory"},
		{"6 GB of memory: room for the 1.7B, too slow", six, Bench{20, 12}, 100 * gib, none, "", "slow"},
		{"no AVX", old, Bench{45, 36}, 100 * gib, none, "", "slow"},
		{"a full disk", m4pro, fast, 2 * gib, none, "", "disk"},
		{"room for the 1.7B only", m4pro, fast, 3 * gib, none, Spark17.ID(), "4b-disk"},
		{"the 4B downloaded already", m4pro, fast, 2 * gib, func(m Model) bool { return m.ID() == Spark4.ID() }, Spark4.ID(), "fast"},
		{"no disk figure", m4pro, fast, 0, none, Spark4.ID(), "fast"},
	} {
		a := assess(c.h, c.b, c.disk, nil, c.got)
		if a.Pick != c.pick || a.Why != c.why {
			t.Errorf("%s: pick %q (%s), want %q (%s): %+v", c.name, a.Pick, a.Why, c.pick, c.why, a.Models)
		}
		if len(a.Models) != len(Candidates) || a.Reason() == "" {
			t.Errorf("%s: %d models, reason %q", c.name, len(a.Models), a.Reason())
		}
		for _, e := range a.Models {
			if e.Measured || e.Corrected || e.Judge <= 0 || e.Prompt <= 0 || e.Gen <= 0 {
				t.Errorf("%s: %+v", c.name, e)
			}
		}
	}
}

// The estimates rest on llama-bench's figures beside the bench's on the
// same computer: the M4 Pro they were taken on comes out at them, within
// a fifth, and never above what was measured by more than that.
func TestEstimatesMatchTheirCalibration(t *testing.T) {
	m4 := Hardware{Unified: true, CPU: "Apple M4 Pro", GPUCores: 20, Cores: 10, RAM: 24 * gib}
	linux4 := Hardware{OS: "linux", Cores: 4, SIMD: "neon", RAM: 8 * gib}
	qwen := Model{Name: "q", Size: 1_280_835_840, Params: 1_880_000_000}
	for _, c := range []struct {
		what               string
		h                  Hardware
		b                  Bench
		m                  Model
		prompt, gen, loGen float64 // measured; loGen: the slowest it wrote
	}{
		{"Spark 1.7B, Metal", m4, Bench{Bandwidth: 224, Compute: 50}, Spark17, 2086, 161, 161},
		{"Spark 1.7B, four cores", linux4, Bench{Bandwidth: 152, Compute: 21}, Spark17, 201, 89, 89},
		{"Spark 4B, Metal", m4, Bench{Bandwidth: 224, Compute: 50}, Spark4, 815, 77, 77},
		{"Spark 4B, four cores", linux4, Bench{Bandwidth: 152, Compute: 21}, Spark4, 83, 40, 40},
		{"Qwen 2B, Metal", m4, Bench{Bandwidth: 244, Compute: 52}, qwen, 1971, 128, 128},
		{"Qwen 2B, four cores", linux4, Bench{Bandwidth: 138, Compute: 20.4}, qwen, 176, 65, 29},
	} {
		e := estimate(c.h, c.b, c.m)
		if e.Prompt < c.prompt*0.8 || e.Prompt > c.prompt*1.2 {
			t.Errorf("%s: reads %.0f tokens a second, measured %.0f", c.what, e.Prompt, c.prompt)
		}
		if e.Gen < c.loGen*0.8 || e.Gen > c.gen*1.2 {
			t.Errorf("%s: writes %.0f tokens a second, measured %.0f–%.0f", c.what, e.Gen, c.loGen, c.gen)
		}
	}
}

// A model measured here is taken as measured, and sets the other's
// estimate right: a 4B that ran slow makes the 1.7B the pick.
func TestMeasuredSpeedsCorrectTheEstimates(t *testing.T) {
	h := Hardware{OS: "darwin", Arch: "arm64", RAM: 24 * gib, Unified: true, CPU: "Apple M4 Pro", Cores: 10, GPUCores: 20}
	b := Bench{Bandwidth: 224, Compute: 52}
	before := assess(h, b, 0, nil, downloaded)
	if before.Pick != Spark4.ID() {
		t.Fatalf("before: %+v", before)
	}
	est, _ := before.Estimate(Spark17.ID())
	slow := map[string]Speed{Spark4.ID(): {Prompt: 100, Gen: 10, Backend: "metal", At: time.Now()}}
	a := assess(h, b, 0, slow, downloaded)
	big, _ := a.Estimate(Spark4.ID())
	small, _ := a.Estimate(Spark17.ID())
	if !big.Measured || big.Prompt != 100 || big.Gen != 10 || big.Grade != Slow {
		t.Errorf("4B: %+v", big)
	}
	if !small.Corrected || small.Measured || small.Prompt >= est.Prompt || small.Gen >= est.Gen {
		t.Errorf("1.7B: %+v (estimated %+v)", small, est)
	}
	if a.Pick != Spark17.ID() || a.Why != "4b-slow" {
		t.Errorf("pick %q (%s)", a.Pick, a.Why)
	}
	// a model of the catalog other than the candidates sets them right too
	other := map[string]Speed{"qwen3.5-2b": {Prompt: 20, Gen: 2, At: time.Now()}}
	if a := assess(h, b, 0, other, downloaded); a.Pick != "" || a.Why != "slow" {
		t.Errorf("after a slow Qwen: %q (%s) %+v", a.Pick, a.Why, a.Models)
	}
}

// The bench reads memory and computes, and is quick about it.
func TestBench(t *testing.T) {
	t0 := time.Now()
	b := measureBench(2*gib, 1)
	if b.Bandwidth <= 0 || b.Compute <= 0 {
		t.Fatalf("%+v", b)
	}
	if d := time.Since(t0); d > 10*time.Second {
		t.Errorf("the bench took %s", d)
	}
}

// An assessment is kept with the speeds measured since, and a computer
// with other hardware isn't taken for this one.
func TestAssessmentKept(t *testing.T) {
	os.RemoveAll(Dir())
	if a := Assessed(); !a.Quick || a.Bench.Bandwidth != 0 {
		t.Fatalf("none kept: %+v", a)
	}
	a := Assess()
	if a.Quick || a.Bench.Bandwidth <= 0 || a.At.IsZero() || !sameHardware(a.Hardware, Detect()) {
		t.Fatalf("assessed: %+v", a)
	}
	if got := Assessed(); got.Quick || got.Bench != a.Bench || got.Pick != a.Pick {
		t.Fatalf("kept: %+v, assessed %+v", got, a)
	}
	if measuredHere(Spark17.ID(), "metal", "b1") {
		t.Fatal("measured before it was")
	}
	RecordSpeed(Spark17.ID(), Speed{Prompt: 500, Gen: 50, Backend: "metal", Build: "b1", At: time.Now()})
	if !measuredHere(Spark17.ID(), "metal", "b1") || measuredHere(Spark17.ID(), "metal", "b2") || measuredHere(Spark17.ID(), "cpu", "b1") {
		t.Fatal("measuredHere")
	}
	got := Assessed()
	if e, _ := got.Estimate(Spark17.ID()); !e.Measured || e.Prompt != 500 {
		t.Fatalf("after a measurement: %+v", got.Models)
	}
	// assessed again, the measurement stays
	if e, _ := Assess().Estimate(Spark17.ID()); !e.Measured {
		t.Fatalf("assessed again: %+v", e)
	}
	// another computer's assessment is not this one's
	b, _ := os.ReadFile(assessPath())
	var kept Assessment
	json.Unmarshal(b, &kept)
	kept.Hardware.RAM++
	saveAssessment(kept)
	if a := Assessed(); !a.Quick || len(a.Speeds) != 0 {
		t.Fatalf("other hardware: %+v", a)
	}
	if Recommend().ID() != Spark17.ID() && Recommend().ID() != Spark4.ID() {
		t.Fatalf("recommend: %+v", Recommend())
	}
}

// Measure reads llama-server's own timings of a prompt and an answer,
// the second time it asks, and tells a server that doesn't give them.
func TestMeasure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if r.URL.Path != "/completion" || in["cache_prompt"] != false || in["ignore_eos"] != true || !strings.Contains(in["prompt"].(string), "retry") {
			http.Error(w, "wrong request", 400)
			return
		}
		calls++
		tps := 300.0 * float64(calls) // the first is a warm-up
		json.NewEncoder(w).Encode(map[string]any{"content": "…", "timings": map[string]any{
			"prompt_n": 700, "prompt_per_second": tps, "predicted_n": 48, "predicted_per_second": tps / 10,
		}})
	}))
	defer srv.Close()
	s, err := Measure(context.Background(), srv.URL+"/v1")
	if err != nil || calls != 2 || s.Prompt != 600 || s.Gen != 60 || s.At.IsZero() {
		t.Fatalf("%+v %v (%d calls)", s, err, calls)
	}

	bare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"content":"…"}`))
	}))
	defer bare.Close()
	if _, err := Measure(context.Background(), bare.URL+"/v1"); err != errNoTimings {
		t.Fatalf("no timings: %v", err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "loading model", http.StatusServiceUnavailable)
	}))
	defer down.Close()
	if _, err := Measure(context.Background(), down.URL); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("loading: %v", err)
	}
	if _, err := MeasureServing(context.Background()); err != ErrOff {
		t.Fatalf("nothing serving: %v", err)
	}
}

// Measure waits while llama-server is busy with a request, throws away a
// run another request shared, and keeps the faster of the clean runs at
// each; when no run is clean it says so rather than keep a slow figure.
func TestMeasureWaitsForIdle(t *testing.T) {
	oldWait, oldEvery := idleWait, idleEvery
	idleWait, idleEvery = 5*time.Second, 20*time.Millisecond
	t.Cleanup(func() { idleWait, idleEvery = oldWait, oldEvery })
	type state struct {
		busy, runs, task int
		agentRuns        map[int]bool // the runs an agent's request ran beside
		mine, other      int          // the slots' last tasks
	}
	serve := func(st *state) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/slots" {
				st.busy--
				json.NewEncoder(w).Encode([]map[string]any{
					{"id": 0, "is_processing": false, "id_task": st.mine},
					{"id": 1, "is_processing": st.busy > 0, "id_task": st.other},
				})
				return
			}
			if st.busy > 0 {
				t.Error("measured while a slot was busy")
			}
			st.runs++
			st.task++
			st.mine = st.task
			p, g := 900.0, 20.0 // the first clean run is slow to write, the next slow to read
			if st.runs%2 == 0 {
				p, g = 400, 60
			}
			if st.agentRuns[st.runs] {
				st.task++
				st.other = st.task
				p, g = 100, 5
			}
			json.NewEncoder(w).Encode(map[string]any{"timings": map[string]any{"prompt_n": 700, "prompt_per_second": p, "predicted_n": 48, "predicted_per_second": g}})
		}))
	}

	st := &state{busy: 3, agentRuns: map[int]bool{}}
	srv := serve(st)
	s, err := Measure(context.Background(), srv.URL+"/v1")
	srv.Close()
	if err != nil || s.Prompt != 900 || s.Gen != 60 || st.runs != 2 {
		t.Fatalf("idle: %+v %v (%d runs)", s, err, st.runs)
	}

	// an agent's request beside the first run: it doesn't count
	st = &state{agentRuns: map[int]bool{1: true}}
	srv = serve(st)
	s, err = Measure(context.Background(), srv.URL+"/v1")
	srv.Close()
	if err != nil || s.Prompt != 900 || s.Gen != 60 || st.runs != 3 {
		t.Fatalf("one run shared: %+v %v (%d runs)", s, err, st.runs)
	}

	// beside every run: nothing is kept
	st = &state{agentRuns: map[int]bool{1: true, 2: true, 3: true, 4: true}}
	srv = serve(st)
	_, err = Measure(context.Background(), srv.URL+"/v1")
	srv.Close()
	if err != errBusy || st.runs != 4 {
		t.Fatalf("every run shared: %v (%d runs)", err, st.runs)
	}
}

// A model given by its repository and file is known by the catalog's name.
func TestCanon(t *testing.T) {
	if m := Canon(Model{Repo: "xhtoken/spark-x2.5-4b-gguf", File: Spark4.File}); m.ID() != Spark4.ID() {
		t.Fatalf("%+v", m)
	}
	own := Model{Repo: "me/mine-GGUF", File: "mine.gguf"}
	if m := Canon(own); m != own {
		t.Fatalf("%+v", m)
	}
}

// A Linux whose C library is older than llama.cpp's builds need is told
// so: no model runs there, however fast the computer.
func TestAssessOldLinux(t *testing.T) {
	h := Hardware{OS: "linux", Arch: "amd64", RAM: 32 * gib, Cores: 8, SIMD: "avx2", NVIDIA: true, GPU: "RTX 4070", VRAM: 12 * gib}
	b := Bench{Bandwidth: 60, Compute: 40}
	for libc, want := range map[string]string{"glibc 2.31": "runtime", "glibc 2.33": "runtime", "glibc 2.34": "fast", "glibc 2.39": "fast", "": "fast", "musl": "fast"} {
		h.Libc = libc
		a := assess(h, b, 0, nil, downloaded)
		if a.Why != want || (want == "runtime") != (a.Pick == "") {
			t.Errorf("%s: %q (%s)", libc, a.Pick, a.Why)
		}
		if want == "runtime" && (a.LibcNeed != "2.34" || a.Models[0].Grade != NoRuntime || !strings.Contains(a.Reason(), strings.TrimPrefix(libc, "glibc "))) {
			t.Errorf("%s: %+v, %s", libc, a, a.Reason())
		}
	}
	h.Arch = "arm64"
	h.Libc = "glibc 2.36" // Debian 12's
	if a := assess(h, b, 0, nil, downloaded); a.Why != "runtime" || a.LibcNeed != "2.38" || !strings.Contains(a.Reason(), "Ubuntu 24.04") {
		t.Errorf("arm64: %+v %s", a, a.Reason())
	}
	if !older("2.9", "2.34") || older("2.34", "2.34") || older("3.0", "2.38") {
		t.Error("older")
	}
}
