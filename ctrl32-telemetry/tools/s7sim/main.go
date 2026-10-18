// s7sim is a small Siemens S7 (S7comm over ISO-on-TCP) PLC simulator for the bench and for e2e tests. Any rack,
// slot and TSAP is accepted.
//
//	go run ./tools/s7sim -port 10102 [-pdu 240] [-put-get-off] [-v]
//
// DB1..DB10 (1 KiB each), M/I/Q (256 bytes). DB1.DBD0 REAL sinusoid, DB1.DBW4 INT counter, DB1.DBX6.0 BOOL
// toggling, DB1.DBD8 REAL setpoint (writable, 21.5), DB1.DBS20.20 STRING "hello", MW10 INT 1234. Other data blocks
// (e.g. DB99) answer "object does not exist".
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7/s7sim"
)

func main() {
	port := flag.Int("port", 102, "TCP port")
	pdu := flag.Int("pdu", 960, "maximum PDU size to negotiate (240..960)")
	putGetOff := flag.Bool("put-get-off", false, "refuse Read/Write Var with error 0x8104 (PUT/GET not permitted)")
	verbose := flag.Bool("v", false, "log writes")
	flag.Parse()
	s := s7sim.New()
	s.PDU, s.PutGetOff, s.Verbose = *pdu, *putGetOff, *verbose
	l, err := net.Listen("tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("s7sim listening on tcp://0.0.0.0:%d (PDU %d, PUT/GET %s): DB1.DBD0 sinus REAL, DB1.DBW4 counter INT, DB1.DBX6.0 toggle, DB1.DBD8 setpoint REAL (rw), DB1.DBS20.20 \"hello\", MW10 1234",
		*port, *pdu, map[bool]string{true: "off", false: "on"}[*putGetOff])
	go func() {
		t := time.NewTicker(time.Second)
		for range t.C {
			s.Tick()
		}
	}()
	go func() {
		if err := s.Serve(l); err != nil {
			log.Fatal(err)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	_ = l.Close()
}
