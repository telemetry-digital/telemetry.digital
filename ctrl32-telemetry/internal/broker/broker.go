// Package broker runs the in-process MQTT broker (mochi-mqtt, MIT) with device authentication and
// per-device topic ACLs (protocol v0.1 §1–2, URS A-02..A-04).
package broker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

// Device is the resolved identity of an authenticated MQTT client.
type Device struct {
	ID         string // uuid
	OrgID      string // uuid
	ExternalID string
	Transport  string
}

// Broker wraps the mochi server and the device registry cache.
type Broker struct {
	Server   *mqtt.Server
	pool     *pgxpool.Pool
	log      *slog.Logger
	auth     Authenticator
	clients  sync.Map // client id -> *Device
	accounts sync.Map // client id -> *Account (MQTT accounts with a topic namespace)
	provs    sync.Map // client id -> struct{} (provisioning sessions)
	// OnDeviceState is called when an authenticated device connects or disconnects (live data hook).
	OnDeviceState func(d *Device, connected bool)
	// OnAccountState is called when an MQTT account client connects or disconnects.
	OnAccountState func(a *Account, clientID string, connected bool)
	// OnProvision answers a provisioning request (ThingsBoard format); nil disables MQTT provisioning.
	OnProvision func(ctx context.Context, payload []byte, remote string) []byte
}

// New creates the broker with the configured listeners. It does not start serving.
func New(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (*Broker, error) {
	srv := mqtt.New(&mqtt.Options{InlineClient: true})
	// mochi's own logger is chatty; route it through slog at debug level.
	srv.Log = log.With("component", "mqtt")

	b := &Broker{Server: srv, pool: pool, log: log, auth: dbAuth{pool}}
	if err := srv.AddHook(&authHook{b: b}, nil); err != nil {
		return nil, err
	}

	if cfg.MQTT.Listen != "" {
		if err := srv.AddListener(listeners.NewTCP(listeners.Config{ID: "tcp", Address: cfg.MQTT.Listen})); err != nil {
			return nil, fmt.Errorf("mqtt tcp listener: %w", err)
		}
	}
	if cfg.MQTT.VPNListen != "" {
		// The VPN address exists only once WireGuard is up (possibly after us at boot): keep trying in the background.
		go func() {
			addr := cfg.MQTT.VPNListen
			for i := 0; ; i++ {
				time.Sleep(3 * time.Second)
				l := listeners.NewTCP(listeners.Config{ID: "vpn", Address: addr})
				if err := srv.AddListener(l); err != nil {
					if i%20 == 0 {
						log.Warn("mqtt vpn listener not ready", "addr", addr, "err", err)
					}
					continue
				}
				go l.Serve(srv.EstablishConnection)
				log.Info("mqtt vpn listener started", "addr", addr)
				return
			}
		}()
	}
	if cfg.MQTT.TLSListen != "" {
		certFile, keyFile := cfg.MQTTTLSFiles()
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			// Certificates may appear later (e.g. copied from the reverse proxy after the first ACME issuance);
			// start without the TLS listener rather than crash-looping, and say so loudly.
			log.Error("mqtt tls listener disabled: certificate not loadable; restart the service once the certificate exists",
				"security", true, "cert", certFile, "err", err)
		} else {
			tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
			if err := srv.AddListener(listeners.NewTCP(listeners.Config{ID: "tls", Address: cfg.MQTT.TLSListen, TLSConfig: tlsCfg})); err != nil {
				return nil, fmt.Errorf("mqtt tls listener: %w", err)
			}
		}
	}
	if cfg.MQTT.WSListen != "" {
		if err := srv.AddListener(listeners.NewWebsocket(listeners.Config{ID: "ws", Address: cfg.MQTT.WSListen})); err != nil {
			return nil, fmt.Errorf("mqtt ws listener: %w", err)
		}
	}
	return b, nil
}

// Serve starts the listeners and blocks until ctx is done.
func (b *Broker) Serve(ctx context.Context) error {
	if err := b.Server.Serve(); err != nil {
		return err
	}
	<-ctx.Done()
	return b.Server.Close()
}

// DeviceOf returns the authenticated device for a client id, if any.
func (b *Broker) DeviceOf(clientID string) (*Device, bool) {
	v, ok := b.clients.Load(clientID)
	if !ok {
		return nil, false
	}
	return v.(*Device), true
}

// DisconnectDevice closes all live MQTT connections of a device (e.g. after revocation).
func (b *Broker) DisconnectDevice(deviceID string) {
	b.clients.Range(func(k, v any) bool {
		if d := v.(*Device); d.ID == deviceID {
			if cl, ok := b.Server.Clients.Get(k.(string)); ok {
				b.Server.DisconnectClient(cl, packets.ErrNotAuthorized)
			}
		}
		return true
	})
}

