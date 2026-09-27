package video

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/onvif"
)

// Camera is the configuration of one camera (core.camera).
type Camera struct {
	ID, OrgID, Name    string
	URL, SubURL        string
	Username, Password string
	Transport, Record  string
	RetentionDays      int
	Enabled            bool
	Push               bool   // the camera (its relay) sends the video to the server
	PushSecret         string // plaintext, only in memory
	OnvifURL           string // device service; events and PTZ
	PTZProfile         string
	EventPre           int      // seconds recorded before an event (record = "events")
	EventPost          int      // seconds recorded after it
	EventKinds         []string // kinds that start event recording
}

// Manager runs the cameras of all organizations: recorders for cameras that record, on-demand streams for viewers,
// and the retention of recordings (per camera in days, and a disk limit for all of them).
type Manager struct {
	pool     *pgxpool.Pool
	dir      string
	maxBytes int64
	box      *iam.SecretBox
	log      *slog.Logger

	mu      sync.Mutex
	set     Settings
	cams    map[string]*cam
	push    *PushServer
	pushURL string

	// OnEvent receives camera events (flows, live stream); set before Run.
	OnEvent func(CameraEvent)
}

type cam struct {
	cfg        Camera
	main, sub  *Source
	rec        *recorder
	onvif      *onvif.Client
	ev         camEvents
	stopEvents context.CancelFunc
}

// New prepares the manager; Start loads the cameras.
func New(pool *pgxpool.Pool, dir string, maxDiskGB int, secretKey string, log *slog.Logger) *Manager {
	box, _ := iam.NewSecretBox(secretKey)
	return &Manager{pool: pool, dir: dir, maxBytes: int64(maxDiskGB) << 30, set: DefaultSettings(maxDiskGB), box: box, log: log.With("component", "video"), cams: map[string]*cam{}}
}

// Dir is the storage directory of recordings.
func (m *Manager) Dir() string { return m.dir }

// MaxBytes is the disk limit of all recordings.
func (m *Manager) MaxBytes() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxBytes
}

// SealPassword encrypts a camera password for the database.
func (m *Manager) SealPassword(pw string) (string, error) {
	if pw == "" {
		return "", nil
	}
	if m.box == nil {
		return "", errors.New("secret key not configured")
	}
	b, err := m.box.Seal([]byte(pw))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// OpenPassword decrypts a stored camera password.
func (m *Manager) OpenPassword(sealed string) string {
	if sealed == "" || m.box == nil {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return ""
	}
	pw, err := m.box.Open(b)
	if err != nil {
		return ""
	}
	return string(pw)
}

// Run loads all cameras, keeps the retention and stops every stream and recording (closing the open files) when ctx
// ends.
func (m *Manager) Run(ctx context.Context) error {
	if err := os.MkdirAll(m.dir, 0o750); err != nil {
		m.log.Error("video storage not writable; recording is off", "dir", m.dir, "err", err)
	}
	m.loadSettings(ctx)
	m.repair(ctx)
	// events left open by a crash end when the server started again
	_, _ = m.pool.Exec(ctx, `UPDATE core.camera_event SET ended_at = now() WHERE ended_at IS NULL`)
	rows, err := m.pool.Query(ctx, `SELECT id::text FROM core.camera WHERE removed_at IS NULL`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		m.Reload(ctx, id)
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	m.Retention(ctx)
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return nil
		case <-t.C:
			m.Retention(ctx)
		}
	}
}

// repair fixes the index of files that were still being written when the server stopped abruptly: their end is
// the file's last modification.
func (m *Manager) repair(ctx context.Context) {
	rows, err := m.pool.Query(ctx, `SELECT id, path, ended_at, bytes FROM core.video_segment WHERE started_at > now() - interval '1 day'`)
	if err != nil {
		return
	}
	type fix struct {
		id    int64
		end   time.Time
		bytes int64
	}
	var fixes []fix
	for rows.Next() {
		var id, bytes int64
		var path string
		var end time.Time
		if rows.Scan(&id, &path, &end, &bytes) != nil {
			continue
		}
		fi, err := os.Stat(filepath.Join(m.dir, filepath.FromSlash(path)))
		if err == nil && (fi.Size() != bytes || fi.ModTime().After(end.Add(2*time.Second))) {
			fixes = append(fixes, fix{id, fi.ModTime(), fi.Size()})
		}
	}
	rows.Close()
	for _, f := range fixes {
		_, _ = m.pool.Exec(ctx, `UPDATE core.video_segment SET ended_at = greatest(ended_at, $2), bytes = $3 WHERE id = $1`, f.id, f.end, f.bytes)
	}
}

