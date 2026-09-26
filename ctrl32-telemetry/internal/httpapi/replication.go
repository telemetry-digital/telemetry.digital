package httpapi

// Redundant database (spec 003): pairing codes, the replica list with state and lag, mode and limit changes, safe
// detach, drop; the endpoints the replica's own server uses (pair by code, tunnel, commands); and the node role for
// fencing checks. The logic lives in internal/replication.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/replication"
)

func (s *Server) replErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, replication.ErrBadCode):
		writeErr(w, http.StatusForbidden, err.Error())
	case errors.Is(err, replication.ErrVersion), errors.Is(err, replication.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, replication.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, replication.ErrConflict), errors.Is(err, replication.ErrMaxReplic):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		s.log.Error("replication", "err", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (s *Server) replActor(r *http.Request) replication.Actor {
	p := PrincipalFrom(r.Context())
	a := replication.Actor{RequestID: RequestID(r.Context()), Type: "user"}
	if p != nil {
		a.OrgID, a.UserID, a.Name = p.OrgID, p.UserID, p.Username
		if p.TokenID != "" {
			a.Type = "api_token"
		}
	}
	return a
}

// GET /api/v1/replication
func (s *Server) replicationStatus(w http.ResponseWriter, r *http.Request) {
	node, err := replication.Node(r.Context(), s.pool)
	if err != nil {
		s.replErr(w, err)
		return
	}
	list, err := s.repl.List(r.Context())
	if err != nil {
		s.replErr(w, err)
		return
	}
	pending, err := s.repl.Pending(r.Context())
	if err != nil {
		s.replErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"node": node, "replicas": list, "pending": pending, "modes": replication.Modes, "max_replicas": replication.MaxReplicas})
}

// POST /api/v1/replication/pairings {mode, name, lag_limit_s, reason}
func (s *Server) createPairing(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode, Name, Reason string
		LagLimitS          int `json:"lag_limit_s"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	p := PrincipalFrom(r.Context())
	code, exp, err := s.repl.CreatePairing(r.Context(), p.UserID, in.Mode, in.Name, in.LagLimitS)
	if err != nil {
		s.replErr(w, err)
		return
	}
	s.audit(r, "replica.pairing", "replica", "", nil, map[string]any{"mode": in.Mode, "name": in.Name, "expires_at": exp}, in.Reason)
	writeJSON(w, 201, map[string]any{"code": code, "expires_at": exp, "mode": in.Mode, "name": in.Name})
}

// POST /api/v1/replication/pair — the replica's CLI with a one-time code (no session; rate-limited).
func (s *Server) pairReplica(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.pairLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many pairing attempts; wait a minute")
		return
	}
	var in replication.PairRequest
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	res, err := s.repl.Pair(r.Context(), in)
	if err != nil {
		if errors.Is(err, replication.ErrBadCode) {
			s.log.Warn("replica pairing refused: wrong or expired code", "security", true, "ip", ip, "hostname", in.Hostname)
			s.audit(r, "replica.pair_refused", "replica", "", nil, map[string]any{"ip": ip, "hostname": in.Hostname, "reason": err.Error()}, "")
		}
		s.replErr(w, err)
		return
	}
	writeJSON(w, 201, res)
}

// replicaFromToken authenticates the replica's own server.
func (s *Server) replicaFromToken(w http.ResponseWriter, r *http.Request) *replication.Replica {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	rp, err := s.repl.ReplicaByToken(r.Context(), tok)
	if err != nil {
		if errors.Is(err, replication.ErrNotFound) {
			s.log.Warn("replication: unknown or revoked replica token", "security", true, "ip", clientIP(r), "path", r.URL.Path)
			writeErr(w, http.StatusUnauthorized, "unknown or revoked replica token")
			return nil
		}
		s.replErr(w, err)
		return nil
	}
	return rp
}

// GET /api/v1/replication/tunnel (Upgrade: ctrl32-pgrepl)
func (s *Server) replicationTunnel(w http.ResponseWriter, r *http.Request) {
	rp := s.replicaFromToken(w, r)
	if rp == nil {
		return
	}
	s.repl.Touch(r.Context(), rp.ID)
	replication.ServeTunnel(w, r, s.repl.DialDB, s.log)
}

// GET /api/v1/replication/agent — the pending command (long poll up to 25 s).
func (s *Server) replicationAgentPoll(w http.ResponseWriter, r *http.Request) {
	rp := s.replicaFromToken(w, r)
	if rp == nil {
		return
	}
	s.repl.Touch(r.Context(), rp.ID)
	deadline := time.Now().Add(25 * time.Second)
	for {
		cmd, err := s.repl.Command(r.Context(), rp.ID)
		if err != nil {
			s.replErr(w, err)
			return
		}
		if cmd != nil || time.Now().After(deadline) {
			writeJSON(w, 200, map[string]any{"command": cmd})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// POST /api/v1/replication/agent {status?, result?}
func (s *Server) replicationAgentReport(w http.ResponseWriter, r *http.Request) {
	rp := s.replicaFromToken(w, r)
	if rp == nil {
		return
	}
	var in struct {
		Status map[string]any            `json:"status"`
		Result *replication.DetachReport `json:"result"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.repl.Touch(r.Context(), rp.ID)
	if in.Result != nil {
		out, err := s.repl.FinishDetach(r.Context(), rp, *in.Result)
		if err != nil {
			s.replErr(w, err)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// PATCH /api/v1/replication/replicas/{id} {mode?, lag_limit_s?, max_retained_mb?, standby_url?, reason}
func (s *Server) patchReplica(w http.ResponseWriter, r *http.Request) {
	var in struct {
		replication.Settings
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	id := r.PathValue("id")
	old, new, err := s.repl.Update(r.Context(), id, in.Settings)
	if err != nil {
		s.replErr(w, err)
		return
	}
	s.audit(r, "replica.update", "replica", id, old, new, in.Reason)
	rp, err := s.repl.Get(r.Context(), id)
	if err != nil {
		s.replErr(w, err)
		return
	}
	writeJSON(w, 200, rp)
}

// POST /api/v1/replication/replicas/{id}/detach {reason, accept_partial}
func (s *Server) detachReplica(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason        string `json:"reason"`
		AcceptPartial bool   `json:"accept_partial"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	rp, err := s.repl.StartDetach(r.Context(), r.PathValue("id"), in.Reason, in.AcceptPartial, s.replActor(r))
	if err != nil {
		s.replErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, rp)
}

// DELETE /api/v1/replication/replicas/{id}?reason=
func (s *Server) dropReplica(w http.ResponseWriter, r *http.Request) {
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	id := r.PathValue("id")
	rp, err := s.repl.Drop(r.Context(), id)
	if err != nil {
		s.replErr(w, err)
		return
	}
	s.audit(r, "replica.drop", "replica", id, nil, map[string]any{"name": rp.Name, "mode": rp.Mode}, reason)
	writeJSON(w, 200, rp)
}

// GET /api/v1/replication/replicas/{id}/events?limit=
func (s *Server) replicaEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	ev, err := s.repl.Events(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		s.replErr(w, err)
		return
	}
	writeJSON(w, 200, ev)
}

// GET /api/v1/replication/role — who this node is (no authentication; used by fencing checks).
func (s *Server) replicationRole(w http.ResponseWriter, r *http.Request) {
	n, err := replication.Node(r.Context(), s.pool)
	if err != nil {
		s.replErr(w, err)
		return
	}
	writeJSON(w, 200, n)
}
