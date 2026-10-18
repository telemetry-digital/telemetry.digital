package s7

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DialConfig describes the PLC connection.
type DialConfig struct {
	Host string
	Port int // default 102
	Rack int // 0..7
	Slot int // 0..31 (S7-300: 2, S7-1200/1500: 1 or 0)
	// ConnType selects the remote TSAP class: "pg" (default), "op" or "s7basic".
	ConnType string
	// Explicit TSAPs as hex ("10.00", "0x1000", "1000"); they override the rack/slot defaults (LOGO!: local
	// "20.00"/remote "10.00" or as configured in LOGO!Soft Comfort). Default local TSAP 01.00.
	LocalTSAP, RemoteTSAP string
	Timeout               time.Duration // per request and for the handshake; default 5 s
	PDUSize               int           // requested PDU size; default 960, the PLC may answer with less
}

// Conn is one connection to a PLC. It is safe for concurrent use; requests are serialized. After an I/O error the
// connection is unusable and must be redialed.
type Conn struct {
	mu      sync.Mutex
	nc      net.Conn
	pdu     int
	ref     uint16
	timeout time.Duration
	broken  error
}

// Result is the outcome of reading one address.
type Result struct {
	Value any // bool, float64 or string
	Err   error
}

// ParseTSAP parses a TSAP written as hex, e.g. "10.00", "0x1000" or "1000".
func ParseTSAP(s string) (uint16, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.TrimPrefix(t, "0x")
	if strings.Contains(t, ".") {
		p := strings.Split(t, ".")
		if len(p) != 2 || len(p[0]) == 0 || len(p[0]) > 2 || len(p[1]) == 0 || len(p[1]) > 2 {
			return 0, fmt.Errorf("invalid TSAP %q (expected e.g. 10.00)", s)
		}
		hi, err1 := strconv.ParseUint(p[0], 16, 8)
		lo, err2 := strconv.ParseUint(p[1], 16, 8)
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid TSAP %q (expected e.g. 10.00)", s)
		}
		return uint16(hi<<8 | lo), nil
	}
	v, err := strconv.ParseUint(t, 16, 16)
	if err != nil || t == "" {
		return 0, fmt.Errorf("invalid TSAP %q (expected e.g. 10.00)", s)
	}
	return uint16(v), nil
}

// tsaps returns the local and remote TSAP of the configuration.
func (cfg DialConfig) tsaps() (uint16, uint16, error) {
	local, remote := uint16(0x0100), uint16(0)
	if cfg.Rack < 0 || cfg.Rack > 7 || cfg.Slot < 0 || cfg.Slot > 31 {
		return 0, 0, errors.New("rack must be 0..7 and slot 0..31")
	}
	var class uint16
	switch strings.ToLower(cfg.ConnType) {
	case "", "pg":
		class = 1
	case "op":
		class = 2
	case "s7basic", "basic":
		class = 3
	default:
		return 0, 0, fmt.Errorf("connection type must be pg, op or s7basic, got %q", cfg.ConnType)
	}
	remote = class<<8 | uint16(cfg.Rack*0x20+cfg.Slot)
	var err error
	if strings.TrimSpace(cfg.LocalTSAP) != "" {
		if local, err = ParseTSAP(cfg.LocalTSAP); err != nil {
			return 0, 0, err
		}
	}
	if strings.TrimSpace(cfg.RemoteTSAP) != "" {
		if remote, err = ParseTSAP(cfg.RemoteTSAP); err != nil {
			return 0, 0, err
		}
	}
	return local, remote, nil
}

