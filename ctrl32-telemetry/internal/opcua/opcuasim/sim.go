// Package opcuasim is a small OPC UA server for the bench and for e2e tests: it publishes a few simulated signals under
// namespace "Simulation" (Sinusoid, Random, Counter, Boolean, Temperature) with anonymous access and no security,
// and a small plant tree (Plant/Line1, Plant/Line2 with Speed, Running, Count, Name) for browsing and for
// connectors that feed several devices from one server. Values change every second and subscribers are notified.
//
//	go run ./tools/opcuasim -port 4850
//	go run ./tools/opcuasim -port 4851 -secure     (also Basic256Sha256 Sign/SignAndEncrypt, username login)
//	nodes: ns=2;s=Sinusoid  ns=2;s=Random  ns=2;s=Counter  ns=2;s=Boolean  ns=2;s=Temperature  ns=2;s=Text
//	       ns=2;s=Setpoint (rw)  ns=2;s=Line1.Speed  ns=2;s=Line1.Running (rw)  ns=2;s=Line1.Count  ns=2;s=Line1.Name
package opcuasim

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"math/big"
	mrand "math/rand/v2"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/server/attrs"
	"github.com/gopcua/opcua/ua"
)

type quiet struct{}

func (quiet) Debug(string, ...any) {}
func (quiet) Info(string, ...any)  {}
func (quiet) Warn(msg string, args ...any) {
	log.Printf("warn: "+msg, args...)
}
func (quiet) Error(msg string, args ...any) {
	log.Printf("error: "+msg, args...)
}

// Config selects the simulator's address and security.
type Config struct {
	Host   string
	Port   int
	Secure bool // also Basic256Sha256 Sign/SignAndEncrypt and username login
}

// Sim is a running simulator.
type Sim struct {
	Namespace  uint16
	Thumbprint string // SHA-1 of the server certificate (Secure only)
	srv        *server.Server
}

// Close stops the server.
func (s *Sim) Close() { _ = s.srv.Close() }

