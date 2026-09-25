// Package push delivers Web Push notifications (URS C-06): one VAPID key pair per server, subscriptions per user
// and browser, encrypted payloads (RFC 8291) through the browser's push service. A push service answering
// 404/410 retires the subscription; other failures are recorded and retried on the next notification.
package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

// Subscription is one browser of one user.
type Subscription struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Endpoint   string     `json:"endpoint"`
	UserAgent  string     `json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	FailedAt   *time.Time `json:"failed_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	p256dh     string
	auth       string
}

// Manager holds the keys and sends notifications.
type Manager struct {
	pool       *pgxpool.Pool
	log        *slog.Logger
	subscriber string
	client     *http.Client
	mu         sync.Mutex
	pub, priv  string
}

// New creates the manager; subscriber is the VAPID contact (a URL or mailto:).
func New(pool *pgxpool.Pool, log *slog.Logger, subscriber string) *Manager {
	if !strings.HasPrefix(subscriber, "http") && !strings.HasPrefix(subscriber, "mailto:") {
		subscriber = "https://telemetry.digital/"
	}
	return &Manager{pool: pool, log: log.With("component", "push"), subscriber: subscriber, client: &http.Client{Timeout: 15 * time.Second}}
}

// Keys returns the server's VAPID key pair, generating and storing it on first use.
func (m *Manager) Keys(ctx context.Context) (pub, priv string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pub != "" {
		return m.pub, m.priv, nil
	}
	err = m.pool.QueryRow(ctx, `SELECT public_key, private_key FROM core.web_push_key WHERE id = 1`).Scan(&pub, &priv)
	if err != nil {
		priv, pub, err = webpush.GenerateVAPIDKeys()
		if err != nil {
			return "", "", err
		}
		if _, err = m.pool.Exec(ctx, `INSERT INTO core.web_push_key (id, public_key, private_key) VALUES (1, $1, $2) ON CONFLICT (id) DO NOTHING`, pub, priv); err != nil {
			return "", "", err
		}
		// another instance may have won the race: read back what is stored
		if err = m.pool.QueryRow(ctx, `SELECT public_key, private_key FROM core.web_push_key WHERE id = 1`).Scan(&pub, &priv); err != nil {
			return "", "", err
		}
		m.log.Info("web push VAPID key pair created")
	}
	m.pub, m.priv = pub, priv
	return pub, priv, nil
}

