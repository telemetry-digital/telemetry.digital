package httpapi

// Online activation of the white-label license (0.59.0, ADR-0016). The ctrl32 licence bought on ctrl32.com (39 EUR,
// Paddle) unlocks either the ctrl32 editor or white labeling on one server. The administrator pastes the order number;
// the server sends it with its host name (http.base_url) to the licence worker (license.activation_url, default
// https://lic.ctrl32.com/telemetry/activate) and installs the license token it returns, verified like a pasted one:
// inside the 14-day refund window a 30-day license signed with the worker's ONLINE key (the binary accepts online-key
// licenses of at most 31 days only), afterwards the perpetual license minted offline with the master key. The order
// number is stored, and a background job re-activates daily while the installed license expires within 20 days; a
// refusal (refunded, order used for the editor, host limit) stops it and is shown on the License page. A pasted token
// (offline installation) keeps working next to this.
//
//	POST /api/v1/server/license/activate   system.admin: {"order","reason"}
//	POST /api/v1/server/license/renew      system.admin: {"reason"} asks the service now (the job does it daily)

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/license"
)

const (
	licenseRenewWithin = 20 * 24 * time.Hour // renew while the installed license expires within this
	licenseRenewEvery  = 23 * time.Hour      // at most one attempt a day (the job ticks hourly)
	licenseRenewTick   = time.Hour
	activationTimeout  = 20 * time.Second
)

// activation is the stored order number and the outcome of the last request (core.license_activation).
type activation struct {
	Order       string     `json:"order"`
	Host        string     `json:"host"`
	Kind        *string    `json:"kind"`
	ActivatedAt time.Time  `json:"activated_at"`
	ActivatedBy string     `json:"activated_by"`
	LastAttempt *time.Time `json:"last_attempt"`
	LastOK      *time.Time `json:"last_ok"`
	LastError   *string    `json:"last_error"`
	Refused     bool       `json:"refused"`
}

