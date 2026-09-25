// devpub simulates a device with the ctrl32 telemetry protocol v0.1 for end-to-end tests.
//
//	go run ./tools/devpub -addr localhost:1883 -user esp32-demo -token <token> -n 20
//
// It publishes one backfilled batch (5 samples, 10 minutes apart, "bf":1), then n live samples one second apart,
// one of which exceeds the demo fridge limit to raise an alarm, then a status heartbeat.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"time"

	"github.com/eclipse/paho.golang/paho"
)

func main() {
	addr := flag.String("addr", "localhost:1883", "broker address")
	user := flag.String("user", "esp32-demo", "device external id")
	token := flag.String("token", os.Getenv("CTRL32_DEVICE_TOKEN"), "device token")
	n := flag.Int("n", 20, "live samples to publish")
	useTLS := flag.Bool("tls", false, "connect with TLS (server name taken from -addr host)")
	flag.Parse()
	if *token == "" {
		log.Fatal("token required (-token or CTRL32_DEVICE_TOKEN)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var conn net.Conn
	var err error
	if *useTLS {
		host, _, _ := net.SplitHostPort(*addr)
		conn, err = tls.Dial("tcp", *addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = net.Dial("tcp", *addr)
	}
	if err != nil {
		log.Fatal(err)
	}
	c := paho.NewClient(paho.ClientConfig{Conn: conn})
	ca, err := c.Connect(ctx, &paho.Connect{KeepAlive: 30, ClientID: *user + "-sim", CleanStart: true,
		Username: *user, UsernameFlag: true, Password: []byte(*token), PasswordFlag: true})
	if err != nil {
		log.Fatal("connect:", err)
	}
	if ca.ReasonCode != 0 {
		log.Fatalf("connect refused: reason %d (%s)", ca.ReasonCode, ca.Properties.ReasonString)
	}
	pub := func(topic string, v any) {
		b, _ := json.Marshal(v)
		if _, err := c.Publish(ctx, &paho.Publish{Topic: topic, QoS: 1, Payload: b}); err != nil {
			log.Fatal("publish:", err)
		}
		fmt.Println("published", topic, string(b))
	}
	base := "d/" + *user + "/"
	now := time.Now().Unix()

	// 1) backfill batch from the device's flash buffer (measured 50..10 minutes ago)
	var batch []map[string]any
	for i := 5; i >= 1; i-- {
		batch = append(batch, map[string]any{"ts": now - int64(i*600), "v": map[string]any{"temp": 4.0 + 0.1*float64(i), "rh": 55.0}, "bf": 1, "seq": 100 + 5 - i})
	}
	pub(base+"telemetry", batch)

	// 2) live samples; sample n/2 goes to 8.4 °C (above the 8.0 action limit), then returns
	for i := 0; i < *n; i++ {
		temp := 4.5 + 0.3*math.Sin(float64(i)/3)
		if i == *n/2 {
			temp = 8.4
		}
		pub(base+"telemetry", map[string]any{"ts": time.Now().Unix(), "v": map[string]any{"temp": math.Round(temp*100) / 100, "rh": 54.0 + float64(i%3)}, "seq": 200 + i})
		time.Sleep(time.Second)
	}
	pub(base+"status", map[string]any{"online": true, "bat": 3.61, "rssi": -71, "up_s": 86400, "fw": "0.1.0-sim", "buf": 0})
	pub(base+"attributes", map[string]any{"fw": "0.1.0-sim", "hw": "esp32-s3-devkitc", "model": "devpub"})

	_ = c.Disconnect(&paho.Disconnect{ReasonCode: 0})
	fmt.Println("done")
}