// Dial connects to the PLC: TCP, COTP connection request and S7 setup communication.
func Dial(ctx context.Context, cfg DialConfig) (*Conn, error) {
	local, remote, err := cfg.tsaps()
	if err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = 102
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.PDUSize <= 0 {
		cfg.PDUSize = defaultPDU
	}
	if cfg.PDUSize < 240 || cfg.PDUSize > 960 {
		return nil, errors.New("PDU size must be 240..960")
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	d := net.Dialer{Timeout: cfg.Timeout}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}
	c := &Conn{nc: nc, timeout: cfg.Timeout}
	if err := c.handshake(ctx, local, remote, cfg.PDUSize); err != nil {
		_ = nc.Close()
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}
	return c, nil
}

// deadline arms the connection deadline from the timeout and the context; the returned func disarms it.
func (c *Conn) deadline(ctx context.Context) func() {
	dl := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(dl) {
		dl = d
	}
	_ = c.nc.SetDeadline(dl)
	stop := context.AfterFunc(ctx, func() { _ = c.nc.SetDeadline(time.Now()) })
	return func() { stop() }
}

func (c *Conn) handshake(ctx context.Context, local, remote uint16, pdu int) error {
	defer c.deadline(ctx)()
	if err := writeTPKT(c.nc, cotpConnect(local, remote)); err != nil {
		return err
	}
	b, err := readTPKT(c.nc)
	if err != nil {
		return fmt.Errorf("no COTP connection confirm: %w", ctxErr(ctx, err))
	}
	if len(b) < 2 || b[1]&0xF0 != cotpCC {
		return fmt.Errorf("connection rejected by the PLC (TSAP %04X/%04X: check rack, slot or TSAPs)", local, remote)
	}
	param := []byte{fnSetup, 0, 0, 1, 0, 1, byte(pdu >> 8), byte(pdu)}
	r, err := c.exchangeLocked(ctx, job(rosJob, c.nextRef(), param, nil))
	if err != nil {
		return fmt.Errorf("setup communication: %w", err)
	}
	if len(r.param) < 8 || r.param[0] != fnSetup {
		return errors.New("setup communication: invalid response")
	}
	c.pdu = int(binary.BigEndian.Uint16(r.param[6:]))
	if c.pdu < 64 {
		return fmt.Errorf("setup communication: PDU size %d too small", c.pdu)
	}
	if c.pdu > pdu {
		c.pdu = pdu
	}
	return nil
}

func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("timeout")
	}
	return err
}

func (c *Conn) nextRef() uint16 {
	c.ref++
	if c.ref == 0 {
		c.ref = 1
	}
	return c.ref
}

// exchangeLocked sends one PDU and waits for the matching response; the caller holds the deadline.
func (c *Conn) exchangeLocked(ctx context.Context, pdu []byte) (response, error) {
	ref := binary.BigEndian.Uint16(pdu[4:])
	if err := writeData(c.nc, pdu); err != nil {
		c.broken = ctxErr(ctx, err)
		return response{}, c.broken
	}
	for {
		b, err := readData(c.nc)
		if err != nil {
			c.broken = ctxErr(ctx, err)
			return response{}, c.broken
		}
		r, err := parseResponse(b)
		if err != nil {
			c.broken = err
			return response{}, err
		}
		if r.ref != ref {
			continue // stale answer of an earlier, timed-out request
		}
		if r.err != 0 {
			return r, &HeaderError{Class: byte(r.err >> 8), Code: byte(r.err)}
		}
		return r, nil
	}
}

// exchange sends one PDU under the connection lock.
func (c *Conn) exchange(ctx context.Context, ros byte, param, data []byte) (response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken != nil {
		return response{}, fmt.Errorf("connection unusable: %w", c.broken)
	}
	if c.nc == nil {
		return response{}, errors.New("connection closed")
	}
	defer c.deadline(ctx)()
	return c.exchangeLocked(ctx, job(ros, c.nextRef(), param, data))
}

// PDU returns the negotiated PDU size.
func (c *Conn) PDU() int { return c.pdu }

// Usable reports whether the connection can still carry requests (not closed, no I/O error so far).
func (c *Conn) Usable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nc != nil && c.broken == nil
}

