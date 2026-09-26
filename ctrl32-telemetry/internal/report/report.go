// Package report renders PDF reports and exports with Typst from versioned templates (ADR-0010).
//
// A template is Typst source that reads `data.json` (the data contract below) and optional assets such as
// `chart.svg` and the organization logo. Built-in templates are embedded in the binary; an organization may
// override a kind+locale with its own versioned template stored in core.report_template. Every generated PDF
// is a record in core.report (append-only, file hash verified on download).
package report

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

//go:embed templates/*.typ
var builtin embed.FS

// MaxRows is the hard cap of table rows in one PDF (CSV/JSONL exports carry the complete data).
const MaxRows = 3000

// Data is the contract every template receives as data.json.
type Data struct {
	Kind        string           `json:"kind"`
	Title       string           `json:"title"`
	Subtitle    string           `json:"subtitle"`
	Org         string           `json:"org"`
	Logo        string           `json:"logo"` // asset file name ("logo.png"/"logo.svg") or ""
	Locale      string           `json:"locale"`
	GeneratedAt string           `json:"generated_at"`
	GeneratedBy string           `json:"generated_by"`
	Period      Period           `json:"period"`
	Scope       map[string]any   `json:"scope"`
	Columns     []Column         `json:"columns"`
	Rows        []map[string]any `json:"rows"`
	Summary     map[string]any   `json:"summary"`
	Chart       string           `json:"chart"` // asset file name ("chart.svg") or ""
	Audit       AuditHead        `json:"audit"`
	System      map[string]any   `json:"system"`
}

// Period of the report.
type Period struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Column describes a table column: key in rows, label, alignment hint.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Align string `json:"align"` // left|right|center
}

// AuditHead is printed on regulatory reports (anchoring of the audit chain, ADR-0005).
type AuditHead struct {
	Seq  int64  `json:"seq"`
	Hash string `json:"hash"`
}

// Template is a resolved template (built-in or organization override).
type Template struct {
	ID       string          `json:"id"` // "" for built-in
	OrgID    string          `json:"org_id,omitempty"`
	Kind     string          `json:"kind"`
	Name     string          `json:"name"`
	Version  int             `json:"version"`
	Locale   string          `json:"locale"`
	Body     string          `json:"body,omitempty"`
	Checksum string          `json:"checksum"`
	Spec     json.RawMessage `json:"spec,omitempty"` // block-designer representation (nil = hand-written source)
	BuiltIn  bool            `json:"builtin"`
}

