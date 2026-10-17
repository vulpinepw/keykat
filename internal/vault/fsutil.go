package vault

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type NoSpaceError struct {
	Path       string
	Need, Free uint64
}

func (e *NoSpaceError) Error() string {
	vol := filepath.VolumeName(e.Path)
	if vol == "" {
		vol = e.Path
	}
	return fmt.Sprintf("not enough free disk space on %s (%s needed, %s free)", vol, formatBytes(e.Need), formatBytes(e.Free))
}

func EnsureFreeSpace(path string, need uint64) error {
	free, err := freeSpace(existingParent(path))
	if err != nil || free >= need {
		return nil
	}
	return &NoSpaceError{Path: path, Need: need, Free: free}
}

func LinkOrCopy(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return CopyFile(src, dst)
}

func CopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(dst)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func WriteFileSynced(path string, data []byte) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

func existingParent(path string) string {
	for p := path; ; {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return path
		}
		p = parent
	}
}

func formatBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", n>>10)
}
