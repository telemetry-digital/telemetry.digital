package httpapi

// OAuth 2.1 authorization server for AI assistants (spec 022, ADR-0017). The MCP endpoint is the only protected
// resource; an assistant such as assistant finds this server from the 401 of /mcp (RFC 9728), registers itself (RFC 7591),
// sends the user to the consent page with PKCE (S256 only), and exchanges the code for a one-hour access token and a
// rotating refresh token (90 days since the last use). Public clients only (no client secrets), exact redirect
// addresses (a loopback address may change its port, RFC 8252), single-use codes valid for ten minutes, a reused code
// ends its grant. Access tokens are API tokens bound to the grant and valid only at /mcp; they carry the user's
// permissions at the time of consent, cut to the current ones on every request, or only reading when the user chose
// so. Nothing works while the organization has AI access switched off.
//
//	GET  /.well-known/oauth-protected-resource[/mcp]   public   resource metadata
//	GET  /.well-known/oauth-authorization-server[/mcp] public   authorization server metadata
//	POST /oauth/register                               public   dynamic client registration (rate limited)
//	GET  /oauth/authorize                              page     consent page (sign-in first)
//	GET  /api/v1/mcp/authorize                         session  what the consent page shows
//	POST /api/v1/mcp/authorize                         session  approve or deny → {redirect}
//	POST /oauth/token                                  public   authorization_code, refresh_token
//	POST /oauth/revoke                                 public   end a refresh or access token (RFC 7009)

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

const (
	oauthAccessTTL  = time.Hour
	oauthRefreshTTL = 90 * 24 * time.Hour
	oauthCodeTTL    = 10 * time.Minute
	oauthScopeAll   = "mcp"
	oauthScopeRead  = "mcp:read"
)

var (
	pkceRe       = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
	challengeRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	clientIDRe   = regexp.MustCompile(`^c32c_[A-Za-z0-9_-]{16,64}$`)
	uriSchemeRe  = regexp.MustCompile(`^[a-z][a-z0-9+.-]{2,40}$`)
	blockSchemes = []string{"javascript", "data", "file", "vbscript", "about", "blob", "http", "https", "ws", "wss", "ftp"}
)

// publicBase is the server's public address (config http.base_url, else from the request).
func (s *Server) publicBase(r *http.Request) string {
	if b := strings.TrimRight(s.cfg.HTTP.BaseURL, "/"); b != "" {
		return b
	}
	scheme := "http"
	if s.isSecure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// validResource accepts the MCP address (or the server's address) as a resource indicator (RFC 8707): under the
// configured public address, or under the address the client reached this server with (an IP address, another name).
func (s *Server) validResource(r *http.Request, res string) bool {
	res = strings.TrimRight(res, "/")
	scheme := "http"
	if s.isSecure(r) {
		scheme = "https"
	}
	for _, b := range []string{s.publicBase(r), scheme + "://" + r.Host} {
		if res == b || res == b+mcpPath {
			return true
		}
	}
	return false
}

func oauthJSON(w http.ResponseWriter, code int, v any) {
	mcpCORS(w)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, code, v)
}

func oauthErr(w http.ResponseWriter, code int, errCode, desc string) {
	oauthJSON(w, code, S{"error": errCode, "error_description": desc})
}

// GET /.well-known/oauth-protected-resource[/mcp]
func (s *Server) oauthResourceMeta(w http.ResponseWriter, r *http.Request) {
	base := s.publicBase(r)
	mcpCORS(w)
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, 200, S{"resource": base + mcpPath, "authorization_servers": []string{base}, "bearer_methods_supported": []string{"header"},
		"scopes_supported": []string{oauthScopeAll, oauthScopeRead}, "resource_name": "ctrl32 telemetry MCP",
		"resource_documentation": base + "/api/docs"})
}

// GET /.well-known/oauth-authorization-server[/mcp]
func (s *Server) oauthServerMeta(w http.ResponseWriter, r *http.Request) {
	base := s.publicBase(r)
	mcpCORS(w)
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, 200, S{
		"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token",
		"registration_endpoint": base + "/oauth/register", "revocation_endpoint": base + "/oauth/revoke",
		"response_types_supported": []string{"code"}, "response_modes_supported": []string{"query"},
		"grant_types_supported":            []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"revocation_endpoint_auth_methods_supported":     []string{"none"},
		"scopes_supported":                               []string{oauthScopeAll, oauthScopeRead},
		"authorization_response_iss_parameter_supported": true,
		"service_documentation":                          base + "/api/docs",
	})
}

