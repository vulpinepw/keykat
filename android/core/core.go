package core

import (
	"errors"
	"os"
	"sync"

	_ "golang.org/x/mobile/bind"

	"keycat"
	"keycat/internal/app"
)

type Host interface {
	ShowHub()
	StartBrowser(profileID, dir, api, token string) error
	KillBrowser(profileID string)
	FocusBrowser(profileID string) bool
	DefaultBrowser() int
	MakeDefaultBrowser() error
}

var (
	mu      sync.Mutex
	running *app.App
)

func Start(dataDir, version, engineVersion, engineBuild string, host Host) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if running != nil {
		return running.HubURL(), nil
	}
	if err := os.Setenv("KEYCAT_DATA_DIR", dataDir); err != nil {
		return "", err
	}
	a, err := app.New(app.Options{
		Version: version,
		Host:    hostAdapter{host},
		Engine:  app.Engine{Version: engineVersion, BuildID: engineBuild},
	}, keycat.LayerFS(), keycat.HubFS())
	if err != nil {
		return "", err
	}
	if err := a.RunHosted(); err != nil {
		return "", err
	}
	running = a
	return a.HubURL(), nil
}

func BrowserExited(profileID string) {
	if a := current(); a != nil {
		a.BrowserExited(profileID)
	}
}

func OpenLink(url string) error {
	a := current()
	if a == nil {
		return errors.New("Keycat is not running")
	}
	return a.ReceiveLinks([]string{url})
}

func Lock() {
	if a := current(); a != nil {
		for _, id := range a.Sessions.Running() {
			_ = a.Sessions.Lock(id)
		}
	}
}

func Busy() bool {
	a := current()
	return a != nil && a.Sessions.ActiveCount() > 0
}

func Shutdown() {
	if a := current(); a != nil {
		a.Shutdown()
	}
}

func current() *app.App {
	mu.Lock()
	defer mu.Unlock()
	return running
}

type hostAdapter struct{ h Host }

func (a hostAdapter) ShowHub() { a.h.ShowHub() }

func (a hostAdapter) StartBrowser(profileID, dir, api, token string) error {
	return a.h.StartBrowser(profileID, dir, api, token)
}

func (a hostAdapter) KillBrowser(profileID string) { a.h.KillBrowser(profileID) }

func (a hostAdapter) FocusBrowser(profileID string) bool { return a.h.FocusBrowser(profileID) }

func (a hostAdapter) DefaultBrowser() (bool, bool) {
	v := a.h.DefaultBrowser()
	return v&1 != 0, v&2 != 0
}

func (a hostAdapter) MakeDefaultBrowser() error { return a.h.MakeDefaultBrowser() }
