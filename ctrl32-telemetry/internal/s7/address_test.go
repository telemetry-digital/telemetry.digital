package s7_test

import (
	"math"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
)

func TestParseAddress(t *testing.T) {
	cases := []struct {
		in, typ string
		want    s7.Address
		canon   string
	}{
		{"DB1.DBX0.0", "", s7.Address{Area: s7.AreaDB, DB: 1, Type: "bool"}, "DB1.DBX0.0"},
		{"db1.dbx6.7", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 6, Bit: 7, Type: "bool"}, "DB1.DBX6.7"},
		{"DB1.DBB2", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 2, Type: "byte"}, "DB1.DBB2"},
		{"DB1.DBB2", "char", s7.Address{Area: s7.AreaDB, DB: 1, Start: 2, Type: "char"}, "DB1.DBB2"},
		{"DB1.DBW4", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 4, Type: "int"}, "DB1.DBW4"},
		{"DB1.DBW4", "word", s7.Address{Area: s7.AreaDB, DB: 1, Start: 4, Type: "word"}, "DB1.DBW4"},
		{"DB1.DBD6", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 6, Type: "real"}, "DB1.DBD6"},
		{"DB1.DBD6", "DINT", s7.Address{Area: s7.AreaDB, DB: 1, Start: 6, Type: "dint"}, "DB1.DBD6"},
		{"DB1.DBS10.20", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 10, Type: "string", Len: 20}, "DB1.DBS10.20"},
		{"I0.0", "", s7.Address{Area: s7.AreaI, Type: "bool"}, "I0.0"},
		{"E1.3", "", s7.Address{Area: s7.AreaI, Start: 1, Bit: 3, Type: "bool"}, "I1.3"},
		{"IB0", "", s7.Address{Area: s7.AreaI, Type: "byte"}, "IB0"},
		{"EW64", "", s7.Address{Area: s7.AreaI, Start: 64, Type: "int"}, "IW64"},
		{"ID0", "", s7.Address{Area: s7.AreaI, Type: "real"}, "ID0"},
		{"Q0.1", "", s7.Address{Area: s7.AreaQ, Bit: 1, Type: "bool"}, "Q0.1"},
		{"A0.1", "", s7.Address{Area: s7.AreaQ, Bit: 1, Type: "bool"}, "Q0.1"},
		{"AB2", "", s7.Address{Area: s7.AreaQ, Start: 2, Type: "byte"}, "QB2"},
		{"QW2", "uint", s7.Address{Area: s7.AreaQ, Start: 2, Type: "uint"}, "QW2"},
		{"AD4", "dword", s7.Address{Area: s7.AreaQ, Start: 4, Type: "dword"}, "QD4"},
		{"M10.0", "", s7.Address{Area: s7.AreaM, Start: 10, Type: "bool"}, "M10.0"},
		{"MB10", "sint", s7.Address{Area: s7.AreaM, Start: 10, Type: "sint"}, "MB10"},
		{"MW10", "", s7.Address{Area: s7.AreaM, Start: 10, Type: "int"}, "MW10"},
		{"MD10", "", s7.Address{Area: s7.AreaM, Start: 10, Type: "real"}, "MD10"},
		{"MR12", "", s7.Address{Area: s7.AreaM, Start: 12, Type: "real"}, "MD12"},
		{"DB1,X0.0", "", s7.Address{Area: s7.AreaDB, DB: 1, Type: "bool"}, "DB1.DBX0.0"},
		{"DB1,B2", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 2, Type: "byte"}, "DB1.DBB2"},
		{"DB1,CHAR3", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 3, Type: "char"}, "DB1.DBB3"},
		{"DB1,INT4", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 4, Type: "int"}, "DB1.DBW4"},
		{"DB1,WORD4", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 4, Type: "word"}, "DB1.DBW4"},
		{"DB1,DINT6", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 6, Type: "dint"}, "DB1.DBD6"},
		{"DB1,DWORD6", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 6, Type: "dword"}, "DB1.DBD6"},
		{"DB1,REAL6", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 6, Type: "real"}, "DB1.DBD6"},
		{"DB1, REAL6", "udint", s7.Address{Area: s7.AreaDB, DB: 1, Start: 6, Type: "udint"}, "DB1.DBD6"},
		{"DB1,LREAL10", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 10, Type: "lreal"}, "DB1,LREAL10"},
		{"DB1,S10.20", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 10, Type: "string", Len: 20}, "DB1.DBS10.20"},
		{"DB2,USINT0", "", s7.Address{Area: s7.AreaDB, DB: 2, Type: "usint"}, "DB2.DBB0"},
		{"VB100", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 100, Type: "byte"}, "DB1.DBB100"},
		{"VW100", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 100, Type: "int"}, "DB1.DBW100"},
		{"VD100", "dint", s7.Address{Area: s7.AreaDB, DB: 1, Start: 100, Type: "dint"}, "DB1.DBD100"},
		{"V10.3", "", s7.Address{Area: s7.AreaDB, DB: 1, Start: 10, Bit: 3, Type: "bool"}, "DB1.DBX10.3"},
	}
	for _, c := range cases {
		got, err := s7.ParseAddress(c.in, c.typ)
		if err != nil {
			t.Errorf("%s/%s: %v", c.in, c.typ, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s/%s: got %+v, want %+v", c.in, c.typ, got, c.want)
		}
		if got.String() != c.canon {
			t.Errorf("%s: canonical %q, want %q", c.in, got.String(), c.canon)
		}
		back, err := s7.ParseAddress(got.String(), got.Type)
		if err != nil || back != got {
			t.Errorf("%s: canonical form does not round-trip: %+v %v", c.in, back, err)
		}
	}
}

