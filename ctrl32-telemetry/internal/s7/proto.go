// Package s7 is a dependency-free Siemens S7 communication client (S7comm over ISO-on-TCP, RFC 1006, port 102)
// for S7-300/400/1200/1500, ET 200SP CPUs and LOGO! 0BA8 and later. It implements the connection handshake
// (TPKT + COTP connection request with TSAPs, S7 setup communication), Read Var with many items split to the
// negotiated PDU size, Write Var of one item and best-effort CPU identification (SZL). S7-1200/1500 need
// "Permit access with PUT/GET communication" and non-optimized data blocks for absolute addressing.
package s7

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// TPKT (RFC 1006) and COTP (ISO 8073) framing.
const (
	tpktHeader = 4
	cotpCR     = 0xE0 // connection request
	cotpCC     = 0xD0 // connection confirm
	cotpDR     = 0x80 // disconnect request
	cotpDT     = 0xF0 // data
	cotpEOT    = 0x80 // last data unit
	maxTPKT    = 4096
)

// S7 header message types (ROSCTR) and functions.
const (
	protoID     = 0x32
	rosJob      = 0x01
	rosAck      = 0x02
	rosAckData  = 0x03
	rosUserData = 0x07

	fnSetup = 0xF0
	fnRead  = 0x04
	fnWrite = 0x05
)

// Transport sizes of request items (ANY pointer) and of response data items.
const (
	tsBit  = 0x01
	tsByte = 0x02

	dtsBit   = 0x03 // length in bits
	dtsByte  = 0x04 // BYTE/WORD/DWORD, length in bits
	dtsInt   = 0x05 // length in bits
	dtsReal  = 0x07 // length in bytes
	dtsOctet = 0x09 // length in bytes
)

const (
	defaultPDU   = 960
	maxItems     = 20 // items per Read Var job
	headerJob    = 10
	headerAck    = 12
	readItemSize = 12
)

