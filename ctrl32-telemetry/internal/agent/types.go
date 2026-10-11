// Package agent implements the privileged helper of ctrl32 telemetry (ADR-0012): a root process listening on a
// Unix socket that performs a closed list of host operations for the unprivileged web service — status, service
// restarts, Caddy (domain/TLS), WireGuard with its VPN firewall, binary updates and the installation of the object
// counting detector (analytics.go). Nothing here builds shell commands from input.
package agent

import (
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/offsite"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/uptunnel"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

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
	// Uplink is this server's connection to a hub (VPN phase 3, uplink.go); nil where the agent cannot run one.
	Uplink *UplinkState `json:"uplink,omitempty"`
}

// UplinkState is the uplink of this server to a hub: the stored configuration (without keys) and the live tunnel.
type UplinkState struct {
	Configured      bool             `json:"configured"`
	Mode            string           `json:"mode"`      // kernel | userspace | simulation
	Transport       string           `json:"transport"` // udp | https
	Name            string           `json:"name,omitempty"`
	HubEndpoint     string           `json:"hub_endpoint,omitempty"`
	HubPublicKey    string           `json:"hub_public_key,omitempty"`
	PublicKey       string           `json:"public_key,omitempty"`
	Address         string           `json:"address,omitempty"`     // this server's address in the hub's VPN
	VPNNet          string           `json:"vpn_net,omitempty"`     // the hub's VPN range
	HubAddress      string           `json:"hub_address,omitempty"` // the hub's own VPN address
	Subnets         []string         `json:"subnets,omitempty"`     // the LAN subnets offered to the hub
	HTTPSAvailable  bool             `json:"https_available"`
	TunnelHost      string           `json:"tunnel_host,omitempty"`
	Tunnel          *uptunnel.Status `json:"tunnel,omitempty"`
	Running         bool             `json:"running"`
	Online          bool             `json:"online"`
	LatestHandshake time.Time        `json:"latest_handshake,omitzero"`
	Endpoint        string           `json:"endpoint,omitempty"`
	RxBytes         int64            `json:"rx_bytes"`
	TxBytes         int64            `json:"tx_bytes"`
	ConnectedAt     time.Time        `json:"connected_at,omitzero"`
	LANForward      []string         `json:"lan_forward,omitempty"`
	Firewall        VPNFirewall      `json:"firewall"`
}

// UplinkRequest connects to a hub (or replaces the configuration) with a pasted token and the compiled rules.
type UplinkRequest struct {
	Token     string       `json:"token"`
	Transport string       `json:"transport"`
	Rules     []vpn.FwRule `json:"rules"`
}

// UplinkRulesRequest applies new compiled uplink access rules.
type UplinkRulesRequest struct {
	Rules []vpn.FwRule `json:"rules"`
}

// UplinkTransportRequest switches the transport.
type UplinkTransportRequest struct {
	Transport string `json:"transport"`
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
	// UpdatesUntil (YYYY-MM-DD) is the last release date the server's licence includes updates for; empty = no limit
	// (no licence, or a licence issued without the limit). The agent refuses a binary released later.
	UpdatesUntil string `json:"updates_until,omitempty"`
	// Mode: manual (System → Updates), auto (maintenance window), cli (ctrl32-telemetry update --file); for the history
	Mode string `json:"mode,omitempty"`
	// Source: "" = the distribution point (update_url), "package" = the staged offline package
	Source string `json:"source,omitempty"`
	// Backup: a backup (database, configuration) before anything changes; the automatic mode always asks for it
	Backup bool `json:"backup,omitempty"`
	// RollbackMinutes: how long the new version has to answer healthy before the previous binary returns (default 10)
	RollbackMinutes int    `json:"rollback_minutes,omitempty"`
	Actor           string `json:"actor,omitempty"`  // who asked (user name, "automatic", "cli"), for the history
	Reason          string `json:"reason,omitempty"` // the reason of the audit trail, for the history
}

