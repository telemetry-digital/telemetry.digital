// Package offsite stores backup bundles away from the server (URS K-01): a folder (a local disk, or an SMB/NFS share
// mounted by the operating system, or a UNC path on Windows), an SFTP server with a pinned host key, or an
// S3-compatible object storage (AWS, MinIO, Wasabi, Backblaze B2, Hetzner, Cloudflare R2 …). Only file names of
// backups are accepted, so a target can never be used to read or write anything else.
package offsite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Object is a stored file.
type Object struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// Target is a place for backup files. Every method takes a bare file name (ValidName).
type Target interface {
	// Put stores the local file under name (replacing an existing one only when the upload completed).
	Put(ctx context.Context, name, localPath string) error
	Get(ctx context.Context, name string) (io.ReadCloser, error)
	// List returns the backup files (names that pass ValidName), in no particular order.
	List(ctx context.Context) ([]Object, error)
	Delete(ctx context.Context, name string) error
	Close() error
}

// Config describes a target. Secrets (Password, PrivateKey, SecretKey) are write-only towards the user interface:
// Redacted removes them and Merge keeps the stored ones when an update leaves them empty.
type Config struct {
	Enabled bool   `json:"enabled"`
	Kind    string `json:"kind"`           // folder, sftp, s3
	Path    string `json:"path,omitempty"` // folder: directory; sftp: remote directory; s3: key prefix
	// SFTP
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	User       string `json:"user,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"` // OpenSSH or PEM private key
	HostKey    string `json:"host_key,omitempty"`    // pinned fingerprint "SHA256:…" (required)
	// S3
	Endpoint  string `json:"endpoint,omitempty"` // https://s3.eu-central-1.amazonaws.com, https://minio.example:9000 …
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
	PathStyle bool   `json:"path_style,omitempty"` // https://endpoint/bucket/key instead of https://bucket.endpoint/key
}

var (
	nameRe   = regexp.MustCompile(`^ctrl32-\d{8}-\d{6}\.tar\.gz(\.enc)?(\.sha256)?$`)
	bucketRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	prefixRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{0,200}$`)
	hostRe   = regexp.MustCompile(`^[A-Za-z0-9.:\[\]-]{1,253}$`)
	userRe   = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,64}$`)
	regionRe = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
)

// ValidName reports whether name is a backup bundle or its checksum file.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// ErrHostKey is returned when the SFTP server's key differs from the pinned one (or none is pinned); Seen holds the
// fingerprint the server presented so the administrator can compare it with the server's own and pin it.
type ErrHostKey struct{ Seen, Pinned string }

func (e *ErrHostKey) Error() string {
	if e.Pinned == "" {
		return "the SFTP server's host key is not pinned yet: it presented " + e.Seen + " — compare it with the server and pin it"
	}
	return "the SFTP server's host key " + e.Seen + " does not match the pinned " + e.Pinned + " (possible interception: nothing was sent)"
}

// Validate checks a configuration before it is stored or used.
func (c Config) Validate() error {
	switch c.Kind {
	case "folder":
		if strings.TrimSpace(c.Path) == "" || strings.ContainsAny(c.Path, "\x00\n\r") {
			return errors.New("folder: the path is required")
		}
		if !isAbs(c.Path) {
			return errors.New("folder: the path must be absolute (e.g. /mnt/backup or \\\\nas\\backup)")
		}
	case "sftp":
		if !hostRe.MatchString(c.Host) {
			return errors.New("sftp: invalid host")
		}
		if c.Port < 0 || c.Port > 65535 {
			return errors.New("sftp: invalid port")
		}
		if !userRe.MatchString(c.User) {
			return errors.New("sftp: invalid user")
		}
		if c.Password == "" && c.PrivateKey == "" {
			return errors.New("sftp: a password or a private key is required")
		}
		if c.HostKey != "" && !strings.HasPrefix(c.HostKey, "SHA256:") {
			return errors.New("sftp: the host key fingerprint must start with SHA256:")
		}
		if strings.ContainsAny(c.Path, "\x00\n\r") {
			return errors.New("sftp: invalid path")
		}
	case "s3":
		u, err := url.Parse(c.Endpoint)
		if err != nil || u.Host == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" {
			return errors.New("s3: the endpoint must be a URL like https://s3.eu-central-1.amazonaws.com")
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackOrPrivate(u.Hostname())) {
			return errors.New("s3: the endpoint must use https (http only to this host or the local network)")
		}
		if !bucketRe.MatchString(c.Bucket) {
			return errors.New("s3: invalid bucket name")
		}
		if c.Region != "" && !regionRe.MatchString(c.Region) {
			return errors.New("s3: invalid region")
		}
		if c.AccessKey == "" || c.SecretKey == "" || len(c.AccessKey) > 128 || len(c.SecretKey) > 256 {
			return errors.New("s3: the access key and the secret key are required")
		}
		if !prefixRe.MatchString(c.Path) || strings.Contains(c.Path, "..") {
			return errors.New("s3: invalid prefix")
		}
	default:
		return errors.New("unknown target kind (folder, sftp or s3)")
	}
	return nil
}

