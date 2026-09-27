package replication

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Host performs the operations on the replica machine's PostgreSQL that need the operating system: stop, start,
// replace the data directory with a base copy, run SQL as the local superuser (research R10). It runs as root
// (the attach/promote CLI, or the agent).
type Host struct {
	Mode      string `json:"mode"`                 // docker | native | manual
	Container string `json:"container,omitempty"`  // docker
	Image     string `json:"image,omitempty"`      // docker: image of the container (same PostgreSQL build)
	Volume    string `json:"volume,omitempty"`     // docker: volume mounted at /var/lib/postgresql
	DataDir   string `json:"data_dir,omitempty"`   // native/manual; docker: $PGDATA inside the container
	BinDir    string `json:"bin_dir,omitempty"`    // native/manual: directory of pg_basebackup, psql, pg_ctl
	Service   string `json:"service,omitempty"`    // native: service name
	SuperUser string `json:"super_user,omitempty"` // role for local superuser SQL
	DB        string `json:"db,omitempty"`
	Port      int    `json:"port,omitempty"` // manual: port of the local cluster
	SockDir   string `json:"sock_dir,omitempty"`
	// TunnelHost is the address the local PostgreSQL uses to reach the tunnel (docker: the bridge gateway).
	TunnelHost string `json:"tunnel_host"`
}

// DetectHost inspects the machine: the Docker container of install.sh, a native service, or (with dataDir and
// binDir) a cluster managed by hand.
func DetectHost(ctx context.Context, container, db, superUser, dataDir, binDir string, port int) (*Host, error) {
	if dataDir != "" {
		if binDir == "" {
			return nil, errors.New("-pgbin is required with -pgdata")
		}
		return &Host{Mode: "manual", DataDir: dataDir, BinDir: binDir, SuperUser: superUser, DB: db, Port: port, TunnelHost: "127.0.0.1", SockDir: "/tmp"}, nil
	}
	if container != "" {
		if out, err := runOut(ctx, 10*time.Second, "docker", "inspect", "-f", "{{.State.Running}}|{{.Config.Image}}|{{range .Mounts}}{{if eq .Destination \"/var/lib/postgresql\"}}{{.Name}}{{end}}{{end}}|{{range .NetworkSettings.Networks}}{{.Gateway}} {{end}}", container); err == nil {
			parts := strings.Split(strings.TrimSpace(out), "|")
			if len(parts) == 4 && parts[0] == "true" {
				gw := strings.Fields(parts[3])
				if parts[2] == "" || len(gw) == 0 {
					return nil, fmt.Errorf("docker container %s: volume or network not found", container)
				}
				pgdata, _ := runOut(ctx, 10*time.Second, "docker", "exec", container, "sh", "-c", "echo $PGDATA")
				return &Host{Mode: "docker", Container: container, Image: parts[1], Volume: parts[2], DataDir: strings.TrimSpace(pgdata),
					SuperUser: superUser, DB: db, TunnelHost: gw[0]}, nil
			}
		}
	}
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("runuser"); err == nil {
			h := &Host{Mode: "native", Service: "postgresql", SuperUser: "postgres", DB: db, TunnelHost: "127.0.0.1"}
			dir, err := h.SQL(ctx, "SHOW data_directory")
			if err == nil && strings.TrimSpace(dir) != "" {
				h.DataDir = strings.TrimSpace(dir)
				ver, _ := h.SQL(ctx, "SELECT current_setting('server_version_num')::int / 10000")
				h.BinDir = "/usr/lib/postgresql/" + strings.TrimSpace(ver) + "/bin"
				return h, nil
			}
		}
	}
	return nil, errors.New("no local PostgreSQL found (docker container, native service), or give -pgdata and -pgbin")
}

// SQL runs one statement as the local superuser without a password (socket trust in the container, peer
// authentication natively, trust in a manual test cluster) and returns the first column(s) as text.
// The maintenance database "postgres" is used, which every cluster has; see SQLIn for the application database.
func (h *Host) SQL(ctx context.Context, sql string) (string, error) {
	return h.SQLIn(ctx, "postgres", sql)
}

// SQLIn runs the statement in the given database.
func (h *Host) SQLIn(ctx context.Context, db, sql string) (string, error) {
	args := []string{"-X", "-At", "-v", "ON_ERROR_STOP=1", "-c", sql}
	switch h.Mode {
	case "docker":
		return runOut(ctx, 2*time.Minute, "docker", append([]string{"exec", h.Container, "psql", "-U", h.SuperUser, "-d", db}, args...)...)
	case "native":
		return runOut(ctx, 2*time.Minute, "runuser", append([]string{"-u", "postgres", "--", "psql", "-d", db}, args...)...)
	case "manual":
		return runOut(ctx, 2*time.Minute, filepath.Join(h.BinDir, "psql"), append([]string{"-h", h.SockDir, "-p", strconv.Itoa(h.Port), "-U", h.SuperUser, "-d", db}, args...)...)
	}
	return "", fmt.Errorf("unknown host mode %q", h.Mode)
}

// PGMajor of the local server.
func (h *Host) PGMajor(ctx context.Context) (int, error) {
	out, err := h.SQL(ctx, "SELECT current_setting('server_version_num')::int / 10000")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(out))
}

// Stop stops the local PostgreSQL.
func (h *Host) Stop(ctx context.Context) error {
	switch h.Mode {
	case "docker":
		_, err := runOut(ctx, 2*time.Minute, "docker", "stop", h.Container)
		return err
	case "native":
		_, err := runOut(ctx, 2*time.Minute, "systemctl", "stop", h.Service)
		return err
	case "manual":
		_, err := runOut(ctx, 2*time.Minute, filepath.Join(h.BinDir, "pg_ctl"), "-D", h.DataDir, "-m", "fast", "-w", "stop")
		return err
	}
	return fmt.Errorf("unknown host mode %q", h.Mode)
}

