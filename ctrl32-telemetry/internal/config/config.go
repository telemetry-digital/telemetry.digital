// Package config loads the server configuration: defaults, then a TOML file, then CTRL32_* environment overrides.
// Secure defaults (ADR-0007): the "internet" profile refuses to start without TLS on HTTP and MQTT.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the complete server configuration.
type Config struct {
	Profile  string         `toml:"profile"` // "intranet" or "internet"
	HTTP     HTTPConfig     `toml:"http"`
	MQTT     MQTTConfig     `toml:"mqtt"`
	DB       DBConfig       `toml:"db"`
	SMTP     SMTPConfig     `toml:"smtp"`
	Log      LogConfig      `toml:"log"`
	Gap      GapConfig      `toml:"gap"`
	Security SecurityConfig `toml:"security"`
	Report   ReportConfig   `toml:"report"`
	FOTA     FOTAConfig     `toml:"fota"`
	Agent    AgentConfig    `toml:"agent"`
	Map      MapConfig      `toml:"map"`
	Time     TimeConfig     `toml:"time"`
	Video    VideoConfig    `toml:"video"`
	License  LicenseConfig  `toml:"license"`
	// Object counting on camera pictures (spec 029): the detector runs in worker processes; empty = not installed.
	Analytics AnalyticsConfig `toml:"analytics"`
}

// LicenseConfig is the online activation of the white-label license (spec 007): the server sends the order number and
// its host name to the licence worker and installs the license it returns.
type LicenseConfig struct {
	ActivationURL string `toml:"activation_url"` // default https://lic.ctrl32.com/telemetry/activate; https, or http to loopback (tests)
	BuyURL        string `toml:"buy_url"`        // where the "Buy" link points; default https://ctrl32.com/pricing/
}

// Default license URLs.
const (
	DefaultActivationURL = "https://lic.ctrl32.com/telemetry/activate"
	DefaultBuyURL        = "https://ctrl32.com/pricing/"
)

// AnalyticsConfig locates what object counting on camera pictures needs (spec 029): an unmodified ffmpeg (LGPL build)
// that decodes the sub-stream, the ONNX Runtime library (MIT) and the YOLOX models (Apache-2.0). Without them the
// counting from sensors works, cameras show "not installed".
type AnalyticsConfig struct {
	FFmpeg      string `toml:"ffmpeg"`      // ffmpeg executable; empty = next to the binary or on PATH
	ONNXRuntime string `toml:"onnxruntime"` // onnxruntime.dll / libonnxruntime.so; empty = not installed
	ModelNano   string `toml:"model_nano"`  // yolox_nano.onnx (fast, default)
	ModelTiny   string `toml:"model_tiny"`  // yolox_tiny.onnx (more accurate, about 3× the CPU)
	NanoSHA256  string `toml:"nano_sha256"` // optional pins of the model files
	TinySHA256  string `toml:"tiny_sha256"` //
	Threads     int    `toml:"threads"`     // inference threads per camera (default 1)
	MaxCameras  int    `toml:"max_cameras"` // cameras analysed at the same time (default 4)
}

// VideoConfig locates camera recordings (spec 010).
type VideoConfig struct {
	StorageDir string `toml:"storage_dir"` // recorded segments; empty = <report.storage_dir>/../video
	MaxDiskGB  int    `toml:"max_disk_gb"` // oldest recordings are deleted beyond this; default 50
	// Remote cameras send their video here (camera-relay at the site, spec 011). Empty = off.
	PushListen  string `toml:"push_listen"`   // e.g. ":8322"
	PushTLSCert string `toml:"push_tls_cert"` // default http.tls_cert; profile "internet" requires TLS
	PushTLSKey  string `toml:"push_tls_key"`
	PushHost    string `toml:"push_host"` // host name the relays connect to; default the host of http.base_url
}

// TimeConfig verifies the server clock against NTP (URS B-06); an unsynchronized clock is a warning on the Server page.
type TimeConfig struct {
	NTPServer     string        `toml:"ntp_server"`     // host[:port]; empty = no check
	CheckInterval time.Duration `toml:"check_interval"` // default 15m
	MaxOffset     time.Duration `toml:"max_offset"`     // default 1s
}

// MapConfig serves map tiles from a local PMTiles archive, so map widgets work without the Internet.
type MapConfig struct {
	PMTiles     string `toml:"pmtiles"`     // raster PMTiles v3 archive (png/jpg/webp); empty = no local tiles
	Attribution string `toml:"attribution"` // shown on the map; default: the archive's metadata attribution
}

