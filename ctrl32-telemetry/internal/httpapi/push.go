package httpapi

import (
	"net/http"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

// Web Push subscriptions of the signed-in user (URS C-06):
//
//	GET    /api/v1/push/key                 {public_key}  VAPID public key for PushManager.subscribe
//	GET    /api/v1/push/subscriptions       own browsers (no keys)
//	POST   /api/v1/push/subscriptions       {endpoint, keys:{p256dh, auth}, user_agent} → 201 (200 when refreshed)
//	DELETE /api/v1/push/subscriptions/{id}
//	POST   /api/v1/push/test                sends a test notification to the caller's browsers → {sent, failed}

func (s *Server) pushAvailable(w http.ResponseWriter) bool {
	if s.push == nil {
		writeErr(w, 503, "web push is not available on this server")
		return false
	}
	return true
}

func (s *Server) pushKey(w http.ResponseWriter, r *http.Request) {
	if !s.pushAvailable(w) {
		return
	}
	pub, _, err := s.push.Keys(r.Context())
	if err != nil {
		s.log.Error("push keys", "err", err)
		writeErr(w, 500, "push keys unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"public_key": pub})
}

func (s *Server) listPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.pushAvailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	subs, err := s.push.List(r.Context(), org, p.UserID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, subs)
}

func (s *Server) createPushSubscription(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.pushAvailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	if p.TokenID != "" {
		writeErr(w, 403, "browser subscriptions are registered from a signed-in browser, not with an API token")
		return
	}
	var in struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		UserAgent      string `json:"user_agent"`
		ExpirationTime any    `json:"expirationTime"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id, created, err := s.push.Subscribe(r.Context(), org, p.UserID, in.Endpoint, in.Keys.P256dh, in.Keys.Auth, in.UserAgent, r.UserAgent())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if created {
		s.audit(r, "push.subscribe", "push_subscription", id, nil, map[string]any{"user": p.Username, "user_agent": in.UserAgent}, "browser subscribed to notifications")
	}
	code := 200
	if created {
		code = 201
	}
	writeJSON(w, code, map[string]any{"id": id, "created": created})
}

func (s *Server) deletePushSubscription(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.pushAvailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	done, err := s.push.Unsubscribe(r.Context(), org, p.UserID, id)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	if !done {
		writeErr(w, 404, "subscription not found")
		return
	}
	s.audit(r, "push.unsubscribe", "push_subscription", id, map[string]any{"user": p.Username}, nil, "browser unsubscribed")
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) pushTest(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.pushAvailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	msg := notify.Message{Event: "test", Subject: s.productName(r, "") + ": test notification", Text: "Notifications reach this browser. Alarms and rule events will arrive the same way."}
	sent, failed, err := s.push.SendToUser(r.Context(), org, p.UserID, msg)
	if err != nil {
		s.log.Error("push test", "err", err)
		writeErr(w, 500, "push failed")
		return
	}
	writeJSON(w, 200, map[string]any{"sent": sent, "failed": failed})
}