// Start starts the local PostgreSQL and waits until it answers.
func (h *Host) Start(ctx context.Context) error {
	var err error
	switch h.Mode {
	case "docker":
		_, err = runOut(ctx, 2*time.Minute, "docker", "start", h.Container)
	case "native":
		_, err = runOut(ctx, 2*time.Minute, "systemctl", "start", h.Service)
	case "manual":
		_, err = runOut(ctx, 2*time.Minute, filepath.Join(h.BinDir, "pg_ctl"), "-D", h.DataDir, "-l", filepath.Join(h.DataDir, "server.log"), "-w",
			"-o", fmt.Sprintf("-p %d -k %s", h.Port, h.SockDir), "start")
	default:
		return fmt.Errorf("unknown host mode %q", h.Mode)
	}
	if err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		if _, err = h.SQL(ctx, "SELECT 1"); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("PostgreSQL did not come up: %w", err)
}

// BaseBackup moves the current data directory aside (it is never deleted) and fills a new one with a base copy of
// the primary through the tunnel at tunnelPort, configured as a standby streaming from the slot.
func (h *Host) BaseBackup(ctx context.Context, tunnelPort int, role, password, slot string, progress io.Writer) (aside string, err error) {
	stamp := time.Now().Format("20060102-150405")
	bb := fmt.Sprintf(`pg_basebackup -h %s -p %d -U %s -D "$D" -X stream -S %s -R -P --checkpoint=fast`, h.TunnelHost, tunnelPort, role, slot)
	script := `set -e; D="` + h.DataDir + `"; A="$D.before-replica-` + stamp + `"; if [ -d "$D" ]; then mv "$D" "$A"; fi; mkdir -p "$D"; chmod 700 "$D"; `
	// A primary whose configuration lives outside its data directory (Debian's /etc/postgresql) gives a copy without
	// postgresql.conf and pg_hba.conf; the local cluster's own files (from the directory moved aside) are used then.
	keepConf := `; for f in postgresql.conf pg_hba.conf pg_ident.conf; do if [ ! -f "$D/$f" ] && [ -f "$A/$f" ]; then cp -p "$A/$f" "$D/$f"; fi; done`
	switch h.Mode {
	case "docker":
		script += `chown postgres:postgres "$D"; gosu postgres ` + bb + keepConf
		err = runStream(ctx, progress, []string{"PGPASSWORD=" + password}, "docker", "run", "--rm", "--network", "host", "-e", "PGPASSWORD",
			"-v", h.Volume+":/var/lib/postgresql", "--entrypoint", "bash", h.Image, "-c", script)
	case "native":
		script += `chown postgres:postgres "$D"; runuser -u postgres -- env PGPASSWORD="$PGPASSWORD" ` + filepath.Join(h.BinDir, "") + "/" + bb + keepConf
		err = runStream(ctx, progress, []string{"PGPASSWORD=" + password}, "sh", "-c", script)
	case "manual":
		script += filepath.Join(h.BinDir, "") + "/" + bb + keepConf
		err = runStream(ctx, progress, []string{"PGPASSWORD=" + password}, "sh", "-c", script)
	default:
		return "", fmt.Errorf("unknown host mode %q", h.Mode)
	}
	return h.DataDir + ".before-replica-" + stamp, err
}

// Promote turns the local standby into an independent primary, gives the local server's own roles their local
// passwords back (the copy carries the primary's), and records the new identity. Returns the new node id.
func (h *Host) Promote(ctx context.Context, appURL, adminURL string) (string, error) {
	inRecovery, err := h.SQL(ctx, "SELECT pg_is_in_recovery()")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(inRecovery) == "t" {
		if _, err := h.SQL(ctx, "SELECT pg_promote(true, 60)"); err != nil {
			return "", fmt.Errorf("promote: %w", err)
		}
	}
	for _, dsn := range []string{appURL, adminURL} {
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil || cfg.User == "" || cfg.Password == "" {
			continue
		}
		stmt := fmt.Sprintf(`DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %[1]s) THEN EXECUTE format('ALTER ROLE %%I PASSWORD %%L', %[1]s, %[2]s); END IF; END $$`,
			quoteLiteral(cfg.User), quoteLiteral(cfg.Password))
		if _, err := h.SQL(ctx, stmt); err != nil {
			return "", fmt.Errorf("restore the local password of %s: %w", cfg.User, err)
		}
	}
	// The replication roles of the copy can no longer log in here.
	_, _ = h.SQL(ctx, `DO $$ DECLARE r text; BEGIN FOR r IN SELECT rolname FROM pg_roles WHERE rolname LIKE 'ctrl32\_repl\_%' LOOP EXECUTE format('ALTER ROLE %I NOLOGIN', r); END LOOP; END $$`)
	id, err := h.SQLIn(ctx, h.DB, "SELECT core.promote_node()")
	if err != nil {
		return "", fmt.Errorf("record the new node identity: %w", err)
	}
	return strings.TrimSpace(id), nil
}

func runOut(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("%s: %w: %s", name, err, firstLines(msg, 3))
	}
	return out.String(), nil
}

func runStream(ctx context.Context, w io.Writer, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	if w == nil {
		w = io.Discard
	}
	var tail bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = io.MultiWriter(w, &tail)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, firstLines(lastLines(tail.String(), 3), 3))
	}
	return nil
}

func firstLines(s string, n int) string {
	l := strings.Split(s, "\n")
	if len(l) > n {
		l = l[:n]
	}
	return strings.Join(l, " / ")
}

func lastLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}
