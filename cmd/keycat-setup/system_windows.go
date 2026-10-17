//go:build windows

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Keycat`

func knownFolder(id *windows.KNOWNFOLDERID, fallback string) string {
	if p, err := windows.KnownFolderPath(id, 0); err == nil && p != "" {
		return p
	}
	return fallback
}

func defaultInstallDir() string {
	base := knownFolder(windows.FOLDERID_UserProgramFiles, filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"))
	return filepath.Join(base, "Keycat")
}

func startMenuShortcut() string {
	return filepath.Join(knownFolder(windows.FOLDERID_Programs, filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)), "Keycat.lnk")
}

func desktopShortcut() string {
	return filepath.Join(knownFolder(windows.FOLDERID_Desktop, filepath.Join(os.Getenv("USERPROFILE"), "Desktop")), "Keycat.lnk")
}

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

	clsidShellLink     = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIShellLinkW     = windows.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIPersistFile    = windows.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIPropertyStore  = windows.GUID{Data1: 0x886D8EEB, Data2: 0x8CF2, Data3: 0x4446, Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	pkeyAppUserModelID = propertyKey{
		fmtid: windows.GUID{Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39, Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}},
		pid:   5,
	}
)

const keycatAUMID = "tordev.Keycat"

type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

type propVariant struct {
	vt       uint16
	reserved [3]uint16
	str      *uint16
	pad      uintptr
}

type comObject struct{ vtbl *[32]uintptr }

func (o *comObject) call(method int, args ...uintptr) error {
	all := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	hr, _, _ := syscall.SyscallN(o.vtbl[method], all...)
	if int32(hr) < 0 {
		return fmt.Errorf("COM call failed (0x%08x)", uint32(hr))
	}
	return nil
}

func (o *comObject) release() { _ = o.call(2) }

func wide(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func withCOM(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err {
	case nil, syscall.Errno(1):
		defer windows.CoUninitialize()
	case syscall.Errno(0x80010106):
	default:
		return err
	}
	return fn()
}

func newShellLink() (*comObject, error) {
	var link *comObject
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1,
		uintptr(unsafe.Pointer(&iidIShellLinkW)), uintptr(unsafe.Pointer(&link)))
	if int32(hr) < 0 || link == nil {
		return nil, fmt.Errorf("could not create a shortcut (0x%08x)", uint32(hr))
	}
	return link, nil
}

func createShortcut(lnk, target, workDir, description string) error {
	return createShortcutWithID(lnk, target, workDir, description, keycatAUMID)
}

func createShortcutWithID(lnk, target, workDir, description, appID string) error {
	return withCOM(func() error {
		link, err := newShellLink()
		if err != nil {
			return err
		}
		defer link.release()
		targetW, dirW, descW, lnkW, idW := wide(target), wide(workDir), wide(description), wide(lnk), wide(appID)
		defer runtime.KeepAlive([]*uint16{targetW, dirW, descW, lnkW, idW})
		if err := link.call(20, uintptr(unsafe.Pointer(targetW))); err != nil {
			return err
		}
		if err := link.call(9, uintptr(unsafe.Pointer(dirW))); err != nil {
			return err
		}
		if err := link.call(7, uintptr(unsafe.Pointer(descW))); err != nil {
			return err
		}
		if err := link.call(17, uintptr(unsafe.Pointer(targetW)), 0); err != nil {
			return err
		}
		var store *comObject
		if appID == "" {
		} else if err := link.call(0, uintptr(unsafe.Pointer(&iidIPropertyStore)), uintptr(unsafe.Pointer(&store))); err == nil && store != nil {
			v := propVariant{vt: 31, str: idW}
			if err := store.call(6, uintptr(unsafe.Pointer(&pkeyAppUserModelID)), uintptr(unsafe.Pointer(&v))); err != nil {
				logf("shortcut app id: %v", err)
			} else if err := store.call(7); err != nil {
				logf("shortcut app id: %v", err)
			}
			store.release()
		}
		var file *comObject
		if err := link.call(0, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&file))); err != nil || file == nil {
			return fmt.Errorf("could not save the shortcut: %v", err)
		}
		defer file.release()
		_ = os.MkdirAll(filepath.Dir(lnk), 0o755)
		return file.call(6, uintptr(unsafe.Pointer(lnkW)), 1)
	})
}

func shortcutTarget(lnk string) (string, error) {
	var target string
	err := withCOM(func() error {
		link, err := newShellLink()
		if err != nil {
			return err
		}
		defer link.release()
		var file *comObject
		if err := link.call(0, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&file))); err != nil || file == nil {
			return fmt.Errorf("could not read the shortcut: %v", err)
		}
		defer file.release()
		lnkW := wide(lnk)
		defer runtime.KeepAlive(lnkW)
		if err := file.call(5, uintptr(unsafe.Pointer(lnkW)), 0); err != nil {
			return err
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		if err := link.call(3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 4); err != nil {
			return err
		}
		target = windows.UTF16ToString(buf)
		return nil
	})
	return target, err
}

func removeEngineShortcuts(dir string) {
	programs := knownFolder(windows.FOLDERID_Programs, filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`))
	removed := removeEngineShortcutsIn(dir,
		programs,
		knownFolder(windows.FOLDERID_Desktop, filepath.Join(os.Getenv("USERPROFILE"), "Desktop")),
		knownFolder(windows.FOLDERID_Startup, filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`)),
	)
	if len(removed) == 0 {
		return
	}
	common := knownFolder(windows.FOLDERID_CommonPrograms, filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`))
	firefox, appID := realFirefox(dir)
	restoreFirefoxShortcuts(removed, programs, common, firefox, appID)
}

