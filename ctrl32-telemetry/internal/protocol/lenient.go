package protocol

import (
	"encoding/json"
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
