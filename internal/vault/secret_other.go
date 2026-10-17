//go:build !windows

package vault

import "errors"

func allocLocked(n int) ([]byte, func()) { return make([]byte, n), nil }

var errNoDPAPI = errors.New("Windows account unlock is only available on Windows")

func dpapiProtect(data, entropy []byte) ([]byte, error)   { return nil, errNoDPAPI }
func dpapiUnprotect(data, entropy []byte) ([]byte, error) { return nil, errNoDPAPI }

func WindowsUnlockAvailable() bool { return false }
