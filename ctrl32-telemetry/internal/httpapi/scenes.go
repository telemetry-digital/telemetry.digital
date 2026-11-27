package httpapi

// Scenes (spec 008): a named set of entity actions ("Evening": hall light on at 30 %, blinds down). Activating a scene
// issues one entity command per action through the normal command path — the user's permission, four eyes when the
// organization requires them, audit with the reason "Scene: <name>". A scene can be captured from the current states
// of chosen entities.
//
//	GET    /api/v1/scenes                    data.read
//	POST   /api/v1/scenes                    device.manage  {name, icon, room_id, actions | capture, reason}
//	PUT    /api/v1/scenes/{id}               device.manage
//	DELETE /api/v1/scenes/{id}?reason=       device.manage
//	POST   /api/v1/scenes/{id}/activate      device.command {reason?}

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/smarthome"
)

const maxSceneActions = 50

// SceneAction is one entity action of a scene.
type SceneAction struct {
	EntityID string           `json:"entity_id"`
	Service  string           `json:"service"`
	Params   smarthome.Params `json:"params"`
}

type sceneIn struct {
	Name    *string       `json:"name"`
	Icon    *string       `json:"icon"`
	RoomID  *string       `json:"room_id"`
	Actions []SceneAction `json:"actions"`
	Capture []string      `json:"capture"` // entity ids: build the actions from their current states
	Sort    *int          `json:"sort"`
	Reason  string        `json:"reason"`
}

var errSceneInvalid = errors.New("invalid scene")

// validateActions checks that each action names an entity of the organization and a service it offers.
func (s *Server) validateActions(ctx context.Context, org string, actions []SceneAction) error {
	if len(actions) == 0 || len(actions) > maxSceneActions {
		return fmt.Errorf("%w: 1 to %d actions", errSceneInvalid, maxSceneActions)
	}
	for _, a := range actions {
		var n int
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.entity WHERE id = $1::uuid AND org_id = $2::uuid AND status <> 'removed'`, a.EntityID, org).Scan(&n)
		if n == 0 {
			return fmt.Errorf("%w: entity %s not found", errSceneInvalid, a.EntityID)
		}
		if s.smart == nil {
			return errors.New("smart home is not available")
		}
		ok := false
		for _, sv := range s.smart.Services(a.EntityID) {
			ok = ok || sv == a.Service
		}
		if !ok {
			return fmt.Errorf("%w: %s is not an action of entity %s", errSceneInvalid, a.Service, a.EntityID)
		}
	}
	return nil
}

func attrNum(attrs map[string]any, k string) (float64, bool) {
	switch v := attrs[k].(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	return 0, false
}

// captureActions turns the current states of entities into actions that restore them.
func (s *Server) captureActions(ctx context.Context, org string, ids []string) ([]SceneAction, error) {
	if len(ids) == 0 || len(ids) > maxSceneActions {
		return nil, fmt.Errorf("%w: capture 1 to %d entities", errSceneInvalid, maxSceneActions)
	}
	var out []SceneAction
	for _, id := range ids {
		var comp string
		var state *string
		var attrs map[string]any
		if err := s.pool.QueryRow(ctx, `SELECT component, state, attributes FROM core.entity WHERE id = $1::uuid AND org_id = $2::uuid AND status <> 'removed'`, id, org).
			Scan(&comp, &state, &attrs); err != nil {
			return nil, fmt.Errorf("%w: entity %s not found", errSceneInvalid, id)
		}
		st := ""
		if state != nil {
			st = *state
		}
		add := func(service string, p smarthome.Params) {
			out = append(out, SceneAction{EntityID: id, Service: service, Params: p})
		}
		switch comp {
		case "switch", "fan", "siren", "humidifier":
			if st == "on" {
				add("turn_on", smarthome.Params{})
			} else {
				add("turn_off", smarthome.Params{})
			}
		case "light":
			if st != "on" {
				add("turn_off", smarthome.Params{})
				break
			}
			p := smarthome.Params{}
			if b, ok := attrNum(attrs, "brightness"); ok {
				pct := math.Max(1, math.Round(b/255*100))
				p.Brightness = &pct
			}
			mode, _ := attrs["color_mode"].(string)
			if rgb, ok := attrs["rgb_color"].([]any); ok && len(rgb) == 3 && mode != "color_temp" && mode != "brightness" && mode != "onoff" {
				for _, c := range rgb {
					f, _ := c.(float64)
					p.RGB = append(p.RGB, f)
				}
			} else if ct, ok := attrNum(attrs, "color_temp"); ok {
				p.ColorTemp = &ct
			}
			add("turn_on", p)
		case "cover", "valve":
			if pos, ok := attrNum(attrs, "position"); ok {
				add("set_position", smarthome.Params{Position: &pos})
			} else if st == "open" || st == "closed" {
				add(map[string]string{"open": "open", "closed": "close"}[st], smarthome.Params{})
			}
		case "lock":
			if st == "locked" || st == "unlocked" {
				add(map[string]string{"locked": "lock", "unlocked": "unlock"}[st], smarthome.Params{})
			}
		case "climate":
			if st != "" {
				add("set_hvac_mode", smarthome.Params{Mode: st})
			}
			if t, ok := attrNum(attrs, "temperature"); ok && st != "off" {
				add("set_temperature", smarthome.Params{Temperature: &t})
			}
		case "number":
			if f, ok := toFloat(st); ok {
				add("set_value", smarthome.Params{Value: f})
			}
		case "select":
			if st != "" {
				add("select_option", smarthome.Params{Value: st})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: none of the entities can be restored by a scene", errSceneInvalid)
	}
	return out, nil
}

func (s *Server) sceneJSON(ctx context.Context, org, id string) (map[string]any, error) {
	var name, icon string
	var room, roomName *string
	var actions []SceneAction
	var sortN int
	if err := s.pool.QueryRow(ctx, `SELECT sc.name, sc.icon, sc.room_id::text, a.name, sc.actions, sc.sort FROM core.scene sc
		LEFT JOIN core.asset a ON a.id = sc.room_id WHERE sc.id = $1::uuid AND sc.org_id = $2::uuid AND sc.removed_at IS NULL`, id, org).
		Scan(&name, &icon, &room, &roomName, &actions, &sortN); err != nil {
		return nil, err
	}
	list := []map[string]any{}
	for _, a := range actions {
		var ename, comp string
		_ = s.pool.QueryRow(ctx, `SELECT name, component FROM core.entity WHERE id = $1::uuid`, a.EntityID).Scan(&ename, &comp)
		list = append(list, map[string]any{"entity_id": a.EntityID, "entity": ename, "component": comp, "service": a.Service, "params": a.Params})
	}
	return map[string]any{"id": id, "name": name, "icon": icon, "room_id": room, "room": roomName, "actions": list, "sort": sortN}, nil
}

func (s *Server) listScenes(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	out, err := s.scenes(r.Context(), org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) scenes(ctx context.Context, org string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM core.scene WHERE org_id = $1::uuid AND removed_at IS NULL ORDER BY sort, name`, org)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	out := []map[string]any{}
	for _, id := range ids {
		if sc, err := s.sceneJSON(ctx, org, id); err == nil {
			out = append(out, sc)
		}
	}
	return out, nil
}