func removeEngineShortcutsIn(dir string, folders ...string) map[string]string {
	engine := strings.ToLower(filepath.Join(filepath.Clean(dir), "core")) + `\`
	removed := map[string]string{}
	for _, folder := range folders {
		entries, err := os.ReadDir(folder)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".lnk") {
				continue
			}
			p := filepath.Join(folder, e.Name())
			target, err := shortcutTarget(p)
			if err != nil || !strings.HasPrefix(strings.ToLower(target), engine) {
				continue
			}
			if err := os.Remove(p); err != nil {
				logf("could not remove %s: %v", p, err)
			} else {
				logf("removed %s (it pointed at the browser engine)", p)
				removed[p] = target
			}
		}
	}
	return removed
}

func restoreFirefoxShortcuts(removed map[string]string, programs, common, firefoxDir, appID string) {
	if firefoxDir == "" {
		return
	}
	for lnk, target := range removed {
		name := filepath.Base(lnk)
		if !strings.EqualFold(filepath.Dir(lnk), filepath.Clean(programs)) || !strings.HasPrefix(strings.ToLower(name), "firefox") {
			continue
		}
		if _, err := os.Stat(filepath.Join(common, name)); err == nil {
			continue
		}
		exe := filepath.Join(firefoxDir, filepath.Base(target))
		if _, err := os.Stat(exe); err != nil {
			continue
		}
		id := appID
		if id != "" && strings.EqualFold(filepath.Base(target), "private_browsing.exe") {
			id += ";PrivateBrowsingAUMID"
		}
		if err := createShortcutWithID(lnk, exe, firefoxDir, strings.TrimSuffix(name, filepath.Ext(name)), id); err != nil {
			logf("could not restore %s: %v", lnk, err)
		} else {
			logf("restored %s for %s", lnk, exe)
		}
	}
}

func realFirefox(dir string) (folder, appID string) {
	engine := filepath.Join(filepath.Clean(dir), "core")
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, taskbarIDsKey, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := k.ReadValueNames(-1)
		for _, name := range names {
			id, _, err := k.GetStringValue(name)
			if err != nil || id == keycatAUMID || strings.EqualFold(filepath.Clean(name), engine) {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Clean(name)), "Keycat.exe")); err == nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(name, "firefox.exe")); err == nil {
				k.Close()
				return filepath.Clean(name), id
			}
		}
		k.Close()
	}
	return "", ""
}

const taskbarIDsKey = `Software\Mozilla\Firefox\TaskBarIDs`

func registerTaskbarID(dir string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, taskbarIDsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(filepath.Join(filepath.Clean(dir), "core"), keycatAUMID)
}

func unregisterTaskbarID(dir string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, taskbarIDsKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(filepath.Join(filepath.Clean(dir), "core"))
}

func removeEngineRegistrations(dir string) {
	engine := strings.ToLower(filepath.Join(filepath.Clean(dir), "core")) + `\`
	inEngine := func(v string) bool {
		return strings.HasPrefix(strings.ToLower(strings.Trim(v, `"`)), engine)
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId`, registry.ENUMERATE_SUB_KEYS); err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, name := range names {
			sub, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			icon, _, _ := sub.GetStringValue("IconUri")
			activator, _, _ := sub.GetStringValue("CustomActivator")
			sub.Close()
			if !inEngine(icon) {
				continue
			}
			if activator != "" {
				deleteTree(`Software\Classes\CLSID\` + activator)
			}
			deleteTree(`Software\Classes\AppUserModelId\` + name)
		}
	}
}

func removeEngineRecords(dir string) {
	removeEngineRecordsIn(`Software\Mozilla\Firefox`, dir)
}

func removeEngineRecordsIn(base, dir string) {
	core := strings.ToLower(filepath.Join(filepath.Clean(dir), "core"))
	exe := core + `\firefox.exe`
	cache := strings.ToLower(filepath.Join(os.Getenv("APPDATA"), "Mozilla", "Firefox"))
	for _, name := range []string{"Launcher", "PreXULSkeletonUISettings", "DllPrefetchExperiment", "Default Browser Agent"} {
		k, err := registry.OpenKey(registry.CURRENT_USER, base+`\`+name, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			continue
		}
		values, _ := k.ReadValueNames(-1)
		for _, v := range values {
			lower := strings.ToLower(v)
			if lower != exe && !strings.HasPrefix(lower, exe+"|") && !strings.HasPrefix(lower, core+"|") {
				continue
			}
			if strings.HasSuffix(lower, "|blocklist") {
				file, _, _ := k.GetStringValue(v)
				if strings.EqualFold(filepath.Dir(file), cache) && strings.HasPrefix(strings.ToLower(filepath.Base(file)), "blocklist-") {
					_ = os.Remove(file)
				}
			}
			_ = k.DeleteValue(v)
		}
		k.Close()
	}
}

func deleteTree(path string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	subs, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, s := range subs {
		deleteTree(path + `\` + s)
	}
	_ = registry.DeleteKey(registry.CURRENT_USER, path)
}

func installed() (dir, ver string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer k.Close()
	dir, _, _ = k.GetStringValue("InstallLocation")
	ver, _, _ = k.GetStringValue("DisplayVersion")
	return dir, ver
}

func register(dir string, sizeBytes int64) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer k.Close()
	uninstaller := filepath.Join(dir, "uninstall.exe")
	values := map[string]string{
		"DisplayName":          "Keycat",
		"DisplayVersion":       version,
		"Publisher":            "tordev",
		"Comments":             "Privacy browser with encrypted profiles",
		"URLInfoAbout":         "https://github.com/vulpinepw",
		"HelpLink":             "https://github.com/vulpinepw",
		"URLUpdateInfo":        "https://github.com/vulpinepw",
		"DisplayIcon":          filepath.Join(dir, "Keycat.exe") + ",0",
		"InstallLocation":      dir,
		"InstallDate":          time.Now().Format("20060102"),
		"UninstallString":      `"` + uninstaller + `"`,
		"QuietUninstallString": `"` + uninstaller + `" --silent`,
	}
	for name, v := range values {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	for name, v := range map[string]uint32{"NoModify": 1, "NoRepair": 1, "EstimatedSize": uint32(sizeBytes >> 10)} {
		if err := k.SetDWordValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

func unregister() error {
	err := registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

func runningFrom(dir string) []string {
	prefix := strings.ToLower(filepath.Clean(dir)) + `\`
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	seen := map[string]bool{}
	var names []string
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buf))
		perr := windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
		windows.CloseHandle(h)
		if perr != nil {
			continue
		}
		path := windows.UTF16ToString(buf[:size])
		if strings.HasPrefix(strings.ToLower(path), prefix) {
			name := filepath.Base(path)
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

func launch(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	return cmd.Start()
}

func deleteLater(path string) {
	cmd := exec.Command("cmd.exe", "/c", `ping 127.0.0.1 -n 4 > nul & del /f /q "`+path+`"`)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Start()
}

var (
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procPrintWindow        = user32.NewProc("PrintWindow")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

func captureWindow(hwnd uintptr, path string) error {
	var r struct{ Left, Top, Right, Bottom int32 }
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 {
		return fmt.Errorf("window has no size")
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(dc)
	bi := bitmapInfoHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits *byte
	bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return fmt.Errorf("could not create a bitmap")
	}
	defer procDeleteObject.Call(bmp)
	procSelectObject.Call(dc, bmp)
	procPrintWindow.Call(hwnd, dc, 2)
	pix := unsafe.Slice(bits, w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4+0] = pix[i*4+2]
		img.Pix[i*4+1] = pix[i*4+1]
		img.Pix[i*4+2] = pix[i*4+0]
		img.Pix[i*4+3] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
