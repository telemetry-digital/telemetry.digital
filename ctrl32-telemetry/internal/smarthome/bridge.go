package smarthome

// Bridges (spec 005 research R5): ctrl32 connects as a client to a foreign broker (for example the Mosquitto of an
// existing zigbee2mqtt or Home Assistant installation), reads the discovery prefix and the entities' topics, and
// publishes commands there. The connection is kept up with backoff; its state is visible on the sources page.

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

type bridge struct {
	m         *Manager
	src       *source
	cm        *autopaho.ConnectionManager
	connected atomic.Bool
	lastErr   atomic.Value // string

	mu   sync.Mutex
	subs map[string]bool
}

// SealPassword encrypts a bridge password for the connector settings.
func (m *Manager) SealPassword(pw string) (string, error) {
	if m.box == nil {
		return "", errors.New("secret key not configured")
	}
	b, err := m.box.Seal([]byte(pw))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (m *Manager) openPassword(sealed string) string {
	if sealed == "" || m.box == nil {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return ""
	}
	pw, err := m.box.Open(b)
	if err != nil {
		return ""
	}
	return string(pw)
}

func (m *Manager) startBridge(ctx context.Context, src *source) *bridge {
	b := &bridge{m: m, src: src, subs: map[string]bool{}}
	b.lastErr.Store("connecting")
	host := str(src.settings, "host")
	port := 1883
	if p, ok := toNum(src.settings["port"]); ok && p > 0 {
		port = int(p)
	}
	useTLS := src.settings["tls"] == true
	scheme := "mqtt"
	if useTLS {
		scheme = "tls"
	}
	u, err := url.Parse(fmt.Sprintf("%s://%s:%d", scheme, host, port))
	if err != nil || host == "" {
		b.lastErr.Store("host is missing or invalid")
		return b
	}
	clientID := str(src.settings, "client_id")
	if clientID == "" {
		clientID = "ctrl32-" + src.id[len(src.id)-12:]
	}
	cfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{u},
		KeepAlive:                     30,
		CleanStartOnInitialConnection: true,
		ConnectRetryDelay:             5 * time.Second,
		ReconnectBackoff: func(n int) time.Duration {
			d := time.Duration(n+1) * 2 * time.Second
			if d > time.Minute {
				d = time.Minute
			}
			return d
		},
		ConnectTimeout:  10 * time.Second,
		ConnectUsername: str(src.settings, "username"),
		ConnectPassword: []byte(m.openPassword(str(src.settings, "password_sealed"))),
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			b.connected.Store(true)
			b.lastErr.Store("")
			b.mu.Lock()
			b.subs = map[string]bool{}
			b.mu.Unlock()
			m.log.Info("bridge connected", "source", src.name, "broker", u.Host)
			go b.subscribe(append(src.topicsToSubscribe(), m.rawFilters(src.id)...))
			go m.publishConfigs(ctx, src)
		},
		OnConnectionDown: func() bool {
			b.connected.Store(false)
			b.lastErr.Store("connection lost")
			m.log.Warn("bridge connection lost", "source", src.name)
			return true
		},
		OnConnectError: func(err error) {
			b.connected.Store(false)
			b.lastErr.Store(err.Error())
		},
		ClientConfig: paho.ClientConfig{
			ClientID: clientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){func(p paho.PublishReceived) (bool, error) {
				src.enqueue(m, p.Packet.Topic, p.Packet.Payload)
				return true, nil
			}},
		},
	}
	if useTLS {
		cfg.TlsCfg = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, InsecureSkipVerify: src.settings["insecure_skip_verify"] == true} //nolint:gosec // opt-in per bridge for self-signed brokers
	}
	cm, err := autopaho.NewConnection(ctx, cfg)
	if err != nil {
		b.lastErr.Store(err.Error())
		return b
	}
	b.cm = cm
	return b
}

// subscribe adds subscriptions for topics not yet subscribed.
func (b *bridge) subscribe(topics []string) {
	if b.cm == nil || !b.connected.Load() {
		return
	}
	b.mu.Lock()
	var opts []paho.SubscribeOptions
	for _, t := range topics {
		if t != "" && !b.subs[t] {
			b.subs[t] = true
			opts = append(opts, paho.SubscribeOptions{Topic: t, QoS: 1})
		}
	}
	b.mu.Unlock()
	for len(opts) > 0 {
		n := min(len(opts), 50)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := b.cm.Subscribe(ctx, &paho.Subscribe{Subscriptions: opts[:n]}); err != nil {
			b.m.log.Warn("bridge subscription failed", "source", b.src.name, "err", err)
			b.mu.Lock()
			for _, o := range opts[:n] {
				delete(b.subs, o.Topic)
			}
			b.mu.Unlock()
		}
		cancel()
		opts = opts[n:]
	}
}

func (b *bridge) publish(ctx context.Context, p Publish) error {
	if b.cm == nil {
		return ErrNoSource
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := b.cm.Publish(pctx, &paho.Publish{Topic: p.Topic, Payload: []byte(p.Payload), Retain: p.Retain, QoS: max(p.QoS, 0)})
	return err
}

// publishRaw publishes a message on the bridge (used by the export of ctrl32 values to Home Assistant).
func (b *bridge) publishRaw(ctx context.Context, topic string, payload []byte, retain bool) error {
	return b.publish(ctx, Publish{Topic: topic, Payload: string(payload), Retain: retain, QoS: 1})
}
