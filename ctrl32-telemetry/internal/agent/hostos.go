package agent

// Operating system maintenance (ADR-0025, spec 034): the pending package upgrades, an upgrade of the system packages,
// a delayed reboot and the journal of a closed list of units — the work that used to need a remote shell. Debian and
// Ubuntu (apt) only; elsewhere GET /os says so and the operations are refused. Nothing comes from a request into a
// command line except the job id the agent generated itself and the fixed unit names below.
//
//	GET  /os                 state: upgrades, reboot required, scheduled reboot, running job with its log, history
//	POST /os/check           apt-get update and the list of upgrades again (a job, a few seconds)
//	POST /os/upgrade         apt-get update + full-upgrade in its own systemd unit (survives an agent restart)
//	POST /os/reboot          a reboot in delay_s seconds (default 60); refused while a job, an update or a backup runs
//	POST /os/reboot/cancel   cancel the scheduled reboot
//	POST /os/ack             the server wrote the finished jobs into its audit trail
//	GET  /logs?unit=&lines=  the journal of one of the units of logUnits

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	osJobUnitPrefix = "ctrl32-os-upgrade-"
	rebootUnit      = "ctrl32-telemetry-reboot"
	maxOSHistory    = 12
	osLogTail       = 64 << 10 // the part of a job's log the state carries
	upgradeListTTL  = 5 * time.Minute
)

var (
	osFileMu   sync.Mutex
	osChecking atomic.Bool
	jobIDRe    = regexp.MustCompile(`^[0-9a-f]{12}$`)
)

// osDir keeps the jobs, their logs and the scheduled reboot (Linux: /var/lib/ctrl32-telemetry/os).
func osDir() string { return filepath.Join(libDir, "os") }

func osJobsFile() string   { return filepath.Join(osDir(), "jobs.json") }
func osRebootFile() string { return filepath.Join(osDir(), "reboot.json") }

type osStore struct {
	Jobs   []OSJob      `json:"jobs"` // newest first
	Reboot *OSReboot    `json:"reboot,omitempty"`
	List   *upgradeList `json:"list,omitempty"`
}

type upgradeList struct {
	Upgrades  []PackageUpgrade `json:"upgrades"`
	CheckedAt time.Time        `json:"checked_at"`
	Error     string           `json:"error,omitempty"`
}

func loadOSStore() osStore {
	var st osStore
	_ = loadJSON(osJobsFile(), &st)
	return st
}

func saveOSStore(st osStore) error {
	if len(st.Jobs) > maxOSHistory {
		st.Jobs = st.Jobs[:maxOSHistory]
	}
	return saveJSON(osJobsFile(), st, 0o640)
}

// saveOSJob puts the job in front of the history (or replaces it there).
func saveOSJob(j OSJob) {
	osFileMu.Lock()
	defer osFileMu.Unlock()
	st := loadOSStore()
	st.Jobs = slices.DeleteFunc(st.Jobs, func(x OSJob) bool { return x.ID == j.ID })
	st.Jobs = append([]OSJob{j}, st.Jobs...)
	_ = saveOSStore(st)
}

// osSupported: the agent maintains the packages of Debian and Ubuntu (apt and systemd); the simulation stands in for
// them in development and tests.
func (a *Server) osSupported() (bool, string) {
	if a.simulate {
		return true, ""
	}
	if isWindows() {
		return false, "Windows: install the Windows updates with Windows Update; this page maintains Linux servers (apt) only"
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return false, "no apt-get on this server: the operating system packages are maintained with apt (Debian, Ubuntu) only"
	}
	if _, err := exec.LookPath("systemd-run"); err != nil {
		return false, "no systemd-run on this server: the upgrade runs in its own systemd unit"
	}
	return true, ""
}

func (a *Server) osRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /os", a.hOS)
	mux.HandleFunc("POST /os/check", a.hOSCheck)
	mux.HandleFunc("POST /os/upgrade", a.hOSUpgrade)
	mux.HandleFunc("POST /os/reboot", a.hOSReboot)
	mux.HandleFunc("POST /os/reboot/cancel", a.hOSRebootCancel)
	mux.HandleFunc("POST /os/ack", a.hOSAck)
	mux.HandleFunc("GET /logs", a.hLogs)
}

