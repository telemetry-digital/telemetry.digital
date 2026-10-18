package s7_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7/s7sim"
)

// startSim runs a simulator on a free local port.
func startSim(t *testing.T, pdu int, putGetOff bool) (*s7sim.Server, int) {
	t.Helper()
	s := s7sim.New()
	s.PDU, s.PutGetOff = pdu, putGetOff
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() { _ = s.Serve(l) }()
	return s, l.Addr().(*net.TCPAddr).Port
}

func dial(t *testing.T, port int) *s7.Conn {
	t.Helper()
	c, err := s7.Dial(context.Background(), s7.DialConfig{Host: "127.0.0.1", Port: port, Rack: 0, Slot: 1, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func addrs(t *testing.T, specs ...string) []s7.Address {
	t.Helper()
	var out []s7.Address
	for _, sp := range specs {
		a, typ, _ := strings.Cut(sp, ":")
		ad, err := s7.ParseAddress(a, typ)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, ad)
	}
	return out
}

func TestReadSeeded(t *testing.T) {
	sim, port := startSim(t, 960, false)
	sim.SetDB(2, 0, []byte{0xFF, 0x85, 0xFF, 0xFF, 0xFF, 0xFE, 0x40, 0x09, 0x21, 0xFB, 0x54, 0x44, 0x2D, 0x18, 'Z'})
	c := dial(t, port)
	if c.PDU() != 960 {
		t.Fatalf("PDU %d", c.PDU())
	}
	as := addrs(t, "DB1.DBD0", "DB1.DBW4", "DB1.DBX6.0", "DB1.DBD8", "DB1.DBS20.20", "MW10",
		"DB2.DBB0", "DB2.DBB1:sint", "DB2.DBW0:uint", "DB2.DBW2:int", "DB2.DBD2:dint", "DB2.DBD2:dword",
		"DB2,LREAL6", "DB2,CHAR14", "I0.0", "QB0", "DB2.DBD0:udint", "DB2.DBW0:word", "DB2,USINT1")
	res, err := c.Read(context.Background(), as)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range res {
		if r.Err != nil {
			t.Fatalf("%s: %v", as[i], r.Err)
		}
	}
	if v := res[0].Value.(float64); v < 29 || v > 71 {
		t.Errorf("sinus %v", v)
	}
	if _, ok := res[2].Value.(bool); !ok {
		t.Errorf("toggle %T", res[2].Value)
	}
	want := map[int]any{3: 21.5, 4: "hello", 5: 1234.0, 6: 255.0, 7: -123.0, 8: 65413.0, 9: -1.0, 10: -2.0, 11: 4294967294.0,
		12: 3.141592653589793, 13: "Z", 14: false, 15: 0.0, 16: 4286971903.0, 17: 65413.0, 18: 133.0}
	for i, w := range want {
		if res[i].Value != w {
			t.Errorf("%s (%s) = %v, want %v", as[i], as[i].Type, res[i].Value, w)
		}
	}
	info, err := c.Identify(context.Background())
	if err != nil || info.OrderCode != "6ES7 214-1AG40-0XB0" || info.ModuleType != "CPU 1214C DC/DC/DC" || info.ModuleName != "PLC_1" {
		t.Errorf("identify %+v %v", info, err)
	}
}

func TestReadSplitSmallPDU(t *testing.T) {
	sim, port := startSim(t, 240, false)
	c := dial(t, port)
	if c.PDU() != 240 {
		t.Fatalf("PDU %d", c.PDU())
	}
	// 60 words, 30 bits and a 254-character string (larger than one PDU) in DB3
	var specs []string
	for i := 0; i < 60; i++ {
		sim.SetDB(3, 2*i, []byte{0, byte(i)})
		specs = append(specs, fmt.Sprintf("DB3.DBW%d", 2*i))
	}
	for i := 0; i < 30; i++ {
		specs = append(specs, fmt.Sprintf("DB3.DBX%d.%d", 200+i/8, i%8))
	}
	sim.SetDB(3, 200, []byte{0x55, 0x55, 0x55, 0x55})
	long := strings.Repeat("abcdefghij", 25) + "wxyz"
	sim.SetDB(3, 300, append([]byte{254, 254}, long...))
	specs = append(specs, "DB3.DBS300.254", "DB3.DBB1")
	as := addrs(t, specs...)
	res, err := c.Read(context.Background(), as)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		if res[i].Err != nil || res[i].Value != float64(i) {
			t.Errorf("%s = %v %v", as[i], res[i].Value, res[i].Err)
		}
	}
	for i := 0; i < 30; i++ {
		if r := res[60+i]; r.Err != nil || r.Value != (i%2 == 0) {
			t.Errorf("%s = %v %v", as[60+i], r.Value, r.Err)
		}
	}
	if r := res[90]; r.Err != nil || r.Value != long {
		t.Errorf("long string = %q %v", r.Value, r.Err)
	}
	if r := res[91]; r.Err != nil || r.Value != 0.0 {
		t.Errorf("DBB1 = %v %v", r.Value, r.Err)
	}
	// a long string written in several jobs
	short := strings.Repeat("Q", 230)
	if err := c.Write(context.Background(), as[90], short); err != nil {
		t.Fatal(err)
	}
	res, err = c.Read(context.Background(), as[90:91])
	if err != nil || res[0].Value != short {
		t.Errorf("long string readback %v %v", res, err)
	}
	if b := sim.DB(3, 300, 1); b[0] != 254 {
		t.Errorf("max length byte overwritten: %d", b[0])
	}
}

func TestItemErrors(t *testing.T) {
	_, port := startSim(t, 960, false)
	c := dial(t, port)
	as := addrs(t, "DB1.DBW4", "DB99.DBW0", "DB1.DBD1022", "MW10", "M300.0")
	res, err := c.Read(context.Background(), as)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Err != nil || res[3].Value != 1234.0 {
		t.Errorf("good items: %+v %+v", res[0], res[3])
	}
	var ie *s7.ItemError
	if !errors.As(res[1].Err, &ie) || ie.Code != 0x0A || !strings.Contains(res[1].Err.Error(), "does not exist") {
		t.Errorf("DB99: %v", res[1].Err)
	}
	for _, i := range []int{2, 4} {
		if !errors.As(res[i].Err, &ie) || ie.Code != 0x05 || !strings.Contains(res[i].Err.Error(), "out of range") {
			t.Errorf("%s: %v", as[i], res[i].Err)
		}
	}
	if err := c.Write(context.Background(), as[1], 5); !errors.As(err, &ie) || ie.Code != 0x0A {
		t.Errorf("write DB99: %v", err)
	}
}

func TestWriteReadback(t *testing.T) {
	_, port := startSim(t, 960, false)
	c := dial(t, port)
	ctx := context.Background()
	cases := []struct {
		addr string
		in   any
		want any
	}{
		{"DB1.DBX6.3", true, true},
		{"DB1.DBX6.3", false, false},
		{"M0.7", true, true},
		{"DB1.DBW12", -321, -321.0},
		{"DB1.DBD8", 22.25, 22.25},
		{"DB1.DBD14:dint", 123456, 123456.0},
		{"DB1.DBS20.20", "world, 20 chars max", "world, 20 chars max"},
		{"DB1.DBS20.20", "", ""},
		{"QW2:word", 65000, 65000.0},
		{"DB1,LREAL40", 1.5e100, 1.5e100},
		{"DB1,CHAR50", "x", "x"},
	}
	for _, cs := range cases {
		as := addrs(t, cs.addr)
		if err := c.Write(ctx, as[0], cs.in); err != nil {
			t.Errorf("write %s: %v", cs.addr, err)
			continue
		}
		res, err := c.Read(ctx, as)
		if err != nil || res[0].Err != nil || res[0].Value != cs.want {
			t.Errorf("%s readback %v %v %v, want %v", cs.addr, res[0].Value, res[0].Err, err, cs.want)
		}
	}
	// a bit write must not disturb its neighbours
	res, _ := c.Read(ctx, addrs(t, "DB1.DBX6.0", "DB1.DBX6.3"))
	if res[1].Value != false {
		t.Errorf("bit neighbour: %v", res[1].Value)
	}
	for _, bad := range []struct {
		addr string
		v    any
	}{{"DB1.DBW12", 40000}, {"DB1.DBS20.20", strings.Repeat("x", 21)}, {"DB1.DBB0", -1}, {"DB1.DBD8", "warm"}} {
		if err := c.Write(ctx, addrs(t, bad.addr)[0], bad.v); err == nil || !strings.Contains(err.Error(), bad.addr) {
			t.Errorf("%s %v: %v", bad.addr, bad.v, err)
		}
	}
}

func TestPutGetOff(t *testing.T) {
	_, port := startSim(t, 960, true)
	c := dial(t, port)
	_, err := c.Read(context.Background(), addrs(t, "DB1.DBW4"))
	if !s7.PutGetDisabled(err) || !strings.Contains(err.Error(), "PUT/GET") {
		t.Fatalf("got %v", err)
	}
	if err := c.Write(context.Background(), addrs(t, "DB1.DBW4")[0], 1); !s7.PutGetDisabled(err) {
		t.Fatalf("write: %v", err)
	}
	// the connection stays usable after a job error
	if _, err := c.Identify(context.Background()); err != nil {
		t.Fatalf("identify: %v", err)
	}
}

func TestTimeouts(t *testing.T) {
	// a closed port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	if _, err := s7.Dial(context.Background(), s7.DialConfig{Host: "127.0.0.1", Port: dead, Timeout: time.Second}); err == nil {
		t.Fatal("dial to a closed port succeeded")
	}
	// a server that accepts and never answers
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	go func() {
		for {
			c, err := silent.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	start := time.Now()
	_, err = s7.Dial(context.Background(), s7.DialConfig{Host: "127.0.0.1", Port: silent.Addr().(*net.TCPAddr).Port, Timeout: 300 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "timeout") || time.Since(start) > 2*time.Second {
		t.Fatalf("silent server: %v after %v", err, time.Since(start))
	}
	// context deadline shorter than the timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start = time.Now()
	_, err = s7.Dial(ctx, s7.DialConfig{Host: "127.0.0.1", Port: silent.Addr().(*net.TCPAddr).Port, Timeout: 5 * time.Second})
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("context deadline: %v after %v", err, time.Since(start))
	}
}

func TestDialConfig(t *testing.T) {
	_, port := startSim(t, 960, false)
	for _, cfg := range []s7.DialConfig{
		{ConnType: "op", Rack: 0, Slot: 2},
		{ConnType: "s7basic"},
		{LocalTSAP: "20.00", RemoteTSAP: "10.00"}, // LOGO!
	} {
		cfg.Host, cfg.Port = "127.0.0.1", port
		c, err := s7.Dial(context.Background(), cfg)
		if err != nil {
			t.Fatalf("%+v: %v", cfg, err)
		}
		_ = c.Close()
	}
	for _, cfg := range []s7.DialConfig{{ConnType: "x"}, {Rack: 8}, {LocalTSAP: "zz"}, {PDUSize: 100}} {
		cfg.Host, cfg.Port = "127.0.0.1", port
		if _, err := s7.Dial(context.Background(), cfg); err == nil {
			t.Errorf("%+v accepted", cfg)
		}
	}
}