type UpdateResult struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	KeyID   string `json:"key_id,omitempty"` // the trusted key that verified the release ("" = unsigned, development only)
	Backup  string `json:"backup,omitempty"`
	Message string `json:"message"`
}

// UpdateRecord is one installation in the agent's history (update.json). Phases: installing, restarting (the guard
// waits for the new version to answer healthy), done, rolled_back, failed.
type UpdateRecord struct {
	ID         string    `json:"id"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Mode       string    `json:"mode"`
	Source     string    `json:"source"`
	KeyID      string    `json:"key_id,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	Actor      string    `json:"actor,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Phase      string    `json:"phase"`
	Backup     string    `json:"backup,omitempty"`
	Error      string    `json:"error,omitempty"`
	RolledBack string    `json:"rolled_back,omitempty"` // "binary" or "binary and database"
	StartedAt  time.Time `json:"started_at"`
	Deadline   time.Time `json:"deadline,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
	Audited    bool      `json:"audited,omitempty"` // the server wrote the result into its audit trail
}

// PackageInfo describes a verified offline package staged on the server.
type PackageInfo struct {
	Version   string    `json:"version"`
	Released  string    `json:"released"`
	KeyID     string    `json:"key_id,omitempty"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	Platform  string    `json:"platform"`
	HasBinary bool      `json:"has_binary"` // the package carries the server binary of this platform
	Detector  []string  `json:"detector,omitempty"`
	StagedAt  time.Time `json:"staged_at"`
}

// UpdateState is what GET /update/state answers.
type UpdateState struct {
	Current          *UpdateRecord  `json:"current,omitempty"` // the installation in progress or the last one
	History          []UpdateRecord `json:"history"`
	Staged           *PackageInfo   `json:"staged,omitempty"`
	TrustedKeys      []string       `json:"trusted_keys"`
	RequireSignature bool           `json:"require_signature"`
	TestBuild        bool           `json:"test_build,omitempty"`
	UpdateURL        string         `json:"update_url"`
	MaxPackageMB     int            `json:"max_package_mb"`
	Simulated        bool           `json:"simulated,omitempty"`
}

// PackageResult answers an upload: the staged package and, when it was installed, the installation.
type PackageResult struct {
	Package *PackageInfo  `json:"package"`
	Update  *UpdateResult `json:"update,omitempty"`
}

// AckRequest marks records of the history as written into the server's audit trail.
type AckRequest struct {
	IDs []string `json:"ids"`
}

// Result is the generic acknowledgement.
type Result struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// ---- backups (URS K-01..K-04, K-14)

type BackupInfo struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	Encrypted bool      `json:"encrypted"`
	// SHA256 of the file from its checksum file (<name>.sha256); empty for bundles made before 0.77.0
	SHA256 string `json:"sha256,omitempty"`
	// Offsite: the file is at the offsite target (as of the last listing of the target)
	Offsite bool `json:"offsite"`
}

