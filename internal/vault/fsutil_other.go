//go:build !windows

package vault

import (
	"errors"
	"os"
	"path/filepath"
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

func freeSpace(string) (uint64, error) {
	return 0, errors.New("free space is not available on this system")
}
