// modbussim is a small Modbus TCP server for the bench and for e2e tests. Unit id 1; addresses are 0-based.
//
//	go run ./tools/modbussim -port 5020
//
// Holding registers: 0 temperature ×10 (int16, slowly varying), 1 setpoint ×10 (int16, writable, default 250),
// 2-3 flow (float32, big-endian words), 4 counter (uint16, increments), 5 state (int16: 0 stopped, 1 running,
// 2 fault), 10 mode (uint16, writable). Input registers: 0 sinus ×100 (int16). Coils: 0 pump (writable),
// 1 alarm (mirrors temperature > 30 °C). Discrete inputs: 0 door (toggles every 20 s).
package main

import (
	"flag"
	"log"
	"math"
	"os"
	"os/signal"
	"sync"
	"time"

	mb "github.com/simonvetter/modbus"
)

type sim struct {
	mu       sync.Mutex
	holding  [128]uint16
	coils    [16]bool
	start    time.Time
	verbose  bool
	lastTick time.Time
}

func (s *sim) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := time.Since(s.start).Seconds()
	temp := 25 + 6*math.Sin(t/60)
	s.holding[0] = uint16(int16(math.Round(temp * 10)))
	flow := math.Float32bits(float32(12.5 + 2*math.Sin(t/30)))
	s.holding[2], s.holding[3] = uint16(flow>>16), uint16(flow&0xffff)
	s.holding[4]++
	s.holding[5] = uint16(int16(int(t/45) % 3))
	s.coils[1] = temp > 30
}

func (s *sim) HandleCoils(req *mb.CoilsRequest) ([]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > len(s.coils) {
		return nil, mb.ErrIllegalDataAddress
	}
	if req.IsWrite {
		for i, v := range req.Args {
			s.coils[int(req.Addr)+i] = v
		}
		if s.verbose {
			log.Printf("write coils %d: %v", req.Addr, req.Args)
		}
		return nil, nil
	}
	return append([]bool(nil), s.coils[req.Addr:int(req.Addr)+int(req.Quantity)]...), nil
}

func (s *sim) HandleDiscreteInputs(req *mb.DiscreteInputsRequest) ([]bool, error) {
	if int(req.Addr)+int(req.Quantity) > 8 {
		return nil, mb.ErrIllegalDataAddress
	}
	out := make([]bool, req.Quantity)
	door := int(time.Since(s.start).Seconds()/20)%2 == 1
	for i := range out {
		if int(req.Addr)+i == 0 {
			out[i] = door
		}
	}
	return out, nil
}

func (s *sim) HandleHoldingRegisters(req *mb.HoldingRegistersRequest) ([]uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > len(s.holding) {
		return nil, mb.ErrIllegalDataAddress
	}
	if req.IsWrite {
		for i, v := range req.Args {
			s.holding[int(req.Addr)+i] = v
		}
		if s.verbose {
			log.Printf("write holding %d: %v", req.Addr, req.Args)
		}
		return nil, nil
	}
	return append([]uint16(nil), s.holding[req.Addr:int(req.Addr)+int(req.Quantity)]...), nil
}

func (s *sim) HandleInputRegisters(req *mb.InputRegistersRequest) ([]uint16, error) {
	if int(req.Addr)+int(req.Quantity) > 16 {
		return nil, mb.ErrIllegalDataAddress
	}
	out := make([]uint16, req.Quantity)
	t := time.Since(s.start).Seconds()
	for i := range out {
		if int(req.Addr)+i == 0 {
			out[i] = uint16(int16(math.Round(100 * math.Sin(t/10))))
		}
	}
	return out, nil
}

func main() {
	port := flag.Int("port", 5020, "TCP port")
	verbose := flag.Bool("v", false, "log writes")
	flag.Parse()
	s := &sim{start: time.Now(), verbose: *verbose}
	s.holding[1] = 250 // setpoint 25.0 °C
	s.holding[10] = 1  // mode
	s.tick()
	srv, err := mb.NewServer(&mb.ServerConfiguration{URL: "tcp://0.0.0.0:" + itoa(*port), Timeout: 60 * time.Second, MaxClients: 16}, s)
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
	log.Printf("modbussim listening on tcp://0.0.0.0:%d (unit 1): holding 0 temp×10, 1 setpoint×10 (rw), 2-3 flow f32, 4 counter, 5 state, 10 mode (rw); input 0 sinus×100; coils 0 pump (rw), 1 alarm; discrete 0 door", *port)
	go func() {
		t := time.NewTicker(time.Second)
		for range t.C {
			s.tick()
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	_ = srv.Stop()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
