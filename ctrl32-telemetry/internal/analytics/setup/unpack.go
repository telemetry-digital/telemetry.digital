package setup

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
)

// maxUnpacked caps what one archive may unpack to (the ffmpeg build is about 120 MB).
const maxUnpacked = 2 << 30

// keepFunc selects the entries of an archive (paths without the archive's top directory), as the installers do.
func keepFunc(name string, windows bool) func(rel string) bool {
	match := func(rel string, patterns ...string) bool {
		for _, p := range patterns {
			if ok, _ := path.Match(p, rel); ok {
				return true
			}
		}
		return false
	}
	switch {
	case name == "onnxruntime" && windows:
		return func(rel string) bool {
			return match(rel, "lib/*.dll") || slices.Contains([]string{"LICENSE", "ThirdPartyNotices.txt", "VERSION_NUMBER", "Privacy.md", "README.md"}, rel)
		}
	case name == "onnxruntime":
		// everything except headers and build files (CMake, pkg-config)
		return func(rel string) bool {
			return !strings.HasPrefix(rel, "include/") && !strings.Contains(rel, "cmake/") && !strings.Contains(rel, "pkgconfig/")
		}
	case name == "ffmpeg" && windows:
		return func(rel string) bool {
			return match(rel, "bin/*.dll", "bin/ffmpeg.exe", "bin/ffprobe.exe", "LICENSE.txt")
		}
	case name == "ffmpeg":
		return func(rel string) bool { return match(rel, "bin/ffmpeg", "bin/ffprobe", "lib/*.so*", "LICENSE.txt") }
	}
	return func(string) bool { return false }
}

// relPath strips the top directory of an archive entry and refuses anything that would leave the target.
func relPath(name string) (string, bool, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	_, rest, ok := strings.Cut(strings.TrimPrefix(name, "./"), "/")
	if !ok || rest == "" {
		return "", false, nil // the top directory itself, or a file next to it
	}
	clean := path.Clean(rest)
	if clean == "." {
		return "", false, nil
	}
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") {
		return "", false, fmt.Errorf("unsafe path in the archive: %s", name)
	}
	return clean, true, nil
}

func unpack(ctx context.Context, o Options, a analytics.Artifact, file, dest string, keep func(string) bool) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	switch a.Format {
	case analytics.FormatZip:
		return unzip(file, dest, keep)
	case analytics.FormatTarGz:
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		return untar(gz, dest, keep)
	case analytics.FormatTarXz:
		r, err := o.Unxz(ctx, file)
		if err != nil {
			return err
		}
		uerr := untar(r, dest, keep)
		cerr := r.Close()
		if uerr != nil {
			return uerr
		}
		return cerr
	}
	return fmt.Errorf("unknown archive format %q", a.Format)
}

func untar(r io.Reader, dest string, keep func(string) bool) error {
	tr := tar.NewReader(r)
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel, ok, err := relPath(h.Name)
		if err != nil {
			return err
		}
		if !ok || !keep(rel) {
			continue
		}
		out := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeReg:
			total += h.Size
			if total > maxUnpacked {
				return errors.New("the archive unpacks to more than expected")
			}
			if err := writeFile(out, tr, h.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// shared libraries link to their versioned file in the same directory; nothing else is accepted
			if strings.ContainsAny(h.Linkname, `/\`) || h.Linkname == ".." || h.Linkname == "." || h.Linkname == "" {
				return fmt.Errorf("unsafe link in the archive: %s -> %s", h.Name, h.Linkname)
			}
			_ = os.Remove(out)
			if err := os.Symlink(h.Linkname, out); err != nil {
				return err
			}
		}
	}
}

func unzip(file, dest string, keep func(string) bool) error {
	z, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer z.Close()
	var total int64
	for _, e := range z.File {
		if e.FileInfo().IsDir() {
			continue
		}
		rel, ok, err := relPath(e.Name)
		if err != nil {
			return err
		}
		if !ok || !keep(rel) {
			continue
		}
		total += int64(e.UncompressedSize64)
		if total > maxUnpacked {
			return errors.New("the archive unpacks to more than expected")
		}
		out := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		rc, err := e.Open()
		if err != nil {
			return err
		}
		err = writeFile(out, rc, e.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// writeFile writes an unpacked file: executables 0755, everything else 0644 (never setuid or writable by others).
func writeFile(out string, r io.Reader, mode os.FileMode) error {
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(r, maxUnpacked)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(out, perm)
}

// execUnxz decompresses with the xz program (Linux; the agent installs xz-utils when it is missing).
func execUnxz(ctx context.Context, file string) (io.ReadCloser, error) {
	if _, err := exec.LookPath("xz"); err != nil {
		return nil, errors.New("the xz program is needed to unpack ffmpeg (package xz-utils)")
	}
	cmd := exec.CommandContext(ctx, "xz", "-dc", file)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &cmdReader{ReadCloser: out, cmd: cmd, stderr: &stderr}, nil
}

type cmdReader struct {
	io.ReadCloser
	cmd    *exec.Cmd
	stderr *strings.Builder
}

func (c *cmdReader) Close() error {
	_, _ = io.Copy(io.Discard, c.ReadCloser)
	if err := c.cmd.Wait(); err != nil {
		return fmt.Errorf("xz: %v %s", err, strings.TrimSpace(c.stderr.String()))
	}
	return nil
}