// DeviceIDFromTopic extracts {externalId} from "d/{externalId}/...".
func DeviceIDFromTopic(topic string) (string, string, bool) {
	parts := strings.SplitN(topic, "/", 3)
	if len(parts) != 3 || parts[0] != "d" || parts[1] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// TokenHash is the storage form of a device token (tokens are 32 random bytes, so a plain SHA-256 is adequate).
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// authHook authenticates devices against core.device_credential and enforces topic ownership.
type authHook struct {
	mqtt.HookBase
	b *Broker
}

func (h *authHook) ID() string { return "ctrl32-device-auth" }

func (h *authHook) Provides(bt byte) bool {
	return bytes.Contains([]byte{mqtt.OnConnectAuthenticate, mqtt.OnACLCheck, mqtt.OnDisconnect, mqtt.OnPublish, mqtt.OnSubscribe,
		mqtt.OnUnsubscribe, mqtt.OnPacketEncode, mqtt.OnWill}, []byte{bt})
}

func (h *authHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	if cl.Net.Inline {
		return true
	}
	username := string(pk.Connect.Username)
	password := string(pk.Connect.Password)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch {
	case username == ProvisionUser:
		if h.b.OnProvision == nil {
			h.b.log.Warn("mqtt provisioning refused: not enabled", "security", true, "client", cl.ID, "remote", cl.Net.Remote)
			return false
		}
		h.b.provs.Store(cl.ID, struct{}{})
		// a provisioning session is short: close it after a minute whatever happens
		time.AfterFunc(time.Minute, func() {
			if _, ok := h.b.provs.Load(cl.ID); ok {
				h.b.Server.DisconnectClient(cl, packets.ErrMaxConnectTime)
			}
		})
		return true
	case strings.HasPrefix(username, AccountPrefix):
		a, err := h.b.auth.Account(ctx, username, password)
		if err != nil {
			h.b.log.Warn("mqtt account auth rejected", "security", true, "client", cl.ID, "username", username, "remote", cl.Net.Remote)
			return false
		}
		h.b.accounts.Store(cl.ID, a)
		h.b.log.Info("mqtt account connected", "account", username, "client", cl.ID, "remote", cl.Net.Remote)
		if h.b.OnAccountState != nil {
			h.b.OnAccountState(a, cl.ID, true)
		}
		return true
	}
	if username == "" {
		h.b.log.Warn("mqtt auth rejected: missing credentials", "security", true, "client", cl.ID, "remote", cl.Net.Remote)
		return false
	}
	var d *Device
	var err error
	if password == "" {
		d, err = h.b.auth.DeviceByToken(ctx, username) // ThingsBoard style: the token is the user name
	} else {
		d, err = h.b.auth.Device(ctx, username, password)
	}
	if err != nil {
		h.b.log.Warn("mqtt auth rejected", "security", true, "client", cl.ID, "username", safeName(username, password), "remote", cl.Net.Remote)
		return false
	}
	h.b.clients.Store(cl.ID, d)
	h.b.log.Info("mqtt device connected", "device", d.ExternalID, "client", cl.ID, "remote", cl.Net.Remote)
	if h.b.OnDeviceState != nil {
		h.b.OnDeviceState(d, true)
	}
	return true
}

// safeName keeps tokens out of the log when the token is the user name.
func safeName(username, password string) string {
	if password == "" && len(username) > 8 {
		return username[:4] + "…(token)"
	}
	return username
}

// OnACLCheck: a device may only use topics under d/{its external id}/ and only in the documented direction; an MQTT
// account may use any topic inside its namespace; a provisioning session only the provisioning topics.
// Subscription filters arrive here already rewritten by OnSubscribe; publish topics before OnPublish rewrites them.
func (h *authHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	if cl.Net.Inline {
		return true
	}
	if _, ok := h.b.provs.Load(cl.ID); ok {
		if write {
			return topic == "/provision/request" || topic == "provision/request"
		}
		return topic == "/provision/response" || topic == "provision/response"
	}
	if a, ok := h.b.AccountOf(cl.ID); ok {
		if write {
			return !strings.HasPrefix(topic, "$")
		}
		if rest, ok := strings.CutPrefix(topic, "$share/"); ok {
			if _, f, ok := strings.Cut(rest, "/"); ok {
				topic = f
			}
		}
		return strings.HasPrefix(topic, a.Namespace+"/")
	}
	dev, ok := h.b.DeviceOf(cl.ID)
	if !ok {
		return false
	}
	// ThingsBoard-style aliases (write) — remapped by the ingest layer or rewritten into the device's own topics.
	if write && (topic == "v1/devices/me/telemetry" || topic == "v1/devices/me/attributes" ||
		strings.HasPrefix(topic, "v1/devices/me/attributes/request/") || strings.HasPrefix(topic, "v1/devices/me/rpc/response/")) {
		return true
	}
	extID, rest, ok := DeviceIDFromTopic(topic)
	if !ok || extID != dev.ExternalID {
		h.b.log.Warn("mqtt acl denied", "security", true, "device", dev.ExternalID, "topic", topic, "write", write)
		return false
	}
	if write {
		switch rest {
		case "telemetry", "attributes", "attributes/request", "status", "cmd/ack", "console/out", "fota/status", "fota/chunk/req":
			return true
		}
	} else {
		switch rest {
		case "attributes/shared", "attributes/response", "cmd", "console/in", "fota/target", "fota/chunk":
			return true
		}
		if strings.HasPrefix(rest, "tb/") {
			return true // ThingsBoard subscriptions, rewritten from v1/devices/me/…
		}
	}
	h.b.log.Warn("mqtt acl denied", "security", true, "device", dev.ExternalID, "topic", topic, "write", write)
	return false
}

func (h *authHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	h.b.provs.Delete(cl.ID)
	if v, ok := h.b.accounts.LoadAndDelete(cl.ID); ok {
		a := v.(*Account)
		h.b.log.Info("mqtt account disconnected", "account", a.Username, "client", cl.ID, "err", err)
		if h.b.OnAccountState != nil {
			h.b.OnAccountState(a, cl.ID, false)
		}
	}
	if v, ok := h.b.clients.LoadAndDelete(cl.ID); ok {
		d := v.(*Device)
		h.b.log.Info("mqtt device disconnected", "device", d.ExternalID, "client", cl.ID, "err", err)
		if h.b.OnDeviceState != nil {
			h.b.OnDeviceState(d, false)
		}
	}
}
