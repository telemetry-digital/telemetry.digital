//go:build windows

package agent

import "golang.org/x/sys/windows"

func diskInfo(path string) (DiskInfo, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return DiskInfo{}, false
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return DiskInfo{}, false
	}
	return DiskInfo{Path: path, TotalGB: float64(total) / 1e9, FreeGB: float64(free) / 1e9}, true
}
