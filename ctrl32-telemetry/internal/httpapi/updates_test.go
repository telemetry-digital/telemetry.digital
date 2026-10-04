package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

func TestMaintenanceWindowTimeZones(t *testing.T) {
	bratislava, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		t.Skip("no time zone database")
	}
	st := updateSettings{Mode: "auto", Weekday: int(time.Sunday), Time: "03:00", TZ: "Europe/Bratislava"}
	// Sunday 4 Oct 2026 03:30 in Bratislava (CEST, UTC+2) = 01:30 UTC
	in, start := inWindow(time.Date(2026, 10, 4, 1, 30, 0, 0, time.UTC), st)
	if !in || !start.Equal(time.Date(2026, 10, 4, 3, 0, 0, 0, bratislava)) {
		t.Fatalf("inside the window: %v %v", in, start)
	}
	// 03:30 UTC is 05:30 in Bratislava: outside the one-hour window
	if in, _ := inWindow(time.Date(2026, 10, 4, 3, 30, 0, 0, time.UTC), st); in {
		t.Fatal("after the window")
	}
	// the same instant with the window in UTC: inside
	stUTC := st
	stUTC.TZ = "UTC"
	if in, _ := inWindow(time.Date(2026, 10, 4, 3, 30, 0, 0, time.UTC), stUTC); !in {
		t.Fatal("UTC window")
	}
	// next window from a Wednesday
	next := nextWindow(time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), st)
	if !next.Equal(time.Date(2026, 10, 11, 3, 0, 0, 0, bratislava)) {
		t.Fatalf("next window %v", next)
	}
	// a window across midnight: Saturday 23:30 + 1 h covers Sunday 00:15
	late := updateSettings{Weekday: int(time.Saturday), Time: "23:30", TZ: "Europe/Bratislava"}
	if in, _ := inWindow(time.Date(2026, 10, 10, 22, 15, 0, 0, time.UTC), late); !in { // Sunday 00:15 local
		t.Fatal("window across midnight")
	}
	// the change to winter time (25 Oct 2026, 03:00 CEST -> 02:00 CET): the window starts at 03:00 CET
	in, start = inWindow(time.Date(2026, 10, 25, 2, 10, 0, 0, time.UTC), st) // 03:10 CET
	if !in || start.UTC().Hour() != 2 {
		t.Fatalf("winter time: %v %v", in, start.UTC())
	}
}

func TestAutoAllowedAndLicence(t *testing.T) {
	cases := []struct {
		cur, av string
		minor   bool
		ok      bool
	}{
		{"0.76.0", "0.76.1", false, true},
		{"0.76.0", "0.77.0", false, false},
		{"0.76.0", "0.77.0", true, true},
		{"0.76.0", "1.0.0", true, false},
		{"0.76.1", "0.76.0", true, false},
		{"0.76.0", "0.76.0", true, false},
	}
	for _, c := range cases {
		if ok, _ := autoAllowed(c.cur, c.av, c.minor); ok != c.ok {
			t.Errorf("%s -> %s (minor %v): %v", c.cur, c.av, c.minor, ok)
		}
	}
	if !licenceCovers("2026-10-04", "") || !licenceCovers("2026-10-04", "2026-10-04") || licenceCovers("2026-10-05", "2026-10-04") {
		t.Fatal("licence coverage")
	}
}

func TestValidateUpdateSettings(t *testing.T) {
	good := updateSettings{Mode: "auto", Weekday: 3, Time: "02:15", TZ: "UTC", RollbackMinutes: 10}
	if err := validateUpdateSettings(good); err != nil {
		t.Fatal(err)
	}
	for why, f := range map[string]func(*updateSettings){
		"mode":     func(s *updateSettings) { s.Mode = "sometimes" },
		"weekday":  func(s *updateSettings) { s.Weekday = 7 },
		"time":     func(s *updateSettings) { s.Time = "25:00" },
		"tz":       func(s *updateSettings) { s.TZ = "Mars/Olympus" },
		"rollback": func(s *updateSettings) { s.RollbackMinutes = 1 },
	} {
		s := good
		f(&s)
		if validateUpdateSettings(s) == nil {
			t.Errorf("%s accepted", why)
		}
	}
}

func TestValidateSMTP(t *testing.T) {
	ok := smtpInput{Host: "smtp.example.com", Security: "starttls", From: "noreply@example.com", User: "u", Password: "p"}
	if _, err := validateSMTP(&ok); err != nil || ok.Port != 587 {
		t.Fatalf("%v %d", err, ok.Port)
	}
	tls := smtpInput{Host: "smtp.example.com", Security: "tls", From: "a@example.com"}
	if _, err := validateSMTP(&tls); err != nil || tls.Port != 465 {
		t.Fatalf("tls default port: %v %d", err, tls.Port)
	}
	for _, h := range []string{"localhost", "127.0.0.1", "192.168.1.10", "10.0.0.5", "mailrelay", "relay.lan", "smtp.corp.internal"} {
		in := smtpInput{Host: h, Port: 25, Security: "none", From: "a@example.com"}
		if warn, err := validateSMTP(&in); err != nil || len(warn) == 0 {
			t.Errorf("unencrypted to %s: %v %v", h, err, warn)
		}
	}
	for why, in := range map[string]smtpInput{
		"public unencrypted": {Host: "smtp.gmail.com", Port: 25, Security: "none", From: "a@example.com"},
		"public ip":          {Host: "8.8.8.8", Port: 25, Security: "none", From: "a@example.com"},
		"security":           {Host: "smtp.example.com", Security: "ssl3", From: "a@example.com"},
		"port":               {Host: "smtp.example.com", Port: 70000, Security: "tls", From: "a@example.com"},
		"from":               {Host: "smtp.example.com", Security: "tls", From: "Name <a@example.com>"},
		"reply-to":           {Host: "smtp.example.com", Security: "tls", From: "a@example.com", ReplyTo: "nope"},
		"recipient":          {Host: "smtp.example.com", Security: "tls", From: "a@example.com", To: []string{"x"}},
		"host":               {Host: "", Security: "tls", From: "a@example.com"},
		"name injection":     {Host: "smtp.example.com", Security: "tls", From: "a@example.com", FromName: "x\r\nBcc: y"},
	} {
		in := in
		if _, err := validateSMTP(&in); err == nil {
			t.Errorf("%s accepted", why)
		}
	}
}

// The SMTP password is stored encrypted with the secret box of security.secret_key and decrypts only with it.
func TestSMTPPasswordEncryption(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	s.cfg.Security.SecretKey = "test-secret-key-0123456789abcdef"
	box, _ := iam.NewSecretBox(s.cfg.Security.SecretKey)
	blob, _ := box.Seal([]byte("s3cret"))
	st := smtpStored{Host: "smtp.example.com", Port: 587, Security: "starttls", User: "u", From: "a@example.com", PasswordEnc: base64.StdEncoding.EncodeToString(blob)}
	if strings.Contains(st.PasswordEnc, "s3cret") {
		t.Fatal("password stored in clear")
	}
	c, err := s.smtpConfig(st)
	if err != nil || c.Password != "s3cret" {
		t.Fatalf("decrypt: %v %q", err, c.Password)
	}
	s.cfg.Security.SecretKey = "another-secret-key-0123456789abc"
	if _, err := s.smtpConfig(st); err == nil {
		t.Fatal("decrypted with another secret key")
	}
}
