// Command smtpsink is a minimal SMTP server for development and tests: it accepts every message without
// authentication or TLS and writes it as an .eml file (scheduled reports, notifications).
//
//	go run ./tools/smtpsink -listen 127.0.0.1:2525 -dir .dev/mail
//
// The server's config.toml then points [smtp] at it: host = "127.0.0.1", port = 2525, from = "reports@example.com".
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var seq atomic.Int64

func main() {
	listen := flag.String("listen", "127.0.0.1:2525", "address to listen on")
	dir := flag.String("dir", "mail", "directory for the received messages")
	flag.Parse()
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("smtpsink on %s, messages in %s", *listen, *dir)
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serve(c, *dir)
	}
}

func serve(c net.Conn, dir string) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Minute))
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	say := func(s string) { _, _ = w.WriteString(s + "\r\n"); _ = w.Flush() }
	say("220 smtpsink ready")
	var from string
	var to []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			say("250-smtpsink")
			say("250-8BITMIME")
			say("250 SIZE 52428800")
		case strings.HasPrefix(cmd, "HELO"):
			say("250 smtpsink")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			from, to = strings.TrimSpace(line[10:]), nil
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			to = append(to, strings.TrimSpace(line[8:]))
			say("250 ok")
		case cmd == "DATA":
			say("354 end with <CRLF>.<CRLF>")
			var b strings.Builder
			fmt.Fprintf(&b, "X-Sink-From: %s\r\nX-Sink-To: %s\r\n", from, strings.Join(to, ", "))
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
				if strings.HasPrefix(l, "..") {
					l = l[1:]
				}
				b.WriteString(l)
			}
			name := filepath.Join(dir, fmt.Sprintf("%s-%04d.eml", time.Now().Format("20060102-150405"), seq.Add(1)))
			if err := os.WriteFile(name, []byte(b.String()), 0o644); err != nil {
				say("451 " + err.Error())
				continue
			}
			log.Printf("message from %s to %v: %s (%d bytes)", from, to, name, b.Len())
			say("250 ok")
		case cmd == "RSET":
			from, to = "", nil
			say("250 ok")
		case cmd == "NOOP":
			say("250 ok")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("502 not implemented")
		}
	}
}
