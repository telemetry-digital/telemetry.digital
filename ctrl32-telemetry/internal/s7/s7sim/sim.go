// Package s7sim is a small S7comm (ISO-on-TCP) PLC simulator for the bench and for tests: COTP connection,
// setup communication, Read Var, Write Var and SZL 0x0011/0x001C. It is independent of package s7 so the client is
// tested against a separate implementation of the protocol.
//
// Memory: DB1..DB10 (1 KiB each), M, I and Q (256 bytes each). Seeded values (Seed/Tick): DB1.DBD0 REAL sinusoid,
// DB1.DBW4 INT counter, DB1.DBX6.0 BOOL toggling, DB1.DBD8 REAL setpoint (initial 21.5), DB1.DBS20.20 STRING
// "hello", MW10 INT 1234.
package s7sim

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"time"
)

// Server is a simulated PLC.
type Server struct {
	PDU       int  // maximum PDU size offered (default 960)
	PutGetOff bool // answer Read/Write Var with error 0x8104 like a CPU without PUT/GET permission
	Verbose   bool // log writes

	mu    sync.Mutex
	dbs   map[int][]byte
	areas map[byte][]byte
	start time.Time
	ticks int
}

// New returns a simulator with seeded memory.
func New() *Server {
	s := &Server{PDU: 960, dbs: map[int][]byte{}, areas: map[byte][]byte{}, start: time.Now()}
	for i := 1; i <= 10; i++ {
		s.dbs[i] = make([]byte, 1024)
	}
	for _, a := range []byte{0x81, 0x82, 0x83} {
		s.areas[a] = make([]byte, 256)
	}
	db := s.dbs[1]
	binary.BigEndian.PutUint32(db[8:], math.Float32bits(21.5))
	db[20], db[21] = 20, 5
	copy(db[22:], "hello")
	binary.BigEndian.PutUint16(s.areas[0x83][10:], 1234)
	s.Tick()
	return s
}

// Tick advances the changing values (called every second by the tool).
func (s *Server) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := time.Since(s.start).Seconds()
	db := s.dbs[1]
	binary.BigEndian.PutUint32(db[0:], math.Float32bits(float32(50+20*math.Sin(t/30))))
	binary.BigEndian.PutUint16(db[4:], uint16(s.ticks))
	db[6] = db[6]&^1 | byte(s.ticks&1)
	s.ticks++
}

// SetDB replaces bytes of a data block (tests).
func (s *Server) SetDB(db, start int, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy(s.dbs[db][start:], b)
}

// DB returns a copy of bytes of a data block (tests).
func (s *Server) DB(db, start, n int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.dbs[db][start:start+n]...)
}

// Serve accepts connections until the listener is closed.
func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(c)
	}
}

func readTPKT(r io.Reader) ([]byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(h[2:]))
	if h[0] != 3 || n < 7 {
		return nil, errors.New("bad TPKT")
	}
	b := make([]byte, n-4)
	_, err := io.ReadFull(r, b)
	return b, err
}

func writeTPKT(w io.Writer, p []byte) error {
	b := []byte{3, 0, byte((len(p) + 4) >> 8), byte(len(p) + 4)}
	_, err := w.Write(append(b, p...))
	return err
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	b, err := readTPKT(c)
	if err != nil || len(b) < 7 || b[1] != 0xE0 {
		return
	}
	// connection confirm: echo the parameters, swap references
	cc := append([]byte{b[0], 0xD0, b[4], b[5], 0x00, 0x02, 0x00}, b[7:]...)
	if writeTPKT(c, cc) != nil {
		return
	}
	pdu := 0
	for {
		var req []byte
		for {
			f, err := readTPKT(c)
			if err != nil || len(f) < 3 || f[1] != 0xF0 {
				return
			}
			req = append(req, f[int(f[0])+1:]...)
			if f[2]&0x80 != 0 {
				break
			}
		}
		resp := s.process(req, &pdu)
		if resp == nil {
			return
		}
		if writeTPKT(c, append([]byte{2, 0xF0, 0x80}, resp...)) != nil {
			return
		}
	}
}

