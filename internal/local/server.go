package local

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/carey569/fogway/internal/proc"
)

// Phase is where the local model is: off, getting ready, or serving.
type Phase string

const (
	Off         Phase = "off"
	Preparing   Phase = "preparing"   // finding or downloading llama.cpp
	Downloading Phase = "downloading" // pulling the model
	Starting    Phase = "starting"    // loading it into llama-server
	Ready       Phase = "ready"
	Failed      Phase = "failed"
)

// Status is the local model as the GUI and the CLI show it.
type Status struct {
	Phase    Phase     `json:"phase"`
	Model    Model     `json:"model"`
	Backend  string    `json:"backend,omitempty"`
	Build    string    `json:"build,omitempty"`
	Port     int       `json:"port,omitempty"`
	Context  int       `json:"context,omitempty"` // the window it runs at, in tokens
	URL      string    `json:"url,omitempty"`     // its OpenAI API: http://127.0.0.1:<port>/v1
	Progress *Progress `json:"progress,omitempty"`
	Error    string    `json:"error,omitempty"`
	Since    time.Time `json:"since"`
	Hardware Hardware  `json:"hardware"`
}

// The one llama-server this process runs.
var srv struct {
	sync.Mutex
	status Status
	cancel context.CancelFunc
	done   chan struct{}
}

// Now is the local model's status.
func Now() Status {
	srv.Lock()
	defer srv.Unlock()
	s := srv.status
	if s.Phase == "" {
		s.Phase = Off
	}
	return s
}

func set(f func(*Status)) {
	srv.Lock()
	f(&srv.status)
	st := srv.status
	srv.Unlock()
	for _, h := range changed {
		h(st)
	}
}

// changed are told of each change of the status (the provider entry the
// gateway serves the model under follows it).
var changed []func(Status)

// OnChange has f told of every change of the status.
func OnChange(f func(Status)) { changed = append(changed, f) }

// Start runs the local model in the background as c says: llama.cpp and the
// model downloaded when missing, llama-server started, and started again
// when it stops, until Stop or ctx's end. A Start while one runs restarts
// it with c.
func Start(ctx context.Context, c Config) {
	Stop()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	srv.Lock()
	srv.cancel, srv.done = cancel, done
	srv.Unlock()
	go func() {
		defer close(done)
		run(ctx, c)
	}()
}

// Stop ends the local model, and waits for llama-server to exit.
func Stop() {
	srv.Lock()
	cancel, done := srv.cancel, srv.done
	srv.cancel, srv.done = nil, nil
	srv.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	set(func(s *Status) { *s = Status{Phase: Off, Since: time.Now(), Hardware: s.Hardware} })
}

