package broker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/eclipse/paho.golang/paho"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

type mqttClient = mqtt.Client

type fakeAuth struct{}

func (fakeAuth) Device(_ context.Context, ext, token string) (*Device, error) {
	if token == "tok-"+ext {
		return &Device{ID: "id-" + ext, OrgID: "org1", ExternalID: ext, Transport: "mqtt"}, nil
	}
	return nil, errors.New("no")
}
func (fakeAuth) DeviceByToken(_ context.Context, token string) (*Device, error) {
	switch token {
	case "TBTOKEN1":
		return &Device{ID: "id-tb1", OrgID: "org1", ExternalID: "tb1", Transport: "mqtt"}, nil
	case "TBTOKEN2":
		return &Device{ID: "id-tb2", OrgID: "org2", ExternalID: "tb2", Transport: "mqtt"}, nil
	}
	return nil, errors.New("no")
}
func (fakeAuth) Account(_ context.Context, user, pass string) (*Account, error) {
	switch {
	case user == "hs_a" && pass == "pa":
		return &Account{ConnectorID: "ca", OrgID: "org1", Username: user, Namespace: "h/aaaaaaaaaaaa"}, nil
	case user == "hs_b" && pass == "pb":
		return &Account{ConnectorID: "cb", OrgID: "org2", Username: user, Namespace: "h/bbbbbbbbbbbb"}, nil
	}
	return nil, errors.New("no")
}

