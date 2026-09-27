package main

// licgen batch — the licensor's half of the online activation (like `visu_lic batch` of the ctrl32 editor). The licence
// worker (lic.ctrl32.com) hands out 30-day licenses signed with its ONLINE key; once an order is past the 14-day refund
// period it queues {order, host, licensee, email, features} at GET /telemetry/queue. This command pulls that queue,
// signs a PERPETUAL license for each entry with the MASTER key (it never leaves this PC), verifies every token, archives
// it and POSTs them to /telemetry/minted; the server receives its perpetual license on its next (automatic) activation.
//
//	licgen batch                                   dry run: lists what would be minted
//	licgen batch -apply                            sign, archive, push
//	licgen batch -queue queue.json -o out          offline: read the queue from a file, write the tokens to out/
//
// The admin token of the worker is read from %CTRL32_KEYS%\worker-token.txt (the file `visu_lic batch` uses) unless
// -token-file is given.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/license"
)

// queueItem is one entry of GET /telemetry/queue.
type queueItem struct {
	Order    string   `json:"order"`
	Host     string   `json:"host"`
	Licensee string   `json:"licensee"`
	Email    string   `json:"email"`
	Features []string `json:"features"`
	ServerID string   `json:"server_id,omitempty"`
}

// mintedItem is one entry of POST /telemetry/minted.
type mintedItem struct {
	Order   string `json:"order"`
	Host    string `json:"host"`
	License string `json:"license"`
}

var hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$|^[0-9.]+$|^[0-9a-f:]+$`)

func defaultMasterKey() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ctrl32-license", "telemetry-digital-2026.key")
}

func defaultTokenFile() string {
	if d := os.Getenv("CTRL32_KEYS"); d != "" {
		return filepath.Join(d, "worker-token.txt")
	}
	return ""
}

func batch(args []string) error {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	base := fs.String("url", "https://lic.ctrl32.com", "licence worker")
	tokenFile := fs.String("token-file", defaultTokenFile(), "file with the worker's ADMIN_TOKEN (default %CTRL32_KEYS%\\worker-token.txt)")
	keyFile := fs.String("key", defaultMasterKey(), "MASTER private key (licgen keygen)")
	queueFile := fs.String("queue", "", "read the queue from this JSON file instead of GET /telemetry/queue")
	outDir := fs.String("o", "", "write the signed tokens and minted.json to this directory instead of pushing them")
	archive := fs.String("archive", "", "archive directory (default <key dir>/issued-telemetry)")
	apply := fs.Bool("apply", false, "sign, archive and push to /telemetry/minted (without it: a dry run that only lists the queue)")
	_ = fs.Parse(args)
	return runBatch(batchOpts{base: strings.TrimRight(*base, "/"), tokenFile: *tokenFile, keyFile: *keyFile, queueFile: *queueFile,
		outDir: *outDir, archive: *archive, apply: *apply, out: os.Stdout, client: &http.Client{Timeout: 30 * time.Second}, now: time.Now()})
}

type batchOpts struct {
	base, tokenFile, keyFile, queueFile, outDir, archive string
	apply                                                bool
	out                                                  io.Writer
	client                                               *http.Client
	now                                                  time.Time
}

func (o batchOpts) token() (string, error) {
	if o.tokenFile == "" {
		return "", errors.New("no admin token: set CTRL32_KEYS or pass -token-file")
	}
	b, err := os.ReadFile(o.tokenFile)
	if err != nil {
		return "", fmt.Errorf("admin token: %w", err)
	}
	t := strings.TrimSpace(string(b))
	if t == "" {
		return "", fmt.Errorf("admin token file %s is empty", o.tokenFile)
	}
	return t, nil
}

func (o batchOpts) call(method, path, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, o.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the worker at %s: %w", o.base, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("the worker rejected the admin token (401)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s: HTTP %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}

func runBatch(o batchOpts) error {
	var token string
	var err error
	if o.queueFile == "" || (o.apply && o.outDir == "") {
		if token, err = o.token(); err != nil {
			return err
		}
	}
	var raw []byte
	if o.queueFile != "" {
		if raw, err = os.ReadFile(o.queueFile); err != nil {
			return err
		}
	} else if raw, err = o.call(http.MethodGet, "/telemetry/queue", token, nil); err != nil {
		return err
	}
	var queue []queueItem
	if err := json.Unmarshal(raw, &queue); err != nil {
		return fmt.Errorf("queue is not the JSON array of GET /telemetry/queue: %w", err)
	}
	if len(queue) == 0 {
		fmt.Fprintln(o.out, "queue empty — nothing to mint")
		return nil
	}
	fmt.Fprintf(o.out, "queue: %d perpetual license(s) to mint\n", len(queue))
	for _, q := range queue {
		fmt.Fprintf(o.out, "  %s  %s  %s <%s>\n", q.Order, q.Host, q.Licensee, q.Email)
	}
	if !o.apply && o.outDir == "" {
		fmt.Fprintln(o.out, "dry run — add -apply to sign with the master key and push, or -o <dir> to write the tokens")
		return nil
	}

	priv, err := readPrivate(o.keyFile)
	if err != nil {
		return err
	}
	pub := priv.Public().(ed25519.PublicKey)
	for id, k := range license.TrustedKeys() {
		if k.Equal(pub) && license.KeyLimits()[id] > 0 {
			return fmt.Errorf("%s is the ONLINE key %s; perpetual licenses are signed with the master key", o.keyFile, id)
		}
	}
	keys := map[string]ed25519.PublicKey{"master": pub}
	now := o.now.UTC().Truncate(time.Second)
	var minted []mintedItem
	var rows [][]string
	for _, q := range queue {
		q.Host = strings.ToLower(strings.TrimSpace(q.Host))
		if q.Order == "" || len(q.Order) > 128 || !hostRe.MatchString(q.Host) {
			fmt.Fprintf(o.out, "  skipped (malformed entry): %q %q\n", q.Order, q.Host)
			continue
		}
		features := q.Features
		if len(features) == 0 {
			features = []string{license.WhiteLabel}
		}
		licensee := strings.TrimSpace(q.Licensee)
		if licensee == "" {
			licensee = "ctrl32 customer"
		}
		r := make([]byte, 3)
		_, _ = rand.Read(r)
		l := license.License{ID: fmt.Sprintf("L-%s-%x", now.Format("20060102"), r), Licensee: licensee, Email: strings.TrimSpace(q.Email), Issued: now,
			Features: features, Domains: []string{q.Host}, Notes: "order " + q.Order + "; perpetual (ctrl32 licence)"}
		tok, err := license.Sign(l, priv)
		if err != nil {
			return fmt.Errorf("%s: %w", q.Order, err)
		}
		// verify every token as the server will before anything leaves this PC
		v, err := license.ParseLimited(tok, keys, nil)
		if err != nil || !v.Perpetual() || !v.Check(q.Host, now.AddDate(50, 0, 0)).Valid {
			return fmt.Errorf("%s: the minted token does not verify (%v)", q.Order, err)
		}
		minted = append(minted, mintedItem{Order: q.Order, Host: q.Host, License: tok})
		rows = append(rows, []string{now.Format(time.RFC3339), l.ID, q.Order, q.Host, licensee, l.Email, strings.Join(features, " ")})
		fmt.Fprintf(o.out, "  minted %s for %s (%s)\n", l.ID, q.Host, q.Order)
	}
	if len(minted) == 0 {
		return errors.New("nothing valid to mint")
	}
	body, _ := json.MarshalIndent(minted, "", "  ")

	// archive first: a failed push must not lose a signed license
	dir := o.outDir
	if dir == "" {
		dir = o.archive
		if dir == "" {
			dir = filepath.Join(filepath.Dir(o.keyFile), "issued-telemetry")
		}
	}
	if err := archiveMinted(dir, minted, rows, body); err != nil {
		return err
	}
	fmt.Fprintf(o.out, "archived %d token(s) in %s\n", len(minted), dir)
	if o.outDir != "" {
		fmt.Fprintf(o.out, "not pushed; later: POST %s/telemetry/minted with %s\n", o.base, filepath.Join(dir, "minted.json"))
		return nil
	}
	res, err := o.call(http.MethodPost, "/telemetry/minted", token, body)
	if err != nil {
		return fmt.Errorf("push failed (the tokens are archived in %s): %w", dir, err)
	}
	var st struct {
		Stored int `json:"stored"`
	}
	_ = json.Unmarshal(res, &st)
	fmt.Fprintf(o.out, "pushed: the worker stored %d of %d\n", st.Stored, len(minted))
	if st.Stored != len(minted) {
		return fmt.Errorf("the worker stored %d of %d", st.Stored, len(minted))
	}
	return nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func archiveMinted(dir string, minted []mintedItem, rows [][]string, body []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, m := range minted {
		name := unsafeName.ReplaceAllString(m.Order+"_"+m.Host, "_") + ".token"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(m.License+"\n"), 0o600); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "minted.json"), body, 0o600); err != nil {
		return err
	}
	ledger := filepath.Join(dir, "ledger.csv")
	_, statErr := os.Stat(ledger)
	f, err := os.OpenFile(ledger, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if statErr != nil {
		_ = w.Write([]string{"minted", "id", "order", "host", "licensee", "email", "features"})
	}
	_ = w.WriteAll(rows)
	return w.Error()
}
