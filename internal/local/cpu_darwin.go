package local

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"github.com/carey569/fogway/internal/proc"
)

// cpuInfo is the processor's name and the cores llama.cpp computes on:
// Apple silicon's performance cores.
func cpuInfo() (string, int) {
	name, _ := unix.Sysctl("machdep.cpu.brand_string")
	n, err := unix.SysctlUint32("hw.perflevel0.physicalcpu")
	if err != nil || n == 0 {
		n, _ = unix.SysctlUint32("hw.physicalcpu")
	}
	return name, int(n)
}

var gpuCoreCount = regexp.MustCompile(`"gpu-core-count" = (\d+)`)

// appleGPUCores is Apple silicon's GPU cores, as the I/O registry has
// them; 0 when it doesn't say.
func appleGPUCores() int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := proc.CommandContext(ctx, "/usr/sbin/ioreg", "-r", "-c", "AGXAccelerator", "-d", "1").Output()
	if err != nil {
		return 0
	}
	m := gpuCoreCount.FindSubmatch(out)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}

// libc matters on Linux only.
func libc() string { return "" }