type BackupSchedule struct {
	Enabled       bool      `json:"enabled"`
	Hour          int       `json:"hour"`
	Minute        int       `json:"minute"`
	Keep          int       `json:"keep"`         // daily: the newest N backups
	KeepWeekly    int       `json:"keep_weekly"`  // plus the newest of each of the last N weeks
	KeepMonthly   int       `json:"keep_monthly"` // plus the newest of each of the last N months
	HasPassphrase bool      `json:"has_passphrase"`
	LastRun       time.Time `json:"last_run,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	LastFile      string    `json:"last_file,omitempty"`
	LastOK        time.Time `json:"last_ok,omitempty"` // last backup that completed (including the offsite copy)
}

// RestoreTestSchedule: the automatic restore test (URS K-02), by default once a month.
type RestoreTestSchedule struct {
	Enabled bool      `json:"enabled"`
	Day     int       `json:"day"` // day of the month, 1–28
	Hour    int       `json:"hour"`
	Minute  int       `json:"minute"`
	Source  string    `json:"source"` // offsite (the default when a target is set) or local
	LastRun time.Time `json:"last_run,omitempty"`
	LastOK  time.Time `json:"last_ok,omitempty"`
}

// OffsiteState is the offsite target without secrets and the last listing of it.
type OffsiteState struct {
	Config    map[string]any  `json:"config"` // offsite.Config.Redacted(); {"enabled":false} when none
	Describe  string          `json:"describe,omitempty"`
	Files     []OffsiteObject `json:"files"`
	ListedAt  time.Time       `json:"listed_at,omitempty"`
	ListError string          `json:"list_error,omitempty"`
}

type OffsiteObject struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// BackupRun is one backup, offsite synchronisation or restore test with its steps (the protocol, URS K-02). The web
// service writes finished runs into the audit trail once and acknowledges them (Audited).
type BackupRun struct {
	ID         string       `json:"id"`
	Kind       string       `json:"kind"`    // backup, offsite_sync, restore_test
	Trigger    string       `json:"trigger"` // schedule, manual, update
	Actor      string       `json:"actor,omitempty"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at,omitempty"`
	Running    bool         `json:"running,omitempty"`
	OK         bool         `json:"ok"`
	File       string       `json:"file,omitempty"`
	SizeBytes  int64        `json:"size_bytes,omitempty"`
	Source     string       `json:"source,omitempty"` // restore test: offsite or local
	Target     string       `json:"target,omitempty"` // offsite target description
	Steps      []BackupStep `json:"steps"`
	Error      string       `json:"error,omitempty"`
	Audited    bool         `json:"audited,omitempty"`
}

type BackupStep struct {
	Name   string `json:"name"` // dump, bundle, encrypt, checksum, upload, verify, prune, prune_offsite, select, fetch, decrypt, manifest, restore, checks, cleanup
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Ms     int64  `json:"ms"`
}

type BackupsState struct {
	Dir            string              `json:"dir"`
	Running        bool                `json:"running"`
	RestoreRunning bool                `json:"restore_running"`
	DBMode         string              `json:"db_mode"` // docker, native, unknown, simulated
	Files          []BackupInfo        `json:"files"`
	Schedule       BackupSchedule      `json:"schedule"`
	Offsite        OffsiteState        `json:"offsite"`
	RestoreTest    RestoreTestSchedule `json:"restore_test"`
	History        []BackupRun         `json:"history"` // newest first
}

type BackupRequest struct {
	Passphrase string `json:"passphrase"`
	UseStored  bool   `json:"use_stored,omitempty"` // encrypt with the passphrase of the schedule (Passphrase empty)
	Actor      string `json:"actor,omitempty"`
}

type ScheduleRequest struct {
	Enabled         bool   `json:"enabled"`
	Hour            int    `json:"hour"`
	Minute          int    `json:"minute"`
	Keep            int    `json:"keep"`
	KeepWeekly      int    `json:"keep_weekly"`
	KeepMonthly     int    `json:"keep_monthly"`
	Passphrase      string `json:"passphrase"`       // empty keeps the stored one
	ClearPassphrase bool   `json:"clear_passphrase"` // true removes it (scheduled backups unencrypted)
}

// OffsiteRequest sets (or tests) the offsite target; empty secrets keep the stored ones for the same place.
type OffsiteRequest struct {
	Config offsite.Config `json:"config"`
}

// OffsiteTestResult is the result of a connection test: a probe file written, read back and deleted.
type OffsiteTestResult struct {
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
	HostKey string `json:"host_key,omitempty"` // SFTP: the fingerprint the server presented
}

type RestoreTestRequest struct {
	Enabled bool   `json:"enabled"`
	Day     int    `json:"day"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	Source  string `json:"source"`
}

// RunRequest starts an offsite synchronisation or a restore test now.
type RunRequest struct {
	Actor  string `json:"actor,omitempty"`
	Source string `json:"source,omitempty"` // restore test: offsite or local (default: the schedule's)
}
