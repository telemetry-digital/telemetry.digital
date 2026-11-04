package video

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/testsrc"
)

// A file left by the overflow of the time of a long connection (it ends before it starts, and it began hours before the
// wanted time) is repaired at the start of the server and is found by playback at the right time. Needs a database with
// the schema (CTRL32_TEST_DB); skipped otherwise.
func TestLongFileRepairedAndPlayedDB(t *testing.T) {
	url := os.Getenv("CTRL32_TEST_DB")
	if url == "" {
		t.Skip("CTRL32_TEST_DB not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var org string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM core.organization ORDER BY created_at LIMIT 1`).Scan(&org); err != nil {
		t.Skip("no organization in the database: ", err)
	}
	var cam string
	if err := pool.QueryRow(ctx, `INSERT INTO core.camera (org_id, name, url, record, enabled, created_by) VALUES ($1::uuid, $2, 'rtsp://127.0.0.1:1/x', 'off', false, 'test') RETURNING id::text`,
		org, fmt.Sprintf("long-%d", time.Now().UnixNano()%1000000)).Scan(&cam); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.video_segment WHERE camera_id = $1::uuid`, cam)
		_, _ = pool.Exec(ctx, `DELETE FROM core.camera WHERE id = $1::uuid`, cam)
	}()

	// three hours in one file, a fragment each second
	const hours = 3
	dir := t.TempDir()
	rel := filepath.ToSlash(filepath.Join(cam, "long.mp4"))
	if err := os.MkdirAll(filepath.Join(dir, cam), 0o750); err != nil {
		t.Fatal(err)
	}
	enc := &testsrc.Encoder{W: 64, H: 48}
	tr := &Track{Codec: H264, SPS: enc.SPS(), PPS: enc.PPS()}
	mux := &Muxer{Track: tr, Batch: ClockRate}
	f, err := os.Create(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	init, err := mux.Init()
	if err != nil {
		t.Fatal(err)
	}
	f.Write(init)
	frame := enc.Frame()
	for i := 0; i <= hours*3600; i++ {
		frag, err := mux.Push(&Sample{DTS: int64(i) * ClockRate, Key: true, AU: frame})
		if err != nil {
			t.Fatal(err)
		}
		f.Write(frag)
	}
	f.Close()
	fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now().Add(-hours * time.Hour).Truncate(time.Second)
	var seg int64
	if err := pool.QueryRow(ctx, `INSERT INTO core.video_segment (camera_id, started_at, ended_at, path, bytes, codec) VALUES ($1::uuid, $2, $3, $4, $5, 'h264') RETURNING id`,
		cam, start, start.Add(-57*time.Hour), rel, int64(1000)).Scan(&seg); err != nil {
		t.Fatal(err)
	}

	m := New(pool, dir, 1, "0123456789abcdef-secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.repair(ctx)
	var end time.Time
	var size int64
	if err := pool.QueryRow(ctx, `SELECT ended_at, bytes FROM core.video_segment WHERE id = $1`, seg).Scan(&end, &size); err != nil {
		t.Fatal(err)
	}
	if end.Sub(fi.ModTime()).Abs() > time.Millisecond {
		t.Errorf("end %v, want the file's last modification %v", end, fi.ModTime())
	}
	if size != fi.Size() {
		t.Errorf("bytes %d, want %d", size, fi.Size())
	}

	// two hours and a half into the file: far beyond the look back for normal files
	from := start.Add(150 * time.Minute)
	p := m.Playback(cam, from, from.Add(10*time.Second))
	defer p.Close()
	pk, err := p.Next(ctx)
	if err != nil {
		t.Fatal("the long file was not found: ", err)
	}
	if d := pk.Sample.Time.Sub(from).Abs(); d > 2*time.Second {
		t.Errorf("playback starts at %v, want %v", pk.Sample.Time, from)
	}

	// an evidence package names the file; a file too large to read at export is marked, not read
	segs, err := m.EvidenceSegments(ctx, cam, from, from.Add(10*time.Second))
	if err != nil || len(segs) != 1 || segs[0].NotChecked || segs[0].SHA256Now == "" {
		t.Fatalf("segments of a normal size: %+v %v", segs, err)
	}
	old := evidenceHashMax
	evidenceHashMax = 1000
	defer func() { evidenceHashMax = old }()
	segs, err = m.EvidenceSegments(ctx, cam, from, from.Add(10*time.Second))
	if err != nil || len(segs) != 1 || !segs[0].NotChecked || segs[0].SHA256Now != "" || segs[0].Unchanged {
		t.Fatalf("too large a file: %+v %v", segs, err)
	}
}