// run gets ready and serves, again after each crash (sooner after a run
// that lasted), until ctx ends.
func run(ctx context.Context, c Config) {
	h := Detect()
	m := c.Picked()
	backend := c.Backend
	if backend == "" {
		backend = h.Backend()
	}
	set(func(s *Status) {
		*s = Status{Phase: Preparing, Model: m, Backend: backend, Context: c.ContextOr(), Since: time.Now(), Hardware: h}
	})
	fail := func(err error) {
		log.Println("local model:", err)
		set(func(s *Status) { s.Phase, s.Error, s.Progress, s.Since = Failed, err.Error(), nil, time.Now() })
	}
	progress := func(p Progress) {
		set(func(s *Status) { s.Progress = &p })
	}
	bin, err := ensureRuntime(ctx, c, backend, progress)
	if err != nil && backend != "cpu" && ctx.Err() == nil {
		// a backend's build may be missing for this platform: the CPU's runs anywhere
		log.Println("local model:", err, "- trying the CPU build")
		backend = "cpu"
		set(func(s *Status) { s.Backend = backend })
		bin, err = ensureRuntime(ctx, c, backend, progress)
	}
	if err != nil {
		fail(err)
		return
	}
	set(func(s *Status) { s.Phase, s.Progress, s.Build = Downloading, nil, buildOf(bin) })
	model, err := pullModel(ctx, c, m, progress)
	if err != nil {
		fail(err)
		return
	}
	wait := time.Second
	for ctx.Err() == nil {
		began := time.Now()
		err := serve(ctx, c, bin, model, m)
		if ctx.Err() != nil {
			return
		}
		fail(err)
		if time.Since(began) > time.Minute {
			wait = time.Second
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
		wait = min(wait*2, time.Minute)
	}
}

// buildOf is the llama.cpp build a llama-server under RuntimeDir is of:
// the folder's name under llama.cpp/, less its backend.
func buildOf(bin string) string {
	for d := filepath.Dir(bin); d != filepath.Dir(d); d = filepath.Dir(d) {
		if filepath.Base(filepath.Dir(d)) == "llama.cpp" {
			tag, _, _ := strings.Cut(filepath.Base(d), "-")
			return tag
		}
	}
	return ""
}

// serve runs llama-server until it exits or ctx ends.
func serve(ctx context.Context, c Config, bin, model string, m Model) error {
	port := c.Port
	if port == 0 {
		port = freePort(3731)
	}
	ngl := "999" // every layer the GPU takes
	if c.GPULayers > 0 {
		ngl = strconv.Itoa(c.GPULayers)
	} else if c.GPULayers < 0 {
		ngl = "0"
	}
	args := []string{
		"-m", model,
		"--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"-a", m.ID(),
		"-c", strconv.Itoa(c.ContextOr()),
		"-ngl", ngl,
		"--jinja",     // the model's own chat template: tool calls
		"-fa", "auto", // flash attention where the backend has it
		"--no-webui",      // the gateway is what agents talk to
		"--parallel", "2", // a turn and the router's question at once
		// the two share the window: each request may take all of it, as
		// the provider says it does, rather than half each
		"--kv-unified",
		"--reasoning-format", "deepseek", // thinking apart, as reasoning_content
		"--reasoning", reasoning(c),
	}
	cmd := proc.Command(bin, args...)
	cmd.Dir = filepath.Dir(bin)
	cmd.Env = append(os.Environ(), libraryPath(filepath.Dir(bin))...)
	logPath := filepath.Join(Dir(), "llama-server.log")
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		cmd.Stdout, cmd.Stderr = lf, lf
		defer lf.Close()
	}
	set(func(s *Status) {
		s.Phase, s.Port, s.URL, s.Error, s.Progress, s.Since = Starting, port, fmt.Sprintf("http://127.0.0.1:%d/v1", port), "", nil, time.Now()
	})
	reapStale()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting llama-server: %w", err)
	}
	os.WriteFile(pidPath(), []byte(fmt.Sprintf("%d %d\n", cmd.Process.Pid, os.Getpid())), 0o600)
	var exitErr error
	exited := make(chan struct{}) // closed once it has exited, exitErr why
	go func() { exitErr = cmd.Wait(); os.Remove(pidPath()); close(exited) }()
	ready := make(chan struct{})
	go func() {
		if waitHealthy(ctx, port, exited) {
			close(ready)
		}
	}()
	select {
	case <-ready:
		set(func(s *Status) { s.Phase, s.Since = Ready, time.Now() })
		go measureOnce(ctx, m)
	case <-exited:
		return fmt.Errorf("llama-server stopped as it loaded %s (%v): see %s", m.ID(), exitErr, logPath)
	case <-ctx.Done():
		stopProcess(cmd, exited)
		return ctx.Err()
	}
	select {
	case <-exited:
		return fmt.Errorf("llama-server stopped (%v): see %s", exitErr, logPath)
	case <-ctx.Done():
		stopProcess(cmd, exited)
		return ctx.Err()
	}
}

