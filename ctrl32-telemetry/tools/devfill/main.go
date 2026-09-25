// devfill publishes one batch of backdated samples for a device over MQTT (plain TCP, token auth).
// The dashboards e2e test uses it; on the bench it fills a datastream with a ramp in one go.
//
//	go run ./tools/devfill -addr 127.0.0.1:1883 -user esp32-a -token c32d_... -key t_c -start 20 -step 0.5 -n 12 -every 300 -lat 48.7 -lon 21.2
//
// The last sample is "now", each earlier one is -every seconds and flagged bf:1 (backfill). When -lat/-lon are
// given they ride along in the last sample, so the server records a reported position for the device.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/eclipse/paho.golang/paho"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:1883", "broker address")
	user := flag.String("user", "", "device external id (MQTT username and client id)")
	token := flag.String("token", os.Getenv("CTRL32_DEVICE_TOKEN"), "device token")
	key := flag.String("key", "t_c", "datastream key")
	start := flag.Float64("start", 20, "first value")
	step := flag.Float64("step", 0.5, "increment per sample")
	n := flag.Int("n", 10, "number of samples")
	every := flag.Int("every", 60, "seconds between samples (backdated)")
	lat := flag.Float64("lat", 0, "latitude reported with the last sample (0 = none)")
	lon := flag.Float64("lon", 0, "longitude reported with the last sample (0 = none)")
	flag.Parse()
	if *user == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "-user and -token are required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	c := paho.NewClient(paho.ClientConfig{Conn: conn})
	ca, err := c.Connect(ctx, &paho.Connect{KeepAlive: 30, ClientID: *user, CleanStart: true,
		Username: *user, UsernameFlag: true, Password: []byte(*token), PasswordFlag: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	if ca.ReasonCode != 0 {
		fmt.Fprintf(os.Stderr, "connect refused: reason %d\n", ca.ReasonCode)
		os.Exit(1)
	}
	now := time.Now().Unix()
	samples := make([]map[string]any, 0, *n)
	for i := 0; i < *n; i++ {
		v := map[string]any{*key: *start + float64(i)*(*step)}
		if i == *n-1 && *lat != 0 && *lon != 0 {
			v["lat"], v["lon"] = *lat, *lon
		}
		s := map[string]any{"ts": now - int64((*n-1-i)*(*every)), "v": v}
		if i < *n-1 {
			s["bf"] = 1
		}
		samples = append(samples, s)
	}
	payload, _ := json.Marshal(samples)
	if _, err := c.Publish(ctx, &paho.Publish{Topic: "d/" + *user + "/telemetry", QoS: 1, Payload: payload}); err != nil {
		fmt.Fprintln(os.Stderr, "publish:", err)
		os.Exit(1)
	}
	time.Sleep(300 * time.Millisecond)
	_ = c.Disconnect(&paho.Disconnect{ReasonCode: 0})
	fmt.Printf("published %d samples for %s (%s)\n", *n, *user, *key)
}
