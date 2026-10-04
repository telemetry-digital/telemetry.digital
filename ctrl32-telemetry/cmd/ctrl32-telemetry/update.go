package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/license"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/logging"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
)

// update — offline updates and mirrors on the server's command line (ADR-0024):
//
//	ctrl32-telemetry update --file ctrl32-telemetry-0.76.0-offline.zip [--yes] [--no-backup] [--reason "…"]
//	        verifies the package (signature, every file) and installs it through the agent service — or, when the
//	        agent does not run, in this process (root / administrator); same checks, backup, migrations and rollback
//	ctrl32-telemetry update verify --file package.zip      checks a package and prints what it contains
//	ctrl32-telemetry update mirror --to /srv/ctrl32-mirror [--from URL] [--no-detector]
//	        downloads the latest release with verification into a directory for a web server in the customer's
//	        network; the servers then use it with [agent] update_url = "http://mirror.example.local/ctrl32"
//	ctrl32-telemetry update guard …                        internal: started by the agent after an installation
func cmdUpdate(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "verify":
			return cmdUpdateVerify(args[1:])
		case "mirror":
			return cmdUpdateMirror(args[1:])
		case "guard":
			return cmdUpdateGuard(args[1:])
		}
	}
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	path := fs.String("config", config.DefaultPath(), "path to config.toml")
	file := fs.String("file", "", "offline update package (.zip)")
	yes := fs.Bool("yes", false, "install without asking")
	noBackup := fs.Bool("no-backup", false, "skip the backup before the update")
	reason := fs.String("reason", "", "reason recorded with the update")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New(`usage: ctrl32-telemetry update --file PACKAGE.zip [--yes] [--no-backup] [--reason "why"]
       ctrl32-telemetry update verify --file PACKAGE.zip
       ctrl32-telemetry update mirror --to DIR [--from URL] [--no-detector]`)
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	info, err := agent.VerifyPackageFile(*file)
	if err != nil {
		return fmt.Errorf("package refused: %w", err)
	}
	printPackage(info)
	if !info.HasBinary {
		fmt.Println("The package has no server binary for " + info.Platform + "; it can only provide the detector parts (upload it in System → Updates).")
		return nil
	}
	if c := release.Compare(version, info.Version); c >= 0 {
		fmt.Printf("Note: the installed version is %s; the package does not make it newer.\n", version)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	until, err := updatesUntilFromDB(ctx, cfg)
	if err != nil {
		return err
	}
	if until != "" {
		fmt.Println("Licence: updates included until", until)
	}
	if !*yes {
		fmt.Printf("Install %s (installed: %s)? A backup is taken first%s. [y/N] ", info.Version, version, map[bool]string{true: " (skipped: --no-backup)", false: ""}[*noBackup])
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			return errors.New("not installed")
		}
	}
	actor := "cli"
	if u, err := user.Current(); err == nil {
		actor = "cli (" + u.Username + ")"
	}
	why := strings.TrimSpace(*reason)
	if why == "" {
		why = "offline update from the command line (" + filepath.Base(*file) + ")"
	}
	sock := agentSocket(cfg)
	cl := agent.NewClient(sock)
	if _, err := cl.UpdateState(ctx); err == nil {
		fmt.Println("Handing the package to the agent service …")
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer f.Close()
		st, _ := f.Stat()
		q := url.Values{"install": {"1"}, "mode": {"cli"}, "actor": {actor}, "reason": {why}}
		if !*noBackup {
			q.Set("backup", "1")
		}
		if until != "" {
			q.Set("updates_until", until)
		}
		res, err := cl.UploadPackage(ctx, f, st.Size(), q)
		if err != nil {
			return err
		}
		printInstalled(res)
		return nil
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		return errors.New("the agent service does not answer; run as root to install directly")
	}
	fmt.Println("The agent service does not answer: installing directly …")
	logger, closeLog, err := logging.Setup(cfg.Log.Level, "", "")
	if err != nil {
		return err
	}
	defer closeLog()
	srv := agent.NewServer(sock, *path, version, cfg.Agent.UpdateURL, logger.With("component", "update"))
	srv.SetDBTools(cfg.Agent.PgDump, cfg.Agent.PgContainer)
	srv.SetUpdatePolicy(cfg.Agent.MaxPackageMB, cfg.Agent.AllowUnsignedUpdates)
	srv.SetDirect()
	res, err := srv.InstallPackageFile(ctx, *file, agent.UpdateRequest{Mode: "cli", Backup: !*noBackup, UpdatesUntil: until, Actor: actor, Reason: why})
	if err != nil {
		return err
	}
	printInstalled(res)
	return nil
}

func agentSocket(cfg *config.Config) string {
	if cfg.Agent.Socket != "" {
		return cfg.Agent.Socket
	}
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "ctrl32-telemetry", "agent.sock")
	}
	return "/run/ctrl32-telemetry/agent.sock"
}

// updatesUntilFromDB reads updates_until of the installed licence ("" = no licence or no limit).
func updatesUntilFromDB(ctx context.Context, cfg *config.Config) (string, error) {
	u := cfg.DB.AdminURL
	if u == "" {
		u = cfg.DB.URL
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, u)
	if err != nil {
		return "", fmt.Errorf("the licence cannot be read: the database does not answer (%v); start PostgreSQL first", err)
	}
	defer conn.Close(ctx)
	var token string
	err = conn.QueryRow(ctx, `SELECT token FROM core.product_license WHERE id = 1`).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("the licence cannot be read: %w", err)
	}
	l, err := license.Parse(token, license.TrustedKeys())
	if err != nil || l == nil {
		return "", nil // not a valid licence: the server runs without one, which has no update limit
	}
	return l.UpdatesUntil, nil
}

