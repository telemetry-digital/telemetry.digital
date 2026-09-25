package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/i18n"
)

// UI languages and translations.
//
//	GET  /api/v1/i18n/languages            public: supported languages, enabled set and default of the instance/org
//	PUT  /api/v1/i18n/languages            content.write: {"enabled": [...], "default": "en", "reason"}
//	GET  /api/v1/i18n/{locale}             public: merged catalog (built-in + organization overrides) {key: text}
//	GET  /api/v1/i18n/{locale}/edit        content.write: [{key, en, builtin, override}]
//	PUT  /api/v1/i18n/{locale}             content.write: {"entries": {key: text}, "reason"}; empty text removes the override
//	PUT  /api/v1/auth/locale               signed in: {"locale": "sk"} user preference

type langSettings struct {
	Enabled []string `json:"enabled"`
	Default string   `json:"default"`
}

func (s *Server) langSettingsOf(r *http.Request, orgID string) langSettings {
	ls := langSettings{Enabled: []string{"en"}, Default: "en"}
	if orgID == "" {
		return ls
	}
	var raw []byte
	if err := s.pool.QueryRow(r.Context(), `SELECT settings->'languages' FROM core.organization WHERE id = $1::uuid`, orgID).Scan(&raw); err != nil || raw == nil {
		return ls
	}
	var st langSettings
	if json.Unmarshal(raw, &st) == nil {
		if len(st.Enabled) > 0 {
			ls.Enabled = st.Enabled
		}
		if i18n.IsSupported(st.Default) {
			ls.Default = st.Default
		}
	}
	has := false
	for _, c := range ls.Enabled {
		if c == "en" {
			has = true
		}
	}
	if !has {
		ls.Enabled = append([]string{"en"}, ls.Enabled...)
	}
	return ls
}

func (s *Server) getLanguages(w http.ResponseWriter, r *http.Request) {
	orgID, _ := s.brandingOrg(r)
	ls := s.langSettingsOf(r, orgID)
	writeJSON(w, 200, map[string]any{"supported": i18n.Languages(), "enabled": ls.Enabled, "default": ls.Default})
}

func (s *Server) putLanguages(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled []string `json:"enabled"`
		Default string   `json:"default"`
		Reason  string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	enabled := []string{"en"}
	for _, c := range in.Enabled {
		if !i18n.IsSupported(c) {
			writeErr(w, 400, "unsupported language "+c)
			return
		}
		if c != "en" {
			enabled = append(enabled, c)
		}
	}
	if in.Default == "" {
		in.Default = "en"
	}
	found := false
	for _, c := range enabled {
		if c == in.Default {
			found = true
		}
	}
	if !found {
		writeErr(w, 400, "default language must be enabled")
		return
	}
	old := s.langSettingsOf(r, org)
	st, _ := json.Marshal(langSettings{Enabled: enabled, Default: in.Default})
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET settings = jsonb_set(settings, '{languages}', $2::jsonb, true) WHERE id = $1::uuid`, org, string(st)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "languages.update", "organization", org, old, langSettings{Enabled: enabled, Default: in.Default}, in.Reason)
	writeJSON(w, 200, map[string]any{"enabled": enabled, "default": in.Default})
}

func localeOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	l := strings.ToLower(r.PathValue("locale"))
	if !i18n.IsSupported(l) {
		writeErr(w, 404, "unsupported language")
		return "", false
	}
	return l, true
}

// merged catalog: built-in of the locale, then organization overrides; missing keys fall back to English on the client
func (s *Server) getCatalog(w http.ResponseWriter, r *http.Request) {
	locale, ok := localeOf(w, r)
	if !ok {
		return
	}
	orgID, _ := s.brandingOrg(r)
	out := map[string]string{}
	for k, v := range i18n.Builtin(locale) {
		out[k] = v
	}
	if orgID != "" {
		rows, err := s.pool.Query(r.Context(), `SELECT key, text FROM core.translation WHERE org_id = $1::uuid AND locale = $2`, orgID, locale)
		if err == nil {
			for rows.Next() {
				var k, v string
				if rows.Scan(&k, &v) == nil {
					out[k] = v
				}
			}
			rows.Close()
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	writeJSON(w, 200, out)
}

func (s *Server) getCatalogEdit(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	locale, ok := localeOf(w, r)
	if !ok {
		return
	}
	overrides := map[string]string{}
	rows, err := s.pool.Query(r.Context(), `SELECT key, text FROM core.translation WHERE org_id = $1::uuid AND locale = $2`, org, locale)
	if err == nil {
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) == nil {
				overrides[k] = v
			}
		}
		rows.Close()
	}
	bi := i18n.Builtin(locale)
	keys := i18n.Keys()
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k] = true
	}
	for k := range overrides { // organization-added keys (custom strings)
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{"key": k, "builtin": bi[k], "override": overrides[k]})
	}
	writeJSON(w, 200, map[string]any{"locale": locale, "entries": out})
}

func (s *Server) putCatalog(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	locale, ok := localeOf(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Entries map[string]string `json:"entries"`
		Reason  string            `json:"reason"`
	}
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if len(in.Entries) == 0 || len(in.Entries) > 5000 {
		writeErr(w, 400, "entries required (max 5000)")
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "tx failed")
		return
	}
	defer tx.Rollback(r.Context())
	set, removed := 0, 0
	for k, v := range in.Entries {
		if len(k) == 0 || len(k) > 500 || len(v) > 2000 {
			writeErr(w, 400, "key/text too long")
			return
		}
		if strings.TrimSpace(v) == "" {
			tag, err := tx.Exec(r.Context(), `DELETE FROM core.translation WHERE org_id = $1::uuid AND locale = $2 AND key = $3`, org, locale, k)
			if err != nil {
				writeErr(w, 500, "delete failed")
				return
			}
			removed += int(tag.RowsAffected())
			continue
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO core.translation (org_id, locale, key, text, updated_by_name) VALUES ($1::uuid, $2, $3, $4, $5)
			ON CONFLICT (org_id, locale, key) DO UPDATE SET text = EXCLUDED.text, updated_by_name = EXCLUDED.updated_by_name, updated_at = now()`, org, locale, k, v, p.Username); err != nil {
			writeErr(w, 500, "upsert failed")
			return
		}
		set++
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	s.audit(r, "translation.update", "organization", org, nil, map[string]any{"locale": locale, "set": set, "removed": removed}, in.Reason)
	writeJSON(w, 200, map[string]any{"locale": locale, "set": set, "removed": removed})
}

func (s *Server) putUserLocale(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct{ Locale string }
	if err := decodeJSON(r, &in, 256); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	loc := strings.ToLower(strings.TrimSpace(in.Locale))
	if !i18n.IsSupported(loc) || !slices.Contains(s.langSettingsOf(r, p.OrgID).Enabled, loc) {
		writeErr(w, 400, "language is not enabled for this organization")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.user_account SET locale = $2 WHERE id = $1::uuid`, p.UserID, loc); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	writeJSON(w, 200, map[string]any{"locale": loc})
}
