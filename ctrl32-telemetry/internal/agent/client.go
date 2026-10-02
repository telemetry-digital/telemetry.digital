package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// Client talks to the agent over its Unix socket. A zero socket path means "no agent" (Windows, development).
type Client struct {
	socket string
	http   *http.Client
}

var ErrNoAgent = errors.New("server agent is not configured")

func NewClient(socket string) *Client {
	c := &Client{socket: socket}
	if socket != "" {
		c.http = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		}}
	}
	return c
}

func (c *Client) Configured() bool { return c != nil && c.socket != "" }

// CodedError is a refusal of the agent with a machine-readable code (updates_ended, …) the server passes on.
type CodedError struct {
	Status int
	Code   string
	Msg    string
}

func (e *CodedError) Error() string { return e.Msg }

func (c *Client) do(ctx context.Context, method, path string, in, out any, timeout time.Duration) error {
	if !c.Configured() {
		return ErrNoAgent
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://agent"+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct{ Error, Code string }
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			if e.Code != "" {
				return &CodedError{Status: resp.StatusCode, Code: e.Code, Msg: e.Error}
			}
			return errors.New(e.Error)
		}
		return fmt.Errorf("agent returned %d", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *Client) Status(ctx context.Context) (*Status, error) {
	var st Status
	if err := c.do(ctx, "GET", "/status", nil, &st, 20*time.Second); err != nil {
		return nil, err
	}
	return &st, nil
}

func (c *Client) Restart(ctx context.Context, unit string) (*Result, error) {
	var r Result
	err := c.do(ctx, "POST", "/restart", RestartRequest{Unit: unit}, &r, 60*time.Second)
	return &r, err
}

func (c *Client) SetCaddy(ctx context.Context, in CaddyRequest) (*Result, error) {
	var r Result
	err := c.do(ctx, "PUT", "/caddy", in, &r, 15*time.Minute) // may build a Caddy image with the DNS module
	return &r, err
}

func (c *Client) WireGuard(ctx context.Context) (*WireGuardState, error) {
	var st WireGuardState
	if err := c.do(ctx, "GET", "/wireguard", nil, &st, 20*time.Second); err != nil {
		return nil, err
	}
	return &st, nil
}

func (c *Client) SetWireGuard(ctx context.Context, in WireGuardRequest) (*WireGuardState, error) {
	var st WireGuardState
	err := c.do(ctx, "PUT", "/wireguard", in, &st, 5*time.Minute) // may apt-get install wireguard-tools
	return &st, err
}

// ApplyVPN sends the complete VPN state (peers and compiled access rules); the agent applies the interface and the
// nftables table together and rolls both back when either fails.
func (c *Client) ApplyVPN(ctx context.Context, d vpn.Desired) (*WireGuardState, error) {
	var st WireGuardState
	err := c.do(ctx, "PUT", "/vpn", d, &st, 2*time.Minute)
	return &st, err
}

func (c *Client) Update(ctx context.Context, version, updatesUntil string) (*UpdateResult, error) {
	var r UpdateResult
	err := c.do(ctx, "POST", "/update", UpdateRequest{Version: version, UpdatesUntil: updatesUntil}, &r, 10*time.Minute)
	return &r, err
}

func (c *Client) Backups(ctx context.Context) (*BackupsState, error) {
	var st BackupsState
	if err := c.do(ctx, "GET", "/backups", nil, &st, 20*time.Second); err != nil {
		return nil, err
	}
	return &st, nil
}

func (c *Client) CreateBackup(ctx context.Context, passphrase string) (*BackupInfo, error) {
	var r BackupInfo
	err := c.do(ctx, "POST", "/backups", BackupRequest{Passphrase: passphrase}, &r, 35*time.Minute)
	return &r, err
}

func (c *Client) DeleteBackup(ctx context.Context, name string) (*Result, error) {
	var r Result
	err := c.do(ctx, "DELETE", "/backups/"+name, nil, &r, 30*time.Second)
	return &r, err
}

func (c *Client) SetBackupSchedule(ctx context.Context, in ScheduleRequest) (*BackupsState, error) {
	var st BackupsState
	err := c.do(ctx, "PUT", "/backups/schedule", in, &st, 30*time.Second)
	return &st, err
}