func printPackage(p *agent.PackageInfo) {
	fmt.Printf("Package: ctrl32 telemetry %s, released %s, signed by %s\n", p.Version, p.Released, p.KeyID)
	fmt.Printf("  %d files verified (%.1f MB); server binary for %s: %v; detector parts: %d\n", p.Files, float64(p.Bytes)/1e6, p.Platform, p.HasBinary, len(p.Detector))
}

func printInstalled(res *agent.PackageResult) {
	if res.Update == nil {
		fmt.Println("Verified and staged; nothing installed.")
		return
	}
	fmt.Printf("Installed %s (SHA-256 %s…)", res.Update.Version, res.Update.SHA256[:16])
	if res.Update.Backup != "" {
		fmt.Printf(", backup %s", res.Update.Backup)
	}
	fmt.Println()
	fmt.Println(res.Update.Message)
}

func cmdUpdateVerify(args []string) error {
	fs := flag.NewFlagSet("update verify", flag.ContinueOnError)
	file := fs.String("file", "", "offline update package (.zip)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("usage: ctrl32-telemetry update verify --file PACKAGE.zip")
	}
	info, err := agent.VerifyPackageFile(*file)
	if err != nil {
		return fmt.Errorf("package refused: %w", err)
	}
	printPackage(info)
	fmt.Println("ok")
	return nil
}

// cmdUpdateMirror copies the latest release of a distribution point into a directory, verifying the signature and
// every file; files already there with the right SHA-256 are kept.
func cmdUpdateMirror(args []string) error {
	fs := flag.NewFlagSet("update mirror", flag.ContinueOnError)
	to := fs.String("to", "", "directory served by a web server in your network")
	from := fs.String("from", "https://portal.telemetry.digital/dl", "distribution point to copy from")
	noDetector := fs.Bool("no-detector", false, "leave out the parts of the object counting detector (analytics/)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *to == "" {
		return errors.New("usage: ctrl32-telemetry update mirror --to DIR [--from URL] [--no-detector]")
	}
	base := strings.TrimRight(*from, "/")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := &http.Client{Timeout: 30 * time.Minute}
	get := func(name string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/"+name, nil)
		req.Header.Set("User-Agent", "ctrl32-telemetry/"+version)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("%s/%s: HTTP %d", base, name, resp.StatusCode)
		}
		return resp.Body, nil
	}
	small := func(name string, max int64) ([]byte, error) {
		rc, err := get(name)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, max))
	}
	mb, err := small(release.ManifestFile, release.MaxManifestBytes)
	if err != nil {
		return err
	}
	sb, err := small(release.SignatureFile, 64<<10)
	if err != nil {
		return err
	}
	m, keyID, err := release.Verify(mb, sb)
	if err != nil {
		return fmt.Errorf("release refused: %w", err)
	}
	fmt.Printf("Release %s, released %s, signed by %s: %d files\n", m.Version, m.Released, keyID, len(m.Files))
	if err := os.MkdirAll(filepath.Join(*to, "analytics"), 0o755); err != nil {
		return err
	}
	for _, f := range m.Files {
		if *noDetector && strings.HasPrefix(f.Name, "analytics/") {
			continue
		}
		dst := filepath.Join(*to, filepath.FromSlash(f.Name))
		if fileSHA256(dst) == f.SHA256 {
			fmt.Println("  kept       ", f.Name)
			continue
		}
		rc, err := get(f.Name)
		if err != nil {
			return err
		}
		tmp := dst + ".part"
		out, err := os.Create(tmp)
		if err != nil {
			rc.Close()
			return err
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(rc, f.Size+1))
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil || n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
			_ = os.Remove(tmp)
			return fmt.Errorf("%s does not match the signed manifest — mirror not updated", f.Name)
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
		fmt.Println("  downloaded ", f.Name)
	}
	// the manifest and its signature last: a mirror never offers a release whose files are not all there
	if err := os.WriteFile(filepath.Join(*to, release.ManifestFile), mb, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*to, release.SignatureFile), sb, 0o644); err != nil {
		return err
	}
	fmt.Printf("Mirror of %s ready in %s.\nServe the directory with any web server and set on each server: [agent] update_url = \"http://<mirror host>/<path>\"\n", m.Version, *to)
	return nil
}

func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cmdUpdateGuard(args []string) error {
	fs := flag.NewFlagSet("update guard", flag.ContinueOnError)
	path := fs.String("config", config.DefaultPath(), "path to config.toml")
	state := fs.String("state", "", "update.json of the agent")
	id := fs.String("id", "", "installation to watch")
	bin := fs.String("bin", "", "the installed binary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *state == "" || *id == "" {
		return errors.New("update guard is started by the agent")
	}
	cfg, err := config.Load(*path)
	level := "info"
	if err == nil {
		level = cfg.Log.Level
	}
	logger, closeLog, err := logging.Setup(level, "", "")
	if err != nil {
		return err
	}
	defer closeLog()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return agent.Guard(ctx, *path, *state, *id, *bin, logger.With("component", "update-guard"))
}
