package vault

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func WipeDir(dir string, skipOverwrite func(rel string) bool) error {
	if dir == "" {
		return nil
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil
	}
	zeros := make([]byte, 1<<20)
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		rel := filepath.ToSlash(mustRel(dir, p))
		if skipOverwrite != nil && skipOverwrite(rel) {
			return nil
		}
		overwriteFile(p, zeros)
		return nil
	})
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
	}
	return err
}

func overwriteFile(p string, zeros []byte) {
	info, err := os.Stat(p)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o200 == 0 {
		_ = os.Chmod(p, 0o600)
	}
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()
	remaining := info.Size()
	for remaining > 0 {
		n := int64(len(zeros))
		if remaining < n {
			n = remaining
		}
		if _, err := f.Write(zeros[:n]); err != nil {
			return
		}
		remaining -= n
	}
	_ = f.Sync()
}
