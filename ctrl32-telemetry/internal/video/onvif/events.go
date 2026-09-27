package onvif

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Event is one notification of the camera, classified.
type Event struct {
	Kind   string            `json:"kind"`   // motion, tamper, line, intrusion, input, other
	Topic  string            `json:"topic"`  // e.g. tns1:RuleEngine/CellMotionDetector/Motion
	Active bool              `json:"active"` // state events: true while it lasts
	Pulse  bool              `json:"pulse"`  // an instant (line crossed): no end follows
	Time   time.Time         `json:"time"`
	Source map[string]string `json:"source,omitempty"`
	Data   map[string]string `json:"data,omitempty"`
	Init   bool              `json:"-"` // the initial state reported right after subscribing
}

// Classify maps an ONVIF topic and its data items to an event kind. Unknown topics are "other".
func Classify(topic string, data map[string]string) (kind string, active, pulse bool) {
	t := strings.ToLower(topic)
	val := func(names ...string) (bool, bool) {
		for _, n := range names {
			if v, ok := data[n]; ok {
				v = strings.ToLower(strings.TrimSpace(v))
				return v == "true" || v == "1" || v == "active", true
			}
		}
		for _, v := range data { // single-item events with a vendor name
			v = strings.ToLower(strings.TrimSpace(v))
			if v == "true" || v == "false" || v == "1" || v == "0" {
				return v == "true" || v == "1", true
			}
		}
		return false, false
	}
	switch {
	case strings.Contains(t, "motion"):
		a, _ := val("IsMotion", "State", "Motion")
		return "motion", a, false
	case strings.Contains(t, "tamper") || strings.Contains(t, "globalscenechange") || strings.Contains(t, "imagetoodark") || strings.Contains(t, "imagetooblurry"):
		a, _ := val("IsTamper", "State", "IsGlobalSceneChange", "IsTooDark", "IsTooBlurry")
		return "tamper", a, false
	case strings.Contains(t, "linedetector") || strings.Contains(t, "crossed") || strings.Contains(t, "linecross"):
		return "line", true, true
	case strings.Contains(t, "fielddetector") || strings.Contains(t, "intrusion") || strings.Contains(t, "objectsinside"):
		a, ok := val("IsInside", "State")
		if !ok {
			return "intrusion", true, true
		}
		return "intrusion", a, false
	case strings.Contains(t, "digitalinput") || strings.Contains(t, "logicalstate") || strings.Contains(t, "/trigger/relay"):
		a, _ := val("LogicalState", "State")
		return "input", a, false
	}
	a, ok := val("State", "Value")
	return "other", a || !ok, !ok
}

// Subscribe opens a pull-point subscription and delivers events until ctx ends or the camera fails; the
// subscription is renewed on the way and removed at the end.
func (c *Client) Subscribe(ctx context.Context, deliver func(Event)) error {
	caps, err := c.Capabilities(ctx)
	if err != nil {
		return err
	}
	if caps.Events == "" {
		return errors.New("the camera offers no events")
	}
	var r struct {
		Addr string `xml:"CreatePullPointSubscriptionResponse>SubscriptionReference>Address"`
	}
	if err := c.call(ctx, caps.Events, nsEvents+"/EventPortType/CreatePullPointSubscriptionRequest",
		`<tev:CreatePullPointSubscription><tev:InitialTerminationTime>PT120S</tev:InitialTerminationTime></tev:CreatePullPointSubscription>`, &r, "", true); err != nil {
		return err
	}
	sub := c.fix(strings.TrimSpace(r.Addr))
	if sub == "" {
		return errors.New("the camera returned no subscription address")
	}
	wsa := func(action string) string {
		return `<wsa:Action>` + action + `</wsa:Action><wsa:To>` + esc(sub) + `</wsa:To>`
	}
	defer func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.call(ctx2, sub, "", `<wsnt:Unsubscribe/>`, nil, wsa("http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/UnsubscribeRequest"), true)
	}()
	renewed := time.Now()
	for ctx.Err() == nil {
		if time.Since(renewed) > 60*time.Second {
			if err := c.call(ctx, sub, "", `<wsnt:Renew><wsnt:TerminationTime>PT120S</wsnt:TerminationTime></wsnt:Renew>`, nil,
				wsa("http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/RenewRequest"), true); err != nil {
				return err
			}
			renewed = time.Now()
		}
		var m struct {
			Msgs []struct {
				Topic string `xml:"Topic"`
				Msg   struct {
					UTC    string `xml:"UtcTime,attr"`
					Op     string `xml:"PropertyOperation,attr"`
					Source []item `xml:"Source>SimpleItem"`
					Data   []item `xml:"Data>SimpleItem"`
				} `xml:"Message>Message"`
			} `xml:"PullMessagesResponse>NotificationMessage"`
		}
		err := c.call(ctx, sub, "", `<tev:PullMessages><tev:Timeout>PT5S</tev:Timeout><tev:MessageLimit>50</tev:MessageLimit></tev:PullMessages>`, &m,
			wsa(nsEvents+"/PullPointSubscription/PullMessagesRequest"), true)
		if err != nil {
			return err
		}
		for _, n := range m.Msgs {
			data, src := items(n.Msg.Data), items(n.Msg.Source)
			topic := strings.TrimSpace(n.Topic)
			kind, active, pulse := Classify(topic, data)
			t, err := time.Parse(time.RFC3339Nano, n.Msg.UTC)
			if err != nil {
				t = time.Now()
			} else {
				c.mu.Lock()
				t = t.Add(-c.offset) // to server time
				c.mu.Unlock()
			}
			deliver(Event{Kind: kind, Topic: topic, Active: active, Pulse: pulse, Time: t, Source: src, Data: data, Init: n.Msg.Op == "Initialized"})
		}
	}
	return ctx.Err()
}

type item struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

func items(list []item) map[string]string {
	if len(list) == 0 {
		return nil
	}
	m := make(map[string]string, len(list))
	for _, it := range list {
		m[it.Name] = it.Value
	}
	return m
}
