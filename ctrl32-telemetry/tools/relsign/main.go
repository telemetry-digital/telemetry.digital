// relsign creates the release signing key and signs releases (ADR-0024).
//
//	relsign keygen [-dir ~/.ctrl32-release] [-id release-2026]
//	        writes release-signing.key (private, readable by the current user only) and release-signing.pub; prints the
//	        public key and its fingerprint. Copy the .pub file to internal/release/trusted/<id>.pub and commit it. The
//	        .key file never enters the repository; back it up offline (two copies, e.g. an encrypted USB key in a safe).
//	relsign sign -dir dist/0.76.0 -version 0.76.0 -released 2026-10-04 [-key file[,file]]
//	        writes release.json (every file of the directory with size and SHA-256, analytics/ included) and
//	        release.json.sig (one signature line per key; several keys during a key rotation)
//	relsign verify -dir dist/0.76.0 | -file package.zip [-pub file.pub]
//	        checks the signature (against the keys built into this tool's release package, or -pub) and every file
//	relsign package -dir dist/0.76.0 -out ctrl32-telemetry-0.76.0-offline.zip
//	        the offline package: the signed manifest, the binaries, SHA256SUMS, VERSION, the SBOM and, when present,
//	        the parts of the object counting detector (analytics/)
//
// Tests generate their own key pair (relsign keygen -dir <temporary directory>) and never use the release key.
package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: relsign keygen|sign|verify|package [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "package":
		err = pack(os.Args[2:])
	case "fetch-detector":
		err = fetchDetector(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "relsign:", err)
		os.Exit(1)
	}
}

// DefaultKeyDir is where the release PC keeps the private key: outside every repository.
func defaultKeyDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ".ctrl32-release"
	}
	return filepath.Join(h, ".ctrl32-release")
}

const keyFileName = "release-signing.key"

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	dir := fs.String("dir", defaultKeyDir(), "directory for the key files (outside the repository)")
	id := fs.String("id", "release-"+time.Now().Format("2006"), "key id (the name of the .pub file in internal/release/trusted)")
	_ = fs.Parse(args)
	if abs, err := filepath.Abs(*dir); err == nil && insideRepository(abs) {
		return fmt.Errorf("%s is inside a git repository; the private release key must never be stored there", abs)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	kf := filepath.Join(*dir, keyFileName)
	if _, err := os.Stat(kf); err == nil {
		return fmt.Errorf("%s exists; refusing to overwrite a private key", kf)
	}
	created := time.Now().UTC().Format(time.RFC3339)
	body := "# ctrl32 telemetry release signing key — PRIVATE, never commit, back up offline\n# key-id: " + *id + "\n# created: " + created + "\n" +
		base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"
	if err := os.WriteFile(kf, []byte(body), 0o600); err != nil {
		return err
	}
	if err := restrict(kf); err != nil {
		return fmt.Errorf("the key was written but its permissions could not be restricted: %w", err)
	}
	pubText := "# ctrl32 telemetry release key " + *id + ", created " + created[:10] + "\n" + base64.StdEncoding.EncodeToString(pub) + "\n"
	pf := filepath.Join(*dir, "release-signing.pub")
	if err := os.WriteFile(pf, []byte(pubText), 0o644); err != nil {
		return err
	}
	fmt.Printf("private key: %s (readable by the current user only — back it up offline)\n", kf)
	fmt.Printf("public key:  %s\n", pf)
	fmt.Printf("key id:      %s\n", *id)
	fmt.Printf("fingerprint: %s\n", release.Fingerprint(pub))
	fmt.Printf("embed it:    copy %s to internal/release/trusted/%s.pub and commit\n", pf, *id)
	return nil
}

// insideRepository reports whether dir (or a parent) contains a .git entry.
func insideRepository(dir string) bool {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		p := filepath.Dir(d)
		if p == d {
			return false
		}
		d = p
	}
}