// validRedirect accepts https addresses, http only on the loopback interface, and private-use schemes of native apps.
func validRedirect(raw string) bool {
	if len(raw) > 300 || strings.ContainsAny(raw, " \t\r\n\\") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		return isLoopbackHost(u.Hostname())
	}
	return uriSchemeRe.MatchString(u.Scheme) && !slices.Contains(blockSchemes, u.Scheme)
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// redirectMatches compares a requested redirect address with a registered one: exactly, except the port of a
// loopback address (RFC 8252 §7.3).
func redirectMatches(registered, requested string) bool {
	if registered == requested {
		return true
	}
	a, err1 := url.Parse(registered)
	b, err2 := url.Parse(requested)
	if err1 != nil || err2 != nil || a.Scheme != "http" || b.Scheme != "http" || !isLoopbackHost(a.Hostname()) {
		return false
	}
	return a.Hostname() == b.Hostname() && a.EscapedPath() == b.EscapedPath() && a.RawQuery == b.RawQuery
}

func cleanName(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

// POST /oauth/register
func (s *Server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	mcpCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.oauthLimiter.allow("register " + clientIP(r)) {
		oauthErr(w, http.StatusTooManyRequests, "slow_down", "too many registrations from this address")
		return
	}
	var in struct {
		ClientName              string   `json:"client_name"`
		ClientURI               string   `json:"client_uri"`
		RedirectURIs            []string `json:"redirect_uris"`
		GrantTypes              []string `json:"grant_types"`
		ResponseTypes           []string `json:"response_types"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
		Scope                   string   `json:"scope"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil { // unknown metadata fields are ignored (RFC 7591)
		oauthErr(w, 400, "invalid_client_metadata", "a JSON object is expected")
		return
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 10 {
		oauthErr(w, 400, "invalid_redirect_uri", "one to ten redirect_uris")
		return
	}
	for _, u := range in.RedirectURIs {
		if !validRedirect(u) {
			oauthErr(w, 400, "invalid_redirect_uri", "redirect_uris must be https, http on localhost, or an app scheme: "+u)
			return
		}
	}
	if m := in.TokenEndpointAuthMethod; m != "" && m != "none" {
		oauthErr(w, 400, "invalid_client_metadata", "only public clients (token_endpoint_auth_method none, PKCE)")
		return
	}
	for _, g := range in.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			oauthErr(w, 400, "invalid_client_metadata", "grant_types: authorization_code, refresh_token")
			return
		}
	}
	for _, t := range in.ResponseTypes {
		if t != "code" {
			oauthErr(w, 400, "invalid_client_metadata", "response_types: code")
			return
		}
	}
	name := cleanName(in.ClientName, 100)
	if name == "" {
		name = "AI assistant"
	}
	curi := ""
	if u, err := url.Parse(strings.TrimSpace(in.ClientURI)); err == nil && u.Scheme == "https" && u.Host != "" && len(in.ClientURI) <= 300 {
		curi = u.String()
	}
	raw, err := iam.RandomToken(24)
	if err != nil {
		oauthErr(w, 500, "server_error", "random")
		return
	}
	clientID := "c32c_" + strings.NewReplacer("+", "-", "/", "_", "=", "").Replace(raw)
	if len(clientID) > 69 {
		clientID = clientID[:69]
	}
	var created time.Time
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.oauth_client (client_id, name, client_uri, redirect_uris, created_ip) VALUES ($1, $2, $3, $4, NULLIF($5,'')::inet) RETURNING created_at`,
		clientID, name, curi, in.RedirectURIs, ipOnlyAddr(clientIP(r))).Scan(&created); err != nil {
		oauthErr(w, 500, "server_error", "registration failed")
		return
	}
	s.log.Info("oauth client registered", "client", name, "remote", clientIP(r))
	oauthJSON(w, http.StatusCreated, S{"client_id": clientID, "client_id_issued_at": created.Unix(), "client_name": name, "client_uri": curi,
		"redirect_uris": in.RedirectURIs, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"},
		"token_endpoint_auth_method": "none", "scope": oauthScopeAll + " " + oauthScopeRead})
}

func ipOnlyAddr(s string) string {
	if ip := net.ParseIP(strings.Trim(s, "[]")); ip != nil {
		return ip.String()
	}
	return ""
}

// oauthClient is a registered client.
type oauthClient struct {
	ID, ClientID, Name, URI string
	Redirects               []string
}

func (s *Server) loadOAuthClient(ctx context.Context, clientID string) (*oauthClient, error) {
	if !clientIDRe.MatchString(clientID) {
		return nil, pgx.ErrNoRows
	}
	c := &oauthClient{}
	err := s.pool.QueryRow(ctx, `SELECT id::text, client_id, name, client_uri, redirect_uris FROM core.oauth_client WHERE client_id = $1`, clientID).
		Scan(&c.ID, &c.ClientID, &c.Name, &c.URI, &c.Redirects)
	return c, err
}

// authorizeRequest is what the consent page carries from the client's authorization request.
type authorizeRequest struct {
	ResponseType        string `json:"response_type"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	State               string `json:"state"`
	Scope               string `json:"scope"`
	Resource            string `json:"resource"`
	// POST only
	Approve  bool `json:"approve"`
	ReadOnly bool `json:"read_only"`
}

// checkAuthorize validates an authorization request. The first error is not redirectable (unknown client or
// redirect address: shown on the page); the second is sent back to the client.
func (s *Server) checkAuthorize(r *http.Request, a *authorizeRequest) (*oauthClient, string, string) {
	c, err := s.loadOAuthClient(r.Context(), a.ClientID)
	if err != nil {
		return nil, "unknown client: register the client again (remove and add the connector)", ""
	}
	if a.RedirectURI == "" && len(c.Redirects) == 1 {
		a.RedirectURI = c.Redirects[0]
	}
	ok := false
	for _, reg := range c.Redirects {
		if redirectMatches(reg, a.RedirectURI) {
			ok = true
		}
	}
	if !ok {
		return nil, "the redirect address is not registered for this client", ""
	}
	if a.ResponseType != "code" {
		return c, "", "unsupported_response_type"
	}
	if a.CodeChallengeMethod != "S256" || !challengeRe.MatchString(a.CodeChallenge) {
		return c, "", "invalid_request" // PKCE with S256 is required
	}
	if len(a.State) > 500 {
		return c, "", "invalid_request"
	}
	for _, sc := range strings.Fields(a.Scope) {
		if sc != oauthScopeAll && sc != oauthScopeRead {
			return c, "", "invalid_scope"
		}
	}
	if a.Resource != "" && !s.validResource(r, a.Resource) {
		return c, "", "invalid_target"
	}
	return c, "", ""
}

func (s *Server) oauthRedirect(r *http.Request, a *authorizeRequest, params url.Values) string {
	u, _ := url.Parse(a.RedirectURI)
	q := u.Query()
	for k, v := range params {
		q[k] = v
	}
	if a.State != "" {
		q.Set("state", a.State)
	}
	q.Set("iss", s.publicBase(r))
	u.RawQuery = q.Encode()
	return u.String()
}

func authorizeFromQuery(q url.Values) *authorizeRequest {
	return &authorizeRequest{ResponseType: q.Get("response_type"), ClientID: q.Get("client_id"), RedirectURI: q.Get("redirect_uri"),
		CodeChallenge: q.Get("code_challenge"), CodeChallengeMethod: q.Get("code_challenge_method"), State: q.Get("state"),
		Scope: q.Get("scope"), Resource: q.Get("resource")}
}

// GET /api/v1/mcp/authorize — what the consent page shows.
func (s *Server) authorizeInfo(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p.SessionID == "" {
		writeErr(w, 403, "consent needs a signed-in browser session")
		return
	}
	a := authorizeFromQuery(r.URL.Query())
	c, pageErr, clientErr := s.checkAuthorize(r, a)
	if pageErr != "" {
		writeErr(w, 400, pageErr)
		return
	}
	var orgName string
	_ = s.pool.QueryRow(r.Context(), `SELECT name FROM core.organization WHERE id = $1::uuid`, p.OrgID).Scan(&orgName)
	u, _ := url.Parse(a.RedirectURI)
	host := u.Host
	if host == "" {
		host = u.Scheme + ":"
	}
	out := S{"client_name": c.Name, "client_uri": c.URI, "redirect_host": host, "user": p.Username, "full_name": p.FullName,
		"organization": orgName, "enabled": s.mcpEnabled(r.Context(), p.OrgID), "can_enable": p.Has("user.admin"), "write_licensed": s.mcpWriteLicensed(r.Context()),
		"read_only_requested": strings.Contains(" "+a.Scope+" ", " "+oauthScopeRead+" ") && !strings.Contains(" "+a.Scope+" ", " "+oauthScopeAll+" ")}
	if clientErr != "" {
		out["error"] = clientErr
		out["redirect"] = s.oauthRedirect(r, a, url.Values{"error": {clientErr}})
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/mcp/authorize — approve or deny; answers where the browser goes next.
func (s *Server) authorizeDecide(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p.SessionID == "" {
		writeErr(w, 403, "consent needs a signed-in browser session")
		return
	}
	var a authorizeRequest
	if err := decodeJSON(r, &a, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, pageErr, clientErr := s.checkAuthorize(r, &a)
	if pageErr != "" {
		writeErr(w, 400, pageErr)
		return
	}
	if clientErr != "" {
		writeJSON(w, 200, S{"redirect": s.oauthRedirect(r, &a, url.Values{"error": {clientErr}})})
		return
	}
	if !a.Approve {
		writeJSON(w, 200, S{"redirect": s.oauthRedirect(r, &a, url.Values{"error": {"access_denied"}, "error_description": {"the user denied access"}})})
		return
	}
	if !s.mcpEnabled(r.Context(), p.OrgID) {
		writeErr(w, 403, "AI access (MCP) is switched off in your organization")
		return
	}
	scopes := make([]string, 0, len(p.Permissions))
	for k, v := range p.Permissions {
		if v {
			scopes = append(scopes, k)
		}
	}
	slices.Sort(scopes)
	raw, err := iam.RandomToken(32)
	if err != nil {
		writeErr(w, 500, "random")
		return
	}
	resource := a.Resource
	if resource == "" {
		resource = s.publicBase(r) + mcpPath
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "begin failed")
		return
	}
	defer tx.Rollback(r.Context())
	// a new approval of the same assistant replaces the user's earlier one (reconnecting leaves one entry)
	if _, err := tx.Exec(r.Context(), `UPDATE core.api_token SET revoked_at = now() WHERE revoked_at IS NULL AND grant_id IN
		(SELECT id FROM core.oauth_grant WHERE user_id = $1::uuid AND client_id = $2::uuid AND revoked_at IS NULL)`, p.UserID, c.ID); err != nil {
		writeErr(w, 500, "grant failed")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE core.oauth_grant SET revoked_at = now(), revoked_by = $1::uuid, revoked_reason = 'replaced by a new approval', refresh_hash = NULL
		WHERE user_id = $1::uuid AND client_id = $2::uuid AND revoked_at IS NULL`, p.UserID, c.ID); err != nil {
		writeErr(w, 500, "grant failed")
		return
	}
	var grantID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.oauth_grant (org_id, user_id, client_id, scopes, read_only) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5) RETURNING id::text`,
		p.OrgID, p.UserID, c.ID, scopes, a.ReadOnly).Scan(&grantID); err != nil {
		writeErr(w, 500, "grant failed")
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO core.oauth_code (code_hash, grant_id, redirect_uri, challenge, resource, expires_at) VALUES ($1, $2::uuid, $3, $4, $5, now() + $6::interval)`,
		iam.HashToken(raw), grantID, a.RedirectURI, a.CodeChallenge, resource, oauthCodeTTL.String()); err != nil {
		writeErr(w, 500, "code failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	s.audit(r, "mcp_grant.create", "oauth_grant", grantID, nil, S{"client": c.Name, "redirect": a.RedirectURI, "read_only": a.ReadOnly}, "approved by the user on the consent page")
	writeJSON(w, 200, S{"redirect": s.oauthRedirect(r, &a, url.Values{"code": {raw}})})
}

// POST /oauth/token
func (s *Server) oauthToken(w http.ResponseWriter, r *http.Request) {
	mcpCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.oauthLimiter.allow("token " + clientIP(r)) {
		oauthErr(w, http.StatusTooManyRequests, "slow_down", "too many token requests from this address")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		oauthErr(w, 400, "invalid_request", "form body expected")
		return
	}
	f := r.PostForm
	switch f.Get("grant_type") {
	case "authorization_code":
		s.tokenFromCode(w, r, f)
	case "refresh_token":
		s.tokenFromRefresh(w, r, f)
	default:
		oauthErr(w, 400, "unsupported_grant_type", "authorization_code or refresh_token")
	}
}

type oauthGrant struct {
	ID, OrgID, UserID, ClientRowID, ClientID, ClientName string
	Scopes                                               []string
	ReadOnly                                             bool
}

func (s *Server) tokenFromCode(w http.ResponseWriter, r *http.Request, f url.Values) {
	code, verifier := f.Get("code"), f.Get("code_verifier")
	if code == "" || !pkceRe.MatchString(verifier) {
		oauthErr(w, 400, "invalid_request", "code and code_verifier (43–128 characters) are required")
		return
	}
	ctx := r.Context()
	var g oauthGrant
	var redirect, challenge, resource string
	var used *time.Time
	var expires time.Time
	err := s.pool.QueryRow(ctx, `SELECT c.grant_id::text, c.redirect_uri, c.challenge, c.resource, c.used_at, c.expires_at,
			g.org_id::text, g.user_id::text, g.scopes, g.read_only, cl.id::text, cl.client_id, cl.name
		FROM core.oauth_code c JOIN core.oauth_grant g ON g.id = c.grant_id JOIN core.oauth_client cl ON cl.id = g.client_id
		WHERE c.code_hash = $1 AND g.revoked_at IS NULL`, iam.HashToken(code)).
		Scan(&g.ID, &redirect, &challenge, &resource, &used, &expires, &g.OrgID, &g.UserID, &g.Scopes, &g.ReadOnly, &g.ClientRowID, &g.ClientID, &g.ClientName)
	if err != nil {
		oauthErr(w, 400, "invalid_grant", "unknown or expired code")
		return
	}
	if used != nil {
		// a code presented twice: someone else may have it — the grant ends (RFC 6749 §4.1.2)
		_ = s.revokeGrant(ctx, g.ID, "", "authorization code used twice")
		s.log.Warn("oauth code reused, grant revoked", "security", true, "grant", g.ID, "remote", clientIP(r))
		oauthErr(w, 400, "invalid_grant", "code already used")
		return
	}
	if time.Now().After(expires) {
		oauthErr(w, 400, "invalid_grant", "code expired")
		return
	}
	if cid := f.Get("client_id"); cid != "" && cid != g.ClientID {
		oauthErr(w, 400, "invalid_grant", "code issued to another client")
		return
	}
	if ru := f.Get("redirect_uri"); ru != "" && ru != redirect {
		oauthErr(w, 400, "invalid_grant", "redirect_uri differs from the authorization request")
		return
	}
	sum := sha256.Sum256([]byte(verifier))
	if subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) != 1 {
		oauthErr(w, 400, "invalid_grant", "code_verifier does not match the code_challenge")
		return
	}
	if res := f.Get("resource"); res != "" && strings.TrimRight(res, "/") != strings.TrimRight(resource, "/") && !s.validResource(r, res) {
		oauthErr(w, 400, "invalid_target", "resource differs from the authorization request")
		return
	}
	tag, err := s.pool.Exec(ctx, `UPDATE core.oauth_code SET used_at = now() WHERE code_hash = $1 AND used_at IS NULL`, iam.HashToken(code))
	if err != nil || tag.RowsAffected() != 1 {
		oauthErr(w, 400, "invalid_grant", "code already used")
		return
	}
	if !s.mcpEnabled(ctx, g.OrgID) {
		oauthErr(w, 400, "invalid_grant", "AI access (MCP) is switched off in this organization")
		return
	}
	s.issueTokens(w, r, &g)
}

func (s *Server) tokenFromRefresh(w http.ResponseWriter, r *http.Request, f url.Values) {
	rt := f.Get("refresh_token")
	if !strings.HasPrefix(rt, "c32r_") {
		oauthErr(w, 400, "invalid_grant", "unknown refresh token")
		return
	}
	ctx := r.Context()
	var g oauthGrant
	var expires *time.Time
	err := s.pool.QueryRow(ctx, `SELECT g.id::text, g.org_id::text, g.user_id::text, g.scopes, g.read_only, g.refresh_expires_at, cl.id::text, cl.client_id, cl.name
		FROM core.oauth_grant g JOIN core.oauth_client cl ON cl.id = g.client_id
		WHERE g.refresh_hash = $1 AND g.revoked_at IS NULL`, iam.HashToken(rt)).
		Scan(&g.ID, &g.OrgID, &g.UserID, &g.Scopes, &g.ReadOnly, &expires, &g.ClientRowID, &g.ClientID, &g.ClientName)
	if err != nil || (expires != nil && time.Now().After(*expires)) {
		oauthErr(w, 400, "invalid_grant", "unknown or expired refresh token: connect the assistant again")
		return
	}
	if cid := f.Get("client_id"); cid != "" && cid != g.ClientID {
		oauthErr(w, 400, "invalid_grant", "refresh token of another client")
		return
	}
	s.issueTokens(w, r, &g)
}

// issueTokens answers a token request: a new access token and a new refresh token (the old one stops working). A
// refresh also works while the organization has AI access switched off: /mcp refuses the token (403), and the assistant
// keeps its approval and continues when access is switched on again.
func (s *Server) issueTokens(w http.ResponseWriter, r *http.Request, g *oauthGrant) {
	ctx := r.Context()
	var disabled bool
	if err := s.pool.QueryRow(ctx, `SELECT disabled_at IS NOT NULL FROM core.user_account WHERE id = $1::uuid`, g.UserID).Scan(&disabled); err != nil || disabled {
		oauthErr(w, 400, "invalid_grant", "the user account is disabled")
		return
	}
	access, err1 := iam.RandomToken(32)
	refresh, err2 := iam.RandomToken(32)
	if err1 != nil || err2 != nil {
		oauthErr(w, 500, "server_error", "random")
		return
	}
	access, refresh = "c32_"+access, "c32r_"+refresh
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		oauthErr(w, 500, "server_error", "begin")
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO core.api_token (org_id, user_id, name, token_hash, scopes, expires_at, grant_id) VALUES ($1::uuid, $2::uuid, $3, $4, $5, now() + $6::interval, $7::uuid)`,
		g.OrgID, g.UserID, "AI: "+g.ClientName, iam.HashToken(access), g.Scopes, oauthAccessTTL.String(), g.ID); err != nil {
		oauthErr(w, 500, "server_error", "token")
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE core.oauth_grant SET refresh_hash = $2, refresh_expires_at = now() + $3::interval, last_used_at = now() WHERE id = $1::uuid`,
		g.ID, iam.HashToken(refresh), oauthRefreshTTL.String()); err != nil {
		oauthErr(w, 500, "server_error", "grant")
		return
	}
	// access tokens of this grant that ended a day ago are of no use any more
	if _, err := tx.Exec(ctx, `DELETE FROM core.api_token WHERE grant_id = $1::uuid AND expires_at < now() - interval '1 day'`, g.ID); err != nil {
		oauthErr(w, 500, "server_error", "cleanup")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		oauthErr(w, 500, "server_error", "commit")
		return
	}
	scope := oauthScopeAll
	if g.ReadOnly {
		scope = oauthScopeRead
	}
	oauthJSON(w, 200, S{"access_token": access, "token_type": "Bearer", "expires_in": int(oauthAccessTTL.Seconds()), "refresh_token": refresh, "scope": scope})
}

// POST /oauth/revoke (RFC 7009): a refresh token ends its grant; an access token only itself. Always 200.
func (s *Server) oauthRevoke(w http.ResponseWriter, r *http.Request) {
	mcpCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		oauthErr(w, 400, "invalid_request", "form body expected")
		return
	}
	tok := r.PostForm.Get("token")
	ctx := r.Context()
	switch {
	case strings.HasPrefix(tok, "c32r_"):
		var id string
		if s.pool.QueryRow(ctx, `SELECT id::text FROM core.oauth_grant WHERE refresh_hash = $1 AND revoked_at IS NULL`, iam.HashToken(tok)).Scan(&id) == nil {
			_ = s.revokeGrant(ctx, id, "", "revoked by the AI client")
		}
	case strings.HasPrefix(tok, "c32_"):
		_, _ = s.pool.Exec(ctx, `UPDATE core.api_token SET revoked_at = now() WHERE token_hash = $1 AND grant_id IS NOT NULL AND revoked_at IS NULL`, iam.HashToken(tok))
	}
	mcpCORS(w)
	w.WriteHeader(http.StatusOK)
}