func freePort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func testBroker(t *testing.T) (*Broker, string) {
	t.Helper()
	addr := freePort(t)
	cfg := &config.Config{}
	cfg.MQTT.Listen = addr
	b, err := New(cfg, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	b.SetAuthenticator(fakeAuth{})
	b.OnProvision = func(_ context.Context, payload []byte, _ string) []byte {
		return append([]byte("resp:"), payload...)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go b.Serve(ctx)
	t.Cleanup(cancel)
	time.Sleep(100 * time.Millisecond)
	return b, addr
}

type rec struct {
	mu   sync.Mutex
	msgs []string // topic=payload
}

func (r *rec) add(topic, payload string) {
	r.mu.Lock()
	r.msgs = append(r.msgs, topic+"="+payload)
	r.mu.Unlock()
}
func (r *rec) all() []string { r.mu.Lock(); defer r.mu.Unlock(); return append([]string(nil), r.msgs...) }
func (r *rec) has(s string) bool {
	for _, m := range r.all() {
		if m == s {
			return true
		}
	}
	return false
}

func dial(t *testing.T, addr, id, user, pass string, will *paho.WillMessage) (*paho.Client, *rec, net.Conn, error) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	r := &rec{}
	c := paho.NewClient(paho.ClientConfig{Conn: conn, ClientID: id, OnPublishReceived: []func(paho.PublishReceived) (bool, error){
		func(p paho.PublishReceived) (bool, error) { r.add(p.Packet.Topic, string(p.Packet.Payload)); return true, nil },
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cp := &paho.Connect{ClientID: id, KeepAlive: 30, CleanStart: true, Username: user, UsernameFlag: user != "", Password: []byte(pass), PasswordFlag: pass != "", WillMessage: will}
	ca, err := c.Connect(ctx, cp)
	if err == nil && ca.ReasonCode != 0 {
		err = fmt.Errorf("connack %d", ca.ReasonCode)
	}
	if err != nil {
		conn.Close()
		return nil, nil, nil, err
	}
	t.Cleanup(func() { _ = c.Disconnect(&paho.Disconnect{}); conn.Close() })
	return c, r, conn, nil
}

func sub(t *testing.T, c *paho.Client, filter string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.Subscribe(ctx, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: filter, QoS: 0}}}); err != nil {
		t.Fatalf("subscribe %s: %v", filter, err)
	}
}

func pub(t *testing.T, c *paho.Client, topic, payload string, retain bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.Publish(ctx, &paho.Publish{Topic: topic, Payload: []byte(payload), Retain: retain}); err != nil {
		t.Fatalf("publish %s: %v", topic, err)
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", what)
}

func TestAccountNamespaces(t *testing.T) {
	b, addr := testBroker(t)
	inline := &rec{}
	if err := b.Server.Subscribe("h/+/#", 1, func(_ *mqttClient, _ packets.Subscription, pk packets.Packet) { inline.add(pk.TopicName, string(pk.Payload)) }); err != nil {
		t.Fatal(err)
	}
	a, ra, _, err := dial(t, addr, "a1", "hs_a", "pa", nil)
	if err != nil {
		t.Fatal(err)
	}
	bb, rb, _, err := dial(t, addr, "b1", "hs_b", "pb", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := dial(t, addr, "x", "hs_a", "wrong", nil); err == nil {
		t.Fatal("wrong account password accepted")
	}
	sub(t, a, "zigbee2mqtt/#")
	sub(t, bb, "zigbee2mqtt/#")
	sub(t, bb, "#")
	pub(t, a, "zigbee2mqtt/Hall light", `{"state":"ON"}`, false)
	eventually(t, "account A sees its own message without the prefix", func() bool { return ra.has(`zigbee2mqtt/Hall light={"state":"ON"}`) })
	eventually(t, "server sees it in the namespace", func() bool { return inline.has(`h/aaaaaaaaaaaa/zigbee2mqtt/Hall light={"state":"ON"}`) })
	time.Sleep(150 * time.Millisecond)
	if len(rb.all()) != 0 {
		t.Fatalf("account B received A's message: %v", rb.all())
	}
	// the server publishes a command into A's namespace
	if err := b.Publish("h/aaaaaaaaaaaa/cmnd/tasmota_1/POWER", []byte("ON"), false); err != nil {
		t.Fatal(err)
	}
	sub(t, a, "cmnd/#")
	_ = b.Publish("h/aaaaaaaaaaaa/cmnd/tasmota_1/POWER", []byte("OFF"), false)
	eventually(t, "command delivered to A", func() bool { return ra.has("cmnd/tasmota_1/POWER=OFF") })
	// retained config stays in the namespace
	pub(t, a, "homeassistant/switch/x/config", `{"a":1}`, true)
	c2, r2, _, err := dial(t, addr, "a2", "hs_a", "pa", nil)
	if err != nil {
		t.Fatal(err)
	}
	sub(t, c2, "homeassistant/#")
	eventually(t, "retained message for a second client of A", func() bool { return r2.has(`homeassistant/switch/x/config={"a":1}`) })
	time.Sleep(100 * time.Millisecond)
	for _, m := range rb.all() {
		t.Fatalf("B got %s", m)
	}
	// $SYS is out of reach
	sub(t, a, "$SYS/#")
	time.Sleep(100 * time.Millisecond)
	for _, m := range ra.all() {
		if len(m) > 4 && m[:4] == "$SYS" {
			t.Fatalf("account saw %s", m)
		}
	}
}

func TestAccountWill(t *testing.T) {
	b, addr := testBroker(t)
	inline := &rec{}
	_ = b.Server.Subscribe("h/+/#", 2, func(_ *mqttClient, _ packets.Subscription, pk packets.Packet) { inline.add(pk.TopicName, string(pk.Payload)) })
	_, _, conn, err := dial(t, addr, "tas", "hs_a", "pa", &paho.WillMessage{Topic: "tele/tasmota_1/LWT", Payload: []byte("Offline")})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close() // no DISCONNECT: the will is published
	eventually(t, "will in the namespace", func() bool { return inline.has("h/aaaaaaaaaaaa/tele/tasmota_1/LWT=Offline") })
}

func TestThingsBoardRPCAndProvisioning(t *testing.T) {
	b, addr := testBroker(t)
	inline := &rec{}
	_ = b.Server.Subscribe("d/+/tb/#", 3, func(_ *mqttClient, _ packets.Subscription, pk packets.Packet) { inline.add(pk.TopicName, string(pk.Payload)) })
	d1, r1, _, err := dial(t, addr, "tb1", "TBTOKEN1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	d2, r2, _, err := dial(t, addr, "tb2", "TBTOKEN2", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := dial(t, addr, "tb3", "NOTATOKEN", "", nil); err == nil {
		t.Fatal("unknown token accepted")
	}
	sub(t, d1, "v1/devices/me/rpc/request/+")
	sub(t, d2, "v1/devices/me/rpc/request/+")
	_ = b.Publish("d/tb1/tb/rpc/request/7", []byte(`{"method":"setValue","params":true}`), false)
	eventually(t, "RPC to device 1", func() bool { return r1.has(`v1/devices/me/rpc/request/7={"method":"setValue","params":true}`) })
	time.Sleep(100 * time.Millisecond)
	if len(r2.all()) != 0 {
		t.Fatalf("device 2 received device 1's RPC: %v", r2.all())
	}
	pub(t, d1, "v1/devices/me/rpc/response/7", `{"ok":true}`, false)
	eventually(t, "RPC response reaches the server as the device's topic", func() bool { return inline.has(`d/tb1/tb/rpc/response/7={"ok":true}`) })
	// a device cannot read another device's topics
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if _, err := d2.Subscribe(ctx, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: "d/tb1/#"}}}); err == nil {
		t.Fatal("subscription to another device's topics accepted")
	}
	cancel()
	_ = b.Publish("d/tb1/tb/rpc/request/8", []byte(`x`), false)
	time.Sleep(150 * time.Millisecond)
	for _, m := range r2.all() {
		t.Fatalf("device 2 got %s", m)
	}

	// provisioning: two sessions at once, each gets only its own answer
	p1, rp1, _, err := dial(t, addr, "p1", "provision", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	p2, rp2, _, err := dial(t, addr, "p2", "provision", "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	sub(t, p1, "/provision/response")
	sub(t, p2, "/provision/response")
	pub(t, p1, "/provision/request", `{"deviceName":"one"}`, false)
	pub(t, p2, "/provision/request", `{"deviceName":"two"}`, false)
	eventually(t, "answers", func() bool {
		return rp1.has(`/provision/response=resp:{"deviceName":"one"}`) && rp2.has(`/provision/response=resp:{"deviceName":"two"}`)
	})
	if len(rp1.all()) != 1 || len(rp2.all()) != 1 {
		t.Fatalf("crossed answers: %v %v", rp1.all(), rp2.all())
	}
	// a provisioning session cannot publish elsewhere
	pub(t, p1, "d/tb1/telemetry", `{"x":1}`, false)
	time.Sleep(100 * time.Millisecond)
	for _, m := range inline.all() {
		if m == `d/tb1/telemetry={"x":1}` {
			t.Fatal("provisioning session published telemetry")
		}
	}
}
