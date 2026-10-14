package local

import "golang.org/x/sys/windows"

// diskFree is the space free to this user on path's disk, 0 when unknown.
func diskFree(path string) uint64 {
	p, err := windows.UTF16PtrFromString(existing(path))
	if err != nil {
		return 0
	}
	var free uint64
	if windows.GetDiskFreeSpaceEx(p, &free, nil, nil) != nil {
		return 0
	}
	return free
}