// AgentConfig connects the web service to the privileged server agent (ADR-0012).
type AgentConfig struct {
	Socket      string `toml:"socket"`       // Unix socket of `ctrl32-telemetry agent`; empty = no agent (development)
	PgDump      string `toml:"pg_dump"`      // Windows: path to pg_dump.exe (default: bundled PostgreSQL next to the binary)
	PgContainer string `toml:"pg_container"` // Docker container of PostgreSQL for backups (default ctrl32-telemetry-postgres)
	UpdateURL   string `toml:"update_url"`   // where the agent downloads releases; default https://portal.telemetry.digital/dl
	// MaxPackageMB caps an uploaded offline update package (System → Updates, `ctrl32-telemetry update --file`);
	// default 2048
	MaxPackageMB int `toml:"max_package_mb"`
	// AllowUnsignedUpdates lets a DEVELOPMENT build (no release date in the binary) install releases without a signed
	// manifest. Release builds ignore it: they install only releases signed with a trusted key (ADR-0024).
	AllowUnsignedUpdates bool `toml:"allow_unsigned_updates"`
	// VPNSimulate (development and tests only): the agent renders the VPN files into this directory instead of applying
	// them to the host and serves only the VPN operations (internal/agent/vpn.go). Never set on a server.
	VPNSimulate string `toml:"vpn_simulate"`
}

// FOTAConfig locates firmware storage.
type FOTAConfig struct {
	StorageDir string `toml:"storage_dir"` // uploaded firmware images; empty = <report.storage_dir>/../firmware
	MaxSizeMB  int    `toml:"max_size_mb"` // upload limit, default 16
}

// ReportConfig locates the Typst CLI and the report storage.
type ReportConfig struct {
	TypstPath  string `toml:"typst_path"`  // empty = "typst" on PATH or next to the executable
	StorageDir string `toml:"storage_dir"` // generated PDFs (records); empty = platform default
}

// SecurityConfig holds secrets that must never enter the database (ADR-0007).
type SecurityConfig struct {
	Pepper    string `toml:"pepper"`     // mixed into password hashes
	SecretKey string `toml:"secret_key"` // AES-GCM key source for TOTP seeds and connector credentials
	Contact   string `toml:"contact"`    // published in /.well-known/security.txt (mailto: or https:)
}

type HTTPConfig struct {
	Listen     string `toml:"listen"`      // e.g. ":8080"
	TLSCert    string `toml:"tls_cert"`    // PEM file; empty = plain HTTP (intranet only)
	TLSKey     string `toml:"tls_key"`     //
	BaseURL    string `toml:"base_url"`    // public URL, used in links and FOTA URLs
	PublicRead bool   `toml:"public_read"` // PoC/demo only: allow unauthenticated read-only API access
}

type MQTTConfig struct {
	Listen    string `toml:"listen"`     // plain TCP listener, e.g. ":1883"; empty = disabled
	TLSListen string `toml:"tls_listen"` // TLS listener, e.g. ":8883"; requires http.tls_cert/key or mqtt.tls_cert/key
	WSListen  string `toml:"ws_listen"`  // MQTT over WebSocket for the UI, e.g. ":1884"
	VPNListen string `toml:"vpn_listen"` // extra plain listener inside the WireGuard network, e.g. "10.66.0.1:1883"; bound with retries
	// UplinkListen: plain listener on this server's address in a hub's VPN (uplink, set by the agent on Linux), e.g.
	// "10.99.0.7:1883"; bound with retries. The uplink's firewall decides who may connect.
	UplinkListen string `toml:"uplink_listen"`
	TLSCert      string `toml:"tls_cert"` // optional separate certificate for MQTT
	TLSKey       string `toml:"tls_key"`
}

type DBConfig struct {
	URL         string `toml:"url"`          // application connection (role ctrl32_app)
	AdminURL    string `toml:"admin_url"`    // migration connection (schema owner); defaults to URL
	AppPassword string `toml:"app_password"` // if set, `migrate` sets the ctrl32_app role password
}

// SMTPConfig is the outgoing e-mail. Since 0.76.0 a system administrator can set it in System → Server → E-mail
// (SMTP); the settings saved there take precedence over this section, unless Override is true (then this section
// applies and the page is read-only). Without saved settings this section applies as before.
type SMTPConfig struct {
	Host     string   `toml:"host"`
	Port     int      `toml:"port"`
	User     string   `toml:"user"`
	Password string   `toml:"password"`
	From     string   `toml:"from"`
	To       []string `toml:"to"` // static alarm recipients
	// Security: "starttls" (default for port 587 and others), "tls" (implicit TLS, port 465) or "none" (unencrypted;
	// only for a relay on this host or in the local network). Empty: implicit TLS on port 465, STARTTLS otherwise.
	Security string `toml:"security"`
	FromName string `toml:"from_name"` // display name of the sender (optional)
	ReplyTo  string `toml:"reply_to"`  // Reply-To address (optional)
	// Override: this section wins over the settings saved in the user interface (which then cannot be changed there)
	Override bool `toml:"override"`
}

