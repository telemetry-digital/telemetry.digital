package notify

import (
	"context"
	"errors"
)

// SendTo delivers one plain-text message to a single recipient (password reset links, account request notices).
func (n *Notifier) SendTo(ctx context.Context, to, subject, body string) error {
	if !n.Available() {
		return errors.New("smtp not configured")
	}
	c := n.SMTP()
	m, err := NewMessage(c, []string{to}, subject, body)
	if err != nil {
		return err
	}
	return sendMsg(ctx, c, m, nil)
}
