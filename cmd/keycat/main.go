package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"keycat"
	"keycat/internal/app"
	"keycat/internal/firefox"
	"keycat/internal/winutil"
)

var version = "1.0.0-dev"

func main() {
	headless := flag.Bool("headless", false, "run Firefox without windows (automated tests)")
	noHub := flag.Bool("no-hub", false, "do not open the hub window (automated tests)")
	marionette := flag.Bool("marionette", false, "enable Firefox remote automation in profiles (automated tests)")
	setupCore := flag.String("setup-core", "", "copy the Firefox engine from a Firefox installation `dir`, install the Keycat layer, and exit")
	applyLayer := flag.Bool("apply-layer", false, "re-install the Keycat layer into the engine and exit")
	waitPID := flag.Int("wait-pid", 0, "wait for this process to exit before starting (used when Keycat restarts itself)")
	flag.Parse()

	if *waitPID > 0 {
		if proc, err := os.FindProcess(*waitPID); err == nil {
			_, _ = proc.Wait()
		}
	}

	opts := app.Options{Version: version, Headless: *headless, NoHub: *noHub, Marionette: *marionette}
	hub := keycat.HubFS()
	if dir := os.Getenv("KEYCAT_HUB_DIR"); dir != "" {
		hub = os.DirFS(dir)
	}
	a, err := app.New(opts, keycat.LayerFS(), hub)
	if err != nil {
		fail(opts, err)
	}

	switch {
	case *setupCore != "":
		src, _ := filepath.Abs(*setupCore)
		if err := setup(a, src); err != nil {
			fail(opts, err)
		}
		return
	case *applyLayer:
		c, err := a.Core()
		if err == nil {
			err = firefox.ApplyLayer(c, a.Layer)
		}
		if err != nil {
			fail(opts, err)
		}
		return
	}

	if err := a.Run(); err != nil {
		fail(opts, err)
	}
}

func setup(a *app.App, src string) error {
	if _, err := firefox.DetectCore(src); err != nil {
		return fmt.Errorf("%s does not contain Firefox", src)
	}
	dst := a.Paths.CoreDir
	tmp := dst + ".new"
	_ = os.RemoveAll(tmp)
	if err := firefox.CopyCore(src, tmp, nil); err != nil {
		return err
	}
	_ = os.RemoveAll(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	c, err := firefox.DetectCore(dst)
	if err != nil {
		return err
	}
	return firefox.ApplyLayer(c, a.Layer)
}

func fail(opts app.Options, err error) {
	if !opts.NoHub {
		winutil.ShowError("Keycat", err.Error())
	}
	fmt.Fprintln(os.Stderr, "keycat:", err)
	os.Exit(1)
}