// Start runs the simulator until ctx ends or Close is called.
func Start(ctx context.Context, cfg Config) (*Sim, error) {
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	hostname, _ := os.Hostname()
	opts := []server.Option{
		server.EnableSecurity("None", ua.MessageSecurityModeNone),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
		server.EndPoint(cfg.Host, cfg.Port), server.EndPoint("localhost", cfg.Port), server.EndPoint("127.0.0.1", cfg.Port), server.EndPoint(hostname, cfg.Port),
		server.SetLogger(quiet{}), server.ServerName("ctrl32 OPC UA simulator"), server.ProductName("opcuasim"),
	}
	sim := &Sim{}
	if cfg.Secure {
		cert, key, err := selfSigned(hostname)
		if err != nil {
			return nil, err
		}
		sum := sha1.Sum(cert)
		sim.Thumbprint = hex.EncodeToString(sum[:])
		opts = append(opts, server.Certificate(cert), server.PrivateKey(key),
			server.EnableSecurity("Basic256Sha256", ua.MessageSecurityModeSign),
			server.EnableSecurity("Basic256Sha256", ua.MessageSecurityModeSignAndEncrypt),
			server.EnableAuthMode(ua.UserTokenTypeUserName))
	}
	s := server.New(opts...)
	sim.srv = s
	rootNS, _ := s.Namespace(0)
	rootObj := rootNS.Objects()
	if err := s.Start(ctx); err != nil {
		return nil, err
	}
	ns := server.NewNodeNameSpace(s, "Simulation")
	sim.Namespace = ns.ID()
	obj := ns.Objects()
	rootObj.AddRef(obj, id.HasComponent, true)
	start := time.Now()
	var mu sync.Mutex
	counter := int32(0)
	var dynamic []*ua.NodeID
	variable := func(parent *server.Node, nodeID, name string, dataType uint32, access ua.AccessLevelType, fn func() any) *server.Node {
		n := server.NewNode(ua.NewStringNodeID(ns.ID(), nodeID), map[ua.AttributeID]*ua.DataValue{
			ua.AttributeIDBrowseName:      server.DataValueFromValue(attrs.BrowseName(name)),
			ua.AttributeIDDisplayName:     server.DataValueFromValue(attrs.DisplayName(name, "")),
			ua.AttributeIDNodeClass:       server.DataValueFromValue(uint32(ua.NodeClassVariable)),
			ua.AttributeIDDataType:        server.DataValueFromValue(ua.NewNumericNodeID(0, dataType)),
			ua.AttributeIDAccessLevel:     server.DataValueFromValue(byte(access)),
			ua.AttributeIDUserAccessLevel: server.DataValueFromValue(byte(access)),
		}, nil, func() *ua.DataValue {
			mu.Lock()
			defer mu.Unlock()
			return server.DataValueFromValue(fn())
		})
		ns.AddNode(n)
		parent.AddRef(n, id.HasComponent, true)
		return n
	}
	object := func(parent *server.Node, nodeID, name string) *server.Node {
		n := server.NewNode(ua.NewStringNodeID(ns.ID(), nodeID), map[ua.AttributeID]*ua.DataValue{
			ua.AttributeIDBrowseName:  server.DataValueFromValue(attrs.BrowseName(name)),
			ua.AttributeIDDisplayName: server.DataValueFromValue(attrs.DisplayName(name, "")),
			ua.AttributeIDNodeClass:   server.DataValueFromValue(uint32(ua.NodeClassObject)),
		}, nil, nil)
		ns.AddNode(n)
		parent.AddRef(n, id.Organizes, true)
		return n
	}
	ro := ua.AccessLevelTypeCurrentRead
	rw := ua.AccessLevelTypeCurrentRead | ua.AccessLevelTypeCurrentWrite
	dyn := func(n *server.Node) { dynamic = append(dynamic, n.ID()) }
	dyn(variable(obj, "Sinusoid", "Sinusoid", id.Double, ro, func() any { return math.Round(math.Sin(time.Since(start).Seconds()/10)*1000) / 1000 }))
	dyn(variable(obj, "Random", "Random", id.Double, ro, func() any { return math.Round(mrand.Float64()*10000) / 100 }))
	dyn(variable(obj, "Counter", "Counter", id.Int32, ro, func() any { counter++; return counter }))
	dyn(variable(obj, "Boolean", "Boolean", id.Boolean, ro, func() any {
		return time.Since(start).Seconds()/15 >= 1 && int(time.Since(start).Seconds()/15)%2 == 0
	}))
	dyn(variable(obj, "Temperature", "Temperature", id.Float, ro, func() any { return float32(21.5 + 2*math.Sin(time.Since(start).Seconds()/60)) }))
	dyn(variable(obj, "Text", "Text", id.String, ro, func() any { return fmt.Sprintf("tick %d", int(time.Since(start).Seconds())) }))
	// Setpoint is writable (CurrentRead|CurrentWrite): the e2e suite writes it through the connector command path.
	variable(obj, "Setpoint", "Setpoint", id.Double, rw, func() any { return 25.0 })

	// Plant tree: two lines, each one "device" with a few signals; Running is writable.
	plant := object(obj, "Plant", "Plant")
	for i, line := range []string{"Line1", "Line2"} {
		base := float64(1000 + 500*i)
		l := object(plant, line, line)
		count := uint32(0)
		dyn(variable(l, line+".Speed", "Speed", id.Double, ro, func() any {
			return math.Round((base+50*math.Sin(time.Since(start).Seconds()/7))*10) / 10
		}))
		dyn(variable(l, line+".Count", "Count", id.UInt32, ro, func() any { count++; return count }))
		variable(l, line+".Running", "Running", id.Boolean, rw, func() any { return true })
		variable(l, line+".Name", "Name", id.String, ro, func() any { return "Packaging " + line })
	}

	// Notify subscribers once a second (the server samples on change notifications only).
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for _, n := range dynamic {
					ns.ChangeNotification(n)
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	return sim, nil
}

func selfSigned(host string) ([]byte, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	uri, _ := url.Parse("urn:ctrl32:opcuasim")
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "opcuasim"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment | x509.KeyUsageContentCommitment | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, URIs: []*url.URL{uri}, DNSNames: []string{host, "localhost"}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return der, key, nil
}