type LogConfig struct {
	Level  string `toml:"level"`  // debug|info|warn|error
	File   string `toml:"file"`   // empty = stdout only
	Syslog string `toml:"syslog"` // udp://host:514 or tcp://host:514: forward the JSON log as RFC 5424 (empty = off)
}

type GapConfig struct {
	CheckInterval time.Duration `toml:"check_interval"`
}

// Defaults returns the built-in configuration (intranet profile, local dev database).
func Defaults() *Config {
	c := &Config{Profile: "intranet"}
	c.HTTP.Listen = ":8080"
	c.HTTP.BaseURL = "http://localhost:8080"
	c.MQTT.Listen = ":1883"
	c.MQTT.WSListen = ":1884"
	c.DB.URL = "postgres://ctrl32:ctrl32-dev-only@localhost:5433/ctrl32?sslmode=disable"
	c.SMTP.Port = 587
	c.Log.Level = "info"
	c.Time.NTPServer = "pool.ntp.org"
	c.Time.CheckInterval = 15 * time.Minute
	c.Time.MaxOffset = time.Second
	c.Gap.CheckInterval = time.Minute
	return c
}

// DefaultPath returns the platform default config file location.
func DefaultPath() string {
	if runtime.GOOS == "windows" {
		if pd := os.Getenv("ProgramData"); pd != "" {
			return filepath.Join(pd, "ctrl32-telemetry", "config.toml")
		}
		return `C:\ProgramData\ctrl32-telemetry\config.toml`
	}
	return "/etc/ctrl32-telemetry/config.toml"
}

// Load reads defaults, then the TOML file (if it exists), then environment overrides, and validates.
func Load(path string) (*Config, error) {
	c := Defaults()
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, c); err != nil {
				return nil, fmt.Errorf("config %s: %w", path, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	}
	c.applyEnv()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) applyEnv() {
	set := func(key string, dst *string) {
		if v, ok := os.LookupEnv("CTRL32_" + key); ok {
			*dst = v
		}
	}
	set("PROFILE", &c.Profile)
	set("HTTP_LISTEN", &c.HTTP.Listen)
	set("HTTP_TLS_CERT", &c.HTTP.TLSCert)
	set("HTTP_TLS_KEY", &c.HTTP.TLSKey)
	set("HTTP_BASE_URL", &c.HTTP.BaseURL)
	set("MQTT_LISTEN", &c.MQTT.Listen)
	set("MQTT_TLS_LISTEN", &c.MQTT.TLSListen)
	set("MQTT_WS_LISTEN", &c.MQTT.WSListen)
	set("DB_URL", &c.DB.URL)
	set("DB_ADMIN_URL", &c.DB.AdminURL)
	set("DB_APP_PASSWORD", &c.DB.AppPassword)
	set("SMTP_HOST", &c.SMTP.Host)
	set("SMTP_USER", &c.SMTP.User)
	set("SMTP_PASSWORD", &c.SMTP.Password)
	set("SMTP_FROM", &c.SMTP.From)
	set("LOG_LEVEL", &c.Log.Level)
	set("LOG_FILE", &c.Log.File)
	set("SECURITY_PEPPER", &c.Security.Pepper)
	set("SECURITY_SECRET_KEY", &c.Security.SecretKey)
	set("REPORT_TYPST_PATH", &c.Report.TypstPath)
	set("REPORT_STORAGE_DIR", &c.Report.StorageDir)
	set("LICENSE_ACTIVATION_URL", &c.License.ActivationURL)
	if v, ok := os.LookupEnv("CTRL32_HTTP_PUBLIC_READ"); ok {
		c.HTTP.PublicRead, _ = strconv.ParseBool(v)
	}
	if v, ok := os.LookupEnv("CTRL32_SMTP_TO"); ok {
		c.SMTP.To = strings.Split(v, ",")
	}
	if v, ok := os.LookupEnv("CTRL32_SMTP_PORT"); ok {
		if p, err := strconv.Atoi(v); err == nil {
			c.SMTP.Port = p
		}
	}
}

