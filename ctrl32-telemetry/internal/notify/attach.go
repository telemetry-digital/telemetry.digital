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
	c := n.SMTP()
	m, err := NewMessage(c, []string{to}, subject, body)
	if err != nil {
		return err
	}
	for _, f := range files {
		ct := f.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		if err := m.AttachReader(f.Name, bytes.NewReader(f.Data), mail.WithFileContentType(mail.ContentType(ct))); err != nil {
			return err
		}
	}
	return sendMsg(ctx, c, m, nil)
}
