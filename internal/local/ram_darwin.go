package local

import "golang.org/x/sys/unix"

func totalRAM() uint64 {
	n, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return n
}