// ack builds an ack_data (or userdata) response.
func ack(ros byte, ref []byte, errClass, errCode byte, param, data []byte) []byte {
	b := []byte{0x32, ros, 0, 0, ref[0], ref[1], byte(len(param) >> 8), byte(len(param)), byte(len(data) >> 8), byte(len(data))}
	if ros != 7 {
		b = append(b, errClass, errCode)
	}
	return append(append(b, param...), data...)
}

func (s *Server) process(req []byte, pdu *int) []byte {
	if len(req) < 10 || req[0] != 0x32 {
		return nil
	}
	ros, ref := req[1], req[4:6]
	pl, dl := int(binary.BigEndian.Uint16(req[6:])), int(binary.BigEndian.Uint16(req[8:]))
	if len(req) < 10+pl+dl || pl < 1 {
		return nil
	}
	param, data := req[10:10+pl], req[10+pl:10+pl+dl]
	if ros == 7 {
		return s.userdata(ref, param, data)
	}
	if ros != 1 {
		return nil
	}
	switch param[0] {
	case 0xF0:
		if len(param) < 8 {
			return nil
		}
		want := int(binary.BigEndian.Uint16(param[6:]))
		*pdu = min(want, s.PDU)
		p := append([]byte(nil), param[:8]...)
		binary.BigEndian.PutUint16(p[6:], uint16(*pdu))
		return ack(3, ref, 0, 0, p, nil)
	case 0x04, 0x05:
		if *pdu == 0 {
			return ack(3, ref, 0x81, 0x04, param[:2], nil)
		}
		if len(req) > *pdu {
			return ack(3, ref, 0x85, 0x00, param[:2], nil)
		}
		if s.PutGetOff {
			return ack(3, ref, 0x81, 0x04, param[:2], nil)
		}
		var resp []byte
		if param[0] == 0x04 {
			resp = s.read(ref, param)
		} else {
			resp = s.write(ref, param, data)
		}
		if len(resp) > *pdu {
			return ack(3, ref, 0x85, 0x00, param[:2], nil)
		}
		return resp
	}
	return ack(3, ref, 0x81, 0x04, param[:1], nil)
}

type item struct {
	ts    byte
	count int
	db    int
	area  byte
	bit   int // bit address
}

func parseItems(param []byte, n int) ([]item, bool) {
	var out []item
	p := param[2:]
	for i := 0; i < n; i++ {
		if len(p) < 12 || p[0] != 0x12 || p[1] != 0x0A || p[2] != 0x10 {
			return nil, false
		}
		out = append(out, item{
			ts: p[3], count: int(binary.BigEndian.Uint16(p[4:])), db: int(binary.BigEndian.Uint16(p[6:])),
			area: p[8], bit: int(p[9])<<16 | int(p[10])<<8 | int(p[11]),
		})
		p = p[12:]
	}
	return out, true
}

var elemSize = map[byte]int{1: 1, 2: 1, 3: 1, 4: 2, 5: 2, 6: 4, 7: 4, 8: 4}

// mem returns the memory of an item and its byte range, or an item return code.
func (s *Server) mem(it item) ([]byte, int, int, byte) {
	var m []byte
	if it.area == 0x84 {
		m = s.dbs[it.db]
	} else {
		m = s.areas[it.area]
	}
	if m == nil {
		return nil, 0, 0, 0x0A
	}
	es, ok := elemSize[it.ts]
	if !ok {
		return nil, 0, 0, 0x06
	}
	start, n := it.bit/8, es*it.count
	if it.ts == 1 {
		n = 1
	}
	if n == 0 || start+n > len(m) {
		return nil, 0, 0, 0x05
	}
	return m, start, n, 0xFF
}

