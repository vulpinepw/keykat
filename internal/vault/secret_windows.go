//go:build windows

package vault

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func allocLocked(n int) ([]byte, func()) {
	if n <= 0 {
		return nil, nil
	}
	addr, err := windows.VirtualAlloc(0, uintptr(n), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		b := make([]byte, n)
		return b, nil
	}
	locked := windows.VirtualLock(addr, uintptr(n)) == nil
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	b := unsafe.Slice((*byte)(ptr), n)
	return b, func() {
		if locked {
			_ = windows.VirtualUnlock(addr, uintptr(n))
		}
		_ = windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
	}
}

func dpapiProtect(data, entropy []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	ent := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	name, _ := windows.UTF16PtrFromString("Keycat profile key")
	if err := windows.CryptProtectData(&in, name, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func dpapiUnprotect(data, entropy []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrAuth
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	ent := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := unsafe.Slice(out.Data, out.Size)
	result := append([]byte(nil), plain...)
	zero(plain)
	return result, nil
}

func WindowsUnlockAvailable() bool { return true }
