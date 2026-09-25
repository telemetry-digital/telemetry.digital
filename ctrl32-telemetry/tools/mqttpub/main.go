// mqttpub is a small MQTT client for testing a ctrl32 telemetry server from a shell — the way a foreign device would:
// it connects with the device's external id as username and its token as password, publishes one payload to one
// topic and optionally prints what the server sends back on a topic for a few seconds.
//
//	go run ./tools/mqttpub -addr telemetry.digital:8883 -tls -user esp32-0b87b0 -token … -topic d/esp32-0b87b0/telemetry -payload '{"temp":4.2}'
//	go run ./tools/mqttpub -addr 127.0.0.1:1883 -user dev1 -token … -sub d/dev1/cmd -wait 30
//
// Payload starting with @ is read from a file. Without -topic nothing is published (subscribe only).
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/paho"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:1883", "broker host:port")
	useTLS := flag.Bool("tls", false, "connect with TLS (port 8883 on a public instance)")
	insecure := flag.Bool("insecure", false, "skip certificate verification (development only)")
	user := flag.String("user", "", "device external id (MQTT username; also the client id unless -client is set)")
	client := flag.String("client", "", "MQTT client id (default: the username)")
	token := flag.String("token", os.Getenv("CTRL32_DEVICE_TOKEN"), "device token (MQTT password); env CTRL32_DEVICE_TOKEN")
	topic := flag.String("topic", "", "topic to publish to (empty = do not publish)")
	payload := flag.String("payload", "", "payload; @file reads it from a file")
	qos := flag.Int("qos", 1, "QoS for the publish")
	sub := flag.String("sub", "", "topic to subscribe to and print for -wait seconds")
	wait := flag.Int("wait", 5, "seconds to keep the subscription open")
	flag.Parse()
	if *user == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "-user and -token are required")
		os.Exit(2)
	}
	if *client == "" {
		*client = *user
	}
	data := []byte(*payload)
	if strings.HasPrefix(*payload, "@") {
		b, err := os.ReadFile((*payload)[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "payload file:", err)
			os.Exit(2)
		}
		data = b
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*wait+30)*time.Second)
	defer cancel()
	var conn net.Conn
	var err error
	if *useTLS {
		host, _, _ := net.SplitHostPort(*addr)
		conn, err = tls.Dial("tcp", *addr, &tls.Config{ServerName: host, InsecureSkipVerify: *insecure, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = net.DialTimeout("tcp", *addr, 10*time.Second)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	c := paho.NewClient(paho.ClientConfig{Conn: conn, Router: paho.NewSingleHandlerRouter(func(p *paho.Publish) {
		fmt.Printf("<< %s %s\n", p.Topic, string(p.Payload))
	})})
	ca, err := c.Connect(ctx, &paho.Connect{KeepAlive: 30, ClientID: *client, CleanStart: true, Username: *user, UsernameFlag: true, Password: []byte(*token), PasswordFlag: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	if ca.ReasonCode != 0 {
		fmt.Fprintf(os.Stderr, "connect refused: reason code %d (bad external id or token, or the device is revoked)\n", ca.ReasonCode)
		os.Exit(1)
	}
	fmt.Printf("connected to %s as %s\n", *addr, *user)
	if *sub != "" {
		if _, err := c.Subscribe(ctx, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: *sub, QoS: 1}}}); err != nil {
			fmt.Fprintln(os.Stderr, "subscribe:", err)
			os.Exit(1)
		}
		fmt.Printf("subscribed to %s\n", *sub)
	}
	if *topic != "" {
		if _, err := c.Publish(ctx, &paho.Publish{Topic: *topic, QoS: byte(*qos), Payload: data}); err != nil {
			fmt.Fprintln(os.Stderr, "publish:", err)
			os.Exit(1)
		}
		fmt.Printf(">> %s %s\n", *topic, string(data))
	}
	if *sub != "" {
		time.Sleep(time.Duration(*wait) * time.Second)
	} else {
		time.Sleep(300 * time.Millisecond)
	}
	_ = c.Disconnect(&paho.Disconnect{ReasonCode: 0})
}
