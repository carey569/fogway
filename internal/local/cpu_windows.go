package local

import (
	"runtime"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// cpuInfo is the processor's name and the cores llama.cpp computes on.
// Windows tells logical processors; an x86 one runs two on each core, so
// half of them are taken for the cores.
func cpuInfo() (string, int) {
	name := ""
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		name, _, _ = k.GetStringValue("ProcessorNameString")
		k.Close()
	}
	n := runtime.NumCPU()
	if runtime.GOARCH == "amd64" && n >= 4 {
		n /= 2
	}
	return strings.TrimSpace(name), n
}

// libc matters on Linux only.
func libc() string { return "" }
