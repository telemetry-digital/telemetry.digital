package opcua

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gopcua/opcua/ua"
)

// Writes as commands (URS A-12): a node marked writable can be set with the command write {key, value} on the
// connector's virtual device. The value is checked against the node's limits, converted to the data type the
// server currently reports for the node, written, read back and acked with the result; the caller records the
// command and the audit entry.

// WriteOp is a validated write for a virtual device.
type WriteOp struct {
	ConnectorID, ConnectorName string
	Settings                   Settings
	Node                       Node
	Value                      any // float64, bool or string
}

func (st Settings) node(key string) *Node {
	for i := range st.Nodes {
		if st.Nodes[i].Key == key {
			return &st.Nodes[i]
		}
	}
	return nil
}

// Prepare validates a command for a virtual device of an OPC UA connector without touching the network.
func (m *Manager) Prepare(ctx context.Context, deviceID, method string, args json.RawMessage) (*WriteOp, error) {
	if method != "write" {
		return nil, errors.New("OPC UA devices accept the method \"write\" with args {key, value}")
	}
	var a struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, errors.New("args must be {key, value}")
	}
	w := m.workerFor(ctx, deviceID)
	if w == nil {
		return nil, errors.New("no running OPC UA connector owns this device")
	}
	n := w.row.Settings.node(a.Key)
	if n == nil {
		return nil, fmt.Errorf("unknown key %q", a.Key)
	}
	if !n.Writable {
		return nil, fmt.Errorf("key %q is not writable", a.Key)
	}
	op := &WriteOp{ConnectorID: w.row.ID, ConnectorName: w.row.Name, Settings: w.row.Settings, Node: *n}
	switch v := a.Value.(type) {
	case bool, string:
		op.Value = v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("value must be a finite number")
		}
		if n.Min != nil && v < *n.Min || n.Max != nil && v > *n.Max {
			return nil, fmt.Errorf("value %v is outside the limits %s..%s", v, fmtLimit(n.Min, "-∞"), fmtLimit(n.Max, "∞"))
		}
		op.Value = v
	default:
		return nil, errors.New("value must be a number, true/false or a string")
	}
	return op, nil
}

func fmtLimit(p *float64, def string) string {
	if p == nil {
		return def
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

func (m *Manager) workerFor(ctx context.Context, deviceID string) *worker {
	m.mu.Lock()
	ws := make([]*worker, 0, len(m.workers))
	for _, w := range m.workers {
		ws = append(ws, w)
	}
	m.mu.Unlock()
	for _, w := range ws {
		d, err := w.device(ctx)
		if err == nil && d.ID == deviceID {
			return w
		}
	}
	return nil
}

// unscale reverses the node's scale/offset so the server receives the raw engineering value.
func unscale(n Node, v float64) float64 {
	if n.Offset != nil {
		v -= *n.Offset
	}
	if n.Scale != nil && *n.Scale != 0 {
		v /= *n.Scale
	}
	return v
}

// coerce converts the requested value to the Go type of the node's current value so the variant type matches.
func coerce(cur any, n Node, v any) (any, error) {
	switch cur.(type) {
	case bool:
		switch x := v.(type) {
		case bool:
			return x, nil
		case float64:
			return x != 0, nil
		}
		return nil, errors.New("node is boolean; send true/false")
	case string:
		return fmt.Sprint(v), nil
	}
	f, ok := v.(float64)
	if !ok {
		return nil, errors.New("node is numeric; send a number")
	}
	raw := unscale(n, f)
	r := math.Round(raw)
	switch cur.(type) {
	case float32:
		return float32(raw), nil
	case float64:
		return raw, nil
	case int8:
		if r < math.MinInt8 || r > math.MaxInt8 {
			return nil, errors.New("value does not fit int8")
		}
		return int8(r), nil
	case int16:
		if r < math.MinInt16 || r > math.MaxInt16 {
			return nil, errors.New("value does not fit int16")
		}
		return int16(r), nil
	case int32:
		if r < math.MinInt32 || r > math.MaxInt32 {
			return nil, errors.New("value does not fit int32")
		}
		return int32(r), nil
	case int64:
		return int64(r), nil
	case uint8:
		if r < 0 || r > math.MaxUint8 {
			return nil, errors.New("value does not fit uint8")
		}
		return uint8(r), nil
	case uint16:
		if r < 0 || r > math.MaxUint16 {
			return nil, errors.New("value does not fit uint16")
		}
		return uint16(r), nil
	case uint32:
		if r < 0 || r > math.MaxUint32 {
			return nil, errors.New("value does not fit uint32")
		}
		return uint32(r), nil
	case uint64:
		if r < 0 {
			return nil, errors.New("value does not fit uint64")
		}
		return uint64(r), nil
	}
	return nil, fmt.Errorf("node type %T cannot be written", cur)
}

// Do reads the node (for its data type), writes the value, reads it back and returns the result for the ack.
func (m *Manager) Do(ctx context.Context, op *WriteOp) (map[string]any, error) {
	c, err := m.connect(ctx, op.Settings)
	if err != nil {
		return nil, err
	}
	defer c.Close(ctx)
	id, err := ua.ParseNodeID(op.Node.NodeID)
	if err != nil {
		return nil, err
	}
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	readValue := func() (any, error) {
		resp, err := c.Read(rctx, &ua.ReadRequest{NodesToRead: []*ua.ReadValueID{{NodeID: id, AttributeID: ua.AttributeIDValue}}})
		if err != nil {
			return nil, err
		}
		if len(resp.Results) != 1 || resp.Results[0].Status != ua.StatusOK || resp.Results[0].Value == nil {
			return nil, errors.New("node could not be read")
		}
		return resp.Results[0].Value.Value(), nil
	}
	cur, err := readValue()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", op.Node.Key, err)
	}
	typed, err := coerce(cur, op.Node, op.Value)
	if err != nil {
		return nil, err
	}
	variant, err := ua.NewVariant(typed)
	if err != nil {
		return nil, err
	}
	wresp, err := c.Write(rctx, &ua.WriteRequest{NodesToWrite: []*ua.WriteValue{{NodeID: id, AttributeID: ua.AttributeIDValue, Value: &ua.DataValue{EncodingMask: ua.DataValueValue, Value: variant}}}})
	if err != nil {
		return nil, fmt.Errorf("write %s: %w", op.Node.Key, err)
	}
	if len(wresp.Results) != 1 || wresp.Results[0] != ua.StatusOK {
		st := ua.StatusBad
		if len(wresp.Results) == 1 {
			st = wresp.Results[0]
		}
		return nil, fmt.Errorf("write %s rejected: %s", op.Node.Key, st.Error())
	}
	after, err := readValue()
	if err != nil {
		return nil, fmt.Errorf("read back %s: %w", op.Node.Key, err)
	}
	back, _ := convert(after, op.Node)
	m.mu.Lock()
	if st, ok := m.status[op.ConnectorID]; ok {
		st.Writes++
		if st.LastValues == nil {
			st.LastValues = map[string]any{}
		}
		st.LastValues[op.Node.Key] = back
	}
	m.mu.Unlock()
	m.log.Info("opcua write", "connector", op.ConnectorName, "key", op.Node.Key, "node", op.Node.NodeID, "value", op.Value, "readback", back)
	return map[string]any{"key": op.Node.Key, "node_id": op.Node.NodeID, "value": op.Value, "readback": back, "written_at": time.Now().UTC()}, nil
}