// readTPKT reads one TPKT frame and returns its payload (COTP header and user data).
func readTPKT(r io.Reader) ([]byte, error) {
	var h [tpktHeader]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	if h[0] != 3 {
		return nil, fmt.Errorf("not an ISO-on-TCP frame (version %d)", h[0])
	}
	n := int(binary.BigEndian.Uint16(h[2:]))
	if n < tpktHeader+2 || n > maxTPKT {
		return nil, fmt.Errorf("invalid TPKT length %d", n)
	}
	b := make([]byte, n-tpktHeader)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

// writeTPKT writes payload framed in TPKT.
func writeTPKT(w io.Writer, payload []byte) error {
	b := make([]byte, tpktHeader, tpktHeader+len(payload))
	b[0] = 3
	binary.BigEndian.PutUint16(b[2:], uint16(tpktHeader+len(payload)))
	_, err := w.Write(append(b, payload...))
	return err
}

// cotpConnect builds a COTP connection request with the TSAPs and a 1024-byte TPDU size.
func cotpConnect(local, remote uint16) []byte {
	return []byte{
		17, cotpCR, 0, 0, 0, 1, 0,
		0xC0, 1, 0x0A, // TPDU size 1024
		0xC1, 2, byte(local >> 8), byte(local),
		0xC2, 2, byte(remote >> 8), byte(remote),
	}
}

// writeData sends one S7 PDU in a COTP data frame.
func writeData(w io.Writer, pdu []byte) error {
	return writeTPKT(w, append([]byte{2, cotpDT, cotpEOT}, pdu...))
}

// readData reads COTP data frames until the last one and returns the reassembled S7 PDU.
func readData(r io.Reader) ([]byte, error) {
	var out []byte
	for {
		b, err := readTPKT(r)
		if err != nil {
			return nil, err
		}
		li := int(b[0])
		if li+1 > len(b) || li < 2 {
			return nil, errors.New("invalid COTP header")
		}
		switch b[1] & 0xF0 {
		case cotpDT:
		case cotpDR:
			return nil, errors.New("connection closed by the PLC (COTP disconnect)")
		default:
			return nil, fmt.Errorf("unexpected COTP frame 0x%02X", b[1])
		}
		out = append(out, b[li+1:]...)
		if len(out) > 65535 {
			return nil, errors.New("S7 PDU too long")
		}
		if b[2]&cotpEOT != 0 {
			return out, nil
		}
	}
}

// job builds an S7 job (or userdata) PDU.
func job(ros byte, ref uint16, param, data []byte) []byte {
	b := make([]byte, headerJob, headerJob+len(param)+len(data))
	b[0], b[1] = protoID, ros
	binary.BigEndian.PutUint16(b[4:], ref)
	binary.BigEndian.PutUint16(b[6:], uint16(len(param)))
	binary.BigEndian.PutUint16(b[8:], uint16(len(data)))
	return append(append(b, param...), data...)
}

// response is a parsed S7 response PDU.
type response struct {
	ros   byte
	ref   uint16
	err   uint16 // error class << 8 | error code (ack and ack_data only)
	param []byte
	data  []byte
}

func parseResponse(b []byte) (response, error) {
	var r response
	if len(b) < headerJob || b[0] != protoID {
		return r, errors.New("not an S7 PDU")
	}
	r.ros = b[1]
	r.ref = binary.BigEndian.Uint16(b[4:])
	pl, dl := int(binary.BigEndian.Uint16(b[6:])), int(binary.BigEndian.Uint16(b[8:]))
	h := headerJob
	if r.ros == rosAck || r.ros == rosAckData {
		if len(b) < headerAck {
			return r, errors.New("short S7 header")
		}
		r.err = binary.BigEndian.Uint16(b[10:])
		h = headerAck
	}
	if len(b) < h+pl+dl {
		return r, errors.New("truncated S7 PDU")
	}
	r.param = b[h : h+pl]
	r.data = b[h+pl : h+pl+dl]
	return r, nil
}

// HeaderError is the error class and code of an S7 response header: the whole job was rejected.
type HeaderError struct {
	Class, Code byte
}

// headerErrors maps class<<8|code to a readable message.
var headerErrors = map[uint16]string{
	0x0001: "application relationship error",
	0x0110: "invalid block number",
	0x0112: "invalid parameter",
	0x011A: "protocol error",
	0x8104: "function not supported or access denied — enable \"Permit access with PUT/GET communication\" in TIA Portal (CPU properties > Protection & Security > Connection mechanisms) and download the configuration",
	0x8304: "context not supported",
	0x8500: "PDU size error (request or response larger than the negotiated PDU)",
	0x8702: "address error",
	0xD201: "syntax error in block name",
	0xD209: "block not found",
	0xD241: "protection level of the CPU too low for this function",
	0xD401: "protocol error (invalid SZL ID)",
	0xD602: "access denied — the CPU protection level requires a password",
	0xD604: "access denied — insufficient protection level",
	0xD802: "unknown function",
	0xDC01: "invalid time",
}

func (e *HeaderError) Error() string {
	c := uint16(e.Class)<<8 | uint16(e.Code)
	if m, ok := headerErrors[c]; ok {
		return fmt.Sprintf("S7 error 0x%04X: %s", c, m)
	}
	switch e.Class {
	case 0x81:
		return fmt.Sprintf("S7 error 0x%04X: application relationship error", c)
	case 0x82:
		return fmt.Sprintf("S7 error 0x%04X: object definition error", c)
	case 0x83:
		return fmt.Sprintf("S7 error 0x%04X: no resources available", c)
	case 0x84:
		return fmt.Sprintf("S7 error 0x%04X: error on service processing", c)
	case 0x85:
		return fmt.Sprintf("S7 error 0x%04X: error on supplies", c)
	case 0x87:
		return fmt.Sprintf("S7 error 0x%04X: access error", c)
	}
	return fmt.Sprintf("S7 error 0x%04X", c)
}

// PutGetDisabled reports whether err is the PLC refusing the job because PUT/GET access is not permitted.
func PutGetDisabled(err error) bool {
	var he *HeaderError
	return errors.As(err, &he) && he.Class == 0x81 && he.Code == 0x04
}

// ItemError is the return code of one item of a Read Var or Write Var job.
type ItemError struct {
	Code byte
}

var itemErrors = map[byte]string{
	0x01: "hardware fault",
	0x03: "access denied (object access not allowed)",
	0x05: "address out of range",
	0x06: "data type not supported",
	0x07: "data type inconsistent",
	0x0A: "object does not exist (data block missing or optimized block access enabled)",
}

func (e *ItemError) Error() string {
	if m, ok := itemErrors[e.Code]; ok {
		return m
	}
	return fmt.Sprintf("item error 0x%02X", e.Code)
}

// anyPointer encodes an S7 ANY pointer request item.
func anyPointer(ts byte, count int, area Area, db int, bitAddr int) []byte {
	return []byte{
		0x12, 0x0A, 0x10, ts,
		byte(count >> 8), byte(count),
		byte(db >> 8), byte(db),
		byte(area),
		byte(bitAddr >> 16), byte(bitAddr >> 8), byte(bitAddr),
	}
}

// dataLen is the length in bytes of a response data item.
func dataLen(ts byte, n int) int {
	switch ts {
	case dtsBit, dtsByte, dtsInt:
		return (n + 7) / 8
	}
	return n
}
