package offsite

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// The example "GET Object" of the AWS documentation (Signature Version 4, header-based authentication).
func TestSignAWSExample(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	now, _ := time.Parse("20060102T150405Z", "20130524T000000Z")
	sign(req, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", emptySHA256, now)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("authorization\n got %s\nwant %s", got, want)
	}
}

func TestValidName(t *testing.T) {
	for _, n := range []string{"ctrl32-20261010-230304.tar.gz", "ctrl32-20261010-230304.tar.gz.enc", "ctrl32-20261010-230304.tar.gz.enc.sha256"} {
		if !ValidName(n) {
			t.Errorf("%s should be valid", n)
		}
	}
	for _, n := range []string{"../ctrl32-20261010-230304.tar.gz", "ctrl32-x.tar.gz", "passwd", "ctrl32-20261010-230304.tar.gz/../x", ".ctrl32-20261010-230304.tar.gz.part"} {
		if ValidName(n) {
			t.Errorf("%s should be invalid", n)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []Config{
		{Kind: "folder", Path: "relative/dir"},
		{Kind: "sftp", Host: "nas", User: "u"},
		{Kind: "sftp", Host: "nas", User: "u", Password: "p", HostKey: "MD5:aa"},
		{Kind: "s3", Endpoint: "http://s3.example.com", Bucket: "b-1", AccessKey: "a", SecretKey: "s"},
		{Kind: "s3", Endpoint: "https://s3.example.com/path", Bucket: "b-1", AccessKey: "a", SecretKey: "s"},
		{Kind: "s3", Endpoint: "https://s3.example.com", Bucket: "B", AccessKey: "a", SecretKey: "s"},
		{Kind: "s3", Endpoint: "https://s3.example.com", Bucket: "bucket", AccessKey: "a", SecretKey: "s", Path: "../x"},
		{Kind: "ftp"},
	}
	for i, c := range bad {
		if c.Validate() == nil {
			t.Errorf("config %d should be invalid: %+v", i, c)
		}
	}
	good := []Config{
		{Kind: "folder", Path: "/mnt/backup"},
		{Kind: "folder", Path: `\\nas\backup`},
		{Kind: "folder", Path: `D:\backup`},
		{Kind: "sftp", Host: "nas.local", User: "backup", Password: "p", HostKey: "SHA256:abc"},
		{Kind: "s3", Endpoint: "https://s3.eu-central-1.amazonaws.com", Region: "eu-central-1", Bucket: "acme-backup", AccessKey: "a", SecretKey: "s", Path: "site-a"},
		{Kind: "s3", Endpoint: "http://192.168.1.10:9000", Bucket: "backup", AccessKey: "a", SecretKey: "s", PathStyle: true},
		{Kind: "s3", Endpoint: "http://minio:9000", Bucket: "backup", AccessKey: "a", SecretKey: "s", PathStyle: true},
		{Kind: "s3", Endpoint: "http://nas.lan:9000", Bucket: "backup", AccessKey: "a", SecretKey: "s", PathStyle: true},
	}
	for i, c := range good {
		if err := c.Validate(); err != nil {
			t.Errorf("config %d should be valid: %v", i, err)
		}
	}
}

func TestRedactedAndMerge(t *testing.T) {
	c := Config{Kind: "s3", Endpoint: "https://e", Bucket: "b", AccessKey: "AK", SecretKey: "SECRET"}
	r := c.Redacted()
	if _, ok := r["secret_key"]; ok || r["has_secret_key"] != true {
		t.Fatalf("redacted: %v", r)
	}
	next := Config{Kind: "s3", Endpoint: "https://e", Bucket: "b2", AccessKey: "AK"}
	if m := next.Merge(c); m.SecretKey != "SECRET" {
		t.Fatal("same key: the stored secret is kept")
	}
	next.AccessKey = "OTHER"
	if m := next.Merge(c); m.SecretKey != "" {
		t.Fatal("another access key: the stored secret must not follow")
	}
	s := Config{Kind: "sftp", Host: "h", User: "u", Password: "pw"}
	if m := (Config{Kind: "sftp", Host: "evil", User: "u"}).Merge(s); m.Password != "" {
		t.Fatal("another host: the stored password must not follow")
	}
}

// exercise runs the same sequence against any target.
func exercise(t *testing.T, tg Target) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	data := bytes.Repeat([]byte("backup-data-"), 50000)
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	names := []string{"ctrl32-20261001-031500.tar.gz.enc", "ctrl32-20261002-031500.tar.gz.enc", "ctrl32-20261002-031500.tar.gz.enc.sha256"}
	for _, n := range names {
		if err := tg.Put(ctx, n, src); err != nil {
			t.Fatalf("put %s: %v", n, err)
		}
	}
	if err := tg.Put(ctx, "../evil", src); err == nil {
		t.Fatal("an invalid name must be refused")
	}
	list, err := tg.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range list {
		got = append(got, o.Name)
		if o.Size != int64(len(data)) {
			t.Errorf("%s: size %d", o.Name, o.Size)
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Fatalf("list: %v", got)
	}
	rc, err := tg.Get(ctx, names[1])
	if err != nil {
		t.Fatal(err)
	}
	back, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(back, data) {
		t.Fatal("content differs after the round trip")
	}
	if err := tg.Delete(ctx, names[0]); err != nil {
		t.Fatal(err)
	}
	if err := tg.Delete(ctx, names[0]); err != nil {
		t.Fatalf("deleting a missing file is not an error: %v", err)
	}
	list, _ = tg.List(ctx)
	if len(list) != 2 {
		t.Fatalf("after delete: %v", list)
	}
}

func TestFolder(t *testing.T) {
	dir := t.TempDir()
	tg, err := Open(context.Background(), Config{Kind: "folder", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	exercise(t, tg)
	if _, err := Open(context.Background(), Config{Kind: "folder", Path: filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("a missing folder must be reported")
	}
}

// ---- a small S3 server that checks the signature with the same algorithm --------------------------------------

type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	ak, sk  string
	t       *testing.T
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// recompute the signature of the received request
	check, _ := http.NewRequest(r.Method, "http://"+r.Host+r.URL.RequestURI(), nil)
	for k, v := range r.Header {
		lk := strings.ToLower(k)
		if lk == "range" || strings.HasPrefix(lk, "x-amz-") && lk != "x-amz-date" && lk != "x-amz-content-sha256" {
			check.Header[k] = v
		}
	}
	now, err := time.Parse("20060102T150405Z", r.Header.Get("x-amz-date"))
	if err != nil {
		http.Error(w, "date", 403)
		return
	}
	sign(check, f.ak, f.sk, "eu-central-1", r.Header.Get("x-amz-content-sha256"), now)
	if check.Header.Get("Authorization") != r.Header.Get("Authorization") {
		w.WriteHeader(403)
		fmt.Fprint(w, `<Error><Code>SignatureDoesNotMatch</Code><Message>bad signature</Message></Error>`)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == "PUT":
		b, _ := io.ReadAll(r.Body)
		f.objects[key] = b
	case r.Method == "GET" && r.URL.Query().Get("list-type") == "2":
		type content struct {
			Key          string
			Size         int
			LastModified string
		}
		var res struct {
			XMLName     xml.Name  `xml:"ListBucketResult"`
			Contents    []content `xml:"Contents"`
			IsTruncated bool
		}
		for k, v := range f.objects {
			if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
				res.Contents = append(res.Contents, content{k, len(v), "2026-10-10T10:00:00.000Z"})
			}
		}
		_ = xml.NewEncoder(w).Encode(res)
	case r.Method == "GET":
		b, ok := f.objects[key]
		if !ok {
			w.WriteHeader(404)
			fmt.Fprint(w, `<Error><Code>NoSuchKey</Code></Error>`)
			return
		}
		w.Write(b)
	case r.Method == "DELETE":
		delete(f.objects, key)
		w.WriteHeader(204)
	}
}

func TestS3(t *testing.T) {
	fs := &fakeS3{objects: map[string][]byte{}, ak: "AK", sk: "SK", t: t}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	c := Config{Kind: "s3", Endpoint: srv.URL, Region: "eu-central-1", Bucket: "bucket", AccessKey: "AK", SecretKey: "SK", Path: "site-a", PathStyle: true}
	tg, err := Open(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	exercise(t, tg)
	for k := range fs.objects {
		if !strings.HasPrefix(k, "site-a/ctrl32-") {
			t.Errorf("object outside the prefix: %s", k)
		}
	}
	c.SecretKey = "WRONG"
	bad, _ := Open(context.Background(), c)
	if _, err := bad.List(context.Background()); err == nil || !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Fatalf("a wrong secret must be refused with the server's message: %v", err)
	}
}

// ---- an SFTP server in the test process ------------------------------------------------------------------------

func sftpServer(t *testing.T, root string) (addr, fingerprint string) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if c.User() == "backup" && string(p) == "secret" {
			return nil, nil
		}
		return nil, errors.New("denied")
	}}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					c.Close()
					return
				}
				go ssh.DiscardRequests(reqs)
				for nc := range chans {
					ch, creqs, err := nc.Accept()
					if err != nil {
						continue
					}
					go func() {
						for r := range creqs {
							ok := r.Type == "subsystem" && len(r.Payload) > 4 && string(r.Payload[4:]) == "sftp"
							r.Reply(ok, nil)
							if ok {
								srv, _ := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(root))
								_ = srv.Serve()
								ch.Close()
							}
						}
					}()
				}
			}(c)
		}
	}()
	return ln.Addr().String(), ssh.FingerprintSHA256(signer.PublicKey())
}

func TestSFTP(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "backups"), 0o755)
	addr, fp := sftpServer(t, root)
	host, port, _ := net.SplitHostPort(addr)
	var p int
	fmt.Sscan(port, &p)
	c := Config{Kind: "sftp", Host: host, Port: p, User: "backup", Password: "secret", Path: "backups"}

	// no pinned key: nothing is sent, the fingerprint is reported
	_, err := Open(context.Background(), c)
	var hk *ErrHostKey
	if !errors.As(err, &hk) || hk.Seen != fp {
		t.Fatalf("unpinned key: %v", err)
	}
	c.HostKey = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if _, err := Open(context.Background(), c); !errors.As(err, &hk) || hk.Pinned == "" {
		t.Fatalf("wrong key: %v", err)
	}
	c.HostKey = fp
	c.Password = "wrong"
	if _, err := Open(context.Background(), c); err == nil {
		t.Fatal("a wrong password must fail")
	}
	c.Password = "secret"
	tg, err := Open(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	defer tg.Close()
	exercise(t, tg)
}
