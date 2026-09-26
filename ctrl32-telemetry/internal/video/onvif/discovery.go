package onvif

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Device is a camera that answered WS-Discovery.
type Device struct {
	Address  string   `json:"address"`  // device service URL
	Host     string   `json:"host"`     // IP or name
	Name     string   `json:"name"`     // from the onvif://www.onvif.org/name/… scope
	Hardware string   `json:"hardware"` // from …/hardware/…
	Scopes   []string `json:"scopes,omitempty"`
}

// DiscoveryAddr is the WS-Discovery multicast group.
var DiscoveryAddr = "239.255.255.250:3702"

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Probe returns the probe message for video transmitters.
func Probe() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl">` +
		`<e:Header><w:MessageID>uuid:` + uuid() + `</w:MessageID><w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To><w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action></e:Header>` +
		`<e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body></e:Envelope>`)
}

// Discover sends a WS-Discovery probe to the multicast group (and to extra unicast addresses, e.g. a camera in
// another subnet) and collects the answers for the duration of ctx.
func Discover(ctx context.Context, extra ...string) ([]Device, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	targets := append([]string{DiscoveryAddr}, extra...)
	msg := Probe()
	for _, t := range targets {
		if a, err := net.ResolveUDPAddr("udp4", t); err == nil {
			_, _ = conn.WriteToUDP(msg, a)
		}
	}
	found := map[string]Device{}
	buf := make([]byte, 64<<10)
	for {
		dl, ok := ctx.Deadline()
		if !ok {
			dl = time.Now().Add(3 * time.Second)
		}
		_ = conn.SetReadDeadline(dl)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		for _, d := range parseProbeMatches(buf[:n]) {
			found[d.Address] = d
		}
	}
	out := make([]Device, 0, len(found))
	for _, d := range found {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out, nil
}

func parseProbeMatches(b []byte) []Device {
	var env struct {
		Matches []struct {
			XAddrs string `xml:"XAddrs"`
			Scopes string `xml:"Scopes"`
		} `xml:"Body>ProbeMatches>ProbeMatch"`
	}
	if xml.Unmarshal(b, &env) != nil {
		return nil
	}
	var out []Device
	for _, m := range env.Matches {
		addrs := strings.Fields(m.XAddrs)
		if len(addrs) == 0 {
			continue
		}
		addr := addrs[0]
		for _, a := range addrs { // prefer an IPv4 address
			if u, err := url.Parse(a); err == nil && net.ParseIP(u.Hostname()).To4() != nil {
				addr = a
				break
			}
		}
		d := Device{Address: addr, Scopes: strings.Fields(m.Scopes)}
		if u, err := url.Parse(addr); err == nil {
			d.Host = u.Hostname()
		}
		for _, s := range d.Scopes {
			if v, ok := strings.CutPrefix(s, "onvif://www.onvif.org/name/"); ok {
				d.Name, _ = url.PathUnescape(v)
			}
			if v, ok := strings.CutPrefix(s, "onvif://www.onvif.org/hardware/"); ok {
				d.Hardware, _ = url.PathUnescape(v)
			}
		}
		out = append(out, d)
	}
	return out
}