// sceneInput validates the request and resolves the actions (explicit or captured).
func (s *Server) sceneInput(ctx context.Context, org string, in *sceneIn, creating bool) (string, int) {
	if strings.TrimSpace(in.Reason) == "" {
		return "reason is required", 400
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len([]rune(n)) > 80 || strings.ContainsAny(n, "<>") {
			return "name: 1 to 80 characters of plain text", 400
		}
		in.Name = &n
	} else if creating {
		return "name is required", 400
	}
	if in.Icon != nil && (len(*in.Icon) > 40 || strings.ContainsAny(*in.Icon, "<>\"'")) {
		return "icon: an icon name", 400
	}
	if in.RoomID != nil && *in.RoomID != "" {
		var n int
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.asset a JOIN core.site si ON si.id = a.site_id WHERE a.id = $1::uuid AND si.org_id = $2::uuid`, *in.RoomID, org).Scan(&n)
		if n == 0 {
			return "room not found", 400
		}
	}
	if len(in.Capture) > 0 {
		acts, err := s.captureActions(ctx, org, in.Capture)
		if err != nil {
			return err.Error(), 400
		}
		in.Actions = acts
	}
	if in.Actions != nil || creating {
		if err := s.validateActions(ctx, org, in.Actions); err != nil {
			return err.Error(), 400
		}
	}
	return "", 0
}

func (s *Server) createScene(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in sceneIn
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if msg, code := s.sceneInput(r.Context(), org, &in, true); msg != "" {
		writeErr(w, code, msg)
		return
	}
	icon, room, sortN := "", "", 0
	if in.Icon != nil {
		icon = *in.Icon
	}
	if in.RoomID != nil {
		room = *in.RoomID
	}
	if in.Sort != nil {
		sortN = *in.Sort
	}
	acts, _ := json.Marshal(in.Actions)
	by := "system"
	if p := PrincipalFrom(r.Context()); p != nil {
		by = p.Username
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.scene (org_id, name, icon, room_id, actions, sort, created_by)
		VALUES ($1::uuid, $2, $3, nullif($4, '')::uuid, $5::jsonb, $6, $7) RETURNING id::text`, org, *in.Name, icon, room, string(acts), sortN, by).Scan(&id); err != nil {
		writeErr(w, 500, "scene not stored")
		return
	}
	s.audit(r, "scene.create", "scene", id, nil, map[string]any{"name": *in.Name, "actions": in.Actions}, in.Reason)
	sc, _ := s.sceneJSON(r.Context(), org, id)
	writeJSON(w, 201, sc)
}