func (s *Server) read(ref, param []byte) []byte {
	n := int(param[1])
	items, ok := parseItems(param, n)
	if !ok {
		return ack(3, ref, 0x81, 0x04, param[:2], nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var data []byte
	for i, it := range items {
		m, start, cnt, code := s.mem(it)
		if code != 0xFF {
			data = append(data, code, 0, 0, 0)
			continue
		}
		var v []byte
		ts, bits := byte(0x04), cnt*8
		if it.ts == 1 {
			v, ts, bits = []byte{m[start] >> (it.bit % 8) & 1}, 0x03, 1
		} else {
			v = m[start : start+cnt]
		}
		data = append(data, 0xFF, ts, byte(bits>>8), byte(bits))
		data = append(data, v...)
		if len(v)&1 == 1 && i < len(items)-1 {
			data = append(data, 0)
		}
	}
	return ack(3, ref, 0, 0, []byte{0x04, byte(n)}, data)
}

func (s *Server) write(ref, param, data []byte) []byte {
	n := int(param[1])
	items, ok := parseItems(param, n)
	if !ok {
		return ack(3, ref, 0x81, 0x04, param[:2], nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var codes []byte
	for i, it := range items {
		if len(data) < 4 {
			codes = append(codes, 0x07)
			continue
		}
		ts, l := data[1], int(binary.BigEndian.Uint16(data[2:]))
		if ts == 0x03 || ts == 0x04 || ts == 0x05 {
			l = (l + 7) / 8
		}
		if len(data) < 4+l {
			codes = append(codes, 0x07)
			continue
		}
		v := data[4 : 4+l]
		data = data[4+l:]
		if l&1 == 1 && i < len(items)-1 && len(data) > 0 {
			data = data[1:]
		}
		m, start, cnt, code := s.mem(it)
		if code != 0xFF {
			codes = append(codes, code)
			continue
		}
		if cnt != l {
			codes = append(codes, 0x07)
			continue
		}
		if it.ts == 1 {
			mask := byte(1) << (it.bit % 8)
			if v[0]&1 == 1 {
				m[start] |= mask
			} else {
				m[start] &^= mask
			}
		} else {
			copy(m[start:], v)
		}
		if s.Verbose {
			log.Printf("write area 0x%02X DB%d byte %d.%d: % X", it.area, it.db, start, it.bit%8, v)
		}
		codes = append(codes, 0xFF)
	}
	return ack(3, ref, 0, 0, []byte{0x05, byte(n)}, codes)
}

// userdata answers SZL reads (0x0011 module identification, 0x001C component identification).
func (s *Server) userdata(ref, param, data []byte) []byte {
	if len(param) < 8 || param[5] != 0x44 || param[6] != 0x01 || len(data) < 8 {
		return ack(7, ref, 0, 0, []byte{0, 1, 0x12, 8, 0x12, 0x84, param[6], 0, 0, 0, 0xD4, 0x01}, []byte{0x0A, 0, 0, 0})
	}
	id := binary.BigEndian.Uint16(data[4:])
	var size int
	var recs [][]byte
	pad := func(idx uint16, s string, n int) []byte {
		r := make([]byte, n)
		binary.BigEndian.PutUint16(r, idx)
		copy(r[2:], s)
		return r
	}
	switch id {
	case 0x0011:
		size = 28
		recs = [][]byte{pad(1, "6ES7 214-1AG40-0XB0 ", 28), pad(6, "6ES7 214-1AG40-0XB0 ", 28)}
	case 0x001C:
		size = 34
		recs = [][]byte{pad(1, "s7sim", 34), pad(2, "PLC_1", 34), pad(5, "S C-SIM00000001", 34), pad(7, "CPU 1214C DC/DC/DC", 34)}
	default:
		return ack(7, ref, 0, 0, []byte{0, 1, 0x12, 8, 0x12, 0x84, 1, 0, 0, 0, 0xD4, 0x01}, []byte{0x0A, 0, 0, 0})
	}
	body := []byte{byte(id >> 8), byte(id), 0, 0, byte(size >> 8), byte(size), 0, byte(len(recs))}
	for _, r := range recs {
		body = append(body, r...)
	}
	d := append([]byte{0xFF, 0x09, byte(len(body) >> 8), byte(len(body))}, body...)
	return ack(7, ref, 0, 0, []byte{0, 1, 0x12, 8, 0x12, 0x84, 1, 1, 0, 0, 0, 0}, d)
}
