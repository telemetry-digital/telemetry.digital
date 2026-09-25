// devcmd simulates a device that handles commands (protocol §3.4), shared attributes (§3.2), the remote console
// (§3.5) and FOTA (§3.6) for end-to-end tests.
//
//	go run ./tools/devcmd -addr localhost:1883 -user <external id> -token <token> -duration 30s
//
// Commands: method "fail_me" -> {"ok":false,"err":"unsupported"}, "noack" -> no answer (expiry test), anything else
// -> {"ok":true,"r":<args>}. Console: every console/in line is echoed back as "echo: <line>". FOTA: a fota/target
// is downloaded over HTTP with the device token, its SHA-256 is verified and the state machine
// downloading -> verifying -> applying -> updated is reported on fota/status.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/paho"
)

func main() {
	addr := flag.String("addr", "localhost:1883", "broker address")
	user := flag.String("user", "esp32-demo", "device external id")
	token := flag.String("token", os.Getenv("CTRL32_DEVICE_TOKEN"), "device token")
	duration := flag.Duration("duration", 30*time.Second, "how long to stay connected")
	flag.Parse()
	if *token == "" {
		log.Fatal("token required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *duration+10*time.Second)
	defer cancel()
	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	base := "d/" + *user + "/"
	var c *paho.Client
	pub := func(topic string, v any) {
		b, _ := json.Marshal(v)
		if _, err := c.Publish(context.Background(), &paho.Publish{Topic: base + topic, QoS: 1, Payload: b}); err != nil {
			log.Println("publish", topic, err)
		} else {
			fmt.Println("published", topic, string(b))
		}
	}
	c = paho.NewClient(paho.ClientConfig{Conn: conn, OnPublishReceived: []func(paho.PublishReceived) (bool, error){
		func(pr paho.PublishReceived) (bool, error) {
			topic := strings.TrimPrefix(pr.Packet.Topic, base)
			fmt.Printf("received %s %s\n", topic, string(pr.Packet.Payload))
			switch topic {
			case "cmd":
				var cmd struct {
					ID string          `json:"id"`
					M  string          `json:"m"`
					A  json.RawMessage `json:"a"`
				}
				if err := json.Unmarshal(pr.Packet.Payload, &cmd); err != nil || cmd.ID == "" {
					return true, nil
				}
				switch cmd.M {
				case "noack":
				case "fail_me":
					pub("cmd/ack", map[string]any{"id": cmd.ID, "ok": false, "err": "unsupported"})
				default:
					pub("cmd/ack", map[string]any{"id": cmd.ID, "ok": true, "r": cmd.A})
				}
			case "console/in":
				var in struct {
					S string `json:"s"`
					L string `json:"l"`
				}
				if json.Unmarshal(pr.Packet.Payload, &in) == nil && in.S != "" {
					pub("console/out", map[string]any{"s": in.S, "t": time.Now().Unix(), "l": "echo: " + in.L})
				}
			case "fota/target":
				if len(pr.Packet.Payload) == 0 {
					fmt.Println("fota target cleared")
					return true, nil
				}
				var t struct {
					FwID   string `json:"fw_id"`
					Ver    string `json:"ver"`
					URL    string `json:"url"`
					SHA256 string `json:"sha256"`
					Size   int64  `json:"size"`
				}
				if json.Unmarshal(pr.Packet.Payload, &t) != nil || t.FwID == "" {
					return true, nil
				}
				go func() {
					pub("fota/status", map[string]any{"fw_id": t.FwID, "st": "downloading", "pct": 0})
					req, _ := http.NewRequest("GET", t.URL, nil)
					req.Header.Set("Authorization", "Bearer "+*token)
					res, err := http.DefaultClient.Do(req)
					if err != nil || res.StatusCode != 200 {
						code := 0
						if res != nil {
							code = res.StatusCode
						}
						pub("fota/status", map[string]any{"fw_id": t.FwID, "st": "failed", "err": fmt.Sprintf("download %d %v", code, err)})
						return
					}
					h := sha256.New()
					n, _ := io.Copy(h, res.Body)
					res.Body.Close()
					pub("fota/status", map[string]any{"fw_id": t.FwID, "st": "downloading", "pct": 100})
					pub("fota/status", map[string]any{"fw_id": t.FwID, "st": "verifying"})
					if hex.EncodeToString(h.Sum(nil)) != t.SHA256 || n != t.Size {
						pub("fota/status", map[string]any{"fw_id": t.FwID, "st": "failed", "err": "sha256 mismatch"})
						return
					}
					pub("fota/status", map[string]any{"fw_id": t.FwID, "st": "applying"})
					time.Sleep(500 * time.Millisecond)
					pub("fota/status", map[string]any{"fw_id": t.FwID, "st": "updated", "ver": t.Ver})
					pub("status", map[string]any{"online": true, "fw": t.Ver})
				}()
			}
			return true, nil
		}}})
	ca, err := c.Connect(ctx, &paho.Connect{KeepAlive: 30, ClientID: *user + "-cmd", CleanStart: true,
		Username: *user, UsernameFlag: true, Password: []byte(*token), PasswordFlag: true})
	if err != nil {
		log.Fatal("connect:", err)
	}
	if ca.ReasonCode != 0 {
		log.Fatalf("connect refused: reason %d", ca.ReasonCode)
	}
	if _, err := c.Subscribe(ctx, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{
		{Topic: base + "cmd", QoS: 1}, {Topic: base + "attributes/shared", QoS: 1}, {Topic: base + "attributes/response", QoS: 1},
		{Topic: base + "console/in", QoS: 1}, {Topic: base + "fota/target", QoS: 1}}}); err != nil {
		log.Fatal("subscribe:", err)
	}
	fmt.Println("subscribed", base+"cmd")
	pub("attributes/request", map[string]any{})
	time.Sleep(*duration)
	_ = c.Disconnect(&paho.Disconnect{ReasonCode: 0})
	fmt.Println("done")
}
