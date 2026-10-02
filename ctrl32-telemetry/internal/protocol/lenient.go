package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Foreign devices rarely speak the ctrl32 format ({"ts","v":{…}}) out of the box. The parser therefore also accepts:
//   - ThingsBoard style: {"ts": 1700000000000, "values": {"temp": 4.2}} (ts in milliseconds),
//   - flat objects: {"temp": 4.2, "rh": 61} or {"ts": 1700000000, "temp": 4.2} — every key that is not one of the
//     reserved fields (p, ts, tsm, dt, bf, seq, v, values) is a value,
//   - arrays of any of these,
//   - a numeric "ts" larger than 10 000 000 000 is treated as milliseconds (a seconds value that big would be year 2286).
// Keys are normalised: letters lower-cased, "-", "." and spaces become "_", so "Temp-1" reaches datastream "temp_1".
//
// Sensor faults (0.67.0): a sample may carry "fault" — a list of keys (["temp"]), an object of keys with true or a
// reason ({"temp": "open circuit"}; false, 0 or null = no fault) or, next to "v"/"values", true for every value of
// the sample. In a flat object "fault" counts as flags only when it is a list or an object; a number, boolean or text
// stays an ordinary value named "fault" (devices that already publish such a key keep working).

var reservedKeys = map[string]bool{"p": true, "ts": true, "tsm": true, "dt": true, "bf": true, "seq": true, "v": true, "values": true}

var looseKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\- ]{0,31}$`)

// NormalizeKey maps a foreign key onto the ctrl32 key grammar; ok is false when it cannot be mapped.
func NormalizeKey(k string) (string, bool) {
	if keyRe.MatchString(k) {
		return k, true
	}
	if !looseKeyRe.MatchString(k) {
		return "", false
	}
	n := strings.ToLower(k)
	n = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(n)
	return n, keyRe.MatchString(n)
}

const msThreshold = 10_000_000_000

// normalizeRaw turns the accepted shapes into the canonical rawSample.
func normalizeRaw(obj map[string]json.RawMessage) (rawSample, error) {
	var r rawSample
	num := func(key string) (*int64, error) {
		raw, ok := obj[key]
		if !ok {
			return nil, nil
		}
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		v := int64(f)
		return &v, nil
	}
	var err error
	if r.TS, err = num("ts"); err != nil {
		return r, err
	}
	if r.TSM, err = num("tsm"); err != nil {
		return r, err
	}
	if r.DT, err = num("dt"); err != nil {
		return r, err
	}
	if r.Seq, err = num("seq"); err != nil {
		return r, err
	}
	if v, err := num("bf"); err != nil {
		return r, err
	} else if v != nil {
		i := int(*v)
		r.BF = &i
	}
	if v, err := num("p"); err != nil {
		return r, err
	} else if v != nil {
		i := int(*v)
		r.P = &i
	}
	if raw, ok := obj["fault"]; ok {
		structured := obj["v"] != nil || obj["values"] != nil
		all, keys, isFlags, err := parseFaults(raw, structured)
		if err != nil {
			return r, err
		}
		if isFlags {
			r.faultAll, r.faults = all, keys
			delete(obj, "fault") // flags, never a value
		}
	}
	if r.TS != nil && r.TSM == nil && *r.TS > msThreshold {
		r.TSM, r.TS = r.TS, nil
	}
	values := map[string]json.RawMessage{}
	switch {
	case obj["v"] != nil:
		if err := json.Unmarshal(obj["v"], &values); err != nil {
			return r, err
		}
	case obj["values"] != nil:
		if err := json.Unmarshal(obj["values"], &values); err != nil {
			return r, err
		}
	default:
		for k, raw := range obj {
			if reservedKeys[k] {
				continue
			}
			values[k] = raw
		}
	}
	r.V = make(map[string]json.RawMessage, len(values))
	for k, raw := range values {
		n, ok := NormalizeKey(k)
		if !ok {
			r.V[k] = raw // left as is so the caller reports the offending key
			continue
		}
		r.V[n] = raw
	}
	return r, nil
}

// parseFaults reads the "fault" field of a sample. isFlags is false when, in a flat object, the field is an ordinary
// value (a number, boolean or text named "fault").
func parseFaults(raw json.RawMessage, structured bool) (all bool, keys map[string]bool, isFlags bool, err error) {
	t := bytesTrimLeft(raw)
	if len(t) == 0 {
		return false, nil, false, nil
	}
	add := func(k string) error {
		n, ok := NormalizeKey(k)
		if !ok {
			return fmt.Errorf("fault: invalid key %q", k)
		}
		if keys == nil {
			keys = map[string]bool{}
		}
		keys[n] = true
		return nil
	}
	switch t[0] {
	case '[':
		var list []string
		if err := json.Unmarshal(t, &list); err != nil {
			return false, nil, true, fmt.Errorf("fault: expected a list of keys: %w", err)
		}
		for _, k := range list {
			if err := add(k); err != nil {
				return false, nil, true, err
			}
		}
		return false, keys, true, nil
	case '{':
		var m map[string]json.RawMessage
		if err := json.Unmarshal(t, &m); err != nil {
			return false, nil, true, fmt.Errorf("fault: expected an object of keys: %w", err)
		}
		for k, v := range m {
			switch string(bytesTrimLeft(v)) {
			case "", "false", "null", "0":
				continue // no fault
			}
			if err := add(k); err != nil {
				return false, nil, true, err
			}
		}
		return false, keys, true, nil
	}
	if !structured {
		return false, nil, false, nil // a flat value named "fault"
	}
	var b bool
	if err := json.Unmarshal(t, &b); err != nil {
		var f float64
		if json.Unmarshal(t, &f) != nil {
			return false, nil, true, errors.New("fault: expected true, a list of keys or an object of keys")
		}
		b = f != 0
	}
	return b, nil, true, nil
}
