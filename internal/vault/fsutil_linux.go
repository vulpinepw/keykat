package vault

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func RenameDurable(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	if d, err := os.Open(filepath.Dir(newPath)); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

func freeSpace(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
