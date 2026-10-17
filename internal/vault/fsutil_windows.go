//go:build windows

package vault

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func RenameDurable(oldPath, newPath string) error {
	from, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	var last error
	for attempt := 0; attempt < 40; attempt++ {
		last = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if last == nil {
			return nil
		}
		if !transientLock(last) {
			break
		}
		time.Sleep(time.Duration(50+10*attempt) * time.Millisecond)
	}
	return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: last}
}

func transientLock(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

func freeSpace(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}
