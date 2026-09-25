package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Notification channels (URS C-06): an e-mail recipient list, an HTTP webhook signed with HMAC-SHA256, a
// generic HTTP SMS gateway (URL/body templates with {to} and {text}), a generic HTTP voice/call provider (same
// templates) and Web Push to the browsers of named users or of everyone in the organization.

// ChannelSettings is the kind-specific configuration stored in core.notification_channel.settings.
type ChannelSettings struct {
	Recipients []string          `json:"recipients,omitempty"` // email: addresses; sms_http: phone numbers
	URL        string            `json:"url,omitempty"`        // webhook, sms_http (may contain {to} and {text})
	Secret     string            `json:"secret,omitempty"`     // webhook: HMAC key; sms_http: optional bearer/API key placed in headers by the operator
	Method     string            `json:"method,omitempty"`     // sms_http: GET (default) or POST
	Body       string            `json:"body,omitempty"`       // sms_http POST body template, may contain {to} and {text}
	Headers    map[string]string `json:"headers,omitempty"`
	Users      []string          `json:"users,omitempty"` // web_push: usernames, or ["all"] for every user of the organization
}

// PushSender delivers Web Push notifications (implemented by internal/push; nil = not available).
type PushSender interface {
	Send(ctx context.Context, org string, users []string, msg Message) (sent, failed int, err error)
}

// Channel is one configured channel.
type Channel struct {
	ID, OrgID, Name, Kind string
	Settings              ChannelSettings
}

// Message is what a channel delivers.
type Message struct {
	Event   string         `json:"event"` // raised | escalated | cleared | test | rule
	Subject string         `json:"subject"`
	Text    string         `json:"text"`
	Alarm   map[string]any `json:"alarm,omitempty"`
	Rule    map[string]any `json:"rule,omitempty"` // automation rule firing: {id, name, version, transition, inputs}
	SentAt  time.Time      `json:"sent_at"`
}

// ValidateChannel parses and checks settings for the kind.
func ValidateChannel(kind string, raw json.RawMessage) (ChannelSettings, error) {
	var st ChannelSettings
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &st); err != nil {
			return st, fmt.Errorf("settings: %w", err)
		}
	}
	clean := st.Recipients[:0]
	for _, r := range st.Recipients {
		if r = strings.TrimSpace(r); r != "" {
			clean = append(clean, r)
		}
	}
	users := st.Users[:0]
	for _, u := range st.Users {
		if u = strings.TrimSpace(u); u != "" {
			users = append(users, u)
		}
	}
	st.Users = users
	st.Recipients = clean
	st.URL = strings.TrimSpace(st.URL)
	st.Method = strings.ToUpper(strings.TrimSpace(st.Method))
	switch kind {
	case "email":
		if len(st.Recipients) == 0 || len(st.Recipients) > 50 {
			return st, errors.New("email: 1..50 recipients required")
		}
		for _, r := range st.Recipients {
			if !strings.Contains(r, "@") {
				return st, fmt.Errorf("email: invalid address %q", r)
			}
		}
	case "webhook":
		u, err := url.Parse(st.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return st, errors.New("webhook: url must be http(s)://host/path")
		}
		if len(st.Secret) > 256 {
			return st, errors.New("webhook: secret too long")
		}
	case "sms_http", "voice_http":
		u, err := url.Parse(strings.NewReplacer("{to}", "x", "{text}", "x").Replace(st.URL))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return st, errors.New(kind + ": url must be http(s)://host/path with {to} and {text} placeholders")
		}
		if len(st.Recipients) == 0 || len(st.Recipients) > 50 {
			return st, errors.New(kind + ": 1..50 recipients (phone numbers) required")
		}
		switch st.Method {
		case "":
			st.Method = "GET"
		case "GET", "POST":
		default:
			return st, errors.New(kind + ": method must be GET or POST")
		}
	case "web_push":
		if len(st.Users) == 0 || len(st.Users) > 200 {
			return st, errors.New("web_push: 1..200 users required (usernames, or \"all\")")
		}
	default:
		return st, errors.New("kind must be email, webhook, sms_http, voice_http or web_push")
	}
	if len(st.Headers) > 20 {
		return st, errors.New("at most 20 headers")
	}
	return st, nil
}

// Redact hides secrets for API responses.
func (st ChannelSettings) Redact() ChannelSettings {
	if st.Secret != "" {
		st.Secret = "••••••"
	}
	return st
}

func (n *Notifier) httpClient() *http.Client {
	if n.client == nil {
		n.client = &http.Client{Timeout: 10 * time.Second}
	}
	return n.client
}

// Deliver sends the message on the channel; the error names the first failure.
func (n *Notifier) Deliver(ctx context.Context, ch Channel, msg Message) error {
	if msg.SentAt.IsZero() {
		msg.SentAt = time.Now().UTC()
	}
	switch ch.Kind {
	case "email":
		if !n.Available() {
			return errors.New("smtp not configured on this server")
		}
		var errs []string
		for _, to := range ch.Settings.Recipients {
			if err := n.SendTo(ctx, to, msg.Subject, msg.Text); err != nil {
				errs = append(errs, to+": "+err.Error())
			}
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return nil
	case "webhook":
		body, _ := json.Marshal(msg)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.Settings.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "ctrl32-telemetry")
		req.Header.Set("X-Ctrl32-Event", msg.Event)
		req.Header.Set("X-Ctrl32-Channel", ch.Name)
		if ch.Settings.Secret != "" {
			mac := hmac.New(sha256.New, []byte(ch.Settings.Secret))
			mac.Write(body)
			req.Header.Set("X-Ctrl32-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		for k, v := range ch.Settings.Headers {
			req.Header.Set(k, v)
		}
		return n.do(req)
	case "web_push":
		if n.Push == nil {
			return errors.New("web push is not available on this server")
		}
		sent, failed, err := n.Push.Send(ctx, ch.OrgID, ch.Settings.Users, msg)
		if err != nil {
			return err
		}
		if sent == 0 {
			return fmt.Errorf("no browser is subscribed for these users (%d failed)", failed)
		}
		return nil
	case "sms_http", "voice_http":
		var errs []string
		for _, to := range ch.Settings.Recipients {
			rep := strings.NewReplacer("{to}", url.QueryEscape(to), "{text}", url.QueryEscape(msg.Subject+" "+msg.Text))
			u := rep.Replace(ch.Settings.URL)
			var req *http.Request
			var err error
			if ch.Settings.Method == "POST" {
				jrep := strings.NewReplacer("{to}", jsonEscape(to), "{text}", jsonEscape(msg.Subject+" "+msg.Text))
				req, err = http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(jrep.Replace(ch.Settings.Body)))
				if err == nil {
					req.Header.Set("Content-Type", "application/json")
				}
			} else {
				req, err = http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			}
			if err != nil {
				errs = append(errs, to+": "+err.Error())
				continue
			}
			req.Header.Set("User-Agent", "ctrl32-telemetry")
			for k, v := range ch.Settings.Headers {
				req.Header.Set(k, v)
			}
			if err := n.do(req); err != nil {
				errs = append(errs, to+": "+err.Error())
			}
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return nil
	}
	return fmt.Errorf("unsupported channel kind %q", ch.Kind)
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

func (n *Notifier) do(req *http.Request) error {
	resp, err := n.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}
