// Package provision lets devices register themselves with a provisioning key and secret (spec 005 US1, research R3),
// ThingsBoard-compatible over MQTT (/provision/request) and HTTP (POST /api/v1/provision).
package provision

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
)

// Request is a ThingsBoard provisioning request.
type Request struct {
	DeviceName            string `json:"deviceName"`
	ProvisionDeviceKey    string `json:"provisionDeviceKey"`
	ProvisionDeviceSecret string `json:"provisionDeviceSecret"`
	CredentialsType       string `json:"credentialsType,omitempty"`
}

// Response is a ThingsBoard provisioning response.
type Response struct {
	Status           string `json:"status"` // SUCCESS | FAILURE
	CredentialsType  string `json:"credentialsType,omitempty"`
	CredentialsValue string `json:"credentialsValue,omitempty"`
	ErrorMsg         string `json:"errorMsg,omitempty"`
	// ctrl32 additions
	DeviceID string `json:"deviceId,omitempty"`
	Pending  bool   `json:"approvalPending,omitempty"`
}

// Service provisions devices.
type Service struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	mu    sync.Mutex
	tries map[string][]time.Time // key or ip -> recent attempts
}

// New creates the service.
func New(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, log: log, tries: map[string][]time.Time{}}
}

// Limit of attempts per key and per source address.
const perMinute = 10

func (s *Service) allow(keys ...string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if len(s.tries) > 10000 {
		s.tries = map[string][]time.Time{}
	}
	ok := true
	for _, k := range keys {
		var kept []time.Time
		for _, t := range s.tries[k] {
			if now.Sub(t) < time.Minute {
				kept = append(kept, t)
			}
		}
		if len(kept) >= perMinute {
			ok = false
		}
		s.tries[k] = append(kept, now)
	}
	return ok
}

var (
	keyRe  = regexp.MustCompile(`^pk_[0-9a-f]{20}$`)
	nameRe = regexp.MustCompile(`^[^/#+\s]{1,64}$`)
)

// NewKey returns a profile key and a secret (the secret is shown once).
func NewKey() (key, secret string, err error) {
	b := make([]byte, 42)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	return "pk_" + hex.EncodeToString(b[:10]), hex.EncodeToString(b[10:]), nil
}

// Hash is the storage form of a secret.
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func fail(msg string) Response { return Response{Status: "FAILURE", ErrorMsg: msg} }

// HandleMQTT answers a provisioning request received over MQTT.
func (s *Service) HandleMQTT(ctx context.Context, payload []byte, remote string) []byte {
	var req Request
	var resp Response
	if err := json.Unmarshal(payload, &req); err != nil {
		resp = fail("request is not JSON")
	} else {
		resp = s.Provision(ctx, req, remote, "mqtt")
	}
	b, _ := json.Marshal(resp)
	return b
}