func (s *Server) updateScene(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	old, err := s.sceneJSON(r.Context(), org, id)
	if err != nil {
		writeErr(w, 404, "scene not found")
		return
	}
	var in sceneIn
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if msg, code := s.sceneInput(r.Context(), org, &in, false); msg != "" {
		writeErr(w, code, msg)
		return
	}
	var acts *string
	if in.Actions != nil {
		b, _ := json.Marshal(in.Actions)
		v := string(b)
		acts = &v
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.scene SET name = coalesce($3, name), icon = coalesce($4, icon),
		room_id = CASE WHEN $5::boolean THEN nullif($6, '')::uuid ELSE room_id END, actions = coalesce($7::jsonb, actions), sort = coalesce($8, sort), updated_at = now()
		WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, in.Name, in.Icon, in.RoomID != nil, deref(in.RoomID), acts, in.Sort); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	sc, _ := s.sceneJSON(r.Context(), org, id)
	s.audit(r, "scene.update", "scene", id, old, sc, in.Reason)
	writeJSON(w, 200, sc)
}

func (s *Server) deleteScene(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if strings.TrimSpace(reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	id := r.PathValue("id")
	old, err := s.sceneJSON(r.Context(), org, id)
	if err != nil {
		writeErr(w, 404, "scene not found")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.scene SET removed_at = now() WHERE id = $1::uuid AND org_id = $2::uuid`, id, org)
	s.audit(r, "scene.delete", "scene", id, old, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// activateScene issues the scene's actions as entity commands of the signed-in user.
func (s *Server) activateScene(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &in, 4096); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	id := r.PathValue("id")
	var name string
	var actions []SceneAction
	if err := s.pool.QueryRow(r.Context(), `SELECT name, actions FROM core.scene WHERE id = $1::uuid AND org_id = $2::uuid AND removed_at IS NULL`, id, org).
		Scan(&name, &actions); err != nil {
		writeErr(w, 404, "scene not found")
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "Scene: " + name
	}
	s.opLog(r, "scene", "scene", id, name, map[string]any{"actions": len(actions)}, reason)
	results := []map[string]any{}
	failed := 0
	for _, a := range actions {
		body, _ := json.Marshal(map[string]any{"service": a.Service, "value": a.Params.Value, "brightness": a.Params.Brightness, "rgb": a.Params.RGB,
			"color_temp": a.Params.ColorTemp, "position": a.Params.Position, "temperature": a.Params.Temperature, "mode": a.Params.Mode,
			"percentage": a.Params.Percentage, "reason": reason})
		r2 := r.Clone(r.Context())
		r2.Body = io.NopCloser(bytes.NewReader(body))
		r2.ContentLength = int64(len(body))
		r2.SetPathValue("id", a.EntityID)
		rec := httptest.NewRecorder()
		s.entityCommand(rec, r2)
		var res map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		item := map[string]any{"entity_id": a.EntityID, "service": a.Service, "http_status": rec.Code}
		if rec.Code >= 300 {
			failed++
			item["error"] = res["error"]
		} else {
			item["command_id"], item["status"] = res["id"], res["status"]
			if e, _ := res["error"].(string); e != "" {
				item["error"] = e
			}
		}
		results = append(results, item)
	}
	code := 200
	if failed == len(actions) {
		code = 409
	}
	writeJSON(w, code, map[string]any{"scene": id, "name": name, "results": results, "failed": failed})
}

// ---- flows ----------------------------------------------------------------------------------------------------------

// SceneActivate runs a scene's actions as commands with the flow as origin.
func (h *flowHost) SceneActivate(ctx context.Context, org, sceneID, flowID, issuer, reason string) (int, int, error) {
	var name string
	var actions []SceneAction
	if err := h.s.pool.QueryRow(ctx, `SELECT name, actions FROM core.scene WHERE id = $1::uuid AND org_id = $2::uuid AND removed_at IS NULL`, sceneID, org).
		Scan(&name, &actions); err != nil {
		return 0, 0, errors.New("scene not found")
	}
	ok, failed := 0, 0
	for _, a := range actions {
		b, _ := json.Marshal(a.Params)
		var params map[string]any
		_ = json.Unmarshal(b, &params)
		if _, status, _, err := h.EntityCommand(ctx, org, a.EntityID, a.Service, params, flowID, issuer, reason+" (scene "+name+")"); err != nil || status == "failed" {
			failed++
		} else {
			ok++
		}
	}
	return ok, failed, nil
}

func (s *Server) sceneName(ctx context.Context, id string) string {
	var n string
	if s.pool.QueryRow(ctx, `SELECT name FROM core.scene WHERE id = $1::uuid`, id).Scan(&n) != nil {
		return id
	}
	return n
}
