package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
)

// Electronic signatures (report.sign, URS H-13, P-14) and reviews of the audit trail (audit.review, URS I-05);
// 0.67.0.
//
// A signature binds one version of a record to a person: the signer re-enters the password (and the code from the
// authenticator app when two-factor sign-in is on), chooses the meaning (authored, reviewed, approved, verified,
// responsible) and the server stores the manifestation — name, user name, roles, time with the time zone — together
// with the SHA-256 of the record version (a stored PDF report: its file hash) in core.esignature. The manifest is
// hashed and signed with the server's Ed25519 key, so a signature row cannot be forged in the database either. A
// signature is valid while the record still has the signed hash: a changed record invalidates it (checked whenever
// signatures are shown). Signing needs a browser session: API tokens and AI assistants cannot sign.
//
// A review of the audit trail records a period (or one entry), the entries it covered (first and last sequence
// number, count, the hash of the last entry), the outcome (no findings / findings with a comment) and is signed with
// the meaning "reviewed".
//
//	GET  /api/v1/reports/{id}/signatures                    (data.export)
//	POST /api/v1/reports/{id}/signatures {meaning, comment, password, code}   (report.sign)
//	GET  /api/v1/audit?from=&to=&action=&actor=&entity_type=&before=&limit=   (audit.read)
//	GET  /api/v1/audit/reviews?limit=                      (audit.read)
//	POST /api/v1/audit/reviews {from, to | seq, outcome, comment, password, code}   (audit.review)

const esignFormat = "ctrl32-esignature/1"

var esignMeanings = map[string]bool{"authored": true, "reviewed": true, "approved": true, "verified": true, "responsible": true}

