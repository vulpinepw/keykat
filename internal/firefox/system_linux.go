package firefox

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const Executable = "firefox"

func systemArgs(headless bool) []string {
	if headless {
		return nil
	}
	return []string{"--class=Keycat", "--name=keycat"}
}

func ProfileInUse(dir string) bool {
	f, err := os.Open(filepath.Join(dir, ".parentlock"))
	if err != nil {
		return false
	}
	defer f.Close()
	lk := unix.Flock_t{Type: unix.F_WRLCK}
	if err := unix.FcntlFlock(f.Fd(), unix.F_GETLK, &lk); err != nil {
		return false
	}
	return lk.Type != unix.F_UNLCK
}

func FindSystemFirefox() string { return "" }
