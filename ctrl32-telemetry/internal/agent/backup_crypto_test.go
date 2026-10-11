package agent

import (
	"bytes"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 15, 16, 17, 65535, 65536, 65537, 200000} {
		data := make([]byte, n)
		rand.Read(data)
		var enc, dec bytes.Buffer
		if err := encryptStream(bytes.NewReader(data), &enc, "pass word"); err != nil {
			t.Fatal(err)
		}
		if enc.Len()%16 != 0 || enc.Len() < n+16 {
			t.Fatalf("%d bytes: ciphertext length %d", n, enc.Len())
		}
		if err := decryptStream(bytes.NewReader(enc.Bytes()), &dec, "pass word"); err != nil {
			t.Fatalf("%d bytes: %v", n, err)
		}
		if !bytes.Equal(dec.Bytes(), data) {
			t.Fatalf("%d bytes: round trip differs", n)
		}
		var bad bytes.Buffer
		if n > 0 && decryptStream(bytes.NewReader(enc.Bytes()), &bad, "other") == nil && bytes.Equal(bad.Bytes(), data) {
			t.Fatalf("%d bytes: a wrong passphrase must not give the data", n)
		}
	}
	if err := decryptStream(bytes.NewReader([]byte("plain text, not encrypted")), &bytes.Buffer{}, "x"); err == nil {
		t.Fatal("a plain file must be refused")
	}
}

// The bundles stay readable with the standard tool (RESTORE.txt) and old bundles made with openssl stay readable.
func TestEncryptCompatibleWithOpenSSL(t *testing.T) {
	ossl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("openssl not installed")
	}
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	data := bytes.Repeat([]byte("ctrl32 backup "), 9000)
	os.WriteFile(plain, data, 0o600)
	ours := filepath.Join(dir, "ours.enc")
	if err := encryptFile(plain, ours, "s3cret"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "by-openssl")
	c := exec.Command(ossl, "enc", "-d", "-aes-256-cbc", "-pbkdf2", "-iter", "200000", "-in", ours, "-out", out, "-pass", "pass:s3cret")
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("openssl cannot read our file: %v %s", err, b)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, data) {
		t.Fatal("openssl decrypted something else")
	}
	theirs := filepath.Join(dir, "theirs.enc")
	c = exec.Command(ossl, "enc", "-aes-256-cbc", "-pbkdf2", "-iter", "200000", "-salt", "-in", plain, "-out", theirs, "-pass", "pass:s3cret")
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("openssl: %v %s", err, b)
	}
	back := filepath.Join(dir, "back")
	if err := decryptFile(theirs, back, "s3cret"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(back); !bytes.Equal(got, data) {
		t.Fatal("we decrypted openssl's file to something else")
	}
}

func TestGFSKeep(t *testing.T) {
	loc := time.Local
	var times []time.Time
	start := time.Date(2026, 1, 1, 3, 15, 0, 0, loc)
	for d := 0; d < 400; d++ { // one backup a day for 400 days
		times = append(times, start.AddDate(0, 0, d))
	}
	keep := gfsKeep(times, 7, 4, 12)
	n := 0
	for i, k := range keep {
		if k {
			n++
		}
		if i >= len(times)-7 && !k {
			t.Fatalf("the last 7 days must be kept (%s)", times[i])
		}
	}
	// 7 daily, the weeks and months overlap with them partly: at most 7+4+12, at least 12+3
	if n > 23 || n < 15 {
		t.Fatalf("kept %d backups", n)
	}
	// the newest of each of the last 12 months is kept
	for m := 0; m < 12; m++ {
		month := times[len(times)-1].AddDate(0, -m, 0)
		found := false
		for i, k := range keep {
			if k && times[i].Year() == month.Year() && times[i].Month() == month.Month() {
				found = true
			}
		}
		if !found {
			t.Fatalf("no backup kept for %s", month.Format("2006-01"))
		}
	}
	// compatibility: weekly and monthly 0 = the newest N, like before 0.77
	keep = gfsKeep(times, 14, 0, 0)
	n = 0
	for _, k := range keep {
		if k {
			n++
		}
	}
	if n != 14 {
		t.Fatalf("keep 14: %d", n)
	}
	if k := gfsKeep(times[:1], 0, 0, 0); !k[0] {
		t.Fatal("the newest backup is always kept")
	}
}

func TestBackupTime(t *testing.T) {
	tm, ok := backupTime("ctrl32-20261010-230304.tar.gz.enc")
	if !ok || tm.Year() != 2026 || tm.Hour() != 23 || tm.Second() != 4 {
		t.Fatal(tm, ok)
	}
	if _, ok := backupTime("ctrl32-x"); ok {
		t.Fatal("invalid name")
	}
}
