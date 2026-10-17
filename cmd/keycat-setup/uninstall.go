//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const tdmClickVerification = wmUser + 113

func uninstall(dir string, deleteData bool, o options) error {
	if !looksInstalled(dir) {
		return fmt.Errorf("no Keycat installation in %s", dir)
	}
	if !o.noRegister {
		removeEngineShortcuts(dir)
		_ = os.Remove(startMenuShortcut())
		_ = os.Remove(desktopShortcut())
		if err := unregister(); err != nil {
			logf("unregister: %v", err)
		}
		unregisterTaskbarID(dir)
		removeEngineRegistrations(dir)
		removeEngineRecords(dir)
	}
	if orig := filepath.Join(dir, "uninstall.exe"); fileExists(orig) {
		self, _ := os.Executable()
		if !strings.EqualFold(filepath.Clean(self), orig) {
			moved := filepath.Join(os.TempDir(), fmt.Sprintf("keycat-uninstall-old-%d.exe", time.Now().UnixNano()))
			if err := os.Rename(orig, moved); err == nil {
				deleteLater(moved)
			}
		}
	}
	if !deleteData && fileExists(filepath.Join(dir, "Data")) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name() == "Data" || e.Name() == "portable" {
				continue
			}
			if err := retryRemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	} else if err := retryRemoveAll(dir); err != nil {
		return fmt.Errorf("could not remove %s: %w", dir, err)
	}
	if deleteData {
		for _, d := range dataDirs() {
			if err := retryRemoveAll(d); err != nil {
				return fmt.Errorf("could not delete %s: %w", d, err)
			}
		}
	}
	logf("uninstalled Keycat from %s (data deleted: %v)", dir, deleteData)
	return nil
}

