//go:build windows

package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
)

const manifestName = ".keycat-payload.json"

type payloadInfo struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
}

func readPayloadInfo() (payloadInfo, error) {
	var m payloadInfo
	zr, err := zstd.NewReader(bytes.NewReader(payload))
	if err != nil {
		return m, err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	hdr, err := tr.Next()
	if err != nil || hdr.Name != manifestName {
		return m, errors.New("the installer is damaged: no manifest")
	}
	if err := json.NewDecoder(io.LimitReader(tr, 1<<16)).Decode(&m); err != nil {
		return m, fmt.Errorf("the installer is damaged: %w", err)
	}
	return m, nil
}

type progress struct {
	mu        sync.Mutex
	done      int64
	total     int64
	phase     string
	finished  bool
	err       error
	cancelled atomic.Bool
}

func (p *progress) set(done, total int64, phase string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done, p.total = done, total
	if phase != "" {
		p.phase = phase
	}
}

func (p *progress) add(n int64) {
	p.mu.Lock()
	p.done += n
	p.mu.Unlock()
}

func (p *progress) finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finished, p.err = true, err
}

func (p *progress) snapshot() (frac float64, phase string, finished bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.total > 0 {
		frac = float64(p.done) / float64(p.total)
	}
	return frac, p.phase, p.finished, p.err
}

func extract(dest string, pr *progress) error {
	zr, err := zstd.NewReader(bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	buf := make([]byte, 1<<20)
	for {
		if pr.cancelled.Load() {
			return errCancelled
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the installer is damaged: %w", err)
		}
		if hdr.Name == manifestName {
			continue
		}
		rel := filepath.FromSlash(strings.TrimSuffix(hdr.Name, "/"))
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("the installer is damaged (path %q)", hdr.Name)
		}
		target := filepath.Join(dest, rel)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			for {
				if pr.cancelled.Load() {
					f.Close()
					return errCancelled
				}
				n, rerr := tr.Read(buf)
				if n > 0 {
					if _, werr := f.Write(buf[:n]); werr != nil {
						f.Close()
						return werr
					}
					pr.add(int64(n))
				}
				if rerr == io.EOF {
					break
				}
				if rerr != nil {
					f.Close()
					return fmt.Errorf("the installer is damaged: %w", rerr)
				}
			}
			if err := f.Close(); err != nil {
				return err
			}
			_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		}
	}
}

