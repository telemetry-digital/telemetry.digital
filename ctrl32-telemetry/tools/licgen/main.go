// licgen creates the licensor's key pair and issues license tokens (spec 007, spec 026).
//
//	licgen keygen -out ~/ctrl32-keys -name ctrl32-2026          writes ctrl32-2026.key (private, 0600) and
//	                                                           ctrl32-2026.pub (commit it to internal/license/trusted/)
//	licgen issue -key ~/ctrl32-keys/ctrl32-2026.key -licensee "Acme s.r.o." -email it@acme.sk \
//	             -domains scada.acme.sk -notes "order 2026-001"
//	                                                           white_label license without expiry, updates included
//	                                                           for one year from today (-updates-until YYYY-MM-DD or
//	                                                           none); -days N or -expires YYYY-MM-DD set an expiry
//	licgen show -token c32l1.… [-pub file.pub]                 prints the payload (and verifies it with -pub)
//	licgen batch [-apply] [-url https://lic.ctrl32.com]         mints the licenses without expiry the licence worker queued
//	             [-token-file …worker-token.txt] [-key …]     for activated orders past the refund period (batch.go)
//
// Keep the private key offline; whoever has it can issue licenses.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/license"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: licgen keygen|issue|show|batch [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "issue":
		err = issue(os.Args[2:])
	case "show":
		err = show(os.Args[2:])
	case "batch":
		err = batch(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "licgen:", err)
		os.Exit(1)
	}
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", ".", "directory for the key files")
	name := fs.String("name", "ctrl32-"+time.Now().Format("2006"), "key name (file names)")
	_ = fs.Parse(args)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	kf := filepath.Join(*out, *name+".key")
	if _, err := os.Stat(kf); err == nil {
		return fmt.Errorf("%s exists; refusing to overwrite a private key", kf)
	}
	if err := os.WriteFile(kf, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	pf := filepath.Join(*out, *name+".pub")
	p := base64.StdEncoding.EncodeToString(pub)
	if err := os.WriteFile(pf, []byte("# ctrl32 license key "+*name+", created "+time.Now().UTC().Format("2006-01-02")+"\n"+p+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("private key: %s (keep it offline)\npublic key:  %s → copy to internal/license/trusted/%s.pub\n%s\n", kf, pf, *name, p)
	return nil
}

func readPrivate(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s is not a licgen private key", path)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func issue(args []string) error {
	fs := flag.NewFlagSet("issue", flag.ExitOnError)
	key := fs.String("key", "", "private key file (licgen keygen)")
	id := fs.String("id", "", "license id (default L-<date>-<random>)")
	licensee := fs.String("licensee", "", "customer name")
	email := fs.String("email", "", "customer contact")
	features := fs.String("features", license.WhiteLabel, "comma-separated features")
	days := fs.Int("days", 0, "validity in days; 0 = no expiry (the default)")
	expires := fs.String("expires", "", "expiry date YYYY-MM-DD (UTC, end of day) instead of -days; empty = no expiry")
	updates := fs.String("updates-until", "", "last release date YYYY-MM-DD the license includes updates for; default: one year after today; none = no limit")
	domains := fs.String("domains", "", "comma-separated server host names (*.example.com allowed); empty = any")
	notes := fs.String("notes", "", "free text (order number …)")
	_ = fs.Parse(args)
	if *key == "" || *licensee == "" {
		return fmt.Errorf("-key and -licensee are required")
	}
	priv, err := readPrivate(*key)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Second)
	l := license.License{ID: *id, Licensee: *licensee, Email: *email, Issued: now, Features: split(*features), Domains: split(*domains), Notes: *notes}
	if l.ID == "" {
		r := make([]byte, 3)
		_, _ = rand.Read(r)
		l.ID = fmt.Sprintf("L-%s-%x", now.Format("20060102"), r)
	}
	switch {
	case *expires != "" && *days > 0:
		return fmt.Errorf("use -days or -expires, not both")
	case *expires != "":
		d, err := time.Parse("2006-01-02", *expires)
		if err != nil {
			return fmt.Errorf("-expires: %v", err)
		}
		l.Expires = d.Add(24*time.Hour - time.Second)
		if !l.Expires.After(now) {
			return fmt.Errorf("-expires %s is in the past", *expires)
		}
	case *days < 0:
		return fmt.Errorf("-days must be 0 (no expiry) or more")
	case *days > 0:
		l.Expires = now.AddDate(0, 0, *days)
	}
	u, err := updatesUntil(*updates, now)
	if err != nil {
		return err
	}
	l.UpdatesUntil = u
	tok, err := license.Sign(l, priv)
	if err != nil {
		return err
	}
	fmt.Println(tok)
	fmt.Fprintf(os.Stderr, "issued %s for %s: %s, %s\n", l.ID, l.Licensee, strings.Join(l.Features, ", "), validity(&l))
	return nil
}

// UpdateYears is the default period of updates a new license includes.
const UpdateYears = 1

// updatesUntil resolves the -updates-until flag: "" = one year after now, "none" = no limit, else a date.
func updatesUntil(flagValue string, now time.Time) (string, error) {
	switch v := strings.TrimSpace(flagValue); v {
	case "":
		return now.AddDate(UpdateYears, 0, 0).Format(license.DateLayout), nil
	case "none":
		return "", nil
	default:
		if _, err := time.Parse(license.DateLayout, v); err != nil {
			return "", fmt.Errorf("-updates-until: %v", err)
		}
		return v, nil
	}
}

// validity describes the expiry and the included updates for people.
func validity(l *license.License) string {
	v := "no expiry"
	if !l.NoExpiry() {
		v = "valid until " + l.Expires.UTC().Format("2006-01-02 15:04 MST")
	}
	if l.UpdatesUntil != "" {
		return v + ", updates included until " + l.UpdatesUntil
	}
	return v + ", updates without limit"
}

func show(args []string) error {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	tok := fs.String("token", "", "license token")
	pubFile := fs.String("pub", "", "public key file to verify with (default: the keys built into this binary)")
	_ = fs.Parse(args)
	keys, limits := license.TrustedKeys(), license.KeyLimits()
	if *pubFile != "" {
		b, err := os.ReadFile(*pubFile)
		if err != nil {
			return err
		}
		kf, err := license.ParseKeyFile(b)
		if err != nil {
			return fmt.Errorf("%s: %w", *pubFile, err)
		}
		id := filepath.Base(*pubFile)
		keys, limits = map[string]ed25519.PublicKey{id: kf.Key}, map[string]int{id: kf.MaxDays} // honours "# max-days: N"
	}
	l, err := license.ParseLimited(*tok, keys, limits)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	fmt.Printf("%s\n%s\nverified with key %s\n", b, validity(l), l.KeyID)
	return nil
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