func (m *Manager) stopAll() {
	m.mu.Lock()
	cams := m.cams
	m.cams = map[string]*cam{}
	m.mu.Unlock()
	for _, c := range cams {
		c.stop()
		m.closeOpen(c, time.Now())
	}
}

func (c *cam) stop() {
	if c.stopEvents != nil {
		c.stopEvents()
	}
	if c.rec != nil {
		c.rec.stop()
	}
	c.main.Stop()
	if c.sub != nil {
		c.sub.Stop()
	}
}

// load reads one camera's configuration; ok is false when it is removed or missing.
func (m *Manager) load(ctx context.Context, id string) (Camera, bool) {
	var c Camera
	var sealed, source, pushSealed string
	err := m.pool.QueryRow(ctx, `SELECT id::text, org_id::text, name, url, sub_url, username, password_sealed, transport, record, retention_days, enabled, source, push_secret_sealed,
		onvif_url, ptz_profile, event_pre_s, event_post_s, event_kinds
		FROM core.camera WHERE id = $1::uuid AND removed_at IS NULL`, id).
		Scan(&c.ID, &c.OrgID, &c.Name, &c.URL, &c.SubURL, &c.Username, &sealed, &c.Transport, &c.Record, &c.RetentionDays, &c.Enabled, &source, &pushSealed,
			&c.OnvifURL, &c.PTZProfile, &c.EventPre, &c.EventPost, &c.EventKinds)
	if err != nil {
		return c, false
	}
	c.Password = m.OpenPassword(sealed)
	c.Push, c.PushSecret = source == "push", m.OpenPassword(pushSealed)
	return c, true
}

// StreamURL puts the credentials into a stream URL.
func StreamURL(raw, user, password string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "rtsp", "rtsps", "http", "https":
	default:
		return nil, errors.New("the URL must start with rtsp://, rtsps://, http:// or https://")
	}
	if u.Host == "" {
		return nil, errors.New("the URL has no host")
	}
	if user != "" {
		u.User = url.UserPassword(user, password)
	}
	return u, nil
}

// Reload applies a changed, added or removed camera.
func (m *Manager) Reload(ctx context.Context, id string) {
	cfg, ok := m.load(ctx, id)
	m.mu.Lock()
	old := m.cams[id]
	delete(m.cams, id)
	m.mu.Unlock()
	if old != nil {
		old.stop()
		m.closeOpen(old, time.Now())
	}
	if !ok || !cfg.Enabled {
		return
	}
	m.mu.Lock()
	linger := m.linger()
	m.mu.Unlock()
	c := &cam{cfg: cfg, ev: camEvents{open: map[string]int64{}, lastClose: map[string]closed{}}}
	if cfg.OnvifURL != "" && !cfg.Push {
		c.onvif = &onvif.Client{Device: cfg.OnvifURL, Username: cfg.Username, Password: cfg.Password}
	}
	if cfg.Push {
		// the relay publishes main and (optionally) sub; both sources stay ready to accept it
		c.main = &Source{URL: &url.URL{Scheme: "push"}, Log: m.log.With("camera", cfg.Name), Linger: linger}
		c.sub = &Source{URL: &url.URL{Scheme: "push"}, Log: m.log.With("camera", cfg.Name+" (sub)"), Linger: linger}
		c.main.Keep(true)
		c.sub.Keep(true)
		c.rec = m.recorderFor(c)
		m.mu.Lock()
		m.cams[id] = c
		m.mu.Unlock()
		return
	}
	mu, err := StreamURL(cfg.URL, cfg.Username, cfg.Password)
	if err != nil {
		m.log.Warn("camera URL invalid", "camera", id, "err", err)
		return
	}
	c.main = &Source{URL: mu, Transport: cfg.Transport, Log: m.log.With("camera", cfg.Name), Linger: linger}
	if cfg.SubURL != "" {
		if su, err := StreamURL(cfg.SubURL, cfg.Username, cfg.Password); err == nil {
			c.sub = &Source{URL: su, Transport: cfg.Transport, Log: m.log.With("camera", cfg.Name+" (sub)"), Linger: linger}
		}
	}
	c.rec = m.recorderFor(c)
	m.mu.Lock()
	m.cams[id] = c
	m.mu.Unlock()
	m.startEvents(c)
}

func (m *Manager) recorderFor(c *cam) *recorder {
	switch c.cfg.Record {
	case "continuous":
		return m.startRecorder(c.cfg.ID, c.main, false, 0)
	case "events":
		return m.startRecorder(c.cfg.ID, c.main, true, time.Duration(c.cfg.EventPre)*time.Second)
	}
	return nil
}