// restrict leaves the key file readable by its owner only (Windows: no inherited permissions, full control for the
// current user only).
func restrict(path string) error {
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0o600)
	}
	user := os.Getenv("USERNAME")
	if d := os.Getenv("USERDOMAIN"); d != "" && user != "" {
		user = d + `\` + user
	}
	if user == "" {
		return errors.New("USERNAME is not set")
	}
	out, err := exec.Command("icacls", path, "/inheritance:r", "/grant:r", user+":F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// loadKey reads a private key file written by keygen.
func loadKey(path string) (string, ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("release signing key: %w (create it with relsign keygen; it lives outside the repository)", err)
	}
	id, seed := "", ""
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "# key-id:"):
			id = strings.TrimSpace(strings.TrimPrefix(l, "# key-id:"))
		case l == "" || strings.HasPrefix(l, "#"):
		default:
			seed = l
		}
	}
	s, err := base64.StdEncoding.DecodeString(seed)
	if err != nil || len(s) != ed25519.SeedSize || id == "" {
		return "", nil, fmt.Errorf("%s is not a release signing key", path)
	}
	return id, ed25519.NewKeyFromSeed(s), nil
}

// releaseFiles lists the files of a release directory: the top level and analytics/ (not the manifest, its
// signature and the offline packages).
func releaseFiles(dir string) ([]string, error) {
	var out []string
	for _, sub := range []string{"", "analytics"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			if sub != "" && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || n == release.ManifestFile || n == release.SignatureFile || strings.HasSuffix(n, ".zip") && strings.Contains(n, "-offline") {
				continue
			}
			if sub != "" {
				n = sub + "/" + n
			}
			if !release.ValidFileName(n) {
				return nil, fmt.Errorf("file name %q cannot be published", n)
			}
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	dir := fs.String("dir", "", "release directory")
	version := fs.String("version", "", "release version")
	released := fs.String("released", time.Now().UTC().Format(release.Layout), "release date YYYY-MM-DD")
	keys := fs.String("key", filepath.Join(defaultKeyDir(), keyFileName), "private key file(s), comma separated during a key rotation")
	_ = fs.Parse(args)
	if *dir == "" || !release.ValidVersion(*version) {
		return errors.New("-dir and a valid -version are required")
	}
	names, err := releaseFiles(*dir)
	if err != nil {
		return err
	}
	m := release.Manifest{Format: 1, Product: release.Product, Version: *version, Released: *released}
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(*dir, filepath.FromSlash(n)))
		if err != nil {
			return err
		}
		m.Files = append(m.Files, release.File{Name: n, Size: int64(len(b)), SHA256: release.HexSHA256(b)})
	}
	body, err := m.Marshal()
	if err != nil {
		return err
	}
	if _, err := release.ParseManifest(body); err != nil {
		return err
	}
	var sig strings.Builder
	for _, kf := range strings.Split(*keys, ",") {
		id, priv, err := loadKey(strings.TrimSpace(kf))
		if err != nil {
			return err
		}
		line, err := release.Sign(body, id, priv)
		if err != nil {
			return err
		}
		sig.WriteString(line + "\n")
		fmt.Printf("signed with %s (%s)\n", id, release.Fingerprint(priv.Public().(ed25519.PublicKey)))
	}
	if err := os.WriteFile(filepath.Join(*dir, release.ManifestFile), body, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*dir, release.SignatureFile), []byte(sig.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %d files, version %s, released %s\n", filepath.Join(*dir, release.ManifestFile), len(m.Files), m.Version, m.Released)
	return nil
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	dir := fs.String("dir", "", "release directory")
	file := fs.String("file", "", "offline package (.zip)")
	pub := fs.String("pub", "", "public key file to verify with (default: the keys built into this tool)")
	_ = fs.Parse(args)
	keys := release.TrustedKeys()
	if *pub != "" {
		b, err := os.ReadFile(*pub)
		if err != nil {
			return err
		}
		k, err := release.ParseKeyFile(b)
		if err != nil {
			return err
		}
		keys = map[string]ed25519.PublicKey{}
		id, _, _ := loadPubID(b)
		keys[id] = k
	}
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(*dir, filepath.FromSlash(name))) }
	if *file != "" {
		zr, err := zip.OpenReader(*file)
		if err != nil {
			return err
		}
		defer zr.Close()
		read = func(name string) ([]byte, error) {
			f, err := zr.Open(name)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			return io.ReadAll(f)
		}
	} else if *dir == "" {
		return errors.New("-dir or -file is required")
	}
	mb, err := read(release.ManifestFile)
	if err != nil {
		return err
	}
	sb, err := read(release.SignatureFile)
	if err != nil {
		return err
	}
	keyID, err := release.VerifySignature(mb, sb, keys)
	if err != nil {
		return err
	}
	m, err := release.ParseManifest(mb)
	if err != nil {
		return err
	}
	present := 0
	for _, f := range m.Files {
		b, err := read(f.Name)
		if err != nil {
			continue // a package may carry a part of the release
		}
		if int64(len(b)) != f.Size || release.HexSHA256(b) != f.SHA256 {
			return fmt.Errorf("%s does not match the manifest", f.Name)
		}
		present++
	}
	fmt.Printf("ok: %s %s released %s, signed by %s, %d of %d files present and verified\n", m.Product, m.Version, m.Released, keyID, present, len(m.Files))
	return nil
}

// loadPubID reads the key id from the comment of a .pub file written by keygen ("release key <id>,").
func loadPubID(b []byte) (string, string, error) {
	for _, l := range strings.Split(string(b), "\n") {
		if i := strings.Index(l, "release key "); i >= 0 {
			rest := strings.TrimSpace(l[i+len("release key "):])
			if j := strings.IndexAny(rest, ", "); j > 0 {
				return rest[:j], "", nil
			}
			return rest, "", nil
		}
	}
	return "key", "", nil
}

func pack(args []string) error {
	fs := flag.NewFlagSet("package", flag.ExitOnError)
	dir := fs.String("dir", "", "signed release directory")
	out := fs.String("out", "", "package file (.zip)")
	_ = fs.Parse(args)
	if *dir == "" || *out == "" {
		return errors.New("-dir and -out are required")
	}
	mb, err := os.ReadFile(filepath.Join(*dir, release.ManifestFile))
	if err != nil {
		return fmt.Errorf("sign the release first: %w", err)
	}
	m, err := release.ParseManifest(mb)
	if err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	add := func(name string) error {
		src, err := os.Open(filepath.Join(*dir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		defer src.Close()
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now().UTC()})
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		return err
	}
	names := []string{release.ManifestFile, release.SignatureFile}
	for _, x := range m.Files {
		names = append(names, x.Name)
	}
	for _, n := range names {
		if err := add(n); err != nil {
			zw.Close()
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	st, _ := os.Stat(*out)
	fmt.Printf("%s: %d files, %.1f MB\n", *out, len(names), float64(st.Size())/1e6)
	return nil
}