// Redacted returns the configuration without secrets, with flags telling which are stored.
func (c Config) Redacted() map[string]any {
	out := map[string]any{"enabled": c.Enabled, "kind": c.Kind, "path": c.Path}
	switch c.Kind {
	case "sftp":
		out["host"], out["port"], out["user"], out["host_key"] = c.Host, c.Port, c.User, c.HostKey
		out["has_password"], out["has_private_key"] = c.Password != "", c.PrivateKey != ""
	case "s3":
		out["endpoint"], out["region"], out["bucket"], out["access_key"], out["path_style"] = c.Endpoint, c.Region, c.Bucket, c.AccessKey, c.PathStyle
		out["has_secret_key"] = c.SecretKey != ""
	}
	return out
}

// Merge keeps the stored secrets of old when next leaves them empty and points to the same place.
func (next Config) Merge(old Config) Config {
	if next.Kind != old.Kind {
		return next
	}
	switch next.Kind {
	case "sftp":
		if next.Host == old.Host && next.User == old.User {
			if next.Password == "" && next.PrivateKey == "" {
				next.Password, next.PrivateKey = old.Password, old.PrivateKey
			}
		}
	case "s3":
		if next.SecretKey == "" && next.AccessKey == old.AccessKey && next.Endpoint == old.Endpoint {
			next.SecretKey = old.SecretKey
		}
	}
	return next
}

// Describe is a short human description without secrets ("sftp backup@nas.local:/backups").
func (c Config) Describe() string {
	switch c.Kind {
	case "folder":
		return "folder " + c.Path
	case "sftp":
		port := ""
		if c.Port != 0 && c.Port != 22 {
			port = ":" + strconv.Itoa(c.Port)
		}
		return "sftp " + c.User + "@" + c.Host + port + ":" + c.Path
	case "s3":
		return "s3 " + c.Bucket + "/" + strings.Trim(c.Path, "/") + " (" + c.Endpoint + ")"
	}
	return c.Kind
}

// Open connects to the target (SFTP opens a session; folder and S3 are stateless).
func Open(ctx context.Context, c Config) (Target, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	switch c.Kind {
	case "folder":
		return newFolder(c.Path)
	case "sftp":
		return dialSFTP(ctx, c)
	case "s3":
		return newS3(c)
	}
	return nil, fmt.Errorf("unknown target kind %q", c.Kind)
}

func checkName(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid backup file name %q", name)
	}
	return nil
}

func isAbs(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// isLoopbackOrPrivate reports whether a host is this machine or in a local network (the rule of the outgoing e-mail,
// httpapi.localSMTPHost): an address of such a network, a name without a domain or with a local suffix.
func isLoopbackOrPrivate(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(strings.Trim(h, "[]")); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	if !strings.Contains(h, ".") {
		return true
	}
	for _, suf := range []string{".local", ".lan", ".internal", ".home.arpa", ".intranet", ".corp"} {
		if strings.HasSuffix(h, suf) {
			return true
		}
	}
	return false
}
