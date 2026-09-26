package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Fence is written next to the configuration when this server learns that its hot standby was promoted from it
// (research R8). A fenced server never accepts writes again until it is re-attached as a replica.
type Fence struct {
	At         time.Time `json:"at"`
	ByNode     string    `json:"by_node"`
	StandbyURL string    `json:"standby_url"`
	OwnNode    string    `json:"own_node"`
}

// FencePath is the fence file for a configuration file path.
func FencePath(configPath string) string { return filepath.Join(filepath.Dir(configPath), "fenced.json") }

// ReadFence returns the fence, or nil when this server is not fenced.
func ReadFence(configPath string) *Fence {
	b, err := os.ReadFile(FencePath(configPath))
	if err != nil {
		return nil
	}
	var f Fence
	if json.Unmarshal(b, &f) != nil {
		return &Fence{At: time.Now()} // unreadable fence file: stay fenced
	}
	return &f
}

// RoleOf asks another node who it is.
func RoleOf(ctx context.Context, baseURL string) (*NodeInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/replication/role", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %d", baseURL, resp.StatusCode)
	}
	var n NodeInfo
	return &n, json.NewDecoder(resp.Body).Decode(&n)
}

// CheckFence asks every registered hot standby whether it was promoted from this node; if one was, it writes the
// fence file and returns it.
func (p *Primary) CheckFence(ctx context.Context, configPath string) (*Fence, error) {
	own, err := Node(ctx, p.Pool)
	if err != nil {
		return nil, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT standby_url FROM core.replica WHERE mode = 'standby' AND standby_url IS NOT NULL AND state NOT IN ('dropped', 'detached', 'failed')`)
	if err != nil {
		return nil, err
	}
	var urls []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			urls = append(urls, u)
		}
	}
	rows.Close()
	for _, u := range urls {
		n, err := RoleOf(ctx, u)
		if err != nil || n.Role != "primary" || n.PromotedFrom == nil || *n.PromotedFrom != own.ID {
			continue
		}
		f := &Fence{At: time.Now().UTC(), ByNode: n.ID, StandbyURL: u, OwnNode: own.ID}
		b, _ := json.MarshalIndent(f, "", "  ")
		if err := os.WriteFile(FencePath(configPath), b, 0o640); err != nil {
			return f, fmt.Errorf("write fence file: %w", err)
		}
		p.Log.Error("FENCED: the hot standby was promoted from this server; refusing writes", "security", true, "standby", u, "new_primary", n.ID)
		return f, nil
	}
	return nil, nil
}