// Record is a stored report (core.report row).
type Record struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"org_id,omitempty"`
	Kind        string         `json:"kind"`
	TemplateID  string         `json:"template_id,omitempty"`
	TemplateVer int            `json:"template_ver"`
	Scope       map[string]any `json:"scope"`
	DataHash    string         `json:"data_hash"`
	FileHash    string         `json:"file_hash"`
	FilePath    string         `json:"-"`
	Bytes       int64          `json:"bytes"`
	GeneratedAt time.Time      `json:"generated_at"`
	GeneratedBy string         `json:"generated_by"`
	AuditSeq    int64          `json:"audit_seq"`
}

// Result of a render.
type Result struct {
	Record   Record
	PDF      []byte
	Template *Template
}

// Engine runs Typst.
type Engine struct {
	typst   string
	storage string
	pool    *pgxpool.Pool
	log     *slog.Logger
	version string
}

// New locates the Typst CLI (config path, PATH, or next to the executable) and prepares storage.
func New(cfg config.ReportConfig, pool *pgxpool.Pool, log *slog.Logger) (*Engine, error) {
	e := &Engine{storage: cfg.StorageDir, pool: pool, log: log.With("component", "report")}
	candidates := []string{}
	if cfg.TypstPath != "" {
		candidates = append(candidates, cfg.TypstPath)
	}
	if p, err := exec.LookPath("typst"); err == nil {
		candidates = append(candidates, p)
	}
	if exe, err := os.Executable(); err == nil {
		name := "typst"
		if strings.HasSuffix(exe, ".exe") {
			name = "typst.exe"
		}
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}
	for _, c := range candidates {
		out, err := exec.Command(c, "--version").Output()
		if err == nil {
			e.typst = c
			e.version = strings.TrimSpace(string(out))
			break
		}
	}
	if e.typst == "" {
		return nil, errors.New("typst CLI not found (set report.typst_path or install typst next to the binary)")
	}
	if err := os.MkdirAll(e.storage, 0o750); err != nil {
		return nil, fmt.Errorf("storage dir: %w", err)
	}
	e.log.Info("report engine ready", "typst", e.version, "storage", e.storage)
	return e, nil
}

// Version returns the Typst version string ("" when unavailable).
func (e *Engine) Version() string {
	if e == nil {
		return ""
	}
	return e.version
}

// Kinds lists built-in template kinds.
func Kinds() []string {
	entries, _ := fs.ReadDir(builtin, "templates")
	var out []string
	for _, en := range entries {
		n := en.Name()
		if strings.HasSuffix(n, ".typ") && n != "layout.typ" {
			out = append(out, strings.TrimSuffix(n, ".typ"))
		}
	}
	return out
}

// Layout returns the shared layout source (read-only helper library available to every template).
func Layout() string {
	b, _ := fs.ReadFile(builtin, "templates/layout.typ")
	return string(b)
}

// BuiltIn returns the embedded template for a kind; unknown kinds fall back to "generic".
func BuiltIn(kind string) (*Template, error) {
	b, err := fs.ReadFile(builtin, "templates/"+kind+".typ")
	if err != nil {
		b, err = fs.ReadFile(builtin, "templates/generic.typ")
		if err != nil {
			return nil, fmt.Errorf("no built-in template for kind %q", kind)
		}
	}
	t := &Template{Kind: kind, Name: "built-in " + kind, Version: 0, Locale: "*", Body: string(b), Checksum: sum(b), BuiltIn: true}
	t.Spec = json.RawMessage(SpecJSON(DefaultSpec(kind)))
	return t, nil
}

// Resolve returns the organization's current override for kind+locale, else the built-in template.
func (e *Engine) Resolve(ctx context.Context, orgID, kind, locale string) (*Template, error) {
	if orgID != "" {
		var t Template
		var spec *string
		err := e.pool.QueryRow(ctx, `SELECT id::text, org_id::text, kind, name, version, locale, body, checksum, spec::text FROM core.report_template
			WHERE org_id = $1::uuid AND kind = $2 AND locale IN ($3, '*') AND valid_to IS NULL ORDER BY (locale = $3) DESC, version DESC LIMIT 1`, orgID, kind, locale).
			Scan(&t.ID, &t.OrgID, &t.Kind, &t.Name, &t.Version, &t.Locale, &t.Body, &t.Checksum, &spec)
		if err == nil {
			if spec != nil && *spec != "" {
				t.Spec = json.RawMessage(*spec)
			}
			return &t, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return BuiltIn(kind)
}

// SaveVersion stores a new version of an organization override (append-only history; supersedes the current one).
// The template must compile with sample data before it is accepted. spec is the optional block-designer JSON
// the body was generated from ("" = hand-written source).
func (e *Engine) SaveVersion(ctx context.Context, orgID, kind, locale, name, body, spec, createdBy, reason string) (*Template, error) {
	if strings.TrimSpace(body) == "" || len(body) > 512*1024 {
		return nil, errors.New("body must be 1 byte to 512 KiB")
	}
	if _, err := e.compile(ctx, body, SampleData(kind, locale), sampleAssets()); err != nil {
		return nil, fmt.Errorf("template does not compile: %w", err)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var next int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM core.report_template WHERE org_id = $1::uuid AND kind = $2 AND locale = $3`, orgID, kind, locale).Scan(&next); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE core.report_template SET valid_to = now() WHERE org_id = $1::uuid AND kind = $2 AND locale = $3 AND valid_to IS NULL`, orgID, kind, locale); err != nil {
		return nil, err
	}
	if name == "" {
		name = kind + " (" + locale + ")"
	}
	var t Template
	var specOut *string
	err = tx.QueryRow(ctx, `INSERT INTO core.report_template (org_id, kind, name, version, locale, body, spec, checksum, created_by, reason)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7,'')::jsonb, $8, NULLIF($9,'')::uuid, $10) RETURNING id::text, org_id::text, kind, name, version, locale, body, checksum, spec::text`,
		orgID, kind, name, next, locale, body, spec, sum([]byte(body)), createdBy, reason).Scan(&t.ID, &t.OrgID, &t.Kind, &t.Name, &t.Version, &t.Locale, &t.Body, &t.Checksum, &specOut)
	if err != nil {
		return nil, err
	}
	if specOut != nil && *specOut != "" {
		t.Spec = json.RawMessage(*specOut)
	}
	return &t, tx.Commit(ctx)
}

// Revert removes the organization override (sets valid_to on the current version) so the built-in applies again.
func (e *Engine) Revert(ctx context.Context, orgID, kind, locale string) (int64, error) {
	tag, err := e.pool.Exec(ctx, `UPDATE core.report_template SET valid_to = now() WHERE org_id = $1::uuid AND kind = $2 AND locale = $3 AND valid_to IS NULL`, orgID, kind, locale)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ListOverrides returns the organization's current template versions (without bodies).
func (e *Engine) ListOverrides(ctx context.Context, orgID string) ([]Template, error) {
	out := []Template{}
	if orgID == "" {
		return out, nil
	}
	rows, err := e.pool.Query(ctx, `SELECT id::text, org_id::text, kind, name, version, locale, checksum FROM core.report_template WHERE org_id = $1::uuid AND valid_to IS NULL ORDER BY kind, locale`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Kind, &t.Name, &t.Version, &t.Locale, &t.Checksum); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Render compiles the resolved template with data, stores the PDF as a record and returns it.
// The record row and the file are written together: if the file cannot be stored, the row is rolled back.
func (e *Engine) Render(ctx context.Context, orgID string, d Data, assets map[string][]byte, generatedBy string) (*Result, error) {
	t, err := e.Resolve(ctx, orgID, d.Kind, d.Locale)
	if err != nil {
		return nil, err
	}
	if len(d.Rows) > MaxRows {
		d.Rows = d.Rows[:MaxRows]
		if d.Summary == nil {
			d.Summary = map[string]any{}
		}
		d.Summary["truncated"] = fmt.Sprintf("first %d rows", MaxRows)
	}
	d.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	d.GeneratedBy = generatedBy
	dataJSON, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	pdf, err := e.compile(ctx, t.Body, d, assets)
	if err != nil {
		return nil, err
	}
	rec := Record{OrgID: orgID, Kind: d.Kind, Scope: d.Scope, DataHash: sum(dataJSON), FileHash: sum(pdf), Bytes: int64(len(pdf)), GeneratedBy: generatedBy, AuditSeq: d.Audit.Seq}
	var tmplID *string
	var tmplVer *int
	if !t.BuiltIn {
		tmplID, tmplVer = &t.ID, &t.Version
		rec.TemplateID, rec.TemplateVer = t.ID, t.Version
	}
	scope, _ := json.Marshal(d.Scope)
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `WITH n AS (SELECT uuidv7() AS id, now() AS ts)
		INSERT INTO core.report (id, org_id, kind, template_id, template_ver, scope, data_hash, file_hash, file_path, bytes, generated_at, generated_by, audit_seq)
		SELECT n.id, NULLIF($1,'')::uuid, $2, $3::uuid, $4, $5::jsonb, $6, $7, to_char(n.ts AT TIME ZONE 'UTC', 'YYYY/MM') || '/' || n.id::text || '.pdf', $8, n.ts, $9, NULLIF($10, 0) FROM n
		RETURNING id::text, file_path, generated_at`,
		orgID, d.Kind, tmplID, tmplVer, string(scope), rec.DataHash, rec.FileHash, rec.Bytes, generatedBy, rec.AuditSeq).Scan(&rec.ID, &rec.FilePath, &rec.GeneratedAt)
	if err != nil {
		return nil, fmt.Errorf("record: %w", err)
	}
	full := filepath.Join(e.storage, filepath.FromSlash(rec.FilePath))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(full, pdf, 0o640); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		_ = os.Remove(full)
		return nil, err
	}
	return &Result{Record: rec, PDF: pdf, Template: t}, nil
}

// Preview compiles arbitrary template source with sample data (nothing is stored). An optional logo asset is passed through.
func (e *Engine) Preview(ctx context.Context, body, kind, locale string, logo []byte, logoName string) ([]byte, error) {
	d := SampleData(kind, locale)
	assets := sampleAssets()
	if len(logo) > 0 && logoName != "" {
		assets[logoName] = logo
		d.Logo = logoName
	}
	return e.compile(ctx, body, d, assets)
}

// List returns stored reports of an organization, newest first.
func (e *Engine) List(ctx context.Context, orgID string, limit int) ([]Record, error) {
	rows, err := e.pool.Query(ctx, `SELECT id::text, COALESCE(org_id::text,''), kind, COALESCE(template_id::text,''), COALESCE(template_ver,0), scope, data_hash, file_hash, file_path, bytes, generated_at, generated_by, COALESCE(audit_seq,0)
		FROM core.report WHERE ($1 = '' OR org_id = $1::uuid) ORDER BY generated_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		var scope []byte
		if err := rows.Scan(&r.ID, &r.OrgID, &r.Kind, &r.TemplateID, &r.TemplateVer, &scope, &r.DataHash, &r.FileHash, &r.FilePath, &r.Bytes, &r.GeneratedAt, &r.GeneratedBy, &r.AuditSeq); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scope, &r.Scope)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns one stored report's metadata.
