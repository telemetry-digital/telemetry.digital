package agent

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// The history of backup runs (backup-history.json next to backup.json): every backup, offsite synchronisation and
// restore test with its steps — the protocol the Backups page shows and the web service audits (URS K-02, K-03).

var backupHistoryMu sync.Mutex

const backupHistoryMax = 200

func backupHistoryFile() string {
	return filepath.Join(filepath.Dir(backupStateFile), "backup-history.json")
}

func loadBackupHistory(limit int) []BackupRun {
	backupHistoryMu.Lock()
	defer backupHistoryMu.Unlock()
	var h []BackupRun
	_ = loadJSON(backupHistoryFile(), &h)
	if h == nil {
		h = []BackupRun{}
	}
	if limit > 0 && len(h) > limit {
		h = h[:limit]
	}
	return h
}

// saveBackupRun inserts or replaces a run (newest first).
func saveBackupRun(r BackupRun) {
	backupHistoryMu.Lock()
	defer backupHistoryMu.Unlock()
	var h []BackupRun
	_ = loadJSON(backupHistoryFile(), &h)
	if i := slices.IndexFunc(h, func(x BackupRun) bool { return x.ID == r.ID }); i >= 0 {
		r.Audited = h[i].Audited
		h[i] = r
	} else {
		h = append([]BackupRun{r}, h...)
	}
	if len(h) > backupHistoryMax {
		h = h[:backupHistoryMax]
	}
	_ = saveJSON(backupHistoryFile(), h, 0o640)
}

// runRec records the steps of one run; a nil *runRec runs the steps without recording (preUpdateBackup).
type runRec struct {
	a   *Server
	run BackupRun
}

func (a *Server) newRun(kind, trigger, actor string) *runRec {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	r := &runRec{a: a, run: BackupRun{ID: time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b), Kind: kind, Trigger: trigger, Actor: actor,
		StartedAt: time.Now(), Running: true, Steps: []BackupStep{}}}
	saveBackupRun(r.run)
	return r
}

// step runs fn and records its result and duration; it returns fn's error.
func (r *runRec) step(name string, fn func() (string, error)) error {
	t0 := time.Now()
	detail, err := fn()
	if r == nil {
		return err
	}
	s := BackupStep{Name: name, OK: err == nil, Detail: detail, Ms: time.Since(t0).Milliseconds()}
	if err != nil {
		s.Detail = err.Error()
	}
	r.run.Steps = append(r.run.Steps, s)
	saveBackupRun(r.run)
	return err
}

// note records a step that needed no work (skipped, or a warning) without failing the run.
func (r *runRec) note(name, detail string) {
	if r == nil {
		return
	}
	r.run.Steps = append(r.run.Steps, BackupStep{Name: name, OK: true, Detail: detail})
	saveBackupRun(r.run)
}

func (r *runRec) finish(err error) BackupRun {
	r.run.Running, r.run.FinishedAt, r.run.OK = false, time.Now(), err == nil
	if err != nil {
		r.run.Error = err.Error()
	}
	saveBackupRun(r.run)
	return r.run
}

// hBackupAck marks runs as written into the server's audit trail.
func (a *Server) hBackupAck(w http.ResponseWriter, r *http.Request) {
	var in AckRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	backupHistoryMu.Lock()
	var h []BackupRun
	_ = loadJSON(backupHistoryFile(), &h)
	for i := range h {
		if slices.Contains(in.IDs, h[i].ID) {
			h[i].Audited = true
		}
	}
	err := saveJSON(backupHistoryFile(), h, 0o640)
	backupHistoryMu.Unlock()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, Result{OK: true})
}
