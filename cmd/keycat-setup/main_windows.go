package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

var version = "1.0.0-dev"

type options struct {
	silent       bool
	uninstall    bool
	deleteData   bool
	noRegister   bool
	noLaunch     bool
	fromTemp     bool
	closeRunning bool
	desktop      string
	dir          string
	shot         string
	logFile      string
}

var logger = log.New(os.Stderr, "", log.LstdFlags)

func logf(format string, args ...any) { logger.Printf(format, args...) }

func main() {
	var o options
	flag.BoolVar(&o.silent, "silent", false, "install or uninstall without any window")
	flag.BoolVar(&o.uninstall, "uninstall", false, "remove Keycat")
	flag.BoolVar(&o.deleteData, "delete-data", false, "with --silent --uninstall: also delete all profiles and settings")
	flag.BoolVar(&o.noRegister, "no-register", false, "no shortcuts and no Apps & features entry (tests)")
	flag.BoolVar(&o.noLaunch, "no-launch", false, "do not offer to open Keycat when done")
	flag.BoolVar(&o.fromTemp, "from-temp", false, "internal: the uninstaller runs from a temporary copy")
	flag.BoolVar(&o.closeRunning, "close-running", false, "with --silent: close a running Keycat first (its open profiles are locked and encrypted)")
	flag.StringVar(&o.desktop, "desktop-shortcut", "", "yes or no (default: yes, or the current choice when updating)")
	flag.StringVar(&o.dir, "dir", "", `install folder (default: %LOCALAPPDATA%\Programs\Keycat)`)
	flag.StringVar(&o.shot, "screenshot", "", "internal: walk through the pages without changing anything, saving PNGs with this prefix")
	flag.StringVar(&o.logFile, "log", "", "write a log to this file")
	flag.Parse()
	if o.logFile != "" {
		if f, err := os.OpenFile(o.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			logger.SetOutput(f)
			defer f.Close()
		}
	}
	var code int
	if len(payload) == 0 || o.uninstall {
		code = runUninstall(o)
	} else {
		code = runInstall(o)
	}
	logf("exit %d", code)
	os.Exit(code)
}

const (
	exitOK        = 0
	exitFailed    = 1
	exitCancelled = 2
)

var errCancelled = errors.New("cancelled")

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func retryRename(from, to string) error {
	var err error
	for i := 0; i < 30; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

func retryRemoveAll(dir string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

func looksInstalled(dir string) bool {
	if dir == "" || filepath.Dir(dir) == dir {
		return false
	}
	return fileExists(filepath.Join(dir, "Keycat.exe")) || fileExists(filepath.Join(dir, "core", "firefox.exe"))
}

func dataDirs() []string {
	var dirs []string
	if d, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(d, "Keycat"))
	}
	if d, err := os.UserCacheDir(); err == nil {
		dirs = append(dirs, filepath.Join(d, "Keycat"))
	}
	return dirs
}

func profileCount() int {
	d, err := os.UserConfigDir()
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir(filepath.Join(d, "Keycat", "Profiles"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) == 13 && e.Name()[0] == 'p' {
			n++
		}
	}
	return n
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