func TestParseAddressInvalid(t *testing.T) {
	cases := []struct{ in, typ, msg string }{
		{"", "", "required"},
		{"DB1.DBX0", "", "bit number"},
		{"DB1.DBX0.8", "", "0..7"},
		{"DB0.DBW0", "", "data block number"},
		{"DB1.DBW4", "real", "type real needs a double word"},
		{"DB1.DBD4", "int", "type int needs a word"},
		{"DB1.DBB4", "bool", "type bool needs a bit"},
		{"MW10", "string", "string"},
		{"DB1.DBS10", "", "maximum length"},
		{"DB1.DBS10.255", "", "1..254"},
		{"DB1.DBW4.1", "", ".n suffix"},
		{"DB1,FOO4", "", "unknown type"},
		{"DB1,INT4", "real", "does not match"},
		{"DB1,INT4.3", "", "arrays"},
		{"DB1.DBW4", "float", "unknown data type"},
		{"X10", "", "invalid S7 address"},
		{"MW", "", "invalid S7 address"},
		{"I0", "", "bit number"},
	}
	for _, c := range cases {
		_, err := s7.ParseAddress(c.in, c.typ)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q/%q: got %v, want error containing %q", c.in, c.typ, err, c.msg)
		}
	}
}

func TestEncodeDecode(t *testing.T) {
	cases := []struct {
		addr, typ string
		in        any
		want      any
	}{
		{"DB1.DBX0.0", "", true, true},
		{"DB1.DBB0", "", 200.0, 200.0},
		{"DB1.DBB0", "sint", -5, -5.0},
		{"DB1.DBB0", "char", "A", "A"},
		{"DB1.DBW0", "", -1234.4, -1234.0},
		{"DB1.DBW0", "word", 65535, 65535.0},
		{"DB1.DBD0", "dint", -100000, -100000.0},
		{"DB1.DBD0", "udint", "4000000000", 4000000000.0},
		{"DB1.DBD0", "", 21.5, 21.5},
		{"DB1,LREAL0", "", math.Pi, math.Pi},
	}
	for _, c := range cases {
		a, err := s7.ParseAddress(c.addr, c.typ)
		if err != nil {
			t.Fatal(err)
		}
		b, err := a.Encode(c.in)
		if err != nil {
			t.Errorf("%s/%s encode: %v", c.addr, c.typ, err)
			continue
		}
		v, err := a.Decode(b)
		if err != nil || v != c.want {
			t.Errorf("%s/%s: %v %v, want %v", c.addr, c.typ, v, err, c.want)
		}
	}
	bad := []struct {
		addr, typ string
		in        any
	}{
		{"DB1.DBW0", "", 40000},
		{"DB1.DBW0", "uint", -1},
		{"DB1.DBB0", "", 256},
		{"DB1.DBB0", "sint", 128},
		{"DB1.DBD0", "dint", 3e9},
		{"DB1.DBD0", "", 1e40},
		{"DB1.DBD0", "", math.NaN()},
		{"DB1.DBX0.0", "", 2},
		{"DB1.DBS0.3", "", "toolong"},
		{"DB1.DBS0.3", "", 5},
		{"DB1.DBB0", "char", "AB"},
		{"DB1.DBW0", "", "abc"},
	}
	for _, c := range bad {
		a, _ := s7.ParseAddress(c.addr, c.typ)
		if _, err := a.Encode(c.in); err == nil {
			t.Errorf("%s/%s: %v accepted", c.addr, c.typ, c.in)
		}
	}
}

func TestParseTSAP(t *testing.T) {
	for in, want := range map[string]uint16{"10.00": 0x1000, "20.00": 0x2000, "0x0301": 0x0301, "1.2": 0x0102, "0200": 0x0200} {
		got, err := s7.ParseTSAP(in)
		if err != nil || got != want {
			t.Errorf("%s: %04X %v", in, got, err)
		}
	}
	for _, in := range []string{"", "xyz", "100.00", "1.2.3", "12345"} {
		if _, err := s7.ParseTSAP(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}
