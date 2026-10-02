package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

type ctxKey int

const (
	principalKey ctxKey = iota
	requestIDKey
	badCredentialKey // set when an Authorization header was present but invalid
	viaMCPKey        // set on requests an AI assistant makes through /mcp (mcp.go)
)

const sessionCookie = "ctrl32_session"

// PrincipalFrom returns the authenticated principal, if any.
func PrincipalFrom(ctx context.Context) *iam.Principal {
	p, _ := ctx.Value(principalKey).(*iam.Principal)
	return p
}

// RequestID returns the request id set by the middleware.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// requestID assigns an id to every request (echoed in X-Request-Id, logged, written to the audit trail).
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// authenticate resolves a session cookie or a bearer token into a principal (no rejection here).
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Device endpoints (/api/v1/d/…) authenticate with device tokens inside their handlers.
		if strings.HasPrefix(r.URL.Path, "/api/v1/d/") {
			next.ServeHTTP(w, r)
			return
		}
		var p *iam.Principal
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
			if pp, err := s.iam.AuthenticateToken(r.Context(), strings.TrimSpace(ah[7:])); err == nil && (pp.GrantID == "" || r.URL.Path == mcpPath) {
				p = pp
			} else {
				// an access token issued to an AI client over OAuth is valid only at /mcp (its audience)
				// An explicit but invalid credential is never downgraded to anonymous access.
				s.log.Warn("invalid api token", "security", true, "remote", clientIP(r), "path", r.URL.Path)
				r = r.WithContext(context.WithValue(r.Context(), badCredentialKey, true))
			}
		} else if c, err := r.Cookie(sessionCookie); err == nil {
			if pp, err := s.iam.Authenticate(r.Context(), c.Value); err == nil {
				p = pp
			}
		}
		if p != nil {
			r = r.WithContext(context.WithValue(r.Context(), principalKey, p))
		}
		next.ServeHTTP(w, r)
	})
}

// require enforces authentication, permission, CSRF for cookie sessions, and the must-change/MFA gates.
func (s *Server) require(perm string, fn http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFrom(r.Context())
		readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead
		if p == nil {
			bad, _ := r.Context().Value(badCredentialKey).(bool)
			if readOnly && perm == "data.read" && s.cfg.HTTP.PublicRead && !bad {
				fn(w, r)
				return
			}
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !readOnly && p.SessionID != "" {
			// Cookie-authenticated mutation: require the per-session CSRF token (double submit via header).
			if r.Header.Get("X-CSRF-Token") != p.CSRFSecret {
				s.log.Warn("csrf token missing or wrong", "security", true, "user", p.Username, "path", r.URL.Path)
				writeErr(w, http.StatusForbidden, "csrf token required")
				return
			}
		}
		path := r.URL.Path
		if p.MustChange && !strings.HasPrefix(path, "/api/v1/auth/") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "password change required", "code": "password_change_required"})
			return
		}
		if p.MFARequired && !p.MFAEnabled && p.SessionID != "" && !strings.HasPrefix(path, "/api/v1/auth/") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "multi-factor authentication must be set up", "code": "mfa_setup_required"})
			return
		}
		if p.ReadOnly && !readOnly {
			// an AI client the user approved for reading only (spec 022)
			writeErr(w, http.StatusForbidden, "this AI access is read-only")
			return
		}
		if !permOK(p, perm) {
			s.log.Warn("permission denied", "security", true, "user", p.Username, "perm", perm, "path", path)
			writeErr(w, http.StatusForbidden, "permission denied: "+perm)
			return
		}
		fn(w, r)
	})
}

// permOK: perm "" is any signed-in user, "a|b" any of the permissions (e.g. "system.admin|vpn.manage": the server
// administrator or the organization's VPN manager; the handler then limits the latter to the own organization).
func permOK(p *iam.Principal, perm string) bool {
	if perm == "" {
		return true
	}
	for _, one := range strings.Split(perm, "|") {
		if p.Has(one) {
			return true
		}
	}
	return false
}

// audit records a change performed by the current principal (URS I-01).
func (s *Server) audit(r *http.Request, action, entityType, entityID string, old, new any, reason string) {
	p := PrincipalFrom(r.Context())
	if entityID != "" && !uuidRe.MatchString(strings.ToLower(entityID)) {
		// the trail keys entities by uuid; a name ("caddy", a backup file) goes with the type so the entry is never lost
		entityType, entityID = entityType+":"+entityID, ""
	}
	e := audit.Entry{Action: action, EntityType: entityType, EntityID: entityID, Old: old, New: new, Reason: reason, RequestID: RequestID(r.Context())}
	if p != nil {
		e.OrgID, e.ActorID, e.ActorName = p.OrgID, p.UserID, p.Username
		e.ActorType = "user"
		if p.TokenID != "" {
			e.ActorType = "api_token"
			e.ActorName = p.Username + " (token " + p.TokenID[:8] + ")"
		}
		switch {
		case p.GrantID != "":
			e.ActorName = p.Username + " (AI assistant " + p.ClientName + " via MCP)"
		case r.Context().Value(viaMCPKey) != nil:
			e.ActorName += " via MCP"
		}
	} else {
		e.ActorType, e.ActorName = "system", "anonymous"
	}
	if ci := changeFrom(r.Context()); ci != nil {
		e.Reason = reason + " (approved change " + ci.ID + " requested by " + ci.RequestedBy + ")"
	}
	if _, _, err := audit.Append(r.Context(), s.pool, e); err != nil {
		s.log.Error("audit append failed", "security", true, "action", action, "err", err)
	}
}

// setSessionCookie writes the session cookie with the hardened attributes.
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true,
		Secure: s.isSecure(r), SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) isSecure(r *http.Request) bool {
	return r.TLS != nil || s.cfg.Profile == "internet" || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clientIP(r *http.Request) string {
	// Behind the local reverse proxy (Caddy on the same host) trust X-Forwarded-For's first hop.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "127.0.0.1" || host == "::1" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	return host
}

func decodeJSON(r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.New("invalid JSON body (or larger than the limit)")
	}
	// encoding/json replaces invalid UTF-8 with U+FFFD instead of failing; records must not be altered silently.
	if !utf8.Valid(body) {
		return errors.New("body must be valid UTF-8 (use \\uXXXX escapes for characters the client cannot encode)")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body")
	}
	return nil
}

// --- a tiny in-memory rate limiter (per key token bucket) for login and provisioning --------------

type limiter struct {
	mu     sync.Mutex
	rate   float64 // tokens per second
	burst  float64
	bucket map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(perMinute, burst int) *limiter {
	l := &limiter{rate: float64(perMinute) / 60, burst: float64(burst), bucket: map[string]*bucket{}}
	go func() {
		for range time.Tick(5 * time.Minute) {
			l.mu.Lock()
			for k, b := range l.bucket {
				if time.Since(b.last) > 10*time.Minute {
					delete(l.bucket, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.bucket[key]
	now := time.Now()
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.bucket[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// peek reports whether key has a token left without taking it (limits that count failures only).
func (l *limiter) peek(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.bucket[key]
	if !ok {
		return true
	}
	return b.tokens+time.Since(b.last).Seconds()*l.rate >= 1
}
