//go:build linux

package app

import (
	"os"
	"path/filepath"
)

func defaultDirs() (data, local string, err error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	state := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(config, "keycat"), filepath.Join(state, "keycat"), nil
}
