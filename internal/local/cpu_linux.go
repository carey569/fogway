package local

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/carey569/fogway/internal/proc"
)

// cpuInfo is the processor's name and the cores llama.cpp computes on:
// the physical ones (two threads of a core are one), only the
// performance ones of an Intel CPU with two kinds.
func cpuInfo() (string, int) {
	name := ""
	b, _ := os.ReadFile("/proc/cpuinfo")
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "model name" {
			name = strings.TrimSpace(v)
			break
		}
	}
	only := map[int]bool(nil)
	if l, err := os.ReadFile("/sys/devices/cpu_core/cpus"); err == nil {
		only = cpuList(strings.TrimSpace(string(l)))
	}
	cores := map[string]bool{}
	dirs, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/topology")
	for _, d := range dirs {
		n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(filepath.Dir(d)), "cpu"))
		if err != nil || (only != nil && !only[n]) {
			continue
		}
		pkg, _ := os.ReadFile(filepath.Join(d, "physical_package_id"))
		core, _ := os.ReadFile(filepath.Join(d, "core_id"))
		cores[strings.TrimSpace(string(pkg))+"/"+strings.TrimSpace(string(core))] = true
	}
	return name, len(cores)
}

// cpuList reads a kernel CPU list, such as 0-7,16-19.
func cpuList(s string) map[int]bool {
	out := map[int]bool{}
	for _, part := range strings.Split(s, ",") {
		lo, hi, ok := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			continue
		}
		b := a
		if ok {
			if b, err = strconv.Atoi(hi); err != nil {
				continue
			}
		}
		for i := a; i <= b; i++ {
			out[i] = true
		}
	}
	return out
}

// libc is the C library's name and version, as getconf tells them: "glibc
// 2.35"; "" where getconf doesn't know (musl's, as Alpine's).
func libc() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := proc.CommandContext(ctx, "getconf", "GNU_LIBC_VERSION").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
