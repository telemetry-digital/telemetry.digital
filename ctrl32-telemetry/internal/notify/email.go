// Package notify delivers alarm notifications: e-mail (SMTP), web push, webhooks and an HTTP SMS gateway (URS C-06).
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wneessen/go-mail"
	maillog "github.com/wneessen/go-mail/log"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

// Notifier sends messages on the configured channels.
type Notifier struct {
	mu     sync.RWMutex
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

// SetSMTP replaces the outgoing e-mail settings (System → Server → E-mail (SMTP), 0.76.0) without a restart.
func (n *Notifier) SetSMTP(c config.SMTPConfig) {
	n.mu.Lock()
	n.cfg = c
	n.mu.Unlock()
}

// SMTP returns the settings in use.
func (n *Notifier) SMTP() config.SMTPConfig {
	n.mu.RLock()
	defer n.mu.RUnlock()
	c := n.cfg
	c.To = append([]string(nil), n.cfg.To...)
	return c
}

// Configured reports whether an SMTP host and the static alarm recipients are set.
func (n *Notifier) Configured() bool {
	c := n.SMTP()
	return c.Host != "" && len(c.To) > 0
}

// Available reports whether an SMTP host is configured (recipient lists are not needed for direct mail).
func (n *Notifier) Available() bool {
	c := n.SMTP()
	return c.Host != "" && c.From != ""
}

// Send delivers an e-mail to the static recipient list. Returns (delivered, error).
func (n *Notifier) Send(ctx context.Context, subject, body string) (bool, error) {
	c := n.SMTP()
	if c.Host == "" || len(c.To) == 0 {
		n.log.Info("notification (smtp not configured, logged only)", "subject", subject)
		return false, nil
	}
	m, err := NewMessage(c, c.To, subject, body)
	if err != nil {
		return false, fmt.Errorf("notify: %w", err)
	}
	if err := sendMsg(ctx, c, m, nil); err != nil {
		return false, fmt.Errorf("notify: send: %w", err)
	}
	return true, nil
}

// Security values of the SMTP connection.
const (
	SecuritySTARTTLS = "starttls"
	SecurityTLS      = "tls"
	SecurityNone     = "none"
)

// NewMessage starts a plain-text message with the sender (and its display name), the Reply-To and the recipients.
func NewMessage(c config.SMTPConfig, to []string, subject, body string) (*mail.Msg, error) {
	m := mail.NewMsg()
	var err error
	if c.FromName != "" {
		err = m.FromFormat(c.FromName, c.From)
	} else {
		err = m.From(c.From)
	}
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	if c.ReplyTo != "" {
		if err := m.ReplyTo(c.ReplyTo); err != nil {
			return nil, fmt.Errorf("reply-to: %w", err)
		}
	}
	if err := m.To(to...); err != nil {
		return nil, fmt.Errorf("to: %w", err)
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextPlain, body)
	return m, nil
}

// clientOptions maps the settings to the mail client: implicit TLS, mandatory STARTTLS, no encryption, or (empty
// security, as before 0.76.0) implicit TLS on port 465 and opportunistic STARTTLS elsewhere.
func clientOptions(c config.SMTPConfig) []mail.Option {
	port := c.Port
	if port == 0 {
		port = 587
		if c.Security == SecurityTLS {
			port = 465
		}
	}
	opts := []mail.Option{mail.WithPort(port), mail.WithTimeout(20 * time.Second)}
	switch {
	case c.Security == SecurityTLS || (c.Security == "" && port == 465):
		opts = append(opts, mail.WithSSL())
	case c.Security == SecuritySTARTTLS:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	case c.Security == SecurityNone:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSOpportunistic))
	}
	if c.User != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover), mail.WithUsername(c.User), mail.WithPassword(c.Password))
	}
	return opts
}

// sendMsg connects with the settings and sends one message; a transcript, when given, receives the conversation (the
// authentication data is redacted by the mail client).
func sendMsg(ctx context.Context, c config.SMTPConfig, m *mail.Msg, tr *Transcript) error {
	if c.Host == "" {
		return errors.New("smtp not configured")
	}
	opts := clientOptions(c)
	if tr != nil {
		opts = append(opts, mail.WithDebugLog(), mail.WithLogger(tr))
	}
	cl, err := mail.NewClient(c.Host, opts...)
	if err != nil {
		return err
	}
	return cl.DialAndSendWithContext(ctx, m)
}

// Transcript collects the SMTP conversation of a test message (System → Server → E-mail (SMTP) → Send test e-mail).
type Transcript struct {
	mu    sync.Mutex
	lines []string
	size  int
}

const maxTranscript = 32 << 10

func (t *Transcript) add(prefix string, l maillog.Log) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.size > maxTranscript {
		return
	}
	dir := "S: "
	if l.Direction == maillog.DirClientToServer {
		dir = "C: "
	}
	s := strings.TrimRight(fmt.Sprintf(l.Format, l.Messages...), "\r\n")
	for _, line := range strings.Split(s, "\n") {
		line = prefix + dir + strings.TrimRight(line, "\r")
		t.lines = append(t.lines, line)
		t.size += len(line)
	}
}

func (t *Transcript) Debugf(l maillog.Log) { t.add("", l) }
func (t *Transcript) Infof(l maillog.Log)  { t.add("", l) }
func (t *Transcript) Warnf(l maillog.Log)  { t.add("warning ", l) }
func (t *Transcript) Errorf(l maillog.Log) { t.add("error ", l) }

// Note adds a line of our own (connection target, result).
func (t *Transcript) Note(s string) {
	t.mu.Lock()
	t.lines = append(t.lines, "-- "+s)
	t.mu.Unlock()
}

// String is the whole conversation.
func (t *Transcript) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n")
}

// TestSend sends a test message with the given settings and returns the SMTP conversation (also on failure).
func TestSend(ctx context.Context, c config.SMTPConfig, to, subject, body string) (string, error) {
	tr := &Transcript{}
	sec := c.Security
	if sec == "" {
		sec = "automatic"
	}
	tr.Note(fmt.Sprintf("connecting to %s port %d (security: %s)", c.Host, portOf(c), sec))
	m, err := NewMessage(c, []string{to}, subject, body)
	if err != nil {
		tr.Note("message refused: " + err.Error())
		return tr.String(), err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := sendMsg(ctx, c, m, tr); err != nil {
		tr.Note("failed: " + err.Error())
		return tr.String(), err
	}
	tr.Note("delivered to the SMTP server")
	return tr.String(), nil
}

func portOf(c config.SMTPConfig) int {
	if c.Port != 0 {
		return c.Port
	}
	if c.Security == SecurityTLS {
		return 465
	}
	return 587
}