// ---- state --------------------------------------------------------------------------------------------------------

func (a *Server) hOS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	writeJSON(w, 200, a.osState(r.Context(), q.Get("refresh") == "1", q.Get("jobs") == "1"))
}

// osState: jobsOnly leaves the list of upgrades as it was stored (the server's minute tick reads only the jobs).
func (a *Server) osState(ctx context.Context, refresh, jobsOnly bool) OSState {
	out := OSState{Upgrades: []PackageUpgrade{}, History: []OSJob{}, RebootPackages: []string{}}
	out.Supported, out.Reason = a.osSupported()
	out.BootedAt = bootTime()
	if !out.Supported {
		return out
	}
	out.PackageManager = "apt"
	a.finishOSJobs(ctx)
	osFileMu.Lock()
	st := loadOSStore()
	osFileMu.Unlock()
	running := false
	for i, j := range st.Jobs {
		if j.Phase == "running" {
			running = true
			j.Log = tailFile(filepath.Join(osDir(), j.ID+".log"), osLogTail)
			out.Job = &j
		} else if i == 0 {
			j.Log = tailFile(filepath.Join(osDir(), j.ID+".log"), osLogTail)
			out.Last = &j
		}
		h := j
		h.Log = ""
		out.History = append(out.History, h)
	}
	out.Checking = osChecking.Load()
	list := st.List
	if !jobsOnly && !running && !out.Checking && (refresh || list == nil || time.Since(list.CheckedAt) > upgradeListTTL) {
		l := a.listUpgrades(ctx)
		list = &l
		osFileMu.Lock()
		st2 := loadOSStore()
		st2.List = list
		_ = saveOSStore(st2)
		osFileMu.Unlock()
	}
	if list != nil {
		out.Upgrades, out.CheckedAt, out.ListError = list.Upgrades, list.CheckedAt, list.Error
		if out.Upgrades == nil {
			out.Upgrades = []PackageUpgrade{}
		}
	}
	for _, u := range out.Upgrades {
		if u.Security {
			out.SecurityCount++
		}
	}
	out.UpgradeCount = len(out.Upgrades)
	out.ListsUpdatedAt = a.listsUpdated()
	out.RebootRequired, out.RebootPackages = a.rebootRequired()
	if !jobsOnly {
		out.Reboot = a.scheduledReboot(ctx)
	}
	return out
}

// listUpgrades asks apt what a full upgrade would install (a simulation: nothing changes, no lock is taken).
func (a *Server) listUpgrades(ctx context.Context) upgradeList {
	l := upgradeList{CheckedAt: time.Now().UTC()}
	if a.simulate {
		l.Upgrades = simUpgrades()
		return l
	}
	cmd := exec.CommandContext(ctx, "apt-get", "-s", "-o", "Debug::NoLocking=1", "full-upgrade")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
	b, err := cmd.Output()
	if err != nil {
		l.Error = "apt-get -s full-upgrade: " + err.Error()
	}
	l.Upgrades = parseAptSimulation(string(b))
	return l
}

var aptInstRe = regexp.MustCompile(`^Inst (\S+) (?:\[([^\]]*)\] )?\((\S+) (.*)\)\s*$`)

// parseAptSimulation reads the "Inst" lines of `apt-get -s`:
//
//	Inst libssl3t64 [3.0.13-0ubuntu3.4] (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
//	Inst linux-image-7.0.0-42-generic (7.0.0-42.42 Ubuntu:26.04/resolute-updates [amd64])   (a new package)
func parseAptSimulation(out string) []PackageUpgrade {
	list := []PackageUpgrade{}
	for _, l := range strings.Split(out, "\n") {
		m := aptInstRe.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		origin := m[4]
		if i := strings.LastIndex(origin, " ["); i >= 0 {
			origin = origin[:i]
		}
		u := PackageUpgrade{Name: m[1], From: m[2], To: m[3], Origin: origin, New: m[2] == ""}
		u.Security = strings.Contains(origin, "-security") || strings.Contains(origin, "Debian-Security")
		list = append(list, u)
	}
	return list
}

