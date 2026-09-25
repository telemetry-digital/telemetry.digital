package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/i18n"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// Organizations are the tenants of one server (URS O-01..O-05). Every user, role, device, site, template,
// translation and report belongs to exactly one organization and no query crosses that boundary; the
// principal's org_id scopes everything. Managing organizations needs system.admin.
//
//	GET   /api/v1/organizations                    list with counts and the first administrator
//	POST  /api/v1/organizations                    {slug, name, tz, locale, admin_username, admin_full_name, admin_email, reason}
//	PATCH /api/v1/organizations/{id}               {name?, tz?, locale?, disabled?, reason}
//	POST  /api/v1/organizations/{id}/reset-admin   {username, reason} -> one-time password, must be changed

var orgSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

const orgAdminSQL = `SELECT u.username FROM core.user_account u WHERE u.org_id = o.id AND u.disabled_at IS NULL AND EXISTS (
	SELECT 1 FROM core.user_role_history h JOIN core.role r ON r.id = h.role_id
	WHERE h.user_id = u.id AND r.is_admin AND h.event = 'granted'
	  AND h.ts = (SELECT max(h2.ts) FROM core.user_role_history h2 WHERE h2.user_id = u.id AND h2.role_id = h.role_id))
	ORDER BY u.id LIMIT 1`

func (s *Server) listOrganizations(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT o.id::text, o.slug, o.name, o.tz, o.locale, o.profile, o.created_at, o.disabled_at, COALESCE(o.disabled_reason,''),
		(SELECT count(*) FROM core.user_account u WHERE u.org_id = o.id AND u.disabled_at IS NULL),
		(SELECT count(*) FROM core.device d WHERE d.org_id = o.id AND d.revoked_at IS NULL),
		COALESCE((`+orgAdminSQL+`), '')
		FROM core.organization o ORDER BY o.created_at`)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, slug, name, tz, locale, profile, reason, admin string
		var created time.Time
		var disabled *time.Time
		var users, devices int64
		if err := rows.Scan(&id, &slug, &name, &tz, &locale, &profile, &created, &disabled, &reason, &users, &devices, &admin); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "slug": slug, "name": name, "tz": tz, "locale": locale, "profile": profile, "created_at": created,
			"disabled_at": disabled, "disabled_reason": reason, "users": users, "devices": devices, "admin_username": admin, "current": id == p.OrgID})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createOrganization(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct {
		Slug          string `json:"slug"`
		Name          string `json:"name"`
		TZ            string `json:"tz"`
		Locale        string `json:"locale"`
		AdminUsername string `json:"admin_username"`
		AdminFullName string `json:"admin_full_name"`
		AdminEmail    string `json:"admin_email"`
		Reason        string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Slug, in.Name, in.Reason = strings.ToLower(strings.TrimSpace(in.Slug)), strings.TrimSpace(in.Name), strings.TrimSpace(in.Reason)
	in.TZ, in.Locale = strings.TrimSpace(in.TZ), strings.ToLower(strings.TrimSpace(in.Locale))
	in.AdminUsername = strings.ToLower(strings.TrimSpace(in.AdminUsername))
	switch {
	case !orgSlugRe.MatchString(in.Slug):
		writeErr(w, 400, "slug must be 2–63 lowercase letters, digits or dashes")
		return
	case in.Name == "" || len(in.Name) > 128:
		writeErr(w, 400, "name is required (max 128 characters)")
		return
	case in.Reason == "":
		writeErr(w, 400, "reason required")
		return
	case in.AdminUsername == "":
		in.AdminUsername = in.Slug + "-admin"
	}
	if in.TZ == "" {
		in.TZ = "UTC"
	}
	if _, err := time.LoadLocation(in.TZ); err != nil {
		writeErr(w, 400, "unknown time zone")
		return
	}
	if in.Locale == "" {
		in.Locale = "en"
	}
	if !i18n.IsSupported(in.Locale) {
		writeErr(w, 400, "unsupported language")
		return
	}
	// Refuse early what would leave an organization without its administrator.
	var taken bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.user_account WHERE username = $1 OR ($2 <> '' AND lower(email) = $2))`, in.AdminUsername, strings.ToLower(in.AdminEmail)).Scan(&taken)
	if taken {
		writeErr(w, 400, "administrator username or e-mail is already used on this server")
		return
	}
	pw, err := iam.RandomPassword()
	if err != nil {
		writeErr(w, 500, "password generation failed")
		return
	}
	var orgID string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.organization (slug, name, tz, locale, profile) VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
		in.Slug, in.Name, in.TZ, in.Locale, s.cfg.Profile).Scan(&orgID); err != nil {
		if strings.Contains(err.Error(), "organization_slug_key") || strings.Contains(err.Error(), "duplicate") {
			writeErr(w, 400, "slug is already used")
			return
		}
		s.log.Error("organization insert failed", "err", err)
		writeErr(w, 500, "could not create the organization")
		return
	}
	u, err := s.iam.CreateUser(r.Context(), orgID, in.AdminUsername, in.AdminFullName, in.AdminEmail, pw, []string{"org_admin"}, p.UserID, in.Reason)
	if err != nil {
		// The organization row stays (append-only schema); it is unusable without an administrator, so say so clearly.
		s.log.Error("organization administrator creation failed", "org", orgID, "err", err)
		writeErr(w, 400, "organization created but its administrator could not be: "+err.Error())
		return
	}
	s.audit(r, "organization.create", "organization", orgID, nil, map[string]any{"slug": in.Slug, "name": in.Name, "tz": in.TZ, "locale": in.Locale, "admin": u.Username}, in.Reason)
	writeJSON(w, 201, map[string]any{"organization": map[string]any{"id": orgID, "slug": in.Slug, "name": in.Name}, "admin": u, "initial_password": pw})
}

