package modbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"

	mb "github.com/simonvetter/modbus"
)

// Grouped reads: registers of the same table with contiguous (or nearly contiguous) addresses are fetched in one
// request — up to 120 registers or 1000 coils per request, gaps of at most 8 registers are read and discarded.
// Multi-register values are decoded here according to the connector's byte order, so a poll of 50 registers is a
// handful of round trips instead of 50.

const (
	maxRegSpan  = 120
	maxCoilSpan = 1000
	maxGap      = 8
)

type span struct {
	table     string
	start     int
	end       int // exclusive
	registers []Register
}

// plan groups the registers into read spans per table.
func plan(st Settings) []span {
	byTable := map[string][]Register{}
	for _, r := range st.Registers {
		byTable[r.Table] = append(byTable[r.Table], r)
	}
	var out []span
	for _, table := range []string{"holding", "input", "coil", "discrete"} {
		regs := byTable[table]
		if len(regs) == 0 {
			continue
		}
		sort.Slice(regs, func(i, j int) bool { return regs[i].Address < regs[j].Address })
		limit := maxRegSpan
		if table == "coil" || table == "discrete" {
			limit = maxCoilSpan
		}
		cur := span{table: table, start: regs[0].Address, end: regs[0].Address + types[regs[0].Type]}
		cur.registers = []Register{regs[0]}
		for _, r := range regs[1:] {
			end := r.Address + types[r.Type]
			if r.Address-cur.end <= maxGap && end-cur.start <= limit {
				if end > cur.end {
					cur.end = end
				}
				cur.registers = append(cur.registers, r)
				continue
			}
			out = append(out, cur)
			cur = span{table: table, start: r.Address, end: end, registers: []Register{r}}
		}
		out = append(out, cur)
	}
	return out
}

// decode turns the raw registers of a span into the scaled value of one register according to the byte order.
func decode(regs []uint16, offset int, r Register, byteOrder string) (any, error) {
	n := types[r.Type]
	if offset < 0 || offset+n > len(regs) {
		return nil, errors.New("short response")
	}
	words := append([]uint16(nil), regs[offset:offset+n]...)
	switch byteOrder {
	case "big_swap", "little":
		// low word first: reverse the word order
		for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
			words[i], words[j] = words[j], words[i]
		}
	}
	buf := make([]byte, 2*n)
	for i, w := range words {
		if byteOrder == "little" || byteOrder == "little_swap" {
			binary.LittleEndian.PutUint16(buf[2*i:], w)
		} else {
			binary.BigEndian.PutUint16(buf[2*i:], w)
		}
	}
	var raw float64
	switch r.Type {
	case "int16":
		raw = float64(int16(binary.BigEndian.Uint16(buf)))
	case "uint16":
		raw = float64(binary.BigEndian.Uint16(buf))
	case "int32":
		raw = float64(int32(binary.BigEndian.Uint32(buf)))
	case "uint32":
		raw = float64(binary.BigEndian.Uint32(buf))
	case "float32":
		raw = float64(math.Float32frombits(binary.BigEndian.Uint32(buf)))
	case "int64":
		raw = float64(int64(binary.BigEndian.Uint64(buf)))
	case "uint64":
		raw = float64(binary.BigEndian.Uint64(buf))
	case "float64":
		raw = math.Float64frombits(binary.BigEndian.Uint64(buf))
	default:
		return nil, fmt.Errorf("unsupported type %s", r.Type)
	}
	if math.IsNaN(raw) || math.IsInf(raw, 0) {
		return nil, errors.New("not a number")
	}
	return math.Round(scaled(r, raw)*1e6) / 1e6, nil
}

// read polls every register of the settings with grouped requests; unreadable spans are reported per register.
func read(c *mb.ModbusClient, st Settings) (map[string]any, []string) {
	values := map[string]any{}
	var bad []string
	for _, sp := range plan(st) {
		qty := uint16(sp.end - sp.start)
		switch sp.table {
		case "coil", "discrete":
			var bits []bool
			var err error
			if sp.table == "coil" {
				bits, err = c.ReadCoils(uint16(sp.start), qty)
			} else {
				bits, err = c.ReadDiscreteInputs(uint16(sp.start), qty)
			}
			for _, r := range sp.registers {
				if err != nil {
					bad = append(bad, r.Key+": "+err.Error())
					continue
				}
				i := r.Address - sp.start
				if i < 0 || i >= len(bits) {
					bad = append(bad, r.Key+": short response")
					continue
				}
				values[r.Key] = bits[i]
			}
		default:
			rt := mb.HOLDING_REGISTER
			if sp.table == "input" {
				rt = mb.INPUT_REGISTER
			}
			regs, err := c.ReadRegisters(uint16(sp.start), qty, rt)
			for _, r := range sp.registers {
				if err != nil {
					bad = append(bad, r.Key+": "+err.Error())
					continue
				}
				v, derr := decode(regs, r.Address-sp.start, r, st.ByteOrder)
				if derr != nil {
					bad = append(bad, r.Key+": "+derr.Error())
					continue
				}
				values[r.Key] = v
			}
		}
	}
	sort.Strings(bad)
	return values, bad
}
