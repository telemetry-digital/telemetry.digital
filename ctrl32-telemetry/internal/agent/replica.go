package agent

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/replication"
)

// ReplicaPromoteResult is the new identity of a promoted replica.
type ReplicaPromoteResult struct {
	NewNode string `json:"new_node"`
}

// hReplicaPromote promotes the local standby (spec 003): a closed operation — it acts only on the replica recorded
// in replica.json next to the configuration and restores the local server's own database passwords from the
// configuration. The server calls it while executing the primary's detach command.
func (a *Server) hReplicaPromote(w http.ResponseWriter, r *http.Request) {
	st, err := replication.ReadState(a.cfgPath)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if st == nil {
		writeErr(w, 409, "this machine is not a replica")
		return
	}
	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	node, err := st.Host.Promote(ctx, cfg.DB.URL, cfg.DB.AdminURL)
	if err != nil {
		a.log.Error("replica promote failed", "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("replica promoted", "security", true, "new_node", node, "replica", st.ReplicaID)
	writeJSON(w, 200, ReplicaPromoteResult{NewNode: node})
}

// ReplicaPromote asks the agent to promote the local standby.
func (c *Client) ReplicaPromote(ctx context.Context) (string, error) {
	var r ReplicaPromoteResult
	if err := c.do(ctx, "POST", "/replica/promote", struct{}{}, &r, 5*time.Minute); err != nil {
		return "", err
	}
	if r.NewNode == "" {
		return "", errors.New("agent returned no node id")
	}
	return r.NewNode, nil
}
