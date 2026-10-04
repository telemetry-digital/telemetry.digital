package agent

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
)

// Offline update packages (ADR-0024): one .zip per release, built by deploy/publish.sh (tools/relsign package) with
// the signed manifest, the binaries of every platform, SHA256SUMS, VERSION and the parts of the object counting
// detector. An uploaded package is verified completely before anything is kept: the signature of the manifest, every
// member listed in it with its size and SHA-256, no other member, no path outside the package. The verified content
// is staged in <updates>/release (one release at a time); the installation then reads it like the distribution point,
// and the detector installation takes its parts from there (analytics.go).

const maxPackageMembers = 1024

// stagePackage stores an uploaded package (at most limit bytes), verifies it and stages its content.
func (a *Server) stagePackage(body io.Reader, limit int64) (*PackageInfo, *updateErr) {
	dir := updatesDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, uerr(500, "", "%v", err)
	}
	up := filepath.Join(dir, "upload-"+randomID()[:10]+".zip")
	f, err := os.OpenFile(up, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, uerr(500, "", "%v", err)
	}
	defer os.Remove(up)
	n, err := io.Copy(f, io.LimitReader(body, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, uerr(400, "", "the upload was interrupted: %v", err)
	}
	if n > limit {
		return nil, uerr(413, "too_large", "the package is larger than %d MB (agent.max_package_mb)", limit>>20)
	}
	return a.stageFile(up)
}

