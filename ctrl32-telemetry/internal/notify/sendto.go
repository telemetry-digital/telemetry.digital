package notify

import (
	"context"
	"errors"

	"github.com/wneessen/go-mail"
)

// Available reports whether an SMTP host is configured (recipient lists are not needed for direct mail).
func (n *Notifier) Available() bool { return n.cfg.Host != "" && n.cfg.From != "" }

// SendTo delivers one plain-text message to a single recipient (password reset links, account request notices).
func (n *Notifier) SendTo(ctx context.Context, to, subject, body string) error {
	if !n.Available() {
		return errors.New("smtp not configured")
	}
	m := mail.NewMsg()
	if err := m.From(n.cfg.From); err != nil {
		return err
	}
	if err := m.To(to); err != nil {
		return err
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextPlain, body)
	opts := []mail.Option{mail.WithPort(n.cfg.Port), mail.WithTLSPolicy(mail.TLSOpportunistic)}
	if n.cfg.User != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover), mail.WithUsername(n.cfg.User), mail.WithPassword(n.cfg.Password))
	}
	c, err := mail.NewClient(n.cfg.Host, opts...)
	if err != nil {
		return err
	}
	return c.DialAndSendWithContext(ctx, m)
}