// listsUpdated is when the package lists were last downloaded (apt-get update).
func (a *Server) listsUpdated() time.Time {
	dir := "/var/lib/apt/lists"
	if a.simulate {
		dir = osDir()
	}
	var newest time.Time
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, "Release") && !strings.HasSuffix(n, "InRelease") && n != "lists.stamp" {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	return newest
}

// rebootRequired: Debian and Ubuntu flag a needed reboot in /var/run/reboot-required (with the packages that need it).
func (a *Server) rebootRequired() (bool, []string) {
	flag, pkgs := "/var/run/reboot-required", "/var/run/reboot-required.pkgs"
	if a.simulate {
		flag, pkgs = filepath.Join(osDir(), "reboot-required"), filepath.Join(osDir(), "reboot-required.pkgs")
	}
	if _, err := os.Stat(flag); err != nil {
		return false, []string{}
	}
	return true, parseRebootPackages(readFile(pkgs))
}

func parseRebootPackages(s string) []string {
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// bootTime is when the host started (from /proc/uptime).
func bootTime() time.Time {
	f := strings.Fields(readFile("/proc/uptime"))
	if len(f) == 0 {
		return time.Time{}
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return time.Time{}
	}
	return time.Now().Add(-time.Duration(v * float64(time.Second))).UTC().Truncate(time.Second)
}

func tailFile(path string, max int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	if fi.Size() > max {
		_, _ = f.Seek(-max, io.SeekEnd)
	}
	b, _ := io.ReadAll(io.LimitReader(f, max))
	s := string(b)
	if fi.Size() > max {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
	}
	return s
}

// ---- jobs ---------------------------------------------------------------------------------------------------------

// osBusy names what keeps the host from a new job or a reboot: a running job, an update of ctrl32 telemetry, a backup.
func (a *Server) osBusy(ctx context.Context) string {
	a.finishOSJobs(ctx)
	osFileMu.Lock()
	st := loadOSStore()
	osFileMu.Unlock()
	for _, j := range st.Jobs {
		if j.Phase == "running" {
			return "the operating system job " + j.ID + " (" + j.Kind + ") is still running"
		}
	}
	if osChecking.Load() {
		return "the list of package upgrades is being refreshed"
	}
	if c := busy(time.Now()); c != nil {
		return "an update of ctrl32 telemetry to " + c.To + " is in progress (" + c.Phase + ")"
	}
	if backupRunning.Load() {
		return "a backup is running"
	}
	if restoreRunning.Load() {
		return "a restore test is running"
	}
	return ""
}

func (a *Server) hOSCheck(w http.ResponseWriter, r *http.Request) {
	var in OSRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	if ok, why := a.osSupported(); !ok {
		writeCoded(w, 501, "unsupported", why)
		return
	}
	if why := a.osBusy(r.Context()); why != "" {
		writeJSON(w, 409, map[string]string{"error": why, "code": "busy"})
		return
	}
	if !osChecking.CompareAndSwap(false, true) {
		writeJSON(w, 409, map[string]string{"error": "the list of package upgrades is being refreshed", "code": "busy"})
		return
	}
	j := OSJob{ID: randomID()[:12], Kind: "check", Actor: in.Actor, Reason: in.Reason, Phase: "running", StartedAt: time.Now().UTC()}
	saveOSJob(j)
	go func() {
		defer osChecking.Store(false)
		logPath := filepath.Join(osDir(), j.ID+".log")
		lf, _ := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		var err error
		if a.simulate {
			fmt.Fprintln(lf, "Hit:1 http://archive.example (simulation) InRelease")
			time.Sleep(300 * time.Millisecond)
			_ = os.WriteFile(filepath.Join(osDir(), "lists.stamp"), nil, 0o640)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			cmd := exec.CommandContext(ctx, "apt-get", "-o", "DPkg::Lock::Timeout=120", "update")
			cmd.Env = append(os.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
			cmd.Stdout, cmd.Stderr = lf, lf
			err = cmd.Run()
			cancel()
		}
		l := a.listUpgrades(context.Background())
		fmt.Fprintf(lf, "\n%d package upgrades available\n", len(l.Upgrades))
		if lf != nil {
			_ = lf.Close()
		}
		osFileMu.Lock()
		st := loadOSStore()
		st.List = &l
		_ = saveOSStore(st)
		osFileMu.Unlock()
		j.FinishedAt, j.Phase, j.Upgrades = time.Now().UTC(), "done", len(l.Upgrades)
		if err != nil {
			j.Phase, j.Error = "failed", "apt-get update: "+err.Error()
		}
		saveOSJob(j)
		a.log.Info("package lists refreshed", "job", j.ID, "upgrades", len(l.Upgrades), "err", err)
	}()
	writeJSON(w, 202, j)
}

func (a *Server) hOSUpgrade(w http.ResponseWriter, r *http.Request) {
	var in OSRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	if ok, why := a.osSupported(); !ok {
		writeCoded(w, 501, "unsupported", why)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if why := a.osBusy(r.Context()); why != "" {
		writeJSON(w, 409, map[string]string{"error": why, "code": "busy"})
		return
	}
	if rb := a.scheduledReboot(r.Context()); rb != nil {
		writeJSON(w, 409, map[string]string{"error": "a reboot is scheduled for " + rb.At.Format(time.RFC3339) + "; cancel it first", "code": "busy"})
		return
	}
	j := OSJob{ID: randomID()[:12], Kind: "upgrade", Actor: in.Actor, Reason: in.Reason, Phase: "running", StartedAt: time.Now().UTC()}
	_ = os.MkdirAll(osDir(), 0o750)
	logPath := filepath.Join(osDir(), j.ID+".log")
	_ = os.WriteFile(logPath, []byte(fmt.Sprintf("%s upgrade of the operating system packages by %s: %s\n", j.StartedAt.Format(time.RFC3339), in.Actor, in.Reason)), 0o640)
	saveOSJob(j)
	if a.simulate {
		go simulateUpgrade(j.ID)
	} else {
		args := []string{"--unit=" + osJobUnitPrefix + j.ID, "--collect", "--quiet", "--description=ctrl32 telemetry: upgrade of the operating system packages",
			"--setenv=DEBIAN_FRONTEND=noninteractive", "--setenv=NEEDRESTART_MODE=a", "--setenv=LC_ALL=C",
			"--property=StandardOutput=append:" + logPath, "--property=StandardError=append:" + logPath,
			binPath, "update", "os-upgrade", "-dir", osDir(), "-id", j.ID}
		if out, err := a.exec(r.Context(), 30*time.Second, "systemd-run", args...); err != nil {
			j.Phase, j.Error, j.FinishedAt = "failed", "the upgrade could not be started: "+firstLine(out+" "+err.Error()), time.Now().UTC()
			saveOSJob(j)
			writeErr(w, 500, j.Error)
			return
		}
	}
	a.log.Info("operating system upgrade started", "job", j.ID, "actor", in.Actor, "reason", in.Reason)
	writeJSON(w, 202, j)
}

// finishOSJobs completes the records of upgrade jobs whose unit has ended: the result file of the job, or — when the
// unit is gone without one (the host restarted, the job was killed) — a failed record.
func (a *Server) finishOSJobs(ctx context.Context) {
	osFileMu.Lock()
	defer osFileMu.Unlock()
	st := loadOSStore()
	changed := false
	for i, j := range st.Jobs {
		if j.Phase != "running" {
			continue
		}
		if j.Kind == "check" {
			if !osChecking.Load() { // the agent restarted during a check
				st.Jobs[i].Phase, st.Jobs[i].Error, st.Jobs[i].FinishedAt = "failed", "interrupted (the server agent restarted)", time.Now().UTC()
				changed = true
			}
			continue
		}
		var res osJobResult
		if loadJSON(filepath.Join(osDir(), j.ID+".result.json"), &res) == nil {
			st.Jobs[i].Phase, st.Jobs[i].FinishedAt, st.Jobs[i].ExitCode, st.Jobs[i].Upgrades = "done", res.FinishedAt, res.ExitCode, res.Upgraded
			st.Jobs[i].RebootRequired = res.RebootRequired
			if res.ExitCode != 0 || res.Error != "" {
				st.Jobs[i].Phase, st.Jobs[i].Error = "failed", res.Error
			}
			changed = true
			continue
		}
		if a.simulate {
			continue
		}
		act, _ := a.exec(ctx, 5*time.Second, "systemctl", "is-active", osJobUnitPrefix+j.ID+".service")
		switch firstLine(act) {
		case "active", "activating", "reloading":
		default:
			if time.Since(j.StartedAt) > 20*time.Second { // systemd-run returns before the unit shows as active
				st.Jobs[i].Phase, st.Jobs[i].Error, st.Jobs[i].FinishedAt = "failed", "the upgrade ended without a result (the server restarted or the job was stopped); see its log", time.Now().UTC()
				changed = true
			}
		}
	}
	if changed {
		st.List = nil // what is pending changed
		_ = saveOSStore(st)
	}
}

type osJobResult struct {
	ExitCode       int       `json:"exit_code"`
	Upgraded       int       `json:"upgraded"`
	RebootRequired bool      `json:"reboot_required"`
	Error          string    `json:"error,omitempty"`
	FinishedAt     time.Time `json:"finished_at"`
}

var aptSummaryRe = regexp.MustCompile(`(?m)^(\d+) upgraded, (\d+) newly installed`)

// RunOSUpgrade is `ctrl32-telemetry update os-upgrade`: started by the agent in its own systemd unit, it refreshes
// the package lists and installs every upgrade without questions (configuration files the administrator changed are
// kept), with the output going to the job's log. The result goes to <dir>/<id>.result.json.
func RunOSUpgrade(ctx context.Context, dir, id string, log *slog.Logger) error {
	if !jobIDRe.MatchString(id) {
		return errors.New("invalid job id")
	}
	res := osJobResult{}
	step := func(args ...string) error {
		fmt.Printf("\n$ apt-get %s\n", strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, "apt-get", args...)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "LC_ALL=C")
		var buf strings.Builder
		cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
		cmd.Stderr = os.Stdout
		err := cmd.Run()
		if m := aptSummaryRe.FindStringSubmatch(buf.String()); m != nil {
			n, _ := strconv.Atoi(m[1])
			k, _ := strconv.Atoi(m[2])
			res.Upgraded += n + k
		}
		return err
	}
	lock := []string{"-o", "DPkg::Lock::Timeout=600"}
	err := step(append(lock, "update")...)
	if err == nil {
		err = step(append(lock, "-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "full-upgrade")...)
	}
	if err != nil {
		res.Error = err.Error()
		res.ExitCode = 1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		}
	}
	_, e := os.Stat("/var/run/reboot-required")
	res.RebootRequired = e == nil
	res.FinishedAt = time.Now().UTC()
	fmt.Printf("\n%s finished: %d packages upgraded or installed, exit code %d, reboot required: %v\n", res.FinishedAt.Format(time.RFC3339), res.Upgraded, res.ExitCode, res.RebootRequired)
	if log != nil {
		log.Info("operating system upgrade finished", "job", id, "upgraded", res.Upgraded, "exit", res.ExitCode)
	}
	return saveJSON(filepath.Join(dir, id+".result.json"), res, 0o640)
}

func (a *Server) hOSAck(w http.ResponseWriter, r *http.Request) {
	var in AckRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	osFileMu.Lock()
	st := loadOSStore()
	for i := range st.Jobs {
		if slices.Contains(in.IDs, st.Jobs[i].ID) {
			st.Jobs[i].Audited = true
		}
	}
	err := saveOSStore(st)
	osFileMu.Unlock()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, Result{OK: true})
}

// ---- reboot -------------------------------------------------------------------------------------------------------

func (a *Server) scheduledReboot(ctx context.Context) *OSReboot {
	var rb OSReboot
	if loadJSON(osRebootFile(), &rb) != nil || rb.At.IsZero() {
		return nil
	}
	// a reboot that happened (the host started after it was asked for) or one that was cancelled is no longer pending
	if bt := bootTime(); !a.simulate && !bt.IsZero() && bt.After(rb.RequestedAt) {
		return nil
	}
	if !a.simulate {
		if act, _ := a.exec(ctx, 5*time.Second, "systemctl", "is-active", rebootUnit+".timer"); firstLine(act) != "active" && time.Now().Before(rb.At) {
			return nil
		}
	}
	if time.Since(rb.At) > 10*time.Minute {
		return nil
	}
	return &rb
}

func (a *Server) hOSReboot(w http.ResponseWriter, r *http.Request) {
	var in OSRebootRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	if in.DelaySeconds == 0 {
		in.DelaySeconds = 60
	}
	if in.DelaySeconds < 30 || in.DelaySeconds > 3600 {
		writeCoded(w, 400, "invalid", "delay_s must be between 30 and 3600")
		return
	}
	if isWindows() && !a.simulate {
		writeCoded(w, 501, "unsupported", "Windows: restart the computer with Windows tools; the server agent reboots Linux servers only")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if why := a.osBusy(r.Context()); why != "" {
		writeJSON(w, 409, map[string]string{"error": "no reboot now: " + why, "code": "busy"})
		return
	}
	if rb := a.scheduledReboot(r.Context()); rb != nil {
		writeJSON(w, 409, map[string]string{"error": "a reboot is already scheduled for " + rb.At.Format(time.RFC3339), "code": "busy"})
		return
	}
	now := time.Now().UTC()
	rb := OSReboot{ID: randomID()[:12], RequestedAt: now, At: now.Add(time.Duration(in.DelaySeconds) * time.Second), Actor: in.Actor, Reason: in.Reason}
	if !a.simulate {
		// a timer unit: visible in systemctl list-timers, cancelled by stopping it
		out, err := a.exec(r.Context(), 20*time.Second, "systemd-run", "--unit="+rebootUnit, "--on-active="+strconv.Itoa(in.DelaySeconds)+"s",
			"--timer-property=AccuracySec=1s", "--description=ctrl32 telemetry: reboot requested by "+sanitizeDesc(in.Actor), "/bin/systemctl", "reboot")
		if err != nil {
			writeErr(w, 500, "the reboot could not be scheduled: "+firstLine(out+" "+err.Error()))
			return
		}
	} else {
		_, _ = a.exec(r.Context(), time.Second, "systemd-run", "--unit="+rebootUnit, "--on-active="+strconv.Itoa(in.DelaySeconds)+"s", "/bin/systemctl", "reboot")
	}
	_ = saveJSON(osRebootFile(), rb, 0o640)
	a.log.Warn("reboot scheduled", "at", rb.At, "actor", in.Actor, "reason", in.Reason)
	writeJSON(w, 202, rb)
}

func (a *Server) hOSRebootCancel(w http.ResponseWriter, r *http.Request) {
	var in OSRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	rb := a.scheduledReboot(r.Context())
	if rb == nil {
		writeCoded(w, 404, "not_found", "no reboot is scheduled")
		return
	}
	if out, err := a.exec(r.Context(), 20*time.Second, "systemctl", "stop", rebootUnit+".timer"); err != nil && !a.simulate {
		writeErr(w, 500, "the reboot could not be cancelled: "+firstLine(out+" "+err.Error()))
		return
	}
	_ = os.Remove(osRebootFile())
	a.log.Warn("scheduled reboot cancelled", "actor", in.Actor, "reason", in.Reason)
	writeJSON(w, 200, rb)
}

// writeCoded answers a refusal with a code, so the web service passes the status on (busy → 409, unsupported → 501).
func writeCoded(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// sanitizeDesc keeps a user name printable for a unit description (it is not parsed by a shell: systemd-run gets
// arguments, but a description should not carry control characters).
func sanitizeDesc(s string) string {
	b := []rune{}
	for _, r := range s {
		if r >= 32 && r != 127 && len(b) < 64 {
			b = append(b, r)
		}
	}
	return string(b)
}

// ---- logs ---------------------------------------------------------------------------------------------------------

// logUnits is the closed list of logs the server screen may read (key → systemd unit or Docker container).
var logUnits = map[string]struct {
	label, unit, container string
	kernel                 bool
}{
	"app":         {label: "ctrl32 telemetry", unit: appUnit},
	"agent":       {label: "server agent", unit: agentUnit},
	"caddy":       {label: "reverse proxy (Caddy)", container: caddyContainer},
	"postgresql":  {label: "PostgreSQL", container: pgContainerDefault},
	"wireguard":   {label: "WireGuard (wg0)", unit: wgUnit},
	"cloudflared": {label: "Cloudflare Tunnel", unit: "cloudflared"},
	"kernel":      {label: "kernel", kernel: true},
}

// LogKeys lists the logs in their order on the screen.
var LogKeys = []string{"app", "agent", "caddy", "postgresql", "wireguard", "cloudflared", "kernel"}

func (a *Server) hLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	key := q.Get("unit")
	u, ok := logUnits[key]
	if !ok {
		writeCoded(w, 400, "invalid", "unit must be one of "+strings.Join(LogKeys, ", "))
		return
	}
	lines, _ := strconv.Atoi(q.Get("lines"))
	if lines <= 0 {
		lines = 200
	}
	lines = min(lines, 2000)
	out := LogResult{Unit: key, Label: u.label, Lines: []string{}}
	if a.simulate {
		out.Lines = []string{time.Now().Format(time.RFC3339) + " simulation: log of " + u.label}
		writeJSON(w, 200, out)
		return
	}
	if isWindows() {
		writeCoded(w, 501, "unsupported", "Windows: read the logs in the Event Viewer (Windows Logs → Application) and in the log file of config.toml; this view reads the journal of Linux servers")
		return
	}
	var text string
	var err error
	ctx := r.Context()
	n := strconv.Itoa(lines)
	switch {
	case u.container != "":
		name := u.container
		if key == "postgresql" && a.pgContainer != "" {
			name = a.pgContainer
		}
		text, err = run(ctx, 20*time.Second, "docker", "logs", "--tail", n, "--timestamps", name)
	case u.kernel:
		text, err = run(ctx, 20*time.Second, "journalctl", "-k", "-b", "-n", n, "--no-pager", "-o", "short-iso")
	default:
		text, err = run(ctx, 20*time.Second, "journalctl", "-u", u.unit, "-n", n, "--no-pager", "-o", "short-iso")
	}
	if err != nil && text == "" {
		out.Error = err.Error()
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		if len(l) > 2000 {
			l = l[:2000] + "…"
		}
		out.Lines = append(out.Lines, l)
	}
	writeJSON(w, 200, out)
}

// ---- simulation ---------------------------------------------------------------------------------------------------

func simUpgrades() []PackageUpgrade {
	if _, err := os.Stat(filepath.Join(osDir(), "upgraded.stamp")); err == nil {
		return []PackageUpgrade{}
	}
	return []PackageUpgrade{
		{Name: "libssl3t64", From: "3.0.13-0ubuntu3.4", To: "3.0.13-0ubuntu3.5", Origin: "Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security", Security: true},
		{Name: "tzdata", From: "2026a-0ubuntu0.24.04", To: "2026b-0ubuntu0.24.04", Origin: "Ubuntu:24.04/noble-updates"},
	}
}

// simulateUpgrade stands in for the systemd unit in the simulation: a short log, then the result file.
func simulateUpgrade(id string) {
	logPath := filepath.Join(osDir(), id+".log")
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o640)
	for _, l := range []string{"$ apt-get update", "Hit:1 http://archive.example (simulation) InRelease", "$ apt-get -y full-upgrade",
		"Unpacking libssl3t64 (3.0.13-0ubuntu3.5) …", "Setting up tzdata (2026b-0ubuntu0.24.04) …", "2 upgraded, 0 newly installed, 0 to remove and 0 not upgraded."} {
		if f != nil {
			fmt.Fprintln(f, l)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if f != nil {
		_ = f.Close()
	}
	_ = os.WriteFile(filepath.Join(osDir(), "upgraded.stamp"), nil, 0o640)
	_ = os.WriteFile(filepath.Join(osDir(), "reboot-required"), []byte("*** System restart required ***\n"), 0o640)
	_ = os.WriteFile(filepath.Join(osDir(), "reboot-required.pkgs"), []byte("libssl3t64\n"), 0o640)
	_ = saveJSON(filepath.Join(osDir(), id+".result.json"), osJobResult{Upgraded: 2, RebootRequired: true, FinishedAt: time.Now().UTC()}, 0o640)
}
