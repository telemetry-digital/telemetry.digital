package flows

import (
	"encoding/json"
	"fmt"
	"time"
)

// Message travels along the wires (research R4).
type Message struct {
	Topic   string           `json:"topic"`
	Value   Value            `json:"value"`
	TS      time.Time        `json:"ts"`
	Quality string           `json:"quality,omitempty"`
	Source  *Source          `json:"source,omitempty"`
	Fields  map[string]Value `json:"fields,omitempty"`
	Trigger string           `json:"-"` // description of what started this message (audit origin)
}

// Source of a message.
type Source struct {
	Kind string `json:"kind"` // datastream | alarm | device | schedule | webhook | inject
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// MaxMessageBytes bounds a message's serialized size.
const MaxMessageBytes = 16 << 10

// Clone copies a message (fields are copied shallowly; values are immutable).
func (m *Message) Clone() *Message {
	c := *m
	if m.Fields != nil {
		c.Fields = make(map[string]Value, len(m.Fields))
		for k, v := range m.Fields {
			c.Fields[k] = v
		}
	}
	return &c
}

// Size returns the serialized size.
func (m *Message) Size() int { b, _ := json.Marshal(m); return len(b) }

// Setting describes one node setting (served to the editor, enforced by validation).
type Setting struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // text multiline number bool select multi expr template datastream device channel time times duration rules fields url
	Default  any      `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Required bool     `json:"required,omitempty"`
	Help     string   `json:"help,omitempty"`
}

// NodeDef is one node type of the closed library.
type NodeDef struct {
	Type     string    `json:"type"`
	Group    string    `json:"group"` // trigger | logic | action | other
	Label    string    `json:"label"`
	Help     string    `json:"help"`
	Inputs   int       `json:"inputs"`
	Outputs  int       `json:"outputs"`             // fixed; switch computes its own
	OutNames []string  `json:"out_names,omitempty"` // labels of the outputs
	Icon     string    `json:"icon"`
	Color    string    `json:"color"`
	Settings []Setting `json:"settings"`
	// Breaks marks nodes that make a cycle legal (they separate messages in time).
	Breaks bool `json:"-"`
	// Commands marks nodes that act on devices or connectors (four-eyes for deploys).
	Commands bool `json:"commands,omitempty"`
}

func fp(f float64) *float64 { return &f }

var (
	cTrigger = "#0e7490"
	cLogic   = "#6d28d9"
	cAction  = "#b45309"
	cOther   = "#64748b"
	weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
)

// Catalog is the node library (US3).
var Catalog = []NodeDef{
	// ---- triggers
	{Type: "ds_value", Group: "trigger", Label: "Datastream value", Icon: "activity", Color: cTrigger, Outputs: 1,
		Help: "Starts a message for every stored value of a datastream.",
		Settings: []Setting{{Name: "datastream", Label: "Datastream", Kind: "datastream", Required: true},
			{Name: "only_changes", Label: "Only when the value changes", Kind: "bool"},
			{Name: "min_interval_s", Label: "At most one message every (s)", Kind: "number", Min: fp(0), Max: fp(86400)}}},
	{Type: "alarm_event", Group: "trigger", Label: "Alarm", Icon: "bell", Color: cTrigger, Outputs: 1,
		Help: "Starts a message when an alarm is raised, cleared or acknowledged.",
		Settings: []Setting{{Name: "events", Label: "Events", Kind: "multi", Options: []string{"raised", "cleared", "acknowledged"}, Default: []string{"raised"}},
			{Name: "severity", Label: "Severity", Kind: "select", Options: []string{"", "warning", "action", "critical"}},
			{Name: "datastream", Label: "Only this datastream", Kind: "datastream"}}},
	{Type: "device_state", Group: "trigger", Label: "Device connection", Icon: "wifi", Color: cTrigger, Outputs: 1,
		Help: "Starts a message when a device connects or disconnects.",
		Settings: []Setting{{Name: "device", Label: "Device (empty = any)", Kind: "device"},
			{Name: "events", Label: "Events", Kind: "multi", Options: []string{"connected", "disconnected"}, Default: []string{"connected", "disconnected"}}}},
	{Type: "entity", Group: "trigger", Label: "Entity", Icon: "home", Color: cTrigger, Outputs: 1,
		Help: "Starts a message when a smart-home entity changes its state (optionally only from/to given states). The value is the new state; msg.from the previous one.",
		Settings: []Setting{{Name: "entity", Label: "Entity", Kind: "entity", Required: true},
			{Name: "from", Label: "Only from state (empty = any)", Kind: "text"},
			{Name: "to", Label: "Only to state (empty = any)", Kind: "text"},
			{Name: "attributes", Label: "Also when only attributes change", Kind: "bool"}}},
	{Type: "command_result", Group: "trigger", Label: "Command result", Icon: "circle-check", Color: cTrigger, Outputs: 1,
		Help: "Starts a message when a device command ends (acknowledged, failed, expired or rejected). The value is the status; msg.error the error.",
		Settings: []Setting{{Name: "device", Label: "Device (empty = any)", Kind: "device"},
			{Name: "statuses", Label: "Statuses", Kind: "multi", Options: []string{"acked", "failed", "expired", "rejected"}, Default: []string{"failed", "expired"}}}},
	{Type: "mqtt_in", Group: "trigger", Label: "MQTT in", Icon: "antenna-bars-5", Color: cTrigger, Outputs: 1,
		Help: "Starts a message for every MQTT message on a topic filter (+ and # allowed) of an MQTT account or bridge. The topic is the MQTT topic; a JSON number, text or boolean becomes the value, the fields of a JSON object become msg.<field>.",
		Settings: []Setting{{Name: "source", Label: "MQTT account or bridge", Kind: "mqtt_source", Required: true},
			{Name: "topic", Label: "Topic filter", Kind: "text", Default: "#", Required: true}}},
	{Type: "schedule", Group: "trigger", Label: "Schedule", Icon: "clock", Color: cTrigger, Outputs: 1,
		Help: "Starts a message at an interval or at times of day on chosen weekdays (organization time zone).",
		Settings: []Setting{{Name: "mode", Label: "When", Kind: "select", Options: []string{"interval", "daily"}, Default: "interval"},
			{Name: "every_s", Label: "Every (s)", Kind: "number", Default: 60, Min: fp(1), Max: fp(604800)},
			{Name: "times", Label: "Times of day (HH:MM, comma separated)", Kind: "times", Default: "08:00"},
			{Name: "weekdays", Label: "Weekdays", Kind: "multi", Options: weekdays, Default: weekdays}}},
	{Type: "webhook", Group: "trigger", Label: "Incoming webhook", Icon: "plug", Color: cTrigger, Outputs: 1,
		Help:     "Starts a message for a POST to its URL (shown after deploy). The JSON body's \"value\" and \"topic\" become the message; other fields go to msg.<name>.",
		Settings: []Setting{{Name: "secret", Label: "HMAC secret (empty = token in the URL only)", Kind: "text"}}},
	{Type: "inject", Group: "trigger", Label: "Manual inject", Icon: "player-play", Color: cTrigger, Outputs: 1,
		Help:     "Starts a message when you press its button in the editor (or through the API).",
		Settings: []Setting{{Name: "value", Label: "Value (expression)", Kind: "expr", Default: "0"}, {Name: "topic", Label: "Topic", Kind: "text"}}},
	// ---- logic
	{Type: "filter", Group: "logic", Label: "Condition", Icon: "filter", Color: cLogic, Inputs: 1, Outputs: 2, OutNames: []string{"true", "false"},
		Help:     "Sends the message to the first output when the condition is true, otherwise to the second.",
		Settings: []Setting{{Name: "condition", Label: "Condition", Kind: "expr", Default: "value > 0", Required: true}}},
	{Type: "switch", Group: "logic", Label: "Switch", Icon: "adjustments", Color: cLogic, Inputs: 1, Outputs: 2,
		Help: "Routes the message by rules, one output per rule plus \"otherwise\".",
		Settings: []Setting{{Name: "rules", Label: "Rules (one condition per line)", Kind: "rules", Default: []string{"value < 10", "value >= 10"}, Required: true},
			{Name: "check", Label: "Check", Kind: "select", Options: []string{"first", "all"}, Default: "first"}}},
	{Type: "change", Group: "logic", Label: "Change", Icon: "settings", Color: cLogic, Inputs: 1, Outputs: 1,
		Help:     "Sets, deletes or renames message fields (value, topic, msg.<name>).",
		Settings: []Setting{{Name: "fields", Label: "Changes", Kind: "fields", Default: []any{map[string]any{"op": "set", "name": "value", "expr": "value"}}, Required: true}}},
	{Type: "math", Group: "logic", Label: "Math", Icon: "flask", Color: cLogic, Inputs: 1, Outputs: 1,
		Help: "Computes an expression into the value (or a field).",
		Settings: []Setting{{Name: "expr", Label: "Expression", Kind: "expr", Default: "value * 1", Required: true},
			{Name: "target", Label: "Into", Kind: "text", Default: "value", Help: "value, topic or msg.<name>"}}},
	{Type: "template", Group: "logic", Label: "Template", Icon: "info-circle", Color: cLogic, Inputs: 1, Outputs: 1,
		Help: "Builds a text with {{ expression }} placeholders, e.g. \"{{ topic }} is {{ fixed(value, 1) }} °C\".",
		Settings: []Setting{{Name: "text", Label: "Text", Kind: "template", Default: "{{ topic }} = {{ value }}", Required: true},
			{Name: "target", Label: "Into", Kind: "text", Default: "value"}}},
	{Type: "delay", Group: "logic", Label: "Delay", Icon: "clock", Color: cLogic, Inputs: 1, Outputs: 1, Breaks: true,
		Help:     "Passes every message on after a fixed time.",
		Settings: []Setting{{Name: "seconds", Label: "Delay (s)", Kind: "number", Default: 5, Min: fp(0.01), Max: fp(86400), Required: true}}},
	{Type: "debounce", Group: "logic", Label: "Debounce", Icon: "wave-sine", Color: cLogic, Inputs: 1, Outputs: 1, Breaks: true,
		Help:     "Passes only the last message after the input was quiet for the given time.",
		Settings: []Setting{{Name: "seconds", Label: "Quiet time (s)", Kind: "number", Default: 2, Min: fp(0.01), Max: fp(86400), Required: true}}},
	{Type: "rate_limit", Group: "logic", Label: "Rate limit", Icon: "gauge", Color: cLogic, Inputs: 1, Outputs: 1, Breaks: true,
		Help: "Passes at most N messages per period; the rest are dropped (counted).",
		Settings: []Setting{{Name: "count", Label: "Messages", Kind: "number", Default: 1, Min: fp(1), Max: fp(10000), Required: true},
			{Name: "period_s", Label: "Per (s)", Kind: "number", Default: 60, Min: fp(0.1), Max: fp(86400), Required: true},
			{Name: "per_topic", Label: "Separately per topic", Kind: "bool"}}},
	{Type: "deadband", Group: "logic", Label: "Deadband", Icon: "ripple", Color: cLogic, Inputs: 1, Outputs: 1,
		Help: "Passes a value only when it differs from the last passed value by at least the band (per topic).",
		Settings: []Setting{{Name: "band", Label: "Band", Kind: "number", Default: 0.5, Min: fp(0), Required: true},
			{Name: "percent", Label: "Band in % of the last value", Kind: "bool"}}},
	{Type: "aggregate", Group: "logic", Label: "Aggregate", Icon: "chart-bar", Color: cLogic, Inputs: 1, Outputs: 1, Breaks: true,
		Help: "Collects the values of a time window and sends one message with the result at its end (per topic).",
		Settings: []Setting{{Name: "window_s", Label: "Window (s)", Kind: "number", Default: 60, Min: fp(1), Max: fp(86400), Required: true},
			{Name: "fn", Label: "Function", Kind: "select", Options: []string{"avg", "min", "max", "sum", "count", "first", "last"}, Default: "avg"}}},
	{Type: "join", Group: "logic", Label: "Join", Icon: "recycle", Color: cLogic, Inputs: 1, Outputs: 1,
		Help: "Keeps the latest value of each topic; once all expected topics are present, every message carries them all as msg.<topic>.",
		Settings: []Setting{{Name: "topics", Label: "Topics (comma separated)", Kind: "text", Required: true},
			{Name: "max_age_s", Label: "Forget values older than (s, 0 = never)", Kind: "number", Default: 0, Min: fp(0)}}},
	{Type: "counter", Group: "logic", Label: "Counter", Icon: "activity", Color: cLogic, Inputs: 1, Outputs: 1,
		Help: "Counts messages (or adds an expression); the count becomes the value. Kept across restarts.",
		Settings: []Setting{{Name: "add", Label: "Add (expression)", Kind: "expr", Default: "1"},
			{Name: "reset", Label: "Reset when (expression, empty = never)", Kind: "expr"}}},
	{Type: "latch", Group: "logic", Label: "Latch", Icon: "toggle-right", Color: cLogic, Inputs: 1, Outputs: 1,
		Help: "Remembers on/off: set when one condition is true, reset when the other is; sends the state when it changes. Kept across restarts.",
		Settings: []Setting{{Name: "set", Label: "Set when", Kind: "expr", Default: "value > 80", Required: true},
			{Name: "reset", Label: "Reset when", Kind: "expr", Default: "value < 70", Required: true}}},
	{Type: "time_window", Group: "logic", Label: "Time window", Icon: "calendar", Color: cLogic, Inputs: 1, Outputs: 2, OutNames: []string{"inside", "outside"},
		Help: "Sends messages inside the time window to the first output, the others to the second (organization time zone).",
		Settings: []Setting{{Name: "from", Label: "From (HH:MM)", Kind: "time", Default: "06:00", Required: true},
			{Name: "to", Label: "To (HH:MM)", Kind: "time", Default: "22:00", Required: true},
			{Name: "weekdays", Label: "Weekdays", Kind: "multi", Options: weekdays, Default: weekdays}}},
	{Type: "stable_for", Group: "logic", Label: "True for", Icon: "clock", Color: cLogic, Inputs: 1, Outputs: 2, OutNames: []string{"became true", "became false"}, Breaks: true,
		Help: "Sends one message when the condition has been true for the whole time, and one when it stops being true.",
		Settings: []Setting{{Name: "condition", Label: "Condition", Kind: "expr", Default: "value > 80", Required: true},
			{Name: "seconds", Label: "For (s)", Kind: "number", Default: 120, Min: fp(0), Max: fp(86400), Required: true}}},
	// ---- actions
	{Type: "derive", Group: "action", Label: "Store value", Icon: "database", Color: cAction, Inputs: 1, Outputs: 1,
		Help: "Stores the result as a value of a (derived) datastream.",
		Settings: []Setting{{Name: "datastream", Label: "Datastream", Kind: "datastream", Required: true},
			{Name: "expr", Label: "Value (expression)", Kind: "expr", Default: "value", Required: true}}},
	{Type: "command", Group: "action", Label: "Device command", Icon: "power", Color: cAction, Inputs: 1, Outputs: 1, Commands: true,
		Help: "Sends a command to a device or connector (permissions of the flow's owner, four-eyes policy, audit with the flow as origin).",
		Settings: []Setting{{Name: "device", Label: "Device", Kind: "device", Required: true},
			{Name: "key", Label: "Key (datastream key / register)", Kind: "text", Required: true},
			{Name: "value", Label: "Value (expression)", Kind: "expr", Default: "value", Required: true},
			{Name: "method", Label: "Method", Kind: "text", Default: "write"}}},
	{Type: "attribute", Group: "action", Label: "Device attribute", Icon: "settings", Color: cAction, Inputs: 1, Outputs: 1, Commands: true,
		Help: "Sets a shared attribute of a device (configuration the device receives over MQTT); same governance as commands, audited with the flow as origin.",
		Settings: []Setting{{Name: "device", Label: "Device", Kind: "device", Required: true},
			{Name: "key", Label: "Attribute", Kind: "text", Required: true},
			{Name: "value", Label: "Value (expression)", Kind: "expr", Default: "value", Required: true}}},
	{Type: "entity_action", Group: "action", Label: "Entity action", Icon: "home", Color: cAction, Inputs: 1, Outputs: 1, Commands: true,
		Help: "Controls a smart-home entity (switch, light, blind, thermostat, …) as an audited command with the flow as origin. The value is the brightness %, position %, temperature, mode, percentage or value depending on the action.",
		Settings: []Setting{{Name: "entity", Label: "Entity", Kind: "entity", Required: true},
			{Name: "service", Label: "Action", Kind: "select", Options: []string{"turn_on", "turn_off", "toggle", "open", "close", "stop", "set_position", "lock", "unlock",
				"press", "set_value", "select_option", "set_temperature", "set_hvac_mode", "set_percentage"}, Default: "turn_on", Required: true},
			{Name: "value", Label: "Value (expression, empty = none)", Kind: "expr"}}},
	{Type: "mqtt_out", Group: "action", Label: "MQTT out", Icon: "antenna-bars-5", Color: cAction, Inputs: 1, Outputs: 1, Commands: true,
		Help: "Publishes a message on an MQTT account or bridge (for example a Tasmota or zigbee2mqtt command). Governed like device commands.",
		Settings: []Setting{{Name: "source", Label: "MQTT account or bridge", Kind: "mqtt_source", Required: true},
			{Name: "topic", Label: "Topic (template)", Kind: "template", Required: true},
			{Name: "payload", Label: "Payload (template; empty = the value)", Kind: "template"},
			{Name: "retain", Label: "Retain", Kind: "bool"}}},
	{Type: "notify", Group: "action", Label: "Notify", Icon: "bell", Color: cAction, Inputs: 1, Outputs: 1,
		Help: "Sends a notification through a configured channel (e-mail, SMS, push, voice, webhook).",
		Settings: []Setting{{Name: "channel", Label: "Channel", Kind: "channel", Required: true},
			{Name: "subject", Label: "Subject", Kind: "template", Default: "{{ topic }}: {{ value }}"},
			{Name: "text", Label: "Text", Kind: "template", Default: "{{ topic }} = {{ value }} at {{ fmtTime(ts, 'HH:mm:ss') }}"}}},
	{Type: "incident", Group: "action", Label: "Incident", Icon: "alert-triangle", Color: cAction, Inputs: 1, Outputs: 1,
		Help: "Opens an incident (once until resolved) or resolves it.",
		Settings: []Setting{{Name: "action", Label: "Action", Kind: "select", Options: []string{"open", "resolve"}, Default: "open"},
			{Name: "severity", Label: "Severity", Kind: "select", Options: []string{"minor", "major", "critical"}, Default: "major"},
			{Name: "title", Label: "Title", Kind: "template", Default: "{{ topic }}: {{ value }}", Required: true}}},
	{Type: "http_out", Group: "action", Label: "HTTP request", Icon: "cloud", Color: cAction, Inputs: 1, Outputs: 1,
		Help: "Calls a URL on a host of the organization's allow-list (Flows → Settings); the response status and body go to msg.status and msg.body.",
		Settings: []Setting{{Name: "method", Label: "Method", Kind: "select", Options: []string{"POST", "PUT", "GET"}, Default: "POST"},
			{Name: "url", Label: "URL", Kind: "url", Required: true},
			{Name: "body", Label: "Body (template; empty = the message as JSON)", Kind: "template"}}},
	{Type: "debug", Group: "action", Label: "Debug", Icon: "info-circle", Color: cOther, Inputs: 1, Outputs: 0,
		Help:     "Shows the message (or an expression) in the debug panel.",
		Settings: []Setting{{Name: "expr", Label: "Show (expression, empty = the whole message)", Kind: "expr"}, {Name: "active", Label: "Active", Kind: "bool", Default: true}}},
	// ---- other
	{Type: "comment", Group: "other", Label: "Comment", Icon: "info-circle", Color: cOther,
		Help: "A note on the canvas; does nothing.", Settings: []Setting{{Name: "text", Label: "Text", Kind: "multiline"}}},
}

var catalogIndex = func() map[string]*NodeDef {
	m := map[string]*NodeDef{}
	for i := range Catalog {
		m[Catalog[i].Type] = &Catalog[i]
	}
	return m
}()

// Def returns a node type.
func Def(t string) (*NodeDef, bool) { d, ok := catalogIndex[t]; return d, ok }

// outputsOf returns the number of outputs of a node (switch: one per rule plus otherwise).
func outputsOf(n *NodeSpec) int {
	d, ok := Def(n.Type)
	if !ok {
		return 0
	}
	if n.Type == "switch" {
		if rules, ok := n.Settings["rules"].([]any); ok {
			return len(rules) + 1
		}
	}
	return d.Outputs
}

// Spec is a flow as stored: nodes and wires.
type Spec struct {
	Nodes []NodeSpec `json:"nodes"`
	Wires []Wire     `json:"wires"`
}

// NodeSpec is one node of a flow.
type NodeSpec struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Name     string         `json:"name,omitempty"`
	X        float64        `json:"x"`
	Y        float64        `json:"y"`
	Settings map[string]any `json:"settings"`
	Disabled bool           `json:"disabled,omitempty"`
}

// Wire connects an output port of one node to the input of another.
type Wire struct {
	From string `json:"from"`
	Port int    `json:"port"`
	To   string `json:"to"`
}

func (w Wire) String() string { return fmt.Sprintf("%s:%d → %s", w.From, w.Port, w.To) }
