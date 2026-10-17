//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procPostMessageW             = user32.NewProc("PostMessageW")
)

const wmClose = 0x0010

func launcherFiles(dir string) []string {
	files := []string{filepath.Join(dir, "Data", "Local", "launcher.json")}
	if d, err := os.UserCacheDir(); err == nil {
		files = append(files, filepath.Join(d, "Keycat", "launcher.json"))
	}
	return files
}

func askKeycatToQuit(dir string) bool {
	client := &http.Client{Timeout: 10 * time.Second}
	asked := false
	for _, f := range launcherFiles(dir) {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var info struct {
			URL   string `json:"url"`
			Token string `json:"token"`
		}
		if json.Unmarshal(data, &info) != nil || !strings.HasPrefix(info.URL, "http://127.0.0.1:") {
			continue
		}
		req, err := http.NewRequest("POST", info.URL+"/api/quit", strings.NewReader("{}"))
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+info.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			asked = true
		}
	}
	return asked
}

func closeWindowsOf(dir string) {
	prefix := strings.ToLower(filepath.Clean(dir)) + `\`
	pids := map[uint32]bool{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) == nil &&
			strings.HasPrefix(strings.ToLower(windows.UTF16ToString(buf[:size])), prefix) {
			pids[e.ProcessID] = true
		}
		windows.CloseHandle(h)
	}
	windows.CloseHandle(snap)
	cb := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pids[pid] {
			if visible, _, _ := procIsWindowVisible.Call(hwnd); visible != 0 {
				procPostMessageW.Call(hwnd, wmClose, 0, 0)
			}
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
}

func runningLauncher(dir string) bool {
	for _, name := range runningFrom(dir) {
		if strings.EqualFold(name, "Keycat.exe") {
			return true
		}
	}
	return false
}

func othersRunning(dir string) bool {
	return len(keycatRunning(dir)) > 0
}

func closeKeycat(dir string, timeout time.Duration, status func(string)) error {
	if !othersRunning(dir) {
		return nil
	}
	deadline := time.Now().Add(timeout)
	if runningLauncher(dir) {
		status("Locking your open profiles and closing Keycat")
		if !askKeycatToQuit(dir) {
			return errors.New("Keycat did not answer")
		}
		for runningLauncher(dir) {
			if time.Now().After(deadline) {
				return errors.New("Keycat is still encrypting your profiles")
			}
			time.Sleep(400 * time.Millisecond)
		}
	}
	if othersRunning(dir) {
		status("Closing the remaining Keycat windows")
		closeWindowsOf(dir)
		end := time.Now().Add(20 * time.Second)
		for othersRunning(dir) {
			if time.Now().After(end) {
				return errors.New("a Keycat window did not close")
			}
			time.Sleep(400 * time.Millisecond)
		}
	}
	time.Sleep(700 * time.Millisecond)
	return nil
}
