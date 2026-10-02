// Package protocol parses device messages according to docs/protocol/device-protocol-v0.1.md.
// It is transport-agnostic (MQTT or HTTP) and allocation-conscious enough for large batches.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

const (
	// MaxTelemetryBytes is the maximum size of a single telemetry message (batch) accepted by the server.
	MaxTelemetryBytes = 64 * 1024
	// MaxSamplesPerMessage bounds batch size to keep a single transaction small.
	MaxSamplesPerMessage = 2000
	// MaxStringValue bounds string values in "v".
	MaxStringValue = 256
)

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Sample is one time point with one or more values.
type Sample struct {
	MeasuredAt *time.Time       // nil = device did not supply a time; the server assigns received_at
	Relative   *int64           // "dt": seconds relative to send time (negative = in the past); used when no ts
	Values     map[string]Value // key -> value
	Backfilled bool             // "bf": 1
	Seq        *int64           // "seq": device message sequence, for deduplication
	// Faults are the keys the device reports as coming from a faulty sensor ("fault": ["temp"] or
	// {"temp": true}); FaultAll is "fault": true (every value of the sample). The server stores such values with
	// quality sensor_fault.
	Faults   map[string]bool
	FaultAll bool
}

// Faulty reports whether the device flagged the value of key as coming from a faulty sensor.
func (s Sample) Faulty(key string) bool { return s.FaultAll || s.Faults[key] }

// Any returns the value as a plain Go value (float64, bool, string or json.RawMessage).
func (v Value) Any() any {
	switch {
	case v.Num != nil:
		return *v.Num
	case v.Bool != nil:
		return *v.Bool
	case v.Str != nil:
		return *v.Str
	default:
		return v.JSON
	}
}

// Value holds a numeric, boolean, string or structured value.
type Value struct {
	Num  *float64
	Bool *bool
	Str  *string
	JSON json.RawMessage // objects/arrays (state, GPS, ...)
}

// Kind returns "num", "bool", "str" or "json".
func (v Value) Kind() string {
	switch {
	case v.Num != nil:
		return "num"
	case v.Bool != nil:
		return "bool"
	case v.Str != nil:
		return "str"
	default:
		return "json"
	}
}

type rawSample struct {
	P   *int                       `json:"p"`
	TS  *int64                     `json:"ts"`
	TSM *int64                     `json:"tsm"`
	DT  *int64                     `json:"dt"`
	V   map[string]json.RawMessage `json:"v"`
	BF  *int                       `json:"bf"`
	Seq *int64                     `json:"seq"`

	faults   map[string]bool // "fault" flags, normalised keys
	faultAll bool
}

// ErrTooLarge is returned when a message exceeds the size limits.
var ErrTooLarge = errors.New("protocol: message too large")

// ParseTelemetry accepts a single object or an array of objects in the ctrl32 format, the ThingsBoard format or as a
// flat object (see lenient.go).
func ParseTelemetry(payload []byte) ([]Sample, error) {
	if len(payload) > MaxTelemetryBytes {
		return nil, ErrTooLarge
	}
	trimmed := bytesTrimLeft(payload)
	if len(trimmed) == 0 {
		return nil, errors.New("protocol: empty payload")
	}
	var objs []map[string]json.RawMessage
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &objs); err != nil {
			return nil, fmt.Errorf("protocol: invalid JSON array: %w", err)
		}
	} else {
		var one map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return nil, fmt.Errorf("protocol: invalid JSON object: %w", err)
		}
		objs = []map[string]json.RawMessage{one}
	}
	if len(objs) > MaxSamplesPerMessage {
		return nil, ErrTooLarge
	}
	raws := make([]rawSample, 0, len(objs))
	for i, o := range objs {
		r, err := normalizeRaw(o)
		if err != nil {
			return nil, fmt.Errorf("protocol: sample %d: %w", i, err)
		}
		raws = append(raws, r)
	}
	if len(raws) > MaxSamplesPerMessage {
		return nil, ErrTooLarge
	}
	out := make([]Sample, 0, len(raws))
	for i, r := range raws {
		if r.P != nil && *r.P != 1 {
			return nil, fmt.Errorf("protocol: sample %d: unsupported protocol version %d", i, *r.P)
		}
		if len(r.V) == 0 {
			return nil, fmt.Errorf("protocol: sample %d: no values (expected {\"v\":{…}}, {\"values\":{…}} or a flat object)", i)
		}
		s := Sample{Values: make(map[string]Value, len(r.V)), Seq: r.Seq}
		switch {
		case r.TSM != nil:
			t := time.UnixMilli(*r.TSM).UTC()
			s.MeasuredAt = &t
		case r.TS != nil:
			t := time.Unix(*r.TS, 0).UTC()
			s.MeasuredAt = &t
		case r.DT != nil:
			s.Relative = r.DT
		}
		if r.BF != nil && *r.BF == 1 {
			s.Backfilled = true
		}
		s.Faults, s.FaultAll = r.faults, r.faultAll
		for k, raw := range r.V {
			if !keyRe.MatchString(k) {
				return nil, fmt.Errorf("protocol: sample %d: invalid key %q", i, k)
			}
			v, err := parseValue(raw)
			if err != nil {
				return nil, fmt.Errorf("protocol: sample %d: key %q: %w", i, k, err)
			}
			s.Values[k] = v
		}
		out = append(out, s)
	}
	return out, nil
}

func parseValue(raw json.RawMessage) (Value, error) {
	if len(raw) == 0 {
		return Value{}, errors.New("empty value")
	}
	switch raw[0] {
	case '{', '[':
		return Value{JSON: raw}, nil
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return Value{}, err
		}
		if len(s) > MaxStringValue {
			return Value{}, fmt.Errorf("string value longer than %d bytes", MaxStringValue)
		}
		return Value{Str: &s}, nil
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return Value{}, err
		}
		return Value{Bool: &b}, nil
	case 'n':
		return Value{}, errors.New("null is not a valid value")
	default:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return Value{}, err
		}
		return Value{Num: &f}, nil
	}
}

func bytesTrimLeft(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	return b[i:]
}