func decodeKey(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// Subscribe stores a browser subscription for the user. The same endpoint registered again refreshes the keys
// (created = false) and keeps its label unless a new one is given; an endpoint moving to another user is revoked
// there first. fallbackAgent (the request's User-Agent) labels a new subscription that names none.
func (m *Manager) Subscribe(ctx context.Context, org, userID, endpoint, p256dh, auth, userAgent, fallbackAgent string) (id string, created bool, err error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(endpoint) > 2048 {
		return "", false, errors.New("endpoint must be an http(s) URL of the push service")
	}
	if b, err := decodeKey(p256dh); err != nil || len(b) != 65 || b[0] != 4 {
		return "", false, errors.New("keys.p256dh must be the browser's uncompressed P-256 public key (base64url)")
	}
	if b, err := decodeKey(auth); err != nil || len(b) != 16 {
		return "", false, errors.New("keys.auth must be the browser's 16-byte auth secret (base64url)")
	}
	if len(userAgent) > 200 {
		userAgent = userAgent[:200]
	}
	if len(fallbackAgent) > 200 {
		fallbackAgent = fallbackAgent[:200]
	}
	var existingID, existingUser string
	if err := m.pool.QueryRow(ctx, `SELECT id::text, user_id::text FROM core.push_subscription WHERE endpoint = $1 AND revoked_at IS NULL`, endpoint).Scan(&existingID, &existingUser); err == nil {
		if existingUser == userID {
			_, err = m.pool.Exec(ctx, `UPDATE core.push_subscription SET p256dh = $2, auth = $3, user_agent = COALESCE(NULLIF($4, ''), user_agent), failed_at = NULL WHERE id = $1::uuid`, existingID, p256dh, auth, userAgent)
			return existingID, false, err
		}
		_, _ = m.pool.Exec(ctx, `UPDATE core.push_subscription SET revoked_at = now() WHERE id = $1::uuid`, existingID)
	}
	if userAgent == "" {
		userAgent = fallbackAgent
	}
	err = m.pool.QueryRow(ctx, `INSERT INTO core.push_subscription (org_id, user_id, endpoint, p256dh, auth, user_agent) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6) RETURNING id::text`,
		org, userID, endpoint, p256dh, auth, userAgent).Scan(&id)
	return id, true, err
}

// List returns the user's subscriptions (keys are never returned).
func (m *Manager) List(ctx context.Context, org, userID string) ([]Subscription, error) {
	rows, err := m.pool.Query(ctx, `SELECT id::text, user_id::text, endpoint, user_agent, created_at, last_used_at, failed_at, revoked_at FROM core.push_subscription
		WHERE org_id = $1::uuid AND user_id = $2::uuid AND (revoked_at IS NULL OR revoked_at > now() - interval '7 days') ORDER BY revoked_at NULLS FIRST, created_at DESC`, org, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.UserAgent, &s.CreatedAt, &s.LastUsedAt, &s.FailedAt, &s.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Unsubscribe revokes one of the user's subscriptions.
func (m *Manager) Unsubscribe(ctx context.Context, org, userID, id string) (bool, error) {
	tag, err := m.pool.Exec(ctx, `UPDATE core.push_subscription SET revoked_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND user_id = $3::uuid AND revoked_at IS NULL`, id, org, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

type payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag,omitempty"`
}

func payloadFor(msg notify.Message) []byte {
	p := payload{Title: msg.Subject, Body: msg.Text, URL: "/"}
	if len(p.Body) > 240 {
		p.Body = p.Body[:240] + "…"
	}
	switch {
	case msg.Alarm != nil:
		if id, _ := msg.Alarm["id"].(string); id != "" {
			p.Tag = "alarm-" + id
		}
	case msg.Rule != nil:
		p.URL = "/assets/automation"
		if id, _ := msg.Rule["id"].(string); id != "" {
			p.Tag = "rule-" + id
		}
	case msg.Event == "incident":
		p.URL = "/incidents"
		p.Tag = "incident"
	case msg.Event == "test":
		p.URL = "/account/push"
		p.Tag = "test"
	}
	b, _ := json.Marshal(p)
	return b
}

// Send delivers the message to the browsers of the named users ("all" = every user of the organization).
func (m *Manager) Send(ctx context.Context, org string, users []string, msg notify.Message) (sent, failed int, err error) {
	all := false
	names := make([]string, 0, len(users))
	for _, u := range users {
		if u == "all" {
			all = true
		} else {
			names = append(names, u)
		}
	}
	rows, err := m.pool.Query(ctx, `SELECT s.id::text, s.endpoint, s.p256dh, s.auth FROM core.push_subscription s JOIN core.user_account u ON u.id = s.user_id
		WHERE s.org_id = $1::uuid AND s.revoked_at IS NULL AND ($2 OR u.username = ANY($3))`, org, all, names)
	if err != nil {
		return 0, 0, err
	}
	subs, err := scanSubs(rows)
	if err != nil {
		return 0, 0, err
	}
	return m.deliver(ctx, subs, msg)
}

// SendToUser delivers to one user's browsers (test button).
func (m *Manager) SendToUser(ctx context.Context, org, userID string, msg notify.Message) (sent, failed int, err error) {
	rows, err := m.pool.Query(ctx, `SELECT id::text, endpoint, p256dh, auth FROM core.push_subscription WHERE org_id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL`, org, userID)
	if err != nil {
		return 0, 0, err
	}
	subs, err := scanSubs(rows)
	if err != nil {
		return 0, 0, err
	}
	return m.deliver(ctx, subs, msg)
}

func scanSubs(rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}) ([]Subscription, error) {
	defer rows.Close()
	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.Endpoint, &s.p256dh, &s.auth); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (m *Manager) deliver(ctx context.Context, subs []Subscription, msg notify.Message) (sent, failed int, err error) {
	if len(subs) == 0 {
		return 0, 0, nil
	}
	pub, priv, err := m.Keys(ctx)
	if err != nil {
		return 0, 0, err
	}
	body := payloadFor(msg)
	for _, s := range subs {
		resp, perr := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{Endpoint: s.Endpoint, Keys: webpush.Keys{P256dh: s.p256dh, Auth: s.auth}},
			&webpush.Options{HTTPClient: m.client, Subscriber: m.subscriber, TTL: 3600, Urgency: webpush.UrgencyHigh, VAPIDPublicKey: pub, VAPIDPrivateKey: priv})
		code := 0
		if resp != nil {
			code = resp.StatusCode
			resp.Body.Close()
		}
		switch {
		case perr == nil && code >= 200 && code < 300:
			sent++
			_, _ = m.pool.Exec(ctx, `UPDATE core.push_subscription SET last_used_at = now(), failed_at = NULL WHERE id = $1::uuid`, s.ID)
		case code == 404 || code == 410:
			failed++
			_, _ = m.pool.Exec(ctx, `UPDATE core.push_subscription SET revoked_at = now(), failed_at = now() WHERE id = $1::uuid`, s.ID)
			m.log.Info("push subscription gone, retired", "subscription", s.ID, "status", code)
		default:
			failed++
			_, _ = m.pool.Exec(ctx, `UPDATE core.push_subscription SET failed_at = now() WHERE id = $1::uuid`, s.ID)
			detail := fmt.Sprintf("status %d", code)
			if perr != nil {
				detail = perr.Error()
			}
			m.log.Warn("push delivery failed", "subscription", s.ID, "detail", detail)
		}
	}
	return sent, failed, nil
}
