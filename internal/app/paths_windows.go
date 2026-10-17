package app

import (
	"os"
	"path/filepath"
)

func defaultDirs() (data, local string, err error) {
	roaming, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	machine, err := os.UserCacheDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(roaming, "Keycat"), filepath.Join(machine, "Keycat"), nil
}
