// Package agent implements the privileged helper of ctrl32 telemetry (ADR-0012): a root process listening on a
// Unix socket that performs a closed list of host operations for the unprivileged web service — status, service
// restarts, Caddy (domain/TLS), WireGuard with its VPN firewall and binary updates. Nothing here builds shell commands from input.
package agent

import "time"

// Status is everything the server screen shows about the host.
type Status struct {
	Agent      AgentInfo         `json:"agent"`
	Host       HostInfo          `json:"host"`
	Services   []ServiceState    `json:"services"`
	Containers []ContainerInfo   `json:"containers"`
	Versions   map[string]string `json:"versions"`
	Caddy      CaddyState        `json:"caddy"`
	WireGuard  WireGuardState    `json:"wireguard"`
}

type AgentInfo struct {
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
	Socket    string    `json:"socket"`
	OS        string    `json:"os"`
}

type HostInfo struct {
	Hostname      string     `json:"hostname"`
	OS            string     `json:"os"`
	Kernel        string     `json:"kernel"`
	Arch          string     `json:"arch"`
	CPUs          int        `json:"cpus"`
	UptimeSeconds int64      `json:"uptime_seconds"`
	Load          []float64  `json:"load"`
	MemTotalMB    int64      `json:"mem_total_mb"`
	MemAvailMB    int64      `json:"mem_avail_mb"`
	Disks         []DiskInfo `json:"disks"`
	Time          time.Time  `json:"time"`
	Timezone      string     `json:"timezone"`
	PublicIP      string     `json:"public_ip,omitempty"`
}

type DiskInfo struct {
	Path    string  `json:"path"`
	TotalGB float64 `json:"total_gb"`
	FreeGB  float64 `json:"free_gb"`
}

type ServiceState struct {
	Unit    string `json:"unit"`
	Label   string `json:"label"`
	Active  string `json:"active"`  // active, inactive, failed, unknown
	Enabled string `json:"enabled"` // enabled, disabled, unknown
}

type ContainerInfo struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
}

// CaddyState mirrors /opt/ctrl32-telemetry/caddy.json (never the DNS token).
type CaddyState struct {
	Managed      bool      `json:"managed"` // true once the UI configured it; false = installer default
	Domain       string    `json:"domain"`
	ExtraDomains []string  `json:"extra_domains"`
	Email        string    `json:"email"`
	DNSProvider  string    `json:"dns_provider"` // "" or "cloudflare"
	HasDNSToken  bool      `json:"has_dns_token"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
	CertSubject  string    `json:"cert_subject,omitempty"`
	CertIssuer   string    `json:"cert_issuer,omitempty"`
	CertExpires  time.Time `json:"cert_expires,omitempty"`
	CertDomains  []string  `json:"cert_domains,omitempty"`
	Caddyfile    string    `json:"caddyfile,omitempty"`
}

// CaddyRequest configures the reverse proxy and certificate issuance.
type CaddyRequest struct {
	Domain          string   `json:"domain"`
	ExtraDomains    []string `json:"extra_domains"`
	Email           string   `json:"email"`
	DNSProvider     string   `json:"dns_provider"`     // "" (HTTP/TLS-ALPN challenge) or "cloudflare" (DNS-01, works behind the Cloudflare proxy)
	CloudflareToken string   `json:"cloudflare_token"` // empty keeps the stored token
}

// WireGuardState combines the stored configuration with the live interface and the VPN firewall.
type WireGuardState struct {
	Installed  bool            `json:"installed"`
	Enabled    bool            `json:"enabled"`
	Running    bool            `json:"running"`
	ListenPort int             `json:"listen_port"`
	Address    string          `json:"address"` // server address with prefix, e.g. 10.66.0.1/24
	Endpoint   string          `json:"endpoint"`
	DNS        string          `json:"dns,omitempty"`
	PublicKey  string          `json:"public_key"`
	MQTT       string          `json:"mqtt,omitempty"` // broker address inside the VPN
	Peers      []WireGuardPeer `json:"peers"`
	UpdatedAt  time.Time       `json:"updated_at,omitempty"`
	// Supported is false where the agent does not manage WireGuard.
	Supported bool `json:"supported"`
	// Mode: "kernel" (Linux: kernel WireGuard and the nftables table), "userspace" (Windows: WireGuard and the access
	// rules inside the agent, ADR-0020) or "simulation" (development and tests); empty when not supported.
	Mode string `json:"mode,omitempty"`
	// Simulated: development/test agent ([agent] vpn_simulate) — files are rendered, nothing is applied to the host.
	Simulated bool        `json:"simulated,omitempty"`
	Firewall  VPNFirewall `json:"firewall"`
}

// VPNFirewall is the state of the filter of traffic from the VPN (default deny): the nftables table on Linux, the
// in-process filter of the userspace hub on Windows (Ruleset is then its rules text).
type VPNFirewall struct {
	Applied   bool      `json:"applied"` // the table is loaded (in simulation: rendered)
	Rules     int       `json:"rules"`   // access rules in the table
	Forward   bool      `json:"forward"` // forwarding through wg0 is on (rules towards peers or sites)
	AppliedAt time.Time `json:"applied_at,omitzero"`
	Error     string    `json:"error,omitempty"`   // last failed apply (rolled back)
	Ruleset   string    `json:"ruleset,omitempty"` // the generated table, for review
}

type WireGuardPeer struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	PublicKey       string    `json:"public_key"`
	Address         string    `json:"address"`
	Subnets         []string  `json:"subnets,omitempty"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	CreatedBy       string    `json:"created_by"`
	LatestHandshake time.Time `json:"latest_handshake,omitempty"`
	Endpoint        string    `json:"endpoint,omitempty"`
	RxBytes         int64     `json:"rx_bytes"`
	TxBytes         int64     `json:"tx_bytes"`
	Online          bool      `json:"online"`
}

type WireGuardRequest struct {
	Enabled    bool   `json:"enabled"`
	ListenPort int    `json:"listen_port"`
	Address    string `json:"address"`
	Endpoint   string `json:"endpoint"`
	DNS        string `json:"dns"`
}

type RestartRequest struct {
	Unit string `json:"unit"` // app, agent, caddy, wireguard
}

type UpdateRequest struct {
	Version string `json:"version"`
}

type UpdateResult struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Message string `json:"message"`
}

// Result is the generic acknowledgement.
type Result struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// ---- backups (URS K-14)

type BackupInfo struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	Encrypted bool      `json:"encrypted"`
}

type BackupSchedule struct {
	Enabled       bool      `json:"enabled"`
	Hour          int       `json:"hour"`
	Minute        int       `json:"minute"`
	Keep          int       `json:"keep"`
	HasPassphrase bool      `json:"has_passphrase"`
	LastRun       time.Time `json:"last_run,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	LastFile      string    `json:"last_file,omitempty"`
}

type BackupsState struct {
	Dir      string         `json:"dir"`
	Running  bool           `json:"running"`
	DBMode   string         `json:"db_mode"` // docker, native, unknown
	Files    []BackupInfo   `json:"files"`
	Schedule BackupSchedule `json:"schedule"`
}

type BackupRequest struct {
	Passphrase string `json:"passphrase"`
}

type ScheduleRequest struct {
	Enabled         bool   `json:"enabled"`
	Hour            int    `json:"hour"`
	Minute          int    `json:"minute"`
	Keep            int    `json:"keep"`
	Passphrase      string `json:"passphrase"`       // empty keeps the stored one
	ClearPassphrase bool   `json:"clear_passphrase"` // true removes it (scheduled backups unencrypted)
}
