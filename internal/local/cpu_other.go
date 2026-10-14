//go:build !darwin && !linux && !windows

package local

// cpuInfo knows nothing here: Detect counts the CPUs Go sees.
func cpuInfo() (string, int) { return "", 0 }

// libc matters on Linux only.
func libc() string { return "" }
