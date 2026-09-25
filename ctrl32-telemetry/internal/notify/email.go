// Package notify delivers alarm notifications. PoC: SMTP only; SMS/push/voice/webhook follow in phase 1–2 (URS C-06).
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/wneessen/go-mail"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

// Notifier sends messages on the configured channels.
type Notifier struct {
	cfg    config.SMTPConfig
	log    *slog.Logger
	client *http.Client
	// Push delivers web_push channels; set by main when the push package is wired.
	Push PushSender
}

// New returns a notifier; if SMTP is not configured, Send only logs.
func New(cfg config.SMTPConfig, log *slog.Logger) *Notifier {
	return &Notifier{cfg: cfg, log: log.With("component", "notify")}
}

// Configured reports whether an SMTP host is set.
func (n *Notifier) Configured() bool { return n.cfg.Host != "" && len(n.cfg.To) > 0 }

// Send delivers an e-mail to the static recipient list. Returns (delivered, error).
func (n *Notifier) Send(ctx context.Context, subject, body string) (bool, error) {
	if !n.Configured() {
		n.log.Info("notification (smtp not configured, logged only)", "subject", subject)
		return false, nil
	}
	m := mail.NewMsg()
	if err := m.From(n.cfg.From); err != nil {
		return false, fmt.Errorf("notify: from: %w", err)
	}
	if err := m.To(n.cfg.To...); err != nil {
		return false, fmt.Errorf("notify: to: %w", err)
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextPlain, body)

	opts := []mail.Option{mail.WithPort(n.cfg.Port), mail.WithTLSPolicy(mail.TLSMandatory)}
	if n.cfg.User != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover), mail.WithUsername(n.cfg.User), mail.WithPassword(n.cfg.Password))
	}
	c, err := mail.NewClient(n.cfg.Host, opts...)
	if err != nil {
		return false, fmt.Errorf("notify: client: %w", err)
	}
	if err := c.DialAndSendWithContext(ctx, m); err != nil {
		return false, fmt.Errorf("notify: send: %w", err)
	}
	return true, nil
}