// esignKey returns the server's signing key for e-signatures, creating it on first use (private key sealed with the
// configured secret key, like the evidence key).
func (s *Server) esignKey(ctx context.Context) (ed25519.PrivateKey, error) {
	box, err := iam.NewSecretBox(s.cfg.Security.SecretKey)
	if err != nil {
		return nil, err
	}
	var pub, sealed string
	if err := s.pool.QueryRow(ctx, `SELECT public_key, private_sealed FROM core.server_key WHERE name = 'esignature'`).Scan(&pub, &sealed); err == nil {
		raw, err := base64.StdEncoding.DecodeString(sealed)
		if err != nil {
			return nil, err
		}
		seed, err := box.Open(raw)
		if err != nil {
			return nil, fmt.Errorf("signature key cannot be opened (was the secret key changed?): %w", err)
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sl, err := box.Seal(priv.Seed())
	if err != nil {
		return nil, err
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO core.server_key (name, public_key, private_sealed) VALUES ('esignature', $1, $2) ON CONFLICT (name) DO NOTHING`,
		base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)), base64.StdEncoding.EncodeToString(sl)); err != nil {
		return nil, err
	}
	return s.esignKey(ctx) // the stored one wins if two requests raced
}

func keyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// esignPublic returns the public key of the signing key (nil when none was made yet).
func (s *Server) esignPublic(ctx context.Context) ed25519.PublicKey {
	var pub string
	if s.pool.QueryRow(ctx, `SELECT public_key FROM core.server_key WHERE name = 'esignature'`).Scan(&pub) != nil {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(pub)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return b
}

// manifestHash is the SHA-256 of the manifest's JSON (Go's encoding: sorted keys). The manifest holds only text and
// lists of text, so the JSON read back from the database encodes to the same bytes.
func manifestHash(m map[string]any) (string, []byte) {
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), b
}

// reauth checks the signer: a browser session, the password and, with two-factor sign-in, the authenticator code.
// Failures are limited (5 a minute per user) and logged as security events. Returns the method.
func (s *Server) reauth(w http.ResponseWriter, r *http.Request, password, code, purpose string) (string, bool) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.SessionID == "" || p.GrantID != "" || r.Context().Value(viaMCPKey) != nil {
		writeJSON(w, 403, map[string]string{"error": "an electronic signature is given by the user in the browser, not with an API token or an AI assistant", "code": "session_required"})
		return "", false
	}
	if !s.stepUpLimiter.peek(p.UserID) {
		writeErr(w, 429, "too many failed confirmations; wait a minute")
		return "", false
	}
	if password == "" {
		writeJSON(w, 403, map[string]string{"error": "confirm with your password (and the code from your authenticator app) to sign", "code": "step_up_required"})
		return "", false
	}
	if !s.iam.ReauthPassword(r.Context(), p.UserID, password) || (p.MFAEnabled && !s.iam.ReauthTOTP(r.Context(), p.UserID, strings.TrimSpace(code))) {
		s.log.Warn(purpose+" re-authentication failed", "security", true, "user", p.Username, "remote", clientIP(r))
		s.stepUpLimiter.allow(p.UserID)
		msg := "wrong password"
		if p.MFAEnabled {
			msg = "wrong password or authenticator code"
		}
		writeJSON(w, 403, map[string]string{"error": msg, "code": "step_up_failed"})
		return "", false
	}
	if p.MFAEnabled {
		return "password_totp", true
	}
	return "password", true
}

// esignRow is a stored signature with its validity.
type esignRow struct {
	ID         string    `json:"id"`
	RecordType string    `json:"record_type"`
	RecordID   string    `json:"record_id"`
	RecordHash string    `json:"record_hash"`
	Meaning    string    `json:"meaning"`
	Comment    string    `json:"comment,omitempty"`
	Signer     string    `json:"signer"`      // user name
	SignerName string    `json:"signer_name"` // full name
	Roles      []string  `json:"roles"`
	SignedAt   time.Time `json:"signed_at"`
	Local      string    `json:"signed_at_local"` // manifestation: the time in the organization's time zone
	TZ         string    `json:"tz"`
	Method     string    `json:"method"`
	KeyID      string    `json:"key_id"`
	Valid      bool      `json:"valid"`
	Problem    string    `json:"problem,omitempty"`
}

// signRecord stores a signature (inside tx when given). The manifest names the record, its hash and the signer.
func (s *Server) signRecord(ctx context.Context, tx pgx.Tx, r *http.Request, org, recordType, recordID, recordHash, meaning, comment, method string) (*esignRow, error) {
	p := PrincipalFrom(r.Context())
	key, err := s.esignKey(ctx)
	if err != nil {
		return nil, err
	}
	tz := "UTC"
	_ = s.pool.QueryRow(ctx, `SELECT tz FROM core.organization WHERE id = $1::uuid`, org).Scan(&tz)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, tz = time.UTC, "UTC"
	}
	now := time.Now().UTC().Truncate(time.Second)
	roles := append([]string{}, p.Roles...)
	if roles == nil {
		roles = []string{}
	}
	full := p.FullName
	if full == "" {
		full = p.Username
	}
	local := now.In(loc).Format("2006-01-02 15:04:05 MST (-07:00)")
	m := map[string]any{"format": esignFormat, "org_id": org, "record_type": recordType, "record_id": recordID, "record_hash": recordHash,
		"meaning": meaning, "comment": comment, "signer_id": p.UserID, "signer_username": p.Username, "signer_name": full, "signer_roles": roles,
		"signed_at": now.Format(time.RFC3339), "signed_at_local": local, "tz": tz, "method": method}
	h, body := manifestHash(m)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(h)))
	kid := keyID(key.Public().(ed25519.PublicKey))
	var cm *string
	if comment != "" {
		cm = &comment
	}
	q := `INSERT INTO core.esignature (org_id, record_type, record_id, record_hash, meaning, comment, signer_id, signer_username, signer_name, signer_roles,
		signed_at, tz, method, manifest, manifest_hash, key_id, server_signature)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7::uuid, $8, $9, $10, $11, $12, $13, $14::jsonb, $15, $16, $17) RETURNING id::text`
	args := []any{org, recordType, recordID, recordHash, meaning, cm, p.UserID, p.Username, full, roles, now, tz, method, string(body), h, kid, sig}
	var id string
	if tx != nil {
		err = tx.QueryRow(ctx, q, args...).Scan(&id)
	} else {
		err = s.pool.QueryRow(ctx, q, args...).Scan(&id)
	}
	if err != nil {
		return nil, err
	}
	return &esignRow{ID: id, RecordType: recordType, RecordID: recordID, RecordHash: recordHash, Meaning: meaning, Comment: comment, Signer: p.Username,
		SignerName: full, Roles: roles, SignedAt: now, Local: local, TZ: tz, Method: method, KeyID: kid, Valid: true}, nil
}

// signatures lists the signatures of a record and checks each one against the current hash of the record.
func (s *Server) signatures(ctx context.Context, org, recordType, recordID, currentHash string) ([]esignRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, record_hash, meaning, coalesce(comment, ''), signer_username, signer_name, signer_roles, signed_at, tz, method,
		manifest::text, manifest_hash, key_id, server_signature
		FROM core.esignature WHERE org_id = $1::uuid AND record_type = $2 AND record_id = $3::uuid ORDER BY signed_at LIMIT 100`, org, recordType, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pub := s.esignPublic(ctx)
	out := []esignRow{}
	for rows.Next() {
		e := esignRow{RecordType: recordType, RecordID: recordID}
		var manifest, mhash, sig string
		if err := rows.Scan(&e.ID, &e.RecordHash, &e.Meaning, &e.Comment, &e.Signer, &e.SignerName, &e.Roles, &e.SignedAt, &e.TZ, &e.Method, &manifest, &mhash, &e.KeyID, &sig); err != nil {
			return nil, err
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(manifest), &m)
		if v, ok := m["signed_at_local"].(string); ok {
			e.Local = v
		}
		h, _ := manifestHash(m)
		sb, _ := base64.StdEncoding.DecodeString(sig)
		switch {
		case h != mhash || m["record_hash"] != e.RecordHash || m["meaning"] != e.Meaning || m["signer_username"] != e.Signer:
			e.Problem = "the signature record does not match its manifest"
		case pub == nil || keyID(pub) != e.KeyID || !ed25519.Verify(pub, []byte(mhash), sb):
			e.Problem = "the server signature does not verify"
		case currentHash != e.RecordHash:
			e.Problem = "the record changed after it was signed"
		default:
			e.Valid = true
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// reportForSigning returns a stored report and its verified hash ("" with a problem when the file is missing or
// does not match its recorded hash).
func (s *Server) reportForSigning(r *http.Request, org, id string) (*report.Record, string, error) {
	rec, err := s.rep.Get(r.Context(), org, id)
	if err != nil {
		return nil, "", err
	}
	if _, err := s.rep.ReadFile(rec); err != nil {
		if errors.Is(err, report.ErrTampered) {
			return rec, "", errors.New("the stored file does not match its recorded hash")
		}
		return rec, "", errors.New("the report file is missing")
	}
	return rec, rec.FileHash, nil
}

// GET /api/v1/reports/{id}/signatures
func (s *Server) reportSignatures(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rec, hash, err := s.reportForSigning(r, org, r.PathValue("id"))
	if rec == nil {
		writeErr(w, 404, "report not found")
		return
	}
	sigs, qerr := s.signatures(r.Context(), org, "report", rec.ID, hash)
	if qerr != nil {
		writeErr(w, 500, "query failed")
		return
	}
	out := map[string]any{"report_id": rec.ID, "file_hash": rec.FileHash, "file_ok": err == nil, "signatures": sigs}
	if err != nil {
		out["problem"] = err.Error()
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/reports/{id}/signatures
func (s *Server) signReport(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Meaning  string `json:"meaning"`
		Comment  string `json:"comment"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Comment = strings.TrimSpace(in.Comment)
	if !esignMeanings[in.Meaning] {
		writeErr(w, 400, "meaning must be authored, reviewed, approved, verified or responsible")
		return
	}
	if utf8.RuneCountInString(in.Comment) > 500 {
		writeErr(w, 400, "comment at most 500 characters")
		return
	}
	rec, hash, err := s.reportForSigning(r, org, r.PathValue("id"))
	if rec == nil {
		writeErr(w, 404, "report not found")
		return
	}
	if err != nil {
		writeErr(w, 409, err.Error()+"; it cannot be signed")
		return
	}
	method, ok := s.reauth(w, r, in.Password, in.Code, "report signature")
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var dup bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.esignature WHERE record_type = 'report' AND record_id = $1::uuid AND signer_id = $2::uuid AND meaning = $3 AND record_hash = $4)`,
		rec.ID, p.UserID, in.Meaning, hash).Scan(&dup)
	if dup {
		writeErr(w, 409, "you have already signed this report with this meaning")
		return
	}
	sig, err := s.signRecord(r.Context(), nil, r, org, "report", rec.ID, hash, in.Meaning, in.Comment, method)
	if err != nil {
		s.log.Error("report signature failed", "err", err)
		writeErr(w, 500, "signature could not be stored")
		return
	}
	s.audit(r, "esignature.create", "report", rec.ID, nil, map[string]any{"signature_id": sig.ID, "meaning": in.Meaning, "record_hash": hash, "method": method, "comment": in.Comment}, "signed: "+in.Meaning)
	writeJSON(w, 201, sig)
}

// signatureSummary returns, per report, the signatures still valid by hash: meaning and signer (for the list).
func (s *Server) signatureSummary(ctx context.Context, org string, ids []string, hashes map[string]string) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	if len(ids) == 0 {
		return out
	}
	rows, err := s.pool.Query(ctx, `SELECT record_id::text, record_hash, meaning, signer_name, signer_username, signed_at FROM core.esignature
		WHERE org_id = $1::uuid AND record_type = 'report' AND record_id = ANY($2::uuid[]) ORDER BY signed_at`, org, ids)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, h, meaning, name, user string
		var at time.Time
		if rows.Scan(&id, &h, &meaning, &name, &user, &at) == nil {
			out[id] = append(out[id], map[string]any{"meaning": meaning, "signer_name": name, "signer": user, "signed_at": at, "hash_matches": hashes[id] == h})
		}
	}
	return out
}

// ---- audit trail: list and reviews --------------------------------------------------------------------------------

// GET /api/v1/audit
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	from, to, ok := timeRange(w, r, 7*24*time.Hour)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	before := int64(0)
	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			writeErr(w, 400, "before must be a sequence number")
			return
		}
		before = n
	}
	like := func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(v) + "%"
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT a.seq, a.ts, a.actor_type, a.actor_name, a.action, a.entity_type, coalesce(a.entity_id::text, ''), coalesce(a.reason, ''),
		       a.old_value, a.new_value, a.hash, rv.id::text, rv.reviewer_username, rv.reviewed_at, rv.outcome
		FROM core.audit_trail a
		LEFT JOIN LATERAL (SELECT id, reviewer_username, reviewed_at, outcome FROM core.audit_review v
		                   WHERE v.org_id = a.org_id AND a.seq BETWEEN v.from_seq AND v.to_seq ORDER BY v.reviewed_at DESC LIMIT 1) rv ON true
		WHERE a.org_id = $1::uuid AND a.ts >= $2 AND a.ts < $3 AND ($4 = 0 OR a.seq < $4)
		  AND ($5 = '' OR a.action ILIKE $5) AND ($6 = '' OR a.actor_name ILIKE $6) AND ($7 = '' OR a.entity_type ILIKE $7)
		ORDER BY a.seq DESC LIMIT $8`, org, from, to, before, like(q.Get("action")), like(q.Get("actor")), like(q.Get("entity_type")), limit+1)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	next := ""
	for rows.Next() {
		var seq int64
		var ts time.Time
		var actorType, actor, action, etype, eid, reason, hash string
		var oldV, newV json.RawMessage
		var rvID, rvBy, rvOutcome *string
		var rvAt *time.Time
		if err := rows.Scan(&seq, &ts, &actorType, &actor, &action, &etype, &eid, &reason, &oldV, &newV, &hash, &rvID, &rvBy, &rvAt, &rvOutcome); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		if len(out) == limit {
			next = strconv.FormatInt(out[len(out)-1]["seq"].(int64), 10)
			break
		}
		e := map[string]any{"seq": seq, "ts": ts, "actor_type": actorType, "actor": actor, "action": action, "entity_type": etype, "entity_id": eid, "reason": reason,
			"old_value": oldV, "new_value": newV, "hash": hash}
		if rvID != nil {
			e["review"] = map[string]any{"id": *rvID, "by": *rvBy, "at": *rvAt, "outcome": *rvOutcome}
		}
		out = append(out, e)
	}
	writeJSON(w, 200, map[string]any{"entries": out, "next_before": next})
}

// GET /api/v1/audit/reviews
func (s *Server) listAuditReviews(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := s.pool.Query(r.Context(), `SELECT v.id::text, v.scope, v.period_from, v.period_to, v.from_seq, v.to_seq, v.entries, coalesce(v.last_hash, ''), v.outcome,
		coalesce(v.comment, ''), v.reviewer_username, v.reviewer_name, v.reviewed_at
		FROM core.audit_review v WHERE v.org_id = $1::uuid ORDER BY v.reviewed_at DESC LIMIT $2`, org, limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	type rv struct {
		ID         string     `json:"id"`
		Scope      string     `json:"scope"`
		From       time.Time  `json:"period_from"`
		To         time.Time  `json:"period_to"`
		FromSeq    *int64     `json:"from_seq"`
		ToSeq      *int64     `json:"to_seq"`
		Entries    int64      `json:"entries"`
		LastHash   string     `json:"last_hash,omitempty"`
		Outcome    string     `json:"outcome"`
		Comment    string     `json:"comment,omitempty"`
		Reviewer   string     `json:"reviewer"`
		Name       string     `json:"reviewer_name"`
		ReviewedAt time.Time  `json:"reviewed_at"`
		Signatures []esignRow `json:"signatures"`
		content    reviewContent
	}
	list := []*rv{}
	for rows.Next() {
		x := &rv{}
		if err := rows.Scan(&x.ID, &x.Scope, &x.From, &x.To, &x.FromSeq, &x.ToSeq, &x.Entries, &x.LastHash, &x.Outcome, &x.Comment, &x.Reviewer, &x.Name, &x.ReviewedAt); err != nil {
			rows.Close()
			writeErr(w, 500, "scan failed")
			return
		}
		list = append(list, x)
	}
	rows.Close()
	for _, x := range list {
		c := reviewContent{Org: org, Scope: x.Scope, From: x.From, To: x.To, FromSeq: x.FromSeq, ToSeq: x.ToSeq, Entries: x.Entries, LastHash: x.LastHash, Outcome: x.Outcome, Comment: x.Comment, Reviewer: x.Reviewer}
		x.Signatures, _ = s.signatures(r.Context(), org, "audit_review", x.ID, c.hash())
	}
	writeJSON(w, 200, list)
}

// reviewContent is what a review signature covers; its hash is the record hash of the signature.
type reviewContent struct {
	Org      string    `json:"org_id"`
	Scope    string    `json:"scope"`
	From     time.Time `json:"period_from"`
	To       time.Time `json:"period_to"`
	FromSeq  *int64    `json:"from_seq"`
	ToSeq    *int64    `json:"to_seq"`
	Entries  int64     `json:"entries"`
	LastHash string    `json:"last_hash"`
	Outcome  string    `json:"outcome"`
	Comment  string    `json:"comment"`
	Reviewer string    `json:"reviewer"`
}

func (c reviewContent) hash() string {
	c.From, c.To = c.From.UTC().Truncate(time.Microsecond), c.To.UTC().Truncate(time.Microsecond)
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// POST /api/v1/audit/reviews
func (s *Server) createAuditReview(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		From     *time.Time `json:"from"`
		To       *time.Time `json:"to"`
		Seq      int64      `json:"seq"`
		Outcome  string     `json:"outcome"`
		Comment  string     `json:"comment"`
		Password string     `json:"password"`
		Code     string     `json:"code"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Comment = strings.TrimSpace(in.Comment)
	switch in.Outcome {
	case "no_findings":
	case "findings":
		if len(in.Comment) < 3 {
			writeErr(w, 400, "findings need a comment that describes them")
			return
		}
	default:
		writeErr(w, 400, "outcome must be no_findings or findings")
		return
	}
	if utf8.RuneCountInString(in.Comment) > 2000 {
		writeErr(w, 400, "comment at most 2000 characters")
		return
	}
	ctx := r.Context()
	c := reviewContent{Org: org, Outcome: in.Outcome, Comment: in.Comment, Reviewer: p.Username}
	if in.Seq > 0 {
		var ts time.Time
		var h string
		if err := s.pool.QueryRow(ctx, `SELECT ts, hash FROM core.audit_trail WHERE seq = $1 AND org_id = $2::uuid`, in.Seq, org).Scan(&ts, &h); err != nil {
			writeErr(w, 404, "audit entry not found")
			return
		}
		seq := in.Seq
		c.Scope, c.From, c.To, c.FromSeq, c.ToSeq, c.Entries, c.LastHash = "entry", ts, ts, &seq, &seq, 1, h
	} else {
		if in.From == nil || in.To == nil || !in.To.After(*in.From) {
			writeErr(w, 400, "a period (from before to) or the seq of one entry required")
			return
		}
		if in.To.Sub(*in.From) > 366*24*time.Hour || in.To.After(time.Now().Add(time.Minute)) {
			writeErr(w, 400, "a period of at most 366 days that has ended")
			return
		}
		c.Scope, c.From, c.To = "period", in.From.UTC(), in.To.UTC()
		if err := s.pool.QueryRow(ctx, `SELECT min(seq), max(seq), count(*) FROM core.audit_trail WHERE org_id = $1::uuid AND ts >= $2 AND ts < $3`, org, c.From, c.To).
			Scan(&c.FromSeq, &c.ToSeq, &c.Entries); err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		if c.ToSeq != nil {
			_ = s.pool.QueryRow(ctx, `SELECT hash FROM core.audit_trail WHERE seq = $1`, *c.ToSeq).Scan(&c.LastHash)
		}
	}
	method, ok := s.reauth(w, r, in.Password, in.Code, "audit review")
	if !ok {
		return
	}
	full := p.FullName
	if full == "" {
		full = p.Username
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "tx failed")
		return
	}
	defer tx.Rollback(ctx)
	var id string
	var at time.Time
	var cm *string
	if c.Comment != "" {
		cm = &c.Comment
	}
	var lh *string
	if c.LastHash != "" {
		lh = &c.LastHash
	}
	if err := tx.QueryRow(ctx, `INSERT INTO core.audit_review (org_id, scope, period_from, period_to, from_seq, to_seq, entries, last_hash, outcome, comment, reviewer_id, reviewer_username, reviewer_name)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::uuid, $12, $13) RETURNING id::text, reviewed_at`,
		org, c.Scope, c.From, c.To, c.FromSeq, c.ToSeq, c.Entries, lh, c.Outcome, cm, p.UserID, p.Username, full).Scan(&id, &at); err != nil {
		s.log.Error("audit review insert failed", "err", err)
		writeErr(w, 500, "review could not be stored")
		return
	}
	// the period as stored (microseconds) is what the signature covers
	_ = tx.QueryRow(ctx, `SELECT period_from, period_to FROM core.audit_review WHERE id = $1::uuid`, id).Scan(&c.From, &c.To)
	sig, err := s.signRecord(ctx, tx, r, org, "audit_review", id, c.hash(), "reviewed", c.Comment, method)
	if err != nil {
		s.log.Error("audit review signature failed", "err", err)
		writeErr(w, 500, "signature could not be stored")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	s.audit(r, "audit_review.create", "audit_review", id, nil, map[string]any{"scope": c.Scope, "period_from": c.From, "period_to": c.To, "from_seq": c.FromSeq, "to_seq": c.ToSeq,
		"entries": c.Entries, "last_hash": c.LastHash, "outcome": c.Outcome, "signature_id": sig.ID, "method": method}, nonEmpty(c.Comment, "reviewed: "+c.Outcome))
	writeJSON(w, 201, map[string]any{"id": id, "scope": c.Scope, "period_from": c.From, "period_to": c.To, "from_seq": c.FromSeq, "to_seq": c.ToSeq, "entries": c.Entries,
		"last_hash": c.LastHash, "outcome": c.Outcome, "comment": c.Comment, "reviewer": p.Username, "reviewed_at": at, "signature": sig})
}

func nonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