// Provision registers a device or refuses. via is "mqtt" or "http".
func (s *Service) Provision(ctx context.Context, req Request, remote, via string) Response {
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	if !s.allow("ip:"+hostOf(remote), "key:"+req.ProvisionDeviceKey) {
		s.log.Warn("provisioning rate limited", "security", true, "remote", remote, "via", via)
		return fail("too many attempts, try again in a minute")
	}
	if req.CredentialsType != "" && req.CredentialsType != "ACCESS_TOKEN" {
		return fail("only ACCESS_TOKEN credentials are issued")
	}
	if !keyRe.MatchString(req.ProvisionDeviceKey) {
		s.log.Warn("provisioning rejected: bad key", "security", true, "remote", remote, "via", via)
		return fail("unknown provisioning key")
	}
	var p struct {
		id, org, name, secretHash, strategy, pattern, transport string
		approval, auto                                          bool
		max                                                     int
	}
	err := s.pool.QueryRow(ctx, `SELECT pp.id::text, pp.org_id::text, pp.name, pp.secret_hash, pp.strategy, pp.name_pattern, pp.transport, pp.approval, pp.auto_datastreams, pp.max_devices
		FROM core.provision_profile pp JOIN core.organization o ON o.id = pp.org_id
		WHERE pp.key = $1 AND pp.revoked_at IS NULL AND o.disabled_at IS NULL`, req.ProvisionDeviceKey).
		Scan(&p.id, &p.org, &p.name, &p.secretHash, &p.strategy, &p.pattern, &p.transport, &p.approval, &p.auto, &p.max)
	if err != nil || subtle.ConstantTimeCompare([]byte(Hash(req.ProvisionDeviceSecret)), []byte(p.secretHash)) != 1 {
		s.log.Warn("provisioning rejected: wrong key or secret", "security", true, "remote", remote, "via", via, "key", req.ProvisionDeviceKey)
		return fail("wrong provisioning key or secret")
	}
	if !nameRe.MatchString(req.DeviceName) {
		return fail("deviceName required (1–64 characters, no spaces or / # +)")
	}
	if p.pattern != "" {
		re, err := regexp.Compile(p.pattern)
		if err != nil || !re.MatchString(req.DeviceName) {
			return fail("deviceName does not match the profile's pattern")
		}
	}
	token, err := randomToken()
	if err != nil {
		return fail("internal error")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fail("internal error")
	}
	defer tx.Rollback(ctx)
	var devID string
	var hasToken, revoked bool
	err = tx.QueryRow(ctx, `SELECT d.id::text, d.revoked_at IS NOT NULL,
		EXISTS (SELECT 1 FROM core.device_credential c WHERE c.device_id = d.id AND c.kind = 'token' AND c.revoked_at IS NULL)
		FROM core.device d WHERE d.org_id = $1::uuid AND d.external_id = $2 FOR UPDATE`, p.org, req.DeviceName).Scan(&devID, &revoked, &hasToken)
	created := false
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if p.strategy == "preprovisioned" {
			s.log.Warn("provisioning rejected: device not pre-provisioned", "security", true, "remote", remote, "device", req.DeviceName)
			return fail("device is not registered for provisioning")
		}
		if p.max > 0 {
			var n int
			_ = tx.QueryRow(ctx, `SELECT count(*) FROM core.device WHERE provision_profile_id = $1::uuid`, p.id).Scan(&n)
			if n >= p.max {
				return fail("the provisioning profile reached its device limit")
			}
		}
		var approval *string
		if p.approval {
			a := "pending"
			approval = &a
		}
		if err := tx.QueryRow(ctx, `INSERT INTO core.device (org_id, external_id, name, transport, auto_datastreams, approval, provision_profile_id)
			VALUES ($1::uuid, $2, $2, $3, $4, $5, $6::uuid) RETURNING id::text`, p.org, req.DeviceName, p.transport, p.auto, approval, p.id).Scan(&devID); err != nil {
			s.log.Error("provisioning: device insert failed", "err", err)
			return fail("device could not be created")
		}
		created = true
	case err != nil:
		return fail("internal error")
	case revoked:
		return fail("device is revoked")
	case hasToken:
		s.log.Warn("provisioning rejected: device already provisioned", "security", true, "remote", remote, "device", req.DeviceName)
		return fail("device is already provisioned")
	default:
		if _, err := tx.Exec(ctx, `UPDATE core.device SET auto_datastreams = auto_datastreams OR $2, provision_profile_id = coalesce(provision_profile_id, $3::uuid) WHERE id = $1::uuid`, devID, p.auto, p.id); err != nil {
			return fail("internal error")
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO core.device_credential (device_id, kind, secret_hash) VALUES ($1::uuid, 'token', $2)`, devID, Hash(token)); err != nil {
		return fail("internal error")
	}
	if _, err := tx.Exec(ctx, `UPDATE core.device SET status = status || jsonb_build_object('provisioned_at', now(), 'provisioned_by', $2::text) WHERE id = $1::uuid`, devID, p.name); err != nil {
		return fail("internal error")
	}
	if _, _, err := audit.Append(ctx, tx, audit.Entry{OrgID: p.org, ActorType: "system", ActorName: "provisioning profile " + p.name, Action: "device.provision",
		EntityType: "device", EntityID: devID, New: map[string]any{"external_id": req.DeviceName, "created": created, "via": via, "remote": hostOf(remote), "approval": p.approval},
		Reason: "self-provisioned with profile " + p.name}); err != nil {
		return fail("internal error")
	}
	if err := tx.Commit(ctx); err != nil {
		return fail("internal error")
	}
	s.log.Info("device provisioned", "device", req.DeviceName, "profile", p.name, "created", created, "via", via)
	return Response{Status: "SUCCESS", CredentialsType: "ACCESS_TOKEN", CredentialsValue: token, DeviceID: devID, Pending: p.approval && created}
}

func randomToken() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hostOf(remote string) string {
	if i := strings.LastIndex(remote, ":"); i > 0 && !strings.HasSuffix(remote, "]") {
		return strings.Trim(remote[:i], "[]")
	}
	return remote
}
