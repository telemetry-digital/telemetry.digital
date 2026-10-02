//go:build windows

package analytics

import (
	"time"

	"golang.org/x/sys/windows"
)

// cpuTimes is the CPU time (user + kernel) a process has used so far; 0 when unknown.
func cpuTimes(pid int) time.Duration {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return 0
	}
	ft := func(f windows.Filetime) time.Duration {
		return time.Duration((uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime)) * 100)
	}
	return ft(k) + ft(u)
}