// Close closes the connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc == nil {
		return nil
	}
	err := c.nc.Close()
	c.nc = nil
	return err
}

// fragment is a part of one address read in one request item.
type fragment struct {
	item int
	off  int // byte offset within the address
	n    int // bytes
}

// planRead splits the addresses into requests whose request and response fit the PDU.
func planRead(addrs []Address, pdu int) [][]fragment {
	maxData := (pdu - headerAck - 2 - 4) &^ 1
	var frags []fragment
	for i, a := range addrs {
		size := a.Size()
		for off := 0; off < size; off += maxData {
			frags = append(frags, fragment{item: i, off: off, n: min(maxData, size-off)})
		}
	}
	var out [][]fragment
	var cur []fragment
	resp := headerAck + 2
	for _, f := range frags {
		need := 4 + f.n + f.n&1
		if len(cur) > 0 && (len(cur)+1 > maxItems || headerJob+2+readItemSize*(len(cur)+1) > pdu || resp+need > pdu) {
			out = append(out, cur)
			cur, resp = nil, headerAck+2
		}
		cur = append(cur, f)
		resp += need
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// Read reads the addresses. Item errors (missing data block, address out of range…) are reported per address in
// Result.Err; the returned error is for failures of the whole job (connection, PUT/GET not permitted…).
func (c *Conn) Read(ctx context.Context, addrs []Address) ([]Result, error) {
	res := make([]Result, len(addrs))
	bufs := make([][]byte, len(addrs))
	for i, a := range addrs {
		bufs[i] = make([]byte, a.Size())
	}
	for _, req := range planRead(addrs, c.pdu) {
		param := []byte{fnRead, byte(len(req))}
		for _, f := range req {
			a := addrs[f.item]
			if a.Type == "bool" {
				param = append(param, anyPointer(tsBit, 1, a.Area, a.DB, a.Start*8+a.Bit)...)
			} else {
				param = append(param, anyPointer(tsByte, f.n, a.Area, a.DB, (a.Start+f.off)*8)...)
			}
		}
		r, err := c.exchange(ctx, rosJob, param, nil)
		if err != nil {
			return nil, err
		}
		if len(r.param) < 2 || r.param[0] != fnRead || int(r.param[1]) != len(req) {
			return nil, errors.New("invalid read response")
		}
		d := r.data
		for i, f := range req {
			if len(d) < 4 {
				return nil, errors.New("truncated read response")
			}
			code, ts, n := d[0], d[1], int(binary.BigEndian.Uint16(d[2:]))
			d = d[4:]
			if code != 0xFF {
				if res[f.item].Err == nil {
					res[f.item].Err = &ItemError{Code: code}
				}
				continue
			}
			l := dataLen(ts, n)
			if l > len(d) {
				return nil, errors.New("truncated read response")
			}
			if l < f.n && res[f.item].Err == nil {
				res[f.item].Err = errors.New("short response")
			}
			copy(bufs[f.item][f.off:], d[:min(l, f.n)])
			d = d[l:]
			if l&1 == 1 && i < len(req)-1 && len(d) > 0 {
				d = d[1:] // fill byte
			}
		}
	}
	for i, a := range addrs {
		if res[i].Err != nil {
			continue
		}
		res[i].Value, res[i].Err = a.Decode(bufs[i])
	}
	return res, nil
}

// Write writes one value (see Address.Encode). Strings longer than one PDU are written in several jobs.
func (c *Conn) Write(ctx context.Context, a Address, v any) error {
	b, err := a.Encode(v)
	if err != nil {
		return err
	}
	if a.Type == "bool" {
		return c.writeItem(ctx, anyPointer(tsBit, 1, a.Area, a.DB, a.Start*8+a.Bit), dtsBit, 1, b)
	}
	start := a.Start
	if a.Type == "string" {
		start++ // keep the maximum-length byte
	}
	maxData := (c.pdu - headerAck - 14 - 4) &^ 1
	for off := 0; off < len(b); off += maxData {
		chunk := b[off:min(len(b), off+maxData)]
		if err := c.writeItem(ctx, anyPointer(tsByte, len(chunk), a.Area, a.DB, (start+off)*8), dtsByte, len(chunk)*8, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (c *Conn) writeItem(ctx context.Context, item []byte, ts byte, n int, b []byte) error {
	param := append([]byte{fnWrite, 1}, item...)
	data := append([]byte{0, ts, byte(n >> 8), byte(n)}, b...)
	r, err := c.exchange(ctx, rosJob, param, data)
	if err != nil {
		return err
	}
	if len(r.param) < 2 || r.param[0] != fnWrite || len(r.data) < 1 {
		return errors.New("invalid write response")
	}
	if r.data[0] != 0xFF {
		return &ItemError{Code: r.data[0]}
	}
	return nil
}

// Info is the CPU identification read from the system status lists.
type Info struct {
	OrderCode  string `json:"order_code,omitempty"`  // SZL 0x0011, e.g. 6ES7 214-1AG40-0XB0
	ModuleType string `json:"module_type,omitempty"` // SZL 0x001C, e.g. CPU 1214C DC/DC/DC
	ModuleName string `json:"module_name,omitempty"`
	PLCName    string `json:"plc_name,omitempty"`
	Serial     string `json:"serial,omitempty"`
}

// Identify reads the CPU identification (best effort: LOGO! and some CPUs answer only part of it). It fails only if
// nothing could be read.
func (c *Conn) Identify(ctx context.Context) (Info, error) {
	var info Info
	recs, err1 := c.readSZL(ctx, 0x0011, 0)
	for _, r := range recs {
		if len(r) >= 22 && binary.BigEndian.Uint16(r) == 1 {
			info.OrderCode = clean(r[2:22])
		}
	}
	recs, err2 := c.readSZL(ctx, 0x001C, 0)
	for _, r := range recs {
		if len(r) < 4 {
			continue
		}
		v := clean(r[2:min(len(r), 34)])
		switch binary.BigEndian.Uint16(r) {
		case 1:
			info.PLCName = v
		case 2:
			info.ModuleName = v
		case 5:
			info.Serial = v
		case 7:
			info.ModuleType = v
		}
	}
	if err1 != nil && err2 != nil {
		return info, err1
	}
	return info, nil
}

func clean(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(latin1(b))
}

// readSZL reads the first fragment of a system status list and returns its records.
func (c *Conn) readSZL(ctx context.Context, id, index uint16) ([][]byte, error) {
	param := []byte{0x00, 0x01, 0x12, 0x04, 0x11, 0x44, 0x01, 0x00}
	data := []byte{0xFF, 0x09, 0x00, 0x04, byte(id >> 8), byte(id), byte(index >> 8), byte(index)}
	r, err := c.exchange(ctx, rosUserData, param, data)
	if err != nil {
		return nil, err
	}
	if len(r.param) >= 12 {
		if e := binary.BigEndian.Uint16(r.param[10:]); e != 0 {
			return nil, &HeaderError{Class: byte(e >> 8), Code: byte(e)}
		}
	}
	d := r.data
	if len(d) < 4 || d[0] != 0xFF {
		if len(d) > 0 {
			return nil, &ItemError{Code: d[0]}
		}
		return nil, errors.New("invalid SZL response")
	}
	d = d[4:]
	if len(d) < 8 {
		return nil, errors.New("invalid SZL response")
	}
	size, count := int(binary.BigEndian.Uint16(d[4:])), int(binary.BigEndian.Uint16(d[6:]))
	d = d[8:]
	var recs [][]byte
	for i := 0; i < count && size > 0 && len(d) >= size; i++ {
		recs = append(recs, d[:size])
		d = d[size:]
	}
	return recs, nil
}
