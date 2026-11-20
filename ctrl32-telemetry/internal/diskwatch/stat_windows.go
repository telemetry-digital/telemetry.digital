//go:build windows

package diskwatch

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// diskStat returns the free and total bytes of the volume holding path and the volume.
func diskStat(path string) (free, total uint64, id string, ok bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, 0, "", false
	}
	p, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return 0, 0, "", false
	}
	var totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, 0, "", false
	}
	return free, total, strings.ToUpper(filepath.VolumeName(abs)), true
}