func (e *Engine) Get(ctx context.Context, orgID, id string) (*Record, error) {
	var r Record
	var scope []byte
	err := e.pool.QueryRow(ctx, `SELECT id::text, COALESCE(org_id::text,''), kind, COALESCE(template_id::text,''), COALESCE(template_ver,0), scope, data_hash, file_hash, file_path, bytes, generated_at, generated_by, COALESCE(audit_seq,0)
		FROM core.report WHERE id = $2::uuid AND ($1 = '' OR org_id = $1::uuid)`, orgID, id).
		Scan(&r.ID, &r.OrgID, &r.Kind, &r.TemplateID, &r.TemplateVer, &scope, &r.DataHash, &r.FileHash, &r.FilePath, &r.Bytes, &r.GeneratedAt, &r.GeneratedBy, &r.AuditSeq)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(scope, &r.Scope)
	return &r, nil
}

// ErrTampered is returned when a stored PDF no longer matches its recorded hash.
var ErrTampered = errors.New("stored report does not match its recorded hash")

// ReadFile loads a stored PDF and verifies its hash against the record.
func (e *Engine) ReadFile(r *Record) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(e.storage, filepath.FromSlash(r.FilePath)))
	if err != nil {
		return nil, err
	}
	if sum(b) != r.FileHash {
		e.log.Error("report file hash mismatch", "security", true, "report_id", r.ID)
		return nil, ErrTampered
	}
	return b, nil
}

