package replication

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCodes(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := NewCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 14 || c[4] != '-' || c[9] != '-' {
			t.Fatalf("format %q", c)
		}
		n, ok := NormalizeCode(strings.ToLower(strings.ReplaceAll(c, "-", " ")))
		if !ok || n != strings.ReplaceAll(c, "-", "") {
			t.Fatalf("normalise %q → %q %v", c, n, ok)
		}
		if seen[c] {
			t.Fatal("duplicate code")
		}
		seen[c] = true
	}
	for _, bad := range []string{"", "ABCD-EFGH-JKL", "ABCD-EFGH-JKLO", "ABCD-EFGH-JKL1", "ABCD-EFGH-JKLMN"} {
		if _, ok := NormalizeCode(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if !strings.HasPrefix(NewToken(), "c32r_") || len(NewPassword()) != 64 || len(shortID()) != 12 {
		t.Fatal("secret formats")
	}
}

// The tunnel carries bytes both ways between a local connection on the replica and a server behind the primary,
// refuses requests without the upgrade headers, and surfaces a refusal (e.g. revoked token) as TunnelError.
func TestTunnel(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); c.Close() }()
		}
	}()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, "revoked", 401)
			return
		}
		ServeTunnel(w, r, func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", echo.Addr().String())
		}, log)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/replication/tunnel")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("plain GET without token: %d", resp.StatusCode)
	}

	var contacts atomic.Int32
	cl := &TunnelClient{Primary: srv.URL, Token: "good", Log: log, OnContact: func() { contacts.Add(1) }}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cl.Serve(ctx, ln) }()
	for i := 0; i < 3; i++ {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		msg := strings.Repeat("replication-bytes-", 5000)
		go func() { _, _ = io.WriteString(c, msg) }()
		got := make([]byte, len(msg))
		if _, err := io.ReadFull(c, got); err != nil || string(got) != msg {
			t.Fatalf("round trip %d: %v", i, err)
		}
		c.Close()
	}
	if contacts.Load() != 3 {
		t.Errorf("contacts %d", contacts.Load())
	}
	bad := &TunnelClient{Primary: srv.URL, Token: "revoked"}
	if _, err := bad.Dial(ctx); err == nil {
		t.Fatal("revoked token accepted")
	} else if te, ok := err.(*TunnelError); !ok || te.Status != 401 {
		t.Fatalf("error %v", err)
	}
}
