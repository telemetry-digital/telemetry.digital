package notify

import (
	"bytes"
	"context"
	"errors"

	"github.com/wneessen/go-mail"
)

// Attachment is a file sent with an e-mail (scheduled reports, spec 029).
type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

// SendWithAttachments delivers one plain-text message with attachments to a single recipient.
func (n *Notifier) SendWithAttachments(ctx context.Context, to, subject, body string, files []Attachment) error {
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
	for _, f := range files {
		ct := f.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		if err := m.AttachReader(f.Name, bytes.NewReader(f.Data), mail.WithFileContentType(mail.ContentType(ct))); err != nil {
			return err
		}
	}
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
