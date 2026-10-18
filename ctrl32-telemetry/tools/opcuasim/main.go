// opcuasim is a small OPC UA server for the bench and for e2e tests (see internal/opcua/opcuasim): simulated
// signals under namespace "Simulation" and a small plant tree (Plant/Line1, Plant/Line2) for browsing.
//
//	go run ./tools/opcuasim -port 4850
//	go run ./tools/opcuasim -port 4851 -secure     (also Basic256Sha256 Sign/SignAndEncrypt, username login)
//	nodes: ns=2;s=Sinusoid  ns=2;s=Random  ns=2;s=Counter  ns=2;s=Boolean  ns=2;s=Temperature  ns=2;s=Text
//	       ns=2;s=Setpoint (rw)  ns=2;s=Line1.Speed  ns=2;s=Line1.Running (rw)  ns=2;s=Line1.Count  ns=2;s=Line1.Name
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua/opcuasim"
)

func main() {
	port := flag.Int("port", 4850, "listen port")
	host := flag.String("host", "0.0.0.0", "listen address")
	secure := flag.Bool("secure", false, "also offer Basic256Sha256 Sign and SignAndEncrypt and username login")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	sim, err := opcuasim.Start(ctx, opcuasim.Config{Host: *host, Port: *port, Secure: *secure})
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	defer sim.Close()
	sec := ""
	if *secure {
		sec = " + Basic256Sha256 Sign/SignAndEncrypt, server thumbprint " + sim.Thumbprint
	}
	log.Printf("opcuasim listening on opc.tcp://%s:%d (namespace %d: Sinusoid, Random, Counter, Boolean, Temperature, Text, Setpoint rw, Plant/Line1..2)%s — Ctrl+C to stop", *host, *port, sim.Namespace, sec)
	<-ctx.Done()
}
