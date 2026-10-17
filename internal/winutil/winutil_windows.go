//go:build windows

package winutil

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetWindow                = user32.NewProc("GetWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procMessageBoxW              = user32.NewProc("MessageBoxW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
)

const (
	gwOwner        = 4
	swRestore      = 9
	smShuttingDown = 0x2000
	asfwAny        = 0xFFFFFFFF
	mbIconInfo     = 0x40
	mbIconErr      = 0x10
	mbIconWarn     = 0x30
	mbYesNo        = 0x04
	idYes          = 6
)

var (
	enumMu    sync.Mutex
	enumPID   uint32
	enumFound []windows.HWND
	enumCB    = syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var wpid uint32
		procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wpid)))
		if wpid != enumPID {
			return 1
		}
		if v, _, _ := procIsWindowVisible.Call(uintptr(hwnd)); v == 0 {
			return 1
		}
		if owner, _, _ := procGetWindow.Call(uintptr(hwnd), gwOwner); owner != 0 {
			return 1
		}
		if n, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd)); n == 0 {
			return 1
		}
		enumFound = append(enumFound, hwnd)
		return 1
	})
)

func MainWindows(pid uint32) []windows.HWND {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPID, enumFound = pid, nil
	procEnumWindows.Call(enumCB, 0)
	return enumFound
}

func FocusWindow(hwnd windows.HWND) bool {
	if iconic, _, _ := procIsIconic.Call(uintptr(hwnd)); iconic != 0 {
		procShowWindow.Call(uintptr(hwnd), swRestore)
	}
	fg, _, _ := procGetForegroundWindow.Call()
	cur := windows.GetCurrentThreadId()
	var fgThread uintptr
	if fg != 0 {
		fgThread, _, _ = procGetWindowThreadProcessId.Call(fg, 0)
	}
	attached := false
	if fgThread != 0 && uint32(fgThread) != cur {
		r, _, _ := procAttachThreadInput.Call(uintptr(cur), fgThread, 1)
		attached = r != 0
	}
	procBringWindowToTop.Call(uintptr(hwnd))
	ok, _, _ := procSetForegroundWindow.Call(uintptr(hwnd))
	if attached {
		procAttachThreadInput.Call(uintptr(cur), fgThread, 0)
	}
	return ok != 0
}

func FocusProcess(pid uint32) bool {
	wins := MainWindows(pid)
	if len(wins) == 0 {
		return false
	}
	return FocusWindow(wins[0])
}

func AllowAnyForeground() {
	procAllowSetForegroundWindow.Call(asfwAny)
}

func ShuttingDown() bool {
	r, _, _ := procGetSystemMetrics.Call(smShuttingDown)
	return r != 0
}

func SingleInstance(name string) (release func(), first bool, err error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, err
	}
	h, err := windows.CreateMutex(nil, false, n)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return nil, false, err
	}
	first = err != windows.ERROR_ALREADY_EXISTS
	return func() { windows.CloseHandle(h) }, first, nil
}

func messageBox(title, text string, flags uintptr) int {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return int(r)
}

func ShowError(title, text string) { messageBox(title, text, mbIconErr) }

func ShowInfo(title, text string) { messageBox(title, text, mbIconInfo) }

func AskYesNo(title, text string) bool { return messageBox(title, text, mbIconWarn|mbYesNo) == idYes }

func OpenFolder(path string, selectFile bool) error {
	if selectFile {
		return exec.Command("explorer.exe", "/select,", path).Start()
	}
	return exec.Command("explorer.exe", path).Start()
}