// compile writes a temp workspace (main.typ, layout.typ, data.json, assets) and runs `typst compile`.
func (e *Engine) compile(ctx context.Context, body string, d Data, assets map[string][]byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "ctrl32-report-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	dataJSON, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	layout, _ := fs.ReadFile(builtin, "templates/layout.typ")
	files := map[string][]byte{"main.typ": []byte(body), "layout.typ": layout, "data.json": dataJSON}
	for k, v := range assets {
		if strings.ContainsAny(k, "/\\") || strings.HasPrefix(k, ".") || k == "main.typ" || k == "layout.typ" || k == "data.json" {
			return nil, fmt.Errorf("invalid asset name %q", k)
		}
		files[k] = v
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			return nil, err
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// --root confines file access to the workspace; packages are disabled by pointing the package paths into the workspace.
	// PDF/A-2b for archival (URS J-06); the creation timestamp comes from the data so that the same data, template and
	// assets give byte-identical PDFs (URS J-08). A template that uses a feature PDF/A forbids falls back to plain PDF.
	pdfA := true
	for {
		args := []string{"compile", "--root", dir, "--ignore-system-fonts"}
		if pdfA {
			args = append(args, "--pdf-standard", "a-2b")
		}
		if ts, err := time.Parse(time.RFC3339, d.GeneratedAt); err == nil {
			args = append(args, "--creation-timestamp", strconv.FormatInt(ts.Unix(), 10))
		}
		args = append(args, filepath.Join(dir, "main.typ"), filepath.Join(dir, "out.pdf"))
		cmd := exec.CommandContext(cctx, e.typst, args...)
		cmd.Env = append(os.Environ(), "TYPST_PACKAGE_PATH="+filepath.Join(dir, "pkg"), "TYPST_PACKAGE_CACHE_PATH="+filepath.Join(dir, "pkgcache"), "NO_COLOR=1")
		out, err := cmd.CombinedOutput()
		if err == nil {
			break
		}
		msg := strings.TrimSpace(string(out))
		if len(msg) > 4000 {
			msg = msg[:4000]
		}
		if pdfA && strings.Contains(msg, "PDF/A") {
			e.log.Warn("PDF/A-2b not possible for this template, producing plain PDF", "err", strings.SplitN(msg, "\n", 2)[0])
			pdfA = false
			continue
		}
		return nil, fmt.Errorf("typst: %s", msg)
	}
	return os.ReadFile(filepath.Join(dir, "out.pdf"))
}

func sampleAssets() map[string][]byte {
	return map[string][]byte{"chart.svg": []byte(SampleChartSVG())}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
