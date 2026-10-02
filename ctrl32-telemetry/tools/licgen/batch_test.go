package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/license"
)

func TestBatch(t *testing.T) {
	dir := t.TempDir()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	keyFile := filepath.Join(dir, "master.key")
	_ = os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600)
	tokFile := filepath.Join(dir, "worker-token.txt")
	_ = os.WriteFile(tokFile, []byte("admin-secret\n"), 0o600)

	queue := `[{"order":"txn_1","host":"scada.acme.sk","licensee":"Acme s.r.o.","email":"it@acme.sk","features":["white_label"],"paid_at":"2026-09-20T10:00:00Z"},
	           {"order":"txn_2","host":"Bad Host","licensee":"X"}]`
	var pushed []mintedItem
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		switch r.Method + " " + r.URL.Path {
		case "GET /telemetry/queue":
			_, _ = io.WriteString(w, queue)
		case "POST /telemetry/minted":
			_ = json.NewDecoder(r.Body).Decode(&pushed)
			_, _ = io.WriteString(w, `{"stored":1}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	var out bytes.Buffer
	opts := batchOpts{base: srv.URL, tokenFile: tokFile, keyFile: keyFile, archive: filepath.Join(dir, "arch"), out: &out, client: srv.Client(), now: time.Now()}

	// dry run: lists, signs nothing, pushes nothing
	if err := runBatch(opts); err != nil || pushed != nil || !strings.Contains(out.String(), "dry run") {
		t.Fatalf("dry run: %v %s", err, out.String())
	}
	opts.apply = true
	if err := runBatch(opts); err != nil {
		t.Fatalf("apply: %v\n%s", err, out.String())
	}
	if len(pushed) != 1 || pushed[0].Order != "txn_1" || pushed[0].Host != "scada.acme.sk" {
		t.Fatalf("pushed %+v", pushed)
	}
	for _, a := range auth {
		if a != "Bearer admin-secret" {
			t.Fatalf("auth header %q", a)
		}
	}
	l, err := license.ParseLimited(pushed[0].License, map[string]ed25519.PublicKey{"m": pub}, nil)
	if err != nil || !l.NoExpiry() || l.Licensee != "Acme s.r.o." || l.Email != "it@acme.sk" || !l.Has(license.WhiteLabel, "scada.acme.sk", time.Now().AddDate(30, 0, 0)) ||
		l.Has(license.WhiteLabel, "other.sk", time.Now()) || !strings.Contains(l.Notes, "txn_1") {
		t.Fatalf("minted %+v %v", l, err)
	}
	if l.UpdatesUntil != "2027-09-20" {
		t.Fatalf("updates for a year from the payment: %q", l.UpdatesUntil)
	}
	for _, f := range []string{"txn_1_scada.acme.sk.token", "minted.json", "ledger.csv"} {
		if _, err := os.Stat(filepath.Join(dir, "arch", f)); err != nil {
			t.Fatalf("archive: %v", err)
		}
	}
	csv, _ := os.ReadFile(filepath.Join(dir, "arch", "ledger.csv"))
	if !strings.HasPrefix(string(csv), "minted,id,order") || !strings.Contains(string(csv), "txn_1") {
		t.Fatalf("ledger %s", csv)
	}

	// offline: queue file in, tokens out, no token file and no network needed
	qf := filepath.Join(dir, "q.json")
	_ = os.WriteFile(qf, []byte(queue), 0o600)
	pushed = nil
	off := batchOpts{queueFile: qf, outDir: filepath.Join(dir, "out"), keyFile: keyFile, out: &out, client: srv.Client(), now: time.Now()}
	if err := runBatch(off); err != nil || pushed != nil {
		t.Fatalf("offline: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "minted.json")); err != nil {
		t.Fatal(err)
	}

	// no admin token file: a clear error before any network call
	opts.tokenFile = filepath.Join(dir, "missing.txt")
	if err := runBatch(opts); err == nil || !strings.Contains(err.Error(), "admin token") {
		t.Fatalf("missing token file: %v", err)
	}
	queue = "[]"
	opts.tokenFile = tokFile
	out.Reset()
	if err := runBatch(opts); err != nil || !strings.Contains(out.String(), "queue empty") {
		t.Fatalf("empty queue: %v", err)
	}
}

func TestUpdatesUntil(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{"": "2027-10-02", "none": "", "2028-01-31": "2028-01-31"} {
		if got, err := updatesUntil(in, now); err != nil || got != want {
			t.Fatalf("%q: %q %v", in, got, err)
		}
	}
	if _, err := updatesUntil("31.1.2028", now); err == nil {
		t.Fatal("bad date accepted")
	}
	if got := queueUpdatesUntil(queueItem{}, now); got != "2027-10-02" {
		t.Fatalf("no paid_at: %s", got)
	}
	if got := queueUpdatesUntil(queueItem{PaidAt: "2026-09-01", UpdatesUntil: "2027-12-31"}, now); got != "2027-12-31" {
		t.Fatalf("explicit end: %s", got)
	}
}