// Validate enforces the secure defaults of the chosen profile.
func (c *Config) Validate() error {
	switch c.Profile {
	case "intranet", "internet":
	default:
		return fmt.Errorf("profile must be \"intranet\" or \"internet\", got %q", c.Profile)
	}
	if c.DB.URL == "" {
		return errors.New("db.url is required")
	}
	if len(c.Security.Pepper) < 16 || len(c.Security.SecretKey) < 16 {
		return errors.New("security.pepper and security.secret_key are required (at least 16 characters each; the installer generates them)")
	}
	if c.DB.AdminURL == "" {
		c.DB.AdminURL = c.DB.URL
	}
	if c.Map.PMTiles != "" {
		if _, err := os.Stat(c.Map.PMTiles); err != nil {
			return fmt.Errorf("map.pmtiles: %w", err)
		}
	}
	if c.Profile == "internet" {
		// Either the binary terminates TLS itself, or it listens on loopback behind a TLS-terminating reverse proxy.
		if (c.HTTP.TLSCert == "" || c.HTTP.TLSKey == "") && !isLoopback(c.HTTP.Listen) {
			return errors.New("profile \"internet\" requires http.tls_cert and http.tls_key, or http.listen bound to a loopback address behind a TLS-terminating reverse proxy")
		}
		if c.MQTT.Listen != "" && !isLoopback(c.MQTT.Listen) {
			return errors.New("profile \"internet\" does not allow a plain MQTT listener on a non-loopback address; use mqtt.tls_listen")
		}
		if c.MQTT.TLSListen != "" && c.mqttTLSCert() == "" {
			return errors.New("mqtt.tls_listen requires a certificate (mqtt.tls_cert/tls_key or http.tls_cert/tls_key)")
		}
	}
	if c.Gap.CheckInterval <= 0 {
		c.Gap.CheckInterval = time.Minute
	}
	if c.Report.StorageDir == "" {
		if runtime.GOOS == "windows" {
			c.Report.StorageDir = filepath.Join(filepath.Dir(DefaultPath()), "reports")
		} else {
			c.Report.StorageDir = "/var/lib/ctrl32-telemetry/reports"
		}
	}
	if c.FOTA.StorageDir == "" {
		c.FOTA.StorageDir = filepath.Join(filepath.Dir(c.Report.StorageDir), "firmware")
	}
	if c.Video.StorageDir == "" {
		c.Video.StorageDir = filepath.Join(filepath.Dir(c.Report.StorageDir), "video")
	}
	if c.Video.PushListen != "" && c.Video.PushTLSCert == "" {
		c.Video.PushTLSCert, c.Video.PushTLSKey = c.HTTP.TLSCert, c.HTTP.TLSKey
	}
	if c.Profile == "internet" && c.Video.PushListen != "" && (c.Video.PushTLSCert == "" || c.Video.PushTLSKey == "") {
		return errors.New("profile \"internet\" requires TLS for video.push_listen: set video.push_tls_cert and video.push_tls_key (or http.tls_cert/tls_key)")
	}
	if c.Video.MaxDiskGB <= 0 {
		c.Video.MaxDiskGB = 50
	}
	if c.License.ActivationURL == "" {
		c.License.ActivationURL = DefaultActivationURL
	}
	if c.License.BuyURL == "" {
		c.License.BuyURL = DefaultBuyURL
	}
	if err := checkLicenseURLs(c.License); err != nil {
		return err
	}
	if c.FOTA.MaxSizeMB <= 0 {
		c.FOTA.MaxSizeMB = 16
	}
	return nil
}

func (c *Config) mqttTLSCert() string {
	if c.MQTT.TLSCert != "" {
		return c.MQTT.TLSCert
	}
	return c.HTTP.TLSCert
}

// MQTTTLSFiles returns the certificate and key used for the MQTT TLS listener.
func (c *Config) MQTTTLSFiles() (cert, key string) {
	if c.MQTT.TLSCert != "" {
		return c.MQTT.TLSCert, c.MQTT.TLSKey
	}
	return c.HTTP.TLSCert, c.HTTP.TLSKey
}

func isLoopback(addr string) bool {
	return strings.HasPrefix(addr, "127.") || strings.HasPrefix(addr, "localhost:") || strings.HasPrefix(addr, "[::1]:")
}

// checkLicenseURLs: the activation service is reached over https (a plain http URL only on the loopback interface,
// for tests); the buy link is an http(s) URL.
func checkLicenseURLs(l LicenseConfig) error {
	u, err := url.Parse(l.ActivationURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("license.activation_url: not a URL: %q", l.ActivationURL)
	}
	h := u.Hostname()
	loop := h == "localhost" || h == "::1" || strings.HasPrefix(h, "127.")
	if u.Scheme != "https" && !(u.Scheme == "http" && loop) {
		return errors.New("license.activation_url must be https:// (http:// only to 127.0.0.1 or localhost)")
	}
	b, err := url.Parse(l.BuyURL)
	if err != nil || b.Host == "" || (b.Scheme != "https" && b.Scheme != "http") {
		return fmt.Errorf("license.buy_url: not an http(s) URL: %q", l.BuyURL)
	}
	return nil
}