func install(dir string, desktop bool, o options, pr *progress) error {
	info, err := readPayloadInfo()
	if err != nil {
		return err
	}
	pr.set(0, info.Bytes, "Copying Keycat and its browser engine")
	if fileExists(dir) && !looksInstalled(dir) {
		if entries, _ := os.ReadDir(dir); len(entries) > 0 {
			return fmt.Errorf("%s already contains other files; choose an empty folder", dir)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	staging, old := dir+".new", dir+".old"
	if err := retryRemoveAll(staging); err != nil {
		return fmt.Errorf("could not clear %s: %w", staging, err)
	}
	if err := extract(staging, pr); err != nil {
		_ = retryRemoveAll(staging)
		return err
	}
	pr.set(info.Bytes, info.Bytes, "Finishing")
	if looksInstalled(dir) {
		_ = retryRemoveAll(old)
		if err := retryRename(dir, old); err != nil {
			_ = retryRemoveAll(staging)
			return fmt.Errorf("Keycat's files are in use; close Keycat and try again (%v)", err)
		}
		if err := retryRename(staging, dir); err != nil {
			_ = retryRename(old, dir)
			_ = retryRemoveAll(staging)
			return err
		}
		for _, name := range []string{"Data", "portable"} {
			if fileExists(filepath.Join(old, name)) {
				if err := retryRename(filepath.Join(old, name), filepath.Join(dir, name)); err != nil {
					return fmt.Errorf("could not move the portable data back from %s: %w", old, err)
				}
			}
		}
		if err := retryRemoveAll(old); err != nil {
			logf("could not remove %s: %v", old, err)
		}
	} else {
		_ = os.Remove(dir)
		if err := retryRename(staging, dir); err != nil {
			_ = retryRemoveAll(staging)
			return err
		}
	}
	if !o.noRegister {
		removeEngineShortcuts(dir)
		if err := register(dir, info.Bytes); err != nil {
			return fmt.Errorf("could not add Keycat to Apps & features: %w", err)
		}
		if err := registerTaskbarID(dir); err != nil {
			logf("taskbar id: %v", err)
		}
		exe := filepath.Join(dir, "Keycat.exe")
		if err := createShortcut(startMenuShortcut(), exe, dir, "Keycat: browser profiles in encrypted vaults"); err != nil {
			logf("start menu shortcut: %v", err)
		}
		if desktop {
			if err := createShortcut(desktopShortcut(), exe, dir, "Keycat: browser profiles in encrypted vaults"); err != nil {
				logf("desktop shortcut: %v", err)
			}
		} else {
			_ = os.Remove(desktopShortcut())
		}
	}
	logf("installed Keycat %s into %s", version, dir)
	return nil
}

func runInstall(o options) int {
	dir := o.dir
	prevDir, prevVer := installed()
	if dir == "" {
		dir = prevDir
		if dir == "" || o.noRegister {
			dir = defaultInstallDir()
		}
	}
	dir = filepath.Clean(dir)
	if !strings.EqualFold(filepath.Clean(prevDir), dir) {
		prevVer = ""
	}
	updating := looksInstalled(dir)
	desktop := o.desktop != "no"
	if o.desktop == "" && updating {
		desktop = fileExists(desktopShortcut())
	}
	if o.silent {
		if names := runningFrom(dir); len(names) > 0 {
			if !o.closeRunning {
				logf("Keycat is running (%s); use --close-running to close it first", strings.Join(names, ", "))
				return exitFailed
			}
			if err := closeKeycat(dir, 5*time.Minute, func(s string) { logf("%s", s) }); err != nil {
				logf("could not close Keycat: %v", err)
				return exitFailed
			}
		}
		if err := install(dir, desktop, o, &progress{}); err != nil {
			logf("install failed: %v", err)
			return exitFailed
		}
		return exitOK
	}
	return installUI(dir, desktop, updating, prevVer, o)
}

func newerVersion(a, b string) bool {
	parse := func(v string) [3]int {
		var n [3]int
		for i, part := range strings.SplitN(strings.SplitN(v, "-", 2)[0], ".", 3) {
			fmt.Sscanf(part, "%d", &n[i])
		}
		return n
	}
	x, y := parse(a), parse(b)
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

func installUI(dir string, desktop, updating bool, prevVer string, o options) int {
	demo := o.shot != ""
	d := &Dialog{Title: "Keycat Setup", Offscreen: demo}
	pr := &progress{}
	result := exitCancelled
	launchAfter := false
	wasRunning := false
	shots := 0
	snap := func(d *Dialog, name string) {
		shots++
		if err := captureWindow(d.hwnd, fmt.Sprintf("%s-%d-%s.png", o.shot, shots, name)); err != nil {
			logf("screenshot: %v", err)
		}
	}
	running := len(runningFrom(dir)) > 0 && !demo

	verb, title := "Install", "Install Keycat "+version
	content := "A privacy browser with encrypted profiles, made by tordev.\n\n" +
		"Keycat is installed for your Windows account only, in:\n" + dir
	if updating {
		verb, title = "Update", "Update Keycat to "+version
		installedAs := "Keycat"
		if prevVer != "" {
			installedAs += " " + prevVer
		}
		switch {
		case prevVer == version:
			verb, title = "Reinstall", "Reinstall Keycat "+version
		case prevVer != "" && newerVersion(prevVer, version):
			verb, title = "Install "+version, "Replace Keycat "+prevVer+" with the older "+version+"?"
		}
		content = installedAs + " is installed in:\n" + dir + "\n\n" +
			"Your profiles, passwords and settings stay where they are; the update does not change them."
		if running {
			content += "\n\nKeycat is open. Setup locks your open profiles (they are encrypted as usual), " +
				"closes Keycat, and opens it again when the update is done."
		}
	}

	var welcome, closing, busy, working, done, failed *Page
	var closeStatus atomic.Value
	var closeDone atomic.Bool
	var closeErr error

	startInstall := func(d *Dialog) {
		d.Navigate(working)
		go func() {
			if demo {
				pr.set(0, 100, "Copying Keycat and its browser engine")
				for i := 0; i <= 100; i += 5 {
					pr.set(int64(i), 100, "")
					time.Sleep(60 * time.Millisecond)
				}
				pr.finish(nil)
				return
			}
			pr.finish(install(dir, desktop, o, pr))
		}()
	}
	start := func(d *Dialog) {
		if len(runningFrom(dir)) > 0 && !demo {
			wasRunning = true
			closeDone.Store(false)
			closeStatus.Store("Locking your open profiles and closing Keycat")
			d.Navigate(closing)
			go func() {
				closeErr = closeKeycat(dir, 5*time.Minute, func(s string) { closeStatus.Store(s) })
				closeDone.Store(true)
			}()
			return
		}
		startInstall(d)
	}
	welcome = &Page{
		Instruction: title, Content: content, Icon: iconApp,
		Verification: "Create a desktop shortcut", Checked: desktop,
		Buttons: []Button{{101, verb}}, Common: tdcbfCancel, Default: 101, Cancellable: true,
		Footer: "Keycat by tordev. Help and news: github.com/vulpinepw",
		OnButton: func(d *Dialog, id int32) bool {
			if id != 101 {
				return false
			}
			desktop = welcome.Checked
			start(d)
			return true
		},
	}
	closing = &Page{
		Instruction: "Closing Keycat", Content: "Locking your open profiles and closing Keycat", Icon: iconApp, Marquee: true,
		Common:    tdcbfCancel,
		OnCreated: func(d *Dialog) { d.EnableButton(idCancel, false) },
		OnTimer: func(d *Dialog) {
			if s, ok := closeStatus.Load().(string); ok {
				d.SetContent(s)
			}
			if !closeDone.Load() {
				return
			}
			if closeErr != nil {
				busy.Content = "Keycat could not be closed automatically (" + closeErr.Error() + "). " +
					"Lock or close your open profiles, close the Keycat window, then select Retry."
				d.Navigate(busy)
				return
			}
			startInstall(d)
		},
		OnButton: func(d *Dialog, id int32) bool { return true },
	}
	busy = &Page{
		Instruction: "Close Keycat first", Icon: iconWarning, Common: tdcbfRetry | tdcbfCancel, Cancellable: true,
		OnButton: func(d *Dialog, id int32) bool {
			if id == idRetry {
				start(d)
				return true
			}
			return false
		},
	}
	shotTaken := false
	working = &Page{
		Instruction: strings.TrimSuffix(strings.Fields(verb)[0], "e") + "ing Keycat", Content: "Preparing", Icon: iconApp,
		Progress: true, Common: tdcbfCancel,
		OnTimer: func(d *Dialog) {
			frac, phase, finished, err := pr.snapshot()
			d.SetProgress(frac)
			if phase != "" {
				d.SetContent(fmt.Sprintf("%s: %d%%", phase, int(frac*100)))
			}
			if demo && !shotTaken && frac >= 0.5 {
				shotTaken = true
				snap(d, "progress")
			}
			if !finished {
				return
			}
			switch {
			case errors.Is(err, errCancelled):
				result = exitCancelled
				d.Close()
			case err != nil:
				failed.Content = err.Error()
				result = exitFailed
				d.Navigate(failed)
			default:
				result = exitOK
				if updating {
					done.Instruction = "Keycat is up to date"
					done.Content = "Keycat " + version + " is installed. Your profiles are just as you left them, sealed in their encrypted vaults."
				} else {
					where := "the Start menu"
					if desktop {
						where = "the Start menu or your desktop"
					}
					done.Content = "Open Keycat from " + where + ".\n\nYour profiles live in your user folder, sealed in their encrypted vaults. Updating or uninstalling Keycat keeps them."
				}
				done.Checked = done.Checked || (wasRunning && !o.noLaunch)
				d.Navigate(done)
			}
		},
		OnButton: func(d *Dialog, id int32) bool {
			if _, _, finished, _ := pr.snapshot(); id == idCancel && !finished {
				pr.cancelled.Store(true)
				d.EnableButton(idCancel, false)
				d.SetContent("Cancelling")
				return true
			}
			return false
		},
	}
	done = &Page{
		Instruction: "Keycat is ready", Icon: iconApp, Common: tdcbfClose, Cancellable: true,
		Verification: "Open Keycat now", Checked: !o.noLaunch && !demo,
		OnButton: func(d *Dialog, id int32) bool {
			launchAfter = done.Checked
			return false
		},
	}
	failed = &Page{Instruction: "Keycat could not be installed", Icon: iconError, Common: tdcbfClose, Cancellable: true}
	if demo {
		welcome.OnTimer = func(d *Dialog) {
			if shots == 0 {
				snap(d, "welcome")
				d.Click(101)
			}
		}
		done.OnTimer = func(d *Dialog) {
			if shots < 3 {
				snap(d, "done")
				d.Close()
			}
		}
	}

	if _, _, err := d.Run(welcome); err != nil {
		logf("dialog: %v", err)
		messageBox("Keycat Setup", "Setup could not show its window: "+err.Error(), 0x10)
		return exitFailed
	}
	if launchAfter && result == exitOK {
		if err := launch(filepath.Join(dir, "Keycat.exe")); err != nil {
			logf("launch: %v", err)
		}
	}
	return result
}
