package offsite

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// folder is a directory: a second disk, a USB disk or a network share the operating system mounted (SMB/NFS), or a
// UNC path on Windows. Files are written under a temporary name and renamed when complete.
type folder struct{ dir string }

func newFolder(dir string) (*folder, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("folder %s is not reachable (is the share mounted?): %w", dir, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	return &folder{dir: dir}, nil
}

func (f *folder) Put(ctx context.Context, name, localPath string) error {
	if err := checkName(name); err != nil {
		return err
	}
	in, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := filepath.Join(f.dir, "."+name+".part")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, ctxReader{ctx, in}); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(f.dir, name))
}

func (f *folder) Get(_ context.Context, name string) (io.ReadCloser, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(f.dir, name))
}

func (f *folder) List(_ context.Context) ([]Object, error) {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return nil, err
	}
	var out []Object
	for _, e := range entries {
		if e.IsDir() || !ValidName(e.Name()) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Object{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()})
	}
	return out, nil
}

func (f *folder) Delete(_ context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(f.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (f *folder) Close() error { return nil }

// ctxReader stops a copy when the context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