// stageFile verifies a package file and stages its content.
func (a *Server) stageFile(path string) (*PackageInfo, *updateErr) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, uerr(400, "package", "not an update package (.zip): %v", err)
	}
	defer zr.Close()
	if len(zr.File) > maxPackageMembers {
		return nil, uerr(400, "package", "the package has too many members")
	}
	members := map[string]*zip.File{}
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, "/") {
			if name == "analytics/" {
				continue
			}
			return nil, uerr(400, "package", "unexpected directory %q in the package", name)
		}
		if name != release.ManifestFile && name != release.SignatureFile && !release.ValidFileName(name) {
			return nil, uerr(400, "package", "unexpected member %q in the package", name)
		}
		if members[name] != nil {
			return nil, uerr(400, "package", "%s appears twice in the package", name)
		}
		members[name] = f
	}
	read := func(name string, max int64) ([]byte, error) {
		f := members[name]
		if f == nil {
			return nil, errNotFound
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, max+1))
		if err != nil {
			return nil, err
		}
		if int64(len(b)) > max {
			return nil, fmt.Errorf("%s is too large", name)
		}
		return b, nil
	}
	mb, err := read(release.ManifestFile, release.MaxManifestBytes)
	if err != nil {
		return nil, uerr(400, "package", "the package has no release manifest (%s): %v", release.ManifestFile, err)
	}
	sb, err := read(release.SignatureFile, 64<<10)
	if err != nil && !errors.Is(err, errNotFound) {
		return nil, uerr(400, "package", "%v", err)
	}
	m, keyID, verr := a.verifyManifest(mb, sb)
	if verr != nil {
		a.log.Warn("offline package refused", "err", verr)
		return nil, uerr(400, "signature", "%v", verr)
	}
	for name, f := range members {
		if name == release.ManifestFile || name == release.SignatureFile {
			continue
		}
		mf, ok := m.File(name)
		if !ok {
			return nil, uerr(400, "package", "%s is not listed in the signed manifest — package refused", name)
		}
		if f.UncompressedSize64 != uint64(mf.Size) {
			return nil, uerr(400, "checksum", "%s does not have the size the signed manifest lists — package refused", name)
		}
	}
	stage, err := os.MkdirTemp(updatesDir(), ".stage-")
	if err != nil {
		return nil, uerr(500, "", "%v", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.WriteFile(filepath.Join(stage, release.ManifestFile), mb, 0o644); err != nil {
		return nil, uerr(500, "", "%v", err)
	}
	_ = os.WriteFile(filepath.Join(stage, release.SignatureFile), sb, 0o644)
	info := &PackageInfo{Version: m.Version, Released: m.Released, KeyID: keyID, Platform: runtime.GOOS + "/" + runtime.GOARCH, StagedAt: time.Now().UTC()}
	bin := release.BinaryName(runtime.GOOS, runtime.GOARCH)
	names := make([]string, 0, len(members))
	for n := range members {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, name := range names {
		if name == release.ManifestFile || name == release.SignatureFile {
			continue
		}
		mf, _ := m.File(name)
		if err := extractVerified(members[name], filepath.Join(stage, filepath.FromSlash(name)), mf); err != nil {
			return nil, uerr(400, "checksum", "%v — package refused", err)
		}
		info.Files++
		info.Bytes += mf.Size
		if name == bin {
			info.HasBinary = true
		}
		if strings.HasPrefix(name, "analytics/") {
			info.Detector = append(info.Detector, strings.TrimPrefix(name, "analytics/"))
		}
	}
	// one staged release at a time: the previous one goes
	_ = os.RemoveAll(stagedDir())
	if err := os.Rename(stage, stagedDir()); err != nil {
		return nil, uerr(500, "", "%v", err)
	}
	keep = true
	if err := saveJSON(filepath.Join(updatesDir(), "staged.json"), info, 0o640); err != nil {
		return nil, uerr(500, "", "%v", err)
	}
	a.log.Info("offline package verified and staged", "version", info.Version, "released", info.Released, "key", keyID, "files", info.Files, "binary", info.HasBinary, "detector", len(info.Detector))
	return info, nil
}

// extractVerified writes one member and checks its size and SHA-256 against the manifest.
func extractVerified(f *zip.File, dst string, mf release.File) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := os.FileMode(0o644)
	if strings.HasPrefix(filepath.Base(dst), "ctrl32-telemetry-") {
		mode = 0o755
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(rc, mf.Size+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", mf.Name, err)
	}
	if n != mf.Size || hex.EncodeToString(h.Sum(nil)) != mf.SHA256 {
		return fmt.Errorf("%s does not match the signed manifest", mf.Name)
	}
	return nil
}

// VerifyPackageFile checks a package without staging it (`ctrl32-telemetry update verify`, and the command line before
// it hands a package to the agent): the signature against the keys of this binary and every member against the
// manifest. Unsigned packages are refused.
func VerifyPackageFile(path string) (*PackageInfo, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("not an update package (.zip): %w", err)
	}
	defer zr.Close()
	members := map[string]*zip.File{}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		if members[f.Name] != nil {
			return nil, fmt.Errorf("%s appears twice in the package", f.Name)
		}
		members[f.Name] = f
	}
	read := func(name string, max int64) ([]byte, error) {
		f := members[name]
		if f == nil {
			return nil, fmt.Errorf("the package has no %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, max))
	}
	mb, err := read(release.ManifestFile, release.MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	sb, err := read(release.SignatureFile, 64<<10)
	if err != nil {
		return nil, err
	}
	m, keyID, err := release.Verify(mb, sb)
	if err != nil {
		return nil, err
	}
	info := &PackageInfo{Version: m.Version, Released: m.Released, KeyID: keyID, Platform: runtime.GOOS + "/" + runtime.GOARCH}
	bin := release.BinaryName(runtime.GOOS, runtime.GOARCH)
	for name, f := range members {
		if name == release.ManifestFile || name == release.SignatureFile {
			continue
		}
		mf, ok := m.File(name)
		if !ok || !release.ValidFileName(name) {
			return nil, fmt.Errorf("%s is not listed in the signed manifest", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(rc, mf.Size+1))
		rc.Close()
		if err != nil || n != mf.Size || hex.EncodeToString(h.Sum(nil)) != mf.SHA256 {
			return nil, fmt.Errorf("%s does not match the signed manifest", name)
		}
		info.Files++
		info.Bytes += mf.Size
		if name == bin {
			info.HasBinary = true
		}
		if strings.HasPrefix(name, "analytics/") {
			info.Detector = append(info.Detector, strings.TrimPrefix(name, "analytics/"))
		}
	}
	slices.Sort(info.Detector)
	return info, nil
}

// hUpdatePackage receives a package as the request body; with install=1 it installs it like an online release.
func (a *Server) hUpdatePackage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := a.packageLimit()
	if r.ContentLength > limit {
		writeUpdateErr(w, uerr(413, "too_large", "the package is larger than %d MB (agent.max_package_mb)", limit>>20))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := busy(time.Now()); c != nil {
		writeUpdateErr(w, uerr(409, "busy", "an update to %s is in progress (%s)", c.To, c.Phase))
		return
	}
	info, e := a.stagePackage(r.Body, limit)
	if e != nil {
		writeUpdateErr(w, e)
		return
	}
	res := PackageResult{Package: info}
	if q.Get("install") == "1" && info.HasBinary {
		minutes, _ := strconv.Atoi(q.Get("rollback_minutes"))
		in := UpdateRequest{Version: info.Version, Source: "package", Mode: q.Get("mode"), Backup: q.Get("backup") == "1", RollbackMinutes: minutes,
			UpdatesUntil: q.Get("updates_until"), Actor: q.Get("actor"), Reason: q.Get("reason")}
		up, e := a.install(r.Context(), in, dirSource{dir: stagedDir()})
		if e != nil {
			writeUpdateErr(w, e)
			return
		}
		res.Update = up
	}
	writeJSON(w, 200, res)
}

// InstallPackageFile is `ctrl32-telemetry update --file` when the agent service does not run: the same verification
// and installation in this process (as root / administrator).
func (a *Server) InstallPackageFile(ctx context.Context, path string, in UpdateRequest) (*PackageResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := busy(time.Now()); c != nil {
		return nil, fmt.Errorf("an update to %s is in progress (%s)", c.To, c.Phase)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, e := a.stagePackage(f, a.packageLimit())
	if e != nil {
		return nil, e
	}
	res := &PackageResult{Package: info}
	if !info.HasBinary {
		return res, nil
	}
	in.Version, in.Source = info.Version, "package"
	up, e := a.install(ctx, in, dirSource{dir: stagedDir()})
	if e != nil {
		return res, e
	}
	res.Update = up
	return res, nil
}
