package vault

import (
	"errors"

	"golang.org/x/sys/unix"
)

func allocLocked(n int) ([]byte, func()) {
	if n <= 0 {
		return nil, nil
	}
	b, err := unix.Mmap(-1, 0, n, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		return make([]byte, n), nil
	}
	_ = unix.Madvise(b, unix.MADV_DONTDUMP)
	locked := unix.Mlock(b) == nil
	return b, func() {
		if locked {
			_ = unix.Munlock(b)
		}
		_ = unix.Munmap(b)
	}
}

var errNoDPAPI = errors.New("Windows account unlock is only available on Windows")

func dpapiProtect(data, entropy []byte) ([]byte, error)   { return nil, errNoDPAPI }
func dpapiUnprotect(data, entropy []byte) ([]byte, error) { return nil, errNoDPAPI }

func WindowsUnlockAvailable() bool { return false }
