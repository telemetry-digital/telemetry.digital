package offsite

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpTarget writes over SSH (SFTP). The host key is pinned by its SHA-256 fingerprint: without a pinned key, or with
// a different one, the connection ends before the password or the key is sent.
type sftpTarget struct {
	conn *ssh.Client
	cl   *sftp.Client
	dir  string
}

func dialSFTP(ctx context.Context, c Config) (*sftpTarget, error) {
	var auth []ssh.AuthMethod
	if c.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(c.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("sftp: the private key cannot be read (unencrypted OpenSSH or PEM key expected): %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if c.Password != "" {
		auth = append(auth, ssh.Password(c.Password))
	}
	var seen string
	cfg := &ssh.ClientConfig{
		User: c.User,
		Auth: auth,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			seen = ssh.FingerprintSHA256(key)
			if c.HostKey == "" || subtle.ConstantTimeCompare([]byte(seen), []byte(c.HostKey)) != 1 {
				return &ErrHostKey{Seen: seen, Pinned: c.HostKey}
			}
			return nil
		},
		Timeout: 20 * time.Second,
	}
	port := c.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(trimBrackets(c.Host), strconv.Itoa(port))
	d := net.Dialer{Timeout: 20 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("sftp: cannot connect to %s: %w", addr, err)
	}
	sc, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		raw.Close()
		var hk *ErrHostKey
		if errors.As(err, &hk) {
			return nil, hk
		}
		return nil, fmt.Errorf("sftp: %w", err)
	}
	conn := ssh.NewClient(sc, chans, reqs)
	cl, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sftp: the server does not offer SFTP: %w", err)
	}
	dir := c.Path
	if dir == "" {
		dir = "."
	}
	if st, err := cl.Stat(dir); err != nil || !st.IsDir() {
		cl.Close()
		conn.Close()
		return nil, fmt.Errorf("sftp: the directory %q does not exist on the server", dir)
	}
	return &sftpTarget{conn: conn, cl: cl, dir: dir}, nil
}

func trimBrackets(h string) string {
	if len(h) > 1 && h[0] == '[' && h[len(h)-1] == ']' {
		return h[1 : len(h)-1]
	}
	return h
}

func (s *sftpTarget) Put(ctx context.Context, name, localPath string) error {
	if err := checkName(name); err != nil {
		return err
	}
	in, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := path.Join(s.dir, "."+name+".part")
	out, err := s.cl.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("sftp: cannot write into %s: %w", s.dir, err)
	}
	if _, err := io.Copy(out, ctxReader{ctx, in}); err != nil {
		out.Close()
		s.cl.Remove(tmp)
		return fmt.Errorf("sftp: upload failed: %w", err)
	}
	if err := out.Close(); err != nil {
		s.cl.Remove(tmp)
		return fmt.Errorf("sftp: upload failed: %w", err)
	}
	final := path.Join(s.dir, name)
	if err := s.cl.PosixRename(tmp, final); err != nil {
		// servers without the posix-rename extension: plain rename refuses an existing target
		_ = s.cl.Remove(final)
		if err := s.cl.Rename(tmp, final); err != nil {
			s.cl.Remove(tmp)
			return fmt.Errorf("sftp: rename failed: %w", err)
		}
	}
	return nil
}

func (s *sftpTarget) Get(_ context.Context, name string) (io.ReadCloser, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	return s.cl.Open(path.Join(s.dir, name))
}

func (s *sftpTarget) List(_ context.Context) ([]Object, error) {
	entries, err := s.cl.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []Object
	for _, e := range entries {
		if e.IsDir() || !ValidName(e.Name()) {
			continue
		}
		out = append(out, Object{Name: e.Name(), Size: e.Size(), Modified: e.ModTime()})
	}
	return out, nil
}

func (s *sftpTarget) Delete(_ context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	err := s.cl.Remove(path.Join(s.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *sftpTarget) Close() error {
	s.cl.Close()
	return s.conn.Close()
}