func (s *Server) patchOrganization(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Name     *string `json:"name"`
		TZ       *string `json:"tz"`
		Locale   *string `json:"locale"`
		Disabled *bool   `json:"disabled"`
		Reason   string  `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	var slug, name, tz, locale string
	var disabled *time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT slug, name, tz, locale, disabled_at FROM core.organization WHERE id = $1::uuid`, id).Scan(&slug, &name, &tz, &locale, &disabled); err != nil {
		writeErr(w, 404, "organization not found")
		return
	}
	before := map[string]any{"name": name, "tz": tz, "locale": locale, "disabled": disabled != nil}
	if in.Name != nil {
		if n := strings.TrimSpace(*in.Name); n == "" || len(n) > 128 {
			writeErr(w, 400, "name is required (max 128 characters)")
			return
		} else {
			name = n
		}
	}
	if in.TZ != nil {
		if _, err := time.LoadLocation(strings.TrimSpace(*in.TZ)); err != nil {
			writeErr(w, 400, "unknown time zone")
			return
		}
		tz = strings.TrimSpace(*in.TZ)
	}
	if in.Locale != nil {
		if !i18n.IsSupported(strings.ToLower(*in.Locale)) {
			writeErr(w, 400, "unsupported language")
			return
		}
		locale = strings.ToLower(*in.Locale)
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET name = $2, tz = $3, locale = $4 WHERE id = $1::uuid`, id, name, tz, locale); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if in.Disabled != nil && *in.Disabled != (disabled != nil) {
		if *in.Disabled {
			if id == p.OrgID {
				writeErr(w, 400, "you cannot disable your own organization")
				return
			}
			if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET disabled_at = now(), disabled_reason = $2 WHERE id = $1::uuid`, id, in.Reason); err != nil {
				writeErr(w, 500, "update failed")
				return
			}
			// Sign everyone of that organization out; devices are refused at their next connection.
			_, _ = s.pool.Exec(r.Context(), `UPDATE core.session SET revoked_at = now() WHERE revoked_at IS NULL AND user_id IN (SELECT id FROM core.user_account WHERE org_id = $1::uuid)`, id)
		} else if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET disabled_at = NULL, disabled_reason = NULL WHERE id = $1::uuid`, id); err != nil {
			writeErr(w, 500, "update failed")
			return
		}
	}
	after := map[string]any{"name": name, "tz": tz, "locale": locale, "disabled": disabled != nil}
	if in.Disabled != nil {
		after["disabled"] = *in.Disabled
	}
	s.audit(r, "organization.update", "organization", id, before, after, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "slug": slug, "name": name, "tz": tz, "locale": locale, "disabled": after["disabled"]})
}

func (s *Server) resetOrgAdmin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct{ Username, Reason string }
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if in.Username == "" || strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "username and reason are required")
		return
	}
	var uid string
	if err := s.pool.QueryRow(r.Context(), `SELECT id::text FROM core.user_account WHERE org_id = $1::uuid AND username = $2 AND disabled_at IS NULL`, id, in.Username).Scan(&uid); err != nil {
		writeErr(w, 404, "no such active user in that organization")
		return
	}
	pw, err := iam.RandomPassword()
	if err != nil {
		writeErr(w, 500, "password generation failed")
		return
	}
	if err := s.iam.SetPassword(r.Context(), uid, pw, true, false); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.iam.RevokeSessions(r.Context(), uid)
	s.audit(r, "user.password_reset", "user", uid, nil, map[string]any{"by": "system administrator", "organization": id}, in.Reason)
	writeJSON(w, 200, map[string]any{"username": in.Username, "initial_password": pw})
}
