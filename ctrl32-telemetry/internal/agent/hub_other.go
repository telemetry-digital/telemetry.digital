//go:build !windows

package agent

import "log/slog"

// newUserspaceHub: Linux uses kernel WireGuard and nftables; the userspace hub is linked on Windows only (ADR-0020).
func newUserspaceHub(*slog.Logger) userspaceHub { return nil }

// newUserspaceUplink: the same for the uplink to a hub (Linux: wg1 with nftables, uplink.go).
func newUserspaceUplink() uplinkStartFunc { return nil }
