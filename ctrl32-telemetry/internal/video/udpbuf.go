package video

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// wantUDPBuffer is the socket receive buffer asked for RTP over UDP: a camera's key frame arrives as a burst of
// packets, and a small buffer drops some of them (grey smears in the picture).
const wantUDPBuffer = 4 << 20

// udpBufferSize is the receive buffer RTSP clients ask for. The RTSP library refuses a UDP port when the operating
// system grants less than asked, and Linux caps SO_RCVBUF at net.core.rmem_max (often 208 KB): asking for more would
// make every UDP camera fail. So the size follows the system's limit; 0 keeps the system default.
var udpBufferSize = sync.OnceValue(func() int {
	if runtime.GOOS != "linux" {
		return wantUDPBuffer
	}
	b, err := os.ReadFile("/proc/sys/net/core/rmem_max")
	if err != nil {
		return 0
	}
	max, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return udpBufferFor(max)
})

// udpBufferFor is the buffer to ask for under a system limit of max bytes.
func udpBufferFor(max int) int {
	if max >= wantUDPBuffer {
		return wantUDPBuffer
	}
	if max >= 256<<10 {
		return max
	}
	return 0 // below that the system default is as good; raise net.core.rmem_max for many or large cameras
}