func copySelf(dst string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	in, err := os.Open(self)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func keycatRunning(dir string) []string {
	var out []string
	for _, name := range runningFrom(dir) {
		if !strings.EqualFold(name, "uninstall.exe") {
			out = append(out, name)
		}
	}
	return out
}

func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func runUninstall(o options) int {
	self, _ := os.Executable()
	dir := o.dir
	if dir == "" {
		dir = filepath.Dir(self)
	}
	dir = filepath.Clean(dir)
	if !looksInstalled(dir) {
		logf("no Keycat installation in %s", dir)
		if !o.silent {
			messageBox("Keycat", "No Keycat installation was found in "+dir+".", 0x30)
		}
		return exitFailed
	}
	if !o.fromTemp && isWithin(self, dir) {
		tmp := filepath.Join(os.TempDir(), fmt.Sprintf("keycat-uninstall-%d.exe", time.Now().UnixNano()))
		if err := copySelf(tmp); err != nil {
			logf("copy: %v", err)
			return exitFailed
		}
		args := []string{"--uninstall", "--from-temp", "--dir", dir}
		for flagName, on := range map[string]bool{"--silent": o.silent, "--delete-data": o.deleteData, "--no-register": o.noRegister} {
			if on {
				args = append(args, flagName)
			}
		}
		if o.logFile != "" {
			args = append(args, "--log", o.logFile)
		}
		cmd := exec.Command(tmp, args...)
		if err := cmd.Start(); err != nil {
			logf("start: %v", err)
			return exitFailed
		}
		if o.silent {
			_ = cmd.Wait()
			return cmd.ProcessState.ExitCode()
		}
		return exitOK
	}
	if o.fromTemp {
		defer deleteLater(self)
	}
	if o.silent {
		if names := keycatRunning(dir); len(names) > 0 {
			logf("Keycat is running (%s)", strings.Join(names, ", "))
			return exitFailed
		}
		if err := uninstall(dir, o.deleteData, o); err != nil {
			logf("uninstall failed: %v", err)
			return exitFailed
		}
		return exitOK
	}
	return uninstallUI(dir, o)
}

func uninstallUI(dir string, o options) int {
	demo := o.shot != ""
	d := &Dialog{Title: "Remove Keycat", Offscreen: demo}
	pr := &progress{}
	result := exitCancelled
	deleteData := false
	shots := 0
	snap := func(d *Dialog, name string) {
		shots++
		if err := captureWindow(d.hwnd, fmt.Sprintf("%s-%d-%s.png", o.shot, shots, name)); err != nil {
			logf("screenshot: %v", err)
		}
	}
	_, ver := installed()
	if ver == "" {
		ver = version
	}
	profiles := profileCount()
	what := plural(profiles, "profile", "profiles") + " will be erased for good, with their encrypted vaults, saved passwords, fake identities and rescue copies."
	if profiles == 0 {
		what = "All Keycat settings and data in your user folder will be erased for good."
	}

	var confirm, confirmDelete, closing, busy, working, done, failed *Page
	var closeDone atomic.Bool
	var closeErr error
	remove := func(d *Dialog) {
		d.Navigate(working)
		go func() {
			if demo {
				time.Sleep(900 * time.Millisecond)
				pr.finish(nil)
				return
			}
			pr.finish(uninstall(dir, deleteData, o))
		}()
	}
	start := func(d *Dialog) {
		if names := keycatRunning(dir); len(names) > 0 && !demo {
			closeDone.Store(false)
			d.Navigate(closing)
			go func() {
				closeErr = closeKeycat(dir, 5*time.Minute, func(string) {})
				closeDone.Store(true)
			}()
			return
		}
		remove(d)
	}
	closing = &Page{
		Instruction: "Closing Keycat", Content: "Locking your open profiles and closing Keycat", Icon: iconApp, Marquee: true,
		Common:    tdcbfCancel,
		OnCreated: func(d *Dialog) { d.EnableButton(idCancel, false) },
		OnTimer: func(d *Dialog) {
			if !closeDone.Load() {
				return
			}
			if closeErr != nil || len(keycatRunning(dir)) > 0 {
				busy.Content = "Keycat could not be closed automatically. Lock or close your open profiles, close the Keycat window, then select Retry."
				d.Navigate(busy)
				return
			}
			remove(d)
		},
		OnButton: func(d *Dialog, id int32) bool { return true },
	}
	confirm = &Page{
		Instruction: "Remove Keycat " + ver + "?", Icon: iconApp,
		Content: "Keycat will be removed from this computer.\n\n" +
			"Your profiles, passwords and settings are kept unless you tick the box below. They stay sealed in their encrypted vaults in your user folder, ready if you install Keycat again.",
		Verification: "Also delete my profiles, passwords and settings",
		Buttons:      []Button{{101, "Remove"}}, Common: tdcbfCancel, Default: idCancel, Cancellable: true,
		OnButton: func(d *Dialog, id int32) bool {
			if id != 101 {
				return false
			}
			if confirm.Checked {
				d.Navigate(confirmDelete)
				return true
			}
			deleteData = false
			start(d)
			return true
		},
	}
	confirmDelete = &Page{
		Instruction: "Delete all Keycat profiles?", Icon: iconWarning,
		Content: what + " This cannot be undone.",
		Buttons: []Button{{102, "Delete everything"}, {103, "Keep my profiles"}}, Common: tdcbfCancel, Default: 103, Cancellable: true,
		OnButton: func(d *Dialog, id int32) bool {
			switch id {
			case 102:
				deleteData = true
			case 103:
				deleteData = false
			default:
				return false
			}
			start(d)
			return true
		},
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
	working = &Page{
		Instruction: "Removing Keycat", Content: "Deleting the program files.", Icon: iconApp, Marquee: true,
		Common:    tdcbfCancel,
		OnCreated: func(d *Dialog) { d.EnableButton(idCancel, false) },
		OnTimer: func(d *Dialog) {
			_, _, finished, err := pr.snapshot()
			if !finished {
				return
			}
			if err != nil {
				failed.Content = err.Error()
				result = exitFailed
				d.Navigate(failed)
				return
			}
			result = exitOK
			if deleteData {
				done.Content = "Keycat and all of its profiles were deleted."
			} else {
				done.Content = "Your profiles were kept, sealed in their encrypted vaults. Install Keycat again to open them."
			}
			d.Navigate(done)
		},
		OnButton: func(d *Dialog, id int32) bool { return true },
	}
	done = &Page{Instruction: "Keycat was removed", Icon: iconApp, Common: tdcbfClose, Cancellable: true}
	failed = &Page{Instruction: "Keycat could not be removed completely", Icon: iconError, Common: tdcbfClose, Cancellable: true}
	if demo {
		confirm.OnTimer = func(d *Dialog) {
			if shots == 0 {
				snap(d, "confirm")
				d.send(tdmClickVerification, 1, 0)
				d.Click(101)
			}
		}
		confirmDelete.OnTimer = func(d *Dialog) {
			if shots == 1 {
				snap(d, "confirm-delete")
				d.Click(103)
			}
		}
		done.OnTimer = func(d *Dialog) {
			if shots == 2 {
				snap(d, "done")
				d.Close()
			}
		}
	}
	if _, _, err := d.Run(confirm); err != nil {
		logf("dialog: %v", err)
		messageBox("Remove Keycat", "The uninstaller could not show its window: "+err.Error(), 0x10)
		return exitFailed
	}
	return result
}
