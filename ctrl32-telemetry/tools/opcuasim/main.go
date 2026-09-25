// opcuasim is a small OPC UA server for the bench and for e2e tests: it publishes a few simulated signals under
// namespace "Simulation" (Sinusoid, Random, Counter, Boolean, Temperature) with anonymous access and no security.
//
//	go run ./tools/opcuasim -port 4850
//	nodes: ns=2;s=Sinusoid  ns=2;s=Random  ns=2;s=Counter  ns=2;s=Boolean  ns=2;s=Temperature  ns=2;s=Text
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
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

func main() {
	port := flag.Int("port", 4850, "listen port")
	host := flag.String("host", "0.0.0.0", "listen address")
	flag.Parse()
	hostname, _ := os.Hostname()
	s := server.New(
		server.EnableSecurity("None", ua.MessageSecurityModeNone),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
		server.EndPoint(*host, *port), server.EndPoint("localhost", *port), server.EndPoint("127.0.0.1", *port), server.EndPoint(hostname, *port),
		server.SetLogger(quiet{}),
	)
	rootNS, _ := s.Namespace(0)
	rootObj := rootNS.Objects()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := s.Start(ctx); err != nil {
		log.Fatalf("start: %v", err)
	}
	defer s.Close()
	ns := server.NewNodeNameSpace(s, "Simulation")
	obj := ns.Objects()
	rootObj.AddRef(obj, id.HasComponent, true)
	start := time.Now()
	counter := int32(0)
	add := func(name string, fn func() any) {
		n := server.NewNode(ua.NewStringNodeID(ns.ID(), name), map[ua.AttributeID]*ua.DataValue{
			ua.AttributeIDBrowseName:  server.DataValueFromValue(attrs.BrowseName(name)),
			ua.AttributeIDDisplayName: server.DataValueFromValue(attrs.DisplayName(name, "")),
			ua.AttributeIDNodeClass:   server.DataValueFromValue(uint32(ua.NodeClassVariable)),
			ua.AttributeIDAccessLevel: server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead)),
		}, nil, func() *ua.DataValue { return server.DataValueFromValue(fn()) })
		ns.AddNode(n)
		obj.AddRef(n, id.HasComponent, true)
	}
	add("Sinusoid", func() any { return math.Round(math.Sin(time.Since(start).Seconds()/10)*1000) / 1000 })
	add("Random", func() any { return math.Round(rand.Float64()*10000) / 100 })
	add("Counter", func() any { counter++; return counter })
	add("Boolean", func() any { return time.Since(start).Seconds()/15 >= 1 && int(time.Since(start).Seconds()/15)%2 == 0 })
	add("Temperature", func() any { return float32(21.5 + 2*math.Sin(time.Since(start).Seconds()/60)) })
	add("Text", func() any { return fmt.Sprintf("tick %d", int(time.Since(start).Seconds())) })
	// Setpoint is writable (CurrentRead|CurrentWrite): the e2e suite writes it through the connector command path.
	setpoint := 25.0
	sp := server.NewNode(ua.NewStringNodeID(ns.ID(), "Setpoint"), map[ua.AttributeID]*ua.DataValue{
		ua.AttributeIDBrowseName:  server.DataValueFromValue(attrs.BrowseName("Setpoint")),
		ua.AttributeIDDisplayName: server.DataValueFromValue(attrs.DisplayName("Setpoint", "")),
		ua.AttributeIDNodeClass:   server.DataValueFromValue(uint32(ua.NodeClassVariable)),
		ua.AttributeIDAccessLevel: server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead | ua.AccessLevelTypeCurrentWrite)),
	}, nil, func() *ua.DataValue { return server.DataValueFromValue(setpoint) })
	ns.AddNode(sp)
	obj.AddRef(sp, id.HasComponent, true)
	log.Printf("opcuasim listening on opc.tcp://%s:%d (namespace %d: Sinusoid, Random, Counter, Boolean, Temperature, Text, Setpoint rw) — Ctrl+C to stop", *host, *port, ns.ID())
	<-ctx.Done()
}