// measureOnce measures m as it starts to serve, the first time it runs
// here with this build: what the assessment then goes by.
func measureOnce(ctx context.Context, m Model) {
	st := Now()
	if measuredHere(Canon(m).ID(), st.Backend, st.Build) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, err := MeasureServing(ctx); err != nil && ctx.Err() == nil {
		log.Println("local model: measuring", m.ID()+":", err)
	}
}

// waitHealthy waits for llama-server's /health to answer 200, which it
// does once the model is loaded; false when it exits or ctx ends first.
func waitHealthy(ctx context.Context, port int, exited <-chan struct{}) bool {
	u := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	for {
		if res, err := client.Get(u); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return true
			}
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return false
		case <-exited:
			return false
		}
	}
}

// stopProcess asks llama-server to stop, and kills it if it doesn't.
func stopProcess(cmd *exec.Cmd, exited <-chan struct{}) {
	if cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		cmd.Process.Kill()
	} else {
		cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-exited
	}
}

// libraryPath lets llama-server find the libraries it came with.
func libraryPath(dir string) []string {
	switch runtime.GOOS {
	case "linux":
		return []string{"LD_LIBRARY_PATH=" + dir + string(os.PathListSeparator) + os.Getenv("LD_LIBRARY_PATH")}
	case "darwin":
		return []string{"DYLD_LIBRARY_PATH=" + dir}
	}
	return nil
}

// freePort is the first port from from up that nothing listens on.
func freePort(from int) int {
	for p := from; p < from+100; p++ {
		l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err == nil {
			l.Close()
			return p
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return from
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// ErrOff is a request for the local model while it isn't running.
var ErrOff = errors.New("the local model is off")

// Prepare downloads what c runs when it isn't here yet — llama.cpp's build
// for this computer and the model — without starting it, and returns the
// model's path. fogway local setup runs it in the terminal; Start does the
// same in the background.
func Prepare(ctx context.Context, c Config, tell func(Progress)) (string, error) {
	h := Detect()
	backend := c.Backend
	if backend == "" {
		backend = h.Backend()
	}
	if _, err := ensureRuntime(ctx, c, backend, tell); err != nil {
		if backend == "cpu" || ctx.Err() != nil {
			return "", err
		}
		if _, err := ensureRuntime(ctx, c, "cpu", tell); err != nil {
			return "", err
		}
	}
	return pullModel(ctx, c, c.Picked(), tell)
}

// reasoning is llama-server's --reasoning: off unless Think, as the turns
// the local model is sent are the small ones, answered at once; a model
// that thinks by default (Qwen3.5) spent an agent's short budget thinking
// and answered nothing.
func reasoning(c Config) string {
	if c.Think {
		return "auto"
	}
	return "off"
}

// pidPath keeps the pid of the llama-server running, and of the fogway
// that runs it, so one left behind by a fogway that ended without stopping
// it (killed, crashed) is ended when the next starts its own.
func pidPath() string { return filepath.Join(Dir(), "llama-server.pid") }

// reapStale ends the llama-server pidPath names when the fogway that ran it
// is gone and it is still a program of Dir()'s: one another fogway runs now
// is left to it.
func reapStale() {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return
	}
	var pid, owner int
	if n, _ := fmt.Sscan(string(b), &pid, &owner); n != 2 || pid <= 0 {
		os.Remove(pidPath())
		return
	}
	if exe, err := proc.Executable(owner); err == nil && owner != os.Getpid() && proc.IsFogway(exe) {
		return // its fogway still runs it
	}
	os.Remove(pidPath())
	exe, err := proc.Executable(pid)
	if err != nil || !within(exe, Dir()) {
		return
	}
	log.Printf("local model: ending the llama-server (pid %d) a fogway before this one left running", pid)
	proc.Terminate(pid)
	for range 50 {
		if _, err := proc.Executable(pid); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	proc.Kill(pid)
}

// within says path is a file under dir, symlinks in either resolved.
func within(path, dir string) bool {
	if d, err := filepath.EvalSymlinks(dir); err == nil {
		dir = d
	}
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}
