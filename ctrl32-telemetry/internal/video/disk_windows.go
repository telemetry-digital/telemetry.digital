//go:build windows

package video

import "golang.org/x/sys/windows"

// diskFree returns the free and total bytes of the volume holding path.
func diskFree(path string) (free, total uint64, ok bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, 0, false
	}
	return free, total, true
}
