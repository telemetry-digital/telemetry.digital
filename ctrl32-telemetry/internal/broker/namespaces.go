package broker

// Topic rewriting (spec 005, research R1/R2):
//   - MQTT accounts get a mount point: everything they publish or subscribe is moved under their namespace
//     "h/{12 hex}/" and the prefix is removed again on the way out, so two organizations can use the same topics.
//   - ThingsBoard devices subscribe to v1/devices/me/…; those filters become the device's own d/{ext}/tb/… and are
//     rewritten back on the way out, so an RPC or attribute message reaches only its device. Their attribute requests
//     and RPC responses are moved to d/{ext}/tb/… as well.
//   - Provisioning sessions: a request is answered directly on the client's connection, never published.

import (
	"context"
	"strings"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

const tbPrefix = "v1/devices/me/"

// AccountOf returns the MQTT account of a client id, if any.
func (b *Broker) AccountOf(clientID string) (*Account, bool) {
	v, ok := b.accounts.Load(clientID)
	if !ok {
		return nil, false
	}
	return v.(*Account), true
}

// AccountConnected reports whether a client of the account is connected.
func (b *Broker) AccountConnected(connectorID string) bool {
	found := false
	b.accounts.Range(func(_, v any) bool {
		if v.(*Account).ConnectorID == connectorID {
			found = true
			return false
		}
		return true
	})
	return found
}

// DisconnectAccount closes the connections of an MQTT account (after a password change or deletion).
func (b *Broker) DisconnectAccount(connectorID string) {
	b.accounts.Range(func(k, v any) bool {
		if v.(*Account).ConnectorID == connectorID {
			if cl, ok := b.Server.Clients.Get(k.(string)); ok {
				b.Server.DisconnectClient(cl, packets.ErrNotAuthorized)
			}
		}
		return true
	})
}

func mount(ns, filter string) string {
	if rest, ok := strings.CutPrefix(filter, "$share/"); ok {
		if group, f, ok := strings.Cut(rest, "/"); ok {
			return "$share/" + group + "/" + ns + "/" + f
		}
	}
	return ns + "/" + filter
}

func (h *authHook) OnPublish(cl *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	if cl.Net.Inline {
		return pk, nil
	}
	if _, ok := h.b.provs.Load(cl.ID); ok {
		h.provision(cl, pk)
		return pk, packets.ErrRejectPacket
	}
	if a, ok := h.b.AccountOf(cl.ID); ok {
		pk.TopicName = a.Namespace + "/" + pk.TopicName
		return pk, nil
	}
	if d, ok := h.b.DeviceOf(cl.ID); ok {
		if rest, ok := strings.CutPrefix(pk.TopicName, tbPrefix); ok && (strings.HasPrefix(rest, "attributes/request/") || strings.HasPrefix(rest, "rpc/response/")) {
			pk.TopicName = "d/" + d.ExternalID + "/tb/" + rest
		}
	}
	return pk, nil
}

func (h *authHook) rewriteFilters(cl *mqtt.Client, pk packets.Packet) packets.Packet {
	if cl.Net.Inline {
		return pk
	}
	if a, ok := h.b.AccountOf(cl.ID); ok {
		subs := make(packets.Subscriptions, len(pk.Filters))
		for i, f := range pk.Filters {
			f.Filter = mount(a.Namespace, f.Filter)
			subs[i] = f
		}
		pk.Filters = subs
		return pk
	}
	if d, ok := h.b.DeviceOf(cl.ID); ok {
		subs := make(packets.Subscriptions, len(pk.Filters))
		for i, f := range pk.Filters {
			if rest, ok := strings.CutPrefix(f.Filter, tbPrefix); ok {
				f.Filter = "d/" + d.ExternalID + "/tb/" + rest
			}
			subs[i] = f
		}
		pk.Filters = subs
	}
	return pk
}

func (h *authHook) OnSubscribe(cl *mqtt.Client, pk packets.Packet) packets.Packet {
	return h.rewriteFilters(cl, pk)
}

func (h *authHook) OnUnsubscribe(cl *mqtt.Client, pk packets.Packet) packets.Packet {
	return h.rewriteFilters(cl, pk)
}

func (h *authHook) OnPacketEncode(cl *mqtt.Client, pk packets.Packet) packets.Packet {
	if pk.FixedHeader.Type != packets.Publish || cl.Net.Inline || pk.TopicName == "" {
		return pk
	}
	if a, ok := h.b.AccountOf(cl.ID); ok {
		pk.TopicName = strings.TrimPrefix(pk.TopicName, a.Namespace+"/")
		return pk
	}
	if d, ok := h.b.DeviceOf(cl.ID); ok {
		if rest, ok := strings.CutPrefix(pk.TopicName, "d/"+d.ExternalID+"/tb/"); ok {
			pk.TopicName = tbPrefix + rest
		}
	}
	return pk
}

func (h *authHook) OnWill(cl *mqtt.Client, will mqtt.Will) (mqtt.Will, error) {
	if a, ok := h.b.AccountOf(cl.ID); ok && will.TopicName != "" {
		will.TopicName = a.Namespace + "/" + will.TopicName
	}
	return will, nil
}

// provision answers a provisioning request on the requesting connection only.
func (h *authHook) provision(cl *mqtt.Client, pk packets.Packet) {
	payload := append([]byte(nil), pk.Payload...)
	respTopic := "/provision/response"
	if !strings.HasPrefix(pk.TopicName, "/") {
		respTopic = "provision/response"
	}
	remote := cl.Net.Remote
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp := h.b.OnProvision(ctx, payload, remote)
		out := packets.Packet{FixedHeader: packets.FixedHeader{Type: packets.Publish}, TopicName: respTopic, Payload: resp}
		if err := cl.WritePacket(out); err != nil {
			h.b.log.Warn("mqtt provisioning response not delivered", "client", cl.ID, "err", err)
		}
	}()
}