// Source returns the live stream of a camera; sub asks for the smaller stream (tiles) and falls back to the main one.
func (m *Manager) Source(id string, sub bool) *Source {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.cams[id]
	if c == nil {
		return nil
	}
	if sub && c.sub != nil {
		if c.cfg.Push && c.sub.Status().State != "online" {
			return c.main // the relay sends no sub-stream
		}
		return c.sub
	}
	return c.main
}

// pushTarget is the source a relay publishes to, with the camera's secret.
func (m *Manager) pushTarget(id string, sub bool) (*Source, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.cams[id]
	if c == nil || !c.cfg.Push {
		return nil, ""
	}
	if sub {
		return c.sub, c.cfg.PushSecret
	}
	return c.main, c.cfg.PushSecret
}

// Status of a camera's main stream (and whether it records); ok is false for disabled or unknown cameras.
func (m *Manager) Status(id string) (Status, bool) {
	m.mu.Lock()
	c := m.cams[id]
	m.mu.Unlock()
	if c == nil {
		return Status{State: "disabled"}, false
	}
	st := c.main.Status()
	st.Recording = c.rec != nil && st.State == "online" && (!c.rec.events || c.rec.armed(time.Now()))
	if c.rec != nil && c.rec.events {
		st.RecordMode = "events"
	}
	if c.onvif != nil {
		c.ev.mu.Lock()
		st.Events = c.ev.status
		if st.Events == "" {
			st.Events = "connecting"
		}
		for k := range c.ev.open {
			kind, _, _ := cutKey(k)
			if !slices.Contains(st.Active, kind) {
				st.Active = append(st.Active, kind)
			}
		}
		c.ev.mu.Unlock()
	}
	if c.sub != nil {
		st.Viewers += c.sub.Status().Viewers
	}
	if c.rec != nil && st.Viewers > 0 {
		st.Viewers-- // the recorder is not a viewer
	}
	return st, true
}

// Probe connects to a stream once and reports what it offers (the "test connection" of the camera form).
func Probe(ctx context.Context, u *url.URL, transport string) (Status, []byte) {
	src := &Source{URL: u, Transport: transport, Linger: time.Millisecond}
	sub := src.Subscribe()
	defer src.Stop()
	for {
		select {
		case p, ok := <-sub.C:
			if !ok {
				return src.Status(), nil
			}
			return Status{State: "online", Codec: p.Track.Codec, Width: p.Track.Width, Height: p.Track.Height}, p.Sample.JPEG
		case <-ctx.Done():
			st := src.Status()
			if st.State != "offline" {
				st = Status{State: "offline", Error: "no picture within the time limit"}
			}
			return st, nil
		case <-time.After(200 * time.Millisecond):
			if st := src.Status(); st.State == "offline" {
				return st, nil
			}
		}
	}
}

// Retention deletes recordings older than each camera's retention and, beyond the disk limit, the oldest ones.
func (m *Manager) Retention(ctx context.Context) {
	m.deleteWhere(ctx, `SELECT s.id, s.path, s.bytes FROM core.video_segment s JOIN core.camera c ON c.id = s.camera_id
		WHERE s.ended_at < now() - make_interval(days => c.retention_days) ORDER BY s.started_at LIMIT 500`)
	_, _ = m.pool.Exec(ctx, `DELETE FROM core.camera_event e USING core.camera c WHERE c.id = e.camera_id AND e.started_at < now() - make_interval(days => c.retention_days)`)
	var total int64
	if err := m.pool.QueryRow(ctx, `SELECT COALESCE(sum(bytes), 0) FROM core.video_segment`).Scan(&total); err != nil {
		return
	}
	for total > m.MaxBytes() && ctx.Err() == nil {
		freed := m.deleteWhere(ctx, `SELECT id, path, bytes FROM core.video_segment WHERE ended_at < now() - interval '2 minutes' ORDER BY started_at LIMIT 50`)
		if freed == 0 {
			return
		}
		total -= freed
	}
}

func (m *Manager) deleteWhere(ctx context.Context, q string) int64 {
	rows, err := m.pool.Query(ctx, q)
	if err != nil {
		m.log.Warn("retention query", "err", err)
		return 0
	}
	type row struct {
		id, bytes int64
		path      string
	}
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.path, &r.bytes) == nil {
			list = append(list, r)
		}
	}
	rows.Close()
	var freed int64
	for _, r := range list {
		full := filepath.Join(m.dir, filepath.FromSlash(r.path))
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			m.log.Warn("recording not deleted", "path", r.path, "err", err)
			continue
		}
		_ = os.Remove(filepath.Dir(full)) // the day directory, once empty
		if _, err := m.pool.Exec(ctx, `DELETE FROM core.video_segment WHERE id = $1`, r.id); err != nil {
			m.log.Warn("recording index not deleted", "id", r.id, "err", err)
			continue
		}
		freed += r.bytes
	}
	return freed
}
