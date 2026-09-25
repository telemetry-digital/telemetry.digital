package timecheck

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// fakeServer answers SNTP requests with its clock shifted by shift.
func fakeServer(t *testing.T, shift time.Duration, stratum byte) string {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 64)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 48 {
				continue
			}
			resp := make([]byte, 48)
			resp[0] = 0x24 // version 4, mode 4 (server)
			resp[1] = stratum
			copy(resp[12:16], "RATE")
			copy(resp[24:32], buf[40:48]) // originate = client transmit
			now := time.Now().Add(shift)
			putNTP(resp[32:], now)
			putNTP(resp[40:], now)
			pc.WriteTo(resp, addr)
		}
	}()
	return pc.LocalAddr().String()
}

func TestQueryOffset(t *testing.T) {
	addr := fakeServer(t, 2500*time.Millisecond, 2)
	off, rtt, stratum, err := Query(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if off < 2400*time.Millisecond || off > 2600*time.Millisecond {
		t.Errorf("offset %v, want ≈2.5 s", off)
	}
	if rtt < 0 || rtt > time.Second || stratum != 2 {
		t.Errorf("rtt %v stratum %d", rtt, stratum)
	}
	c := New(Config{Server: addr, MaxOffset: time.Second}, nil)
	st := c.Check(context.Background())
	if st.OK || st.Error == "" || st.OffsetMs < 2400 {
		t.Errorf("2.5 s offset must not be OK: %+v", st)
	}
	c2 := New(Config{Server: fakeServer(t, 20*time.Millisecond, 3), MaxOffset: time.Second}, nil)
	if st := c2.Check(context.Background()); !st.OK || st.Stratum != 3 {
		t.Errorf("20 ms offset must be OK: %+v", st)
	}
}

func TestKissOfDeath(t *testing.T) {
	addr := fakeServer(t, 0, 0)
	if _, _, _, err := Query(context.Background(), addr); err == nil {
		t.Fatal("stratum 0 must be an error")
	}
}

func TestNTPTimestampRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	b := make([]byte, 8)
	putNTP(b, now)
	if got := getNTP(b); got.Sub(now) > time.Microsecond || now.Sub(got) > time.Microsecond {
		t.Errorf("round trip %v != %v", got, now)
	}
	if binary.BigEndian.Uint64(b)>>32 != uint64(now.Unix())+ntpEpochOffset {
		t.Errorf("seconds field")
	}
}