func (s *Server) loadActivation(ctx context.Context) (*activation, error) {
	var a activation
	err := s.pool.QueryRow(ctx, `SELECT order_ref, host, kind, activated_at, activated_by, last_attempt, last_ok, last_error, refused
		FROM core.license_activation WHERE id = 1`).Scan(&a.Order, &a.Host, &a.Kind, &a.ActivatedAt, &a.ActivatedBy, &a.LastAttempt, &a.LastOK, &a.LastError, &a.Refused)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// activationHost is the host name this server is reached at (http.base_url without the port), sent to the service and
// written into the license as its only domain.
func (s *Server) activationHost() string {
	u, err := url.Parse(s.cfg.HTTP.BaseURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

// activationError is an unsuccessful answer of the licence service. A refusal (the order cannot give this server a
// license) stops the automatic renewal; other errors (network, service busy) are retried the next day.
type activationError struct {
	Status    int    // HTTP status of the service, 0 = not reached
	Code      string // error code of the service (unknown-order, refunded, claimed-by-editor, host-cap, …)
	Refusal   bool
	Untrusted bool // the service answered with a token this server does not accept (tampering or misconfiguration)
	Msg       string
}

func (e *activationError) Error() string { return e.Msg }

var activationRefusals = map[string]string{
	"bad-request":          "the licence service refused the request (check http.base_url; it gives the server name)",
	"unknown-order":        "the licence service does not know this order number (check it; a payment can take a few minutes to arrive)",
	"unpaid":               "the order is not paid yet",
	"refunded":             "the order was refunded, so it no longer gives a license",
	"claimed-by-editor":    "this order was already used for the ctrl32 editor; one ctrl32 licence unlocks either the editor or white labeling on one server",
	"claimed-by-telemetry": "this order is already used for ctrl32 telemetry",
	"host-cap":             "this order is already activated on the maximum number of other servers",
}

// activationReply is the answer of POST /telemetry/activate.
type activationReply struct {
	License string `json:"license"`
	Kind    string `json:"kind"`
	Error   string `json:"error"`
}

// fetchActivation asks the licence service for a license of order on host and verifies the answer with keys (limits:
// max-days of online keys). It returns the parsed license, the token and the kind the service reported.
func fetchActivation(ctx context.Context, client *http.Client, endpoint, userAgent, order, host string, keys map[string]ed25519.PublicKey, limits map[string]int) (*license.License, string, string, error) {
	body, _ := json.Marshal(map[string]string{"order": order, "host": host})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", "", &activationError{Msg: "license.activation_url: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", &activationError{Msg: "the licence service is not reachable: " + err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var rep activationReply
	_ = json.Unmarshal(raw, &rep)
	if resp.StatusCode != http.StatusOK {
		e := &activationError{Status: resp.StatusCode, Code: rep.Error}
		if msg, ok := activationRefusals[rep.Error]; ok && resp.StatusCode >= 400 && resp.StatusCode < 500 {
			e.Refusal, e.Msg = true, msg
		} else if rep.Error == "pending" {
			e.Msg = "the perpetual license is being prepared; the server asks again tomorrow"
		} else {
			e.Msg = fmt.Sprintf("the licence service answered HTTP %d %s", resp.StatusCode, rep.Error)
		}
		return nil, "", "", e
	}
	tok := strings.Join(strings.Fields(rep.License), "")
	l, err := license.ParseLimited(tok, keys, limits)
	if err != nil {
		return nil, "", "", &activationError{Status: 200, Untrusted: true, Msg: "the licence service returned a license this server does not accept: " + err.Error()}
	}
	if !l.Has(license.WhiteLabel, host, time.Now()) {
		c := l.Check(host, time.Now())
		return nil, "", "", &activationError{Status: 200, Untrusted: true, Msg: "the licence service returned a license not usable on this server: " + strings.TrimSpace(c.Problem+" (white_label needed)")}
	}
	return l, tok, rep.Kind, nil
}

func (s *Server) requestActivation(ctx context.Context, order, host string) (*license.License, string, string, error) {
	return fetchActivation(ctx, &http.Client{Timeout: activationTimeout}, s.cfg.License.ActivationURL, "ctrl32-telemetry/"+s.version,
		order, host, license.TrustedKeys(), license.KeyLimits())
}

// betterLicense reports whether the new license should replace the installed one: the automatic renewal never
// replaces a valid perpetual license and never shortens a license.
func betterLicense(cur, next *license.License, host string, now time.Time) bool {
	if cur == nil || !cur.Check(host, now).Valid || !cur.Has(license.WhiteLabel, host, now) {
		return true
	}
	if cur.Perpetual() {
		return false
	}
	return next.Perpetual() || next.Expires.After(cur.Expires)
}

// renewDue reports whether the daily job should ask the service now.
func renewDue(a *activation, cur *license.License, now time.Time) bool {
	if a == nil || a.Refused {
		return false
	}
	if cur != nil && (cur.Perpetual() || cur.Expires.Sub(now) > licenseRenewWithin) {
		return false
	}
	return a.LastAttempt == nil || now.Sub(*a.LastAttempt) >= licenseRenewEvery
}

// storeLicense installs a verified token as the server's license.
func (s *Server) storeLicense(ctx context.Context, token, by string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO core.product_license (id, token, installed_at, installed_by) VALUES (1, $1, now(), $2)
		ON CONFLICT (id) DO UPDATE SET token = EXCLUDED.token, installed_at = now(), installed_by = EXCLUDED.installed_by`, token, by)
	s.dropLicenseCache()
	return err
}

func (s *Server) recordAttempt(ctx context.Context, err error, kind string) {
	var msg *string
	refused := false
	if err != nil {
		m := err.Error()
		msg = &m
		var ae *activationError
		refused = errors.As(err, &ae) && ae.Refusal
	}
	var k *string
	if kind == "starter" || kind == "permanent" {
		k = &kind
	}
	if _, e := s.pool.Exec(ctx, `UPDATE core.license_activation SET last_attempt = now(), last_error = $1, refused = $2,
		last_ok = CASE WHEN $1::text IS NULL THEN now() ELSE last_ok END, kind = coalesce($3, kind) WHERE id = 1`, msg, refused, k); e != nil {
		s.log.Error("license activation state not stored", "err", e)
	}
}

func validOrderRef(o string) bool {
	if o == "" || len(o) > 128 {
		return false
	}
	for _, c := range o {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

func activationStatus(err error) int {
	var ae *activationError
	if errors.As(err, &ae) && ae.Refusal {
		return http.StatusConflict
	}
	return http.StatusBadGateway
}

func (s *Server) activationFailed(r *http.Request, err error, order string) {
	var ae *activationError
	if errors.As(err, &ae) && ae.Untrusted {
		s.securityEvent(r, "license.rejected", map[string]any{"source": "activation", "error": err.Error()})
		return
	}
	s.log.Warn("license activation failed", "order", order, "err", err)
}

func (s *Server) activateLicense(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Order  string `json:"order"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Order = strings.TrimSpace(in.Order)
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	if !validOrderRef(in.Order) {
		writeErr(w, 400, "order: the order number from the purchase confirmation (up to 128 characters)")
		return
	}
	host := s.activationHost()
	if host == "" {
		writeErr(w, 400, "http.base_url has no host name; the license is issued for it")
		return
	}
	l, tok, kind, err := s.requestActivation(r.Context(), in.Order, host)
	if err != nil {
		s.activationFailed(r, err, in.Order)
		writeErr(w, activationStatus(err), err.Error())
		return
	}
	cur := s.currentLicense(r.Context()).lic
	now := time.Now()
	if cur != nil && cur.Perpetual() && cur.Has(license.WhiteLabel, host, now) && !l.Perpetual() {
		writeErr(w, 409, "a perpetual license is already installed on this server; remove it first to use another order")
		return
	}
	by := "system"
	if p := PrincipalFrom(r.Context()); p != nil {
		by = p.Username
	}
	old := s.licenseJSON(r.Context())
	if err := s.storeLicense(r.Context(), tok, by); err != nil {
		writeErr(w, 500, "license not stored")
		return
	}
	if kind != "starter" && kind != "permanent" {
		kind = "starter"
		if l.Perpetual() {
			kind = "permanent"
		}
	}
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.license_activation (id, order_ref, host, kind, activated_at, activated_by, last_attempt, last_ok, last_error, refused)
		VALUES (1, $1, $2, $3, now(), $4, now(), now(), NULL, false)
		ON CONFLICT (id) DO UPDATE SET order_ref = EXCLUDED.order_ref, host = EXCLUDED.host, kind = EXCLUDED.kind, activated_at = now(),
		activated_by = EXCLUDED.activated_by, last_attempt = now(), last_ok = now(), last_error = NULL, refused = false`, in.Order, host, kind, by); err != nil {
		s.log.Error("license activation not stored", "err", err)
	}
	nowJ := s.licenseJSON(r.Context())
	newA := auditLicense(nowJ)
	newA["order"], newA["kind"] = in.Order, kind
	s.audit(r, "license.activate", "license", l.ID, auditLicense(old), newA, in.Reason)
	s.log.Info("license activated", "order", in.Order, "host", host, "kind", kind, "id", l.ID)
	writeJSON(w, 200, nowJ)
}

// renewLicense asks the service for the stored order and installs the answer when it is better than the installed
// license. It returns whether a new license was installed.
func (s *Server) renewLicense(ctx context.Context, a *activation, by string) (*license.License, bool, error) {
	host := s.activationHost()
	l, tok, kind, err := s.requestActivation(ctx, a.Order, host)
	if err != nil {
		s.recordAttempt(ctx, err, "")
		return nil, false, err
	}
	installed := false
	if betterLicense(s.currentLicense(ctx).lic, l, host, time.Now()) {
		if err := s.storeLicense(ctx, tok, by); err != nil {
			s.recordAttempt(ctx, errors.New("license not stored"), "")
			return nil, false, err
		}
		installed = true
	}
	s.recordAttempt(ctx, nil, kind)
	return l, installed, nil
}

func (s *Server) renewLicenseNow(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	a, err := s.loadActivation(r.Context())
	if err != nil {
		writeErr(w, 500, "activation not readable")
		return
	}
	if a == nil {
		writeErr(w, 409, "no order number is stored; activate with the order number first")
		return
	}
	by := "system"
	if p := PrincipalFrom(r.Context()); p != nil {
		by = p.Username
	}
	old := s.licenseJSON(r.Context())
	l, installed, err := s.renewLicense(r.Context(), a, by)
	if err != nil {
		s.activationFailed(r, err, a.Order)
		writeErr(w, activationStatus(err), err.Error())
		return
	}
	nowJ := s.licenseJSON(r.Context())
	if installed {
		newA := auditLicense(nowJ)
		newA["order"] = a.Order
		s.audit(r, "license.renew", "license", l.ID, auditLicense(old), newA, in.Reason)
	}
	nowJ["renewed"] = installed
	writeJSON(w, 200, nowJ)
}

// licenseRenewLoop is the daily automatic renewal. It logs what it does; the audit trail gets an entry only when it
// installs the perpetual license (the 30-day renewals would only add noise).
func (s *Server) licenseRenewLoop() {
	time.Sleep(time.Minute)
	for {
		s.renewTick(time.Now())
		time.Sleep(licenseRenewTick)
	}
}

func (s *Server) renewTick(now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a, err := s.loadActivation(ctx)
	if err != nil || a == nil {
		return
	}
	if !renewDue(a, s.currentLicense(ctx).lic, now) {
		return
	}
	l, installed, err := s.renewLicense(ctx, a, "auto-renew")
	if err != nil {
		var ae *activationError
		if errors.As(err, &ae) && ae.Untrusted {
			s.log.Warn("security event", "security", true, "event", "license.rejected", "source", "auto-renew", "error", err.Error())
		} else if errors.As(err, &ae) && ae.Refusal {
			s.log.Warn("license auto-renew refused; it stops until the next manual activation", "order", a.Order, "err", err)
		} else {
			s.log.Warn("license auto-renew failed; retrying tomorrow", "order", a.Order, "err", err)
		}
		return
	}
	s.log.Info("license auto-renew", "order", a.Order, "installed", installed, "id", l.ID, "perpetual", l.Perpetual())
	if installed && l.Perpetual() {
		e := audit.Entry{ActorType: "system", ActorName: "auto-renew", Action: "license.renew", EntityType: "license:" + l.ID,
			New: map[string]any{"id": l.ID, "licensee": l.Licensee, "order": a.Order, "perpetual": true}, Reason: "automatic renewal: the perpetual license was issued"}
		if _, _, err := audit.Append(ctx, s.pool, e); err != nil {
			s.log.Error("audit append failed", "security", true, "action", "license.renew", "err", err)
		}
	}
}
