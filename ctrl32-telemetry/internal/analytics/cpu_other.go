//go:build !windows

package analytics

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// cpuTimes is the CPU time (user + system) a process has used so far, from /proc (Linux); 0 when unknown.
func cpuTimes(pid int) time.Duration {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	if i := strings.LastIndexByte(s, ')'); i >= 0 {
		s = s[i+1:]
	}
	f := strings.Fields(s)
	if len(f) < 13 { // fields after the name: state(0) … utime(11) stime(12)
		return 0
	}
	u, _ := strconv.ParseInt(f[11], 10, 64)
	k, _ := strconv.ParseInt(f[12], 10, 64)
	return time.Duration(u+k) * time.Second / 100 // USER_HZ is 100 on Linux
}
