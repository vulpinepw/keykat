package app

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"keycat/internal/firefox"
	"keycat/internal/session"
)

type Host interface {
	ShowHub()
	StartBrowser(profileID, dir, api, token string) error
	KillBrowser(profileID string)
	FocusBrowser(profileID string) bool
	DefaultBrowser() (registered, isDefault bool)
	MakeDefaultBrowser() error
}

type Engine struct {
	Version string
	BuildID string
}

type hosted struct {
	mu       sync.Mutex
	browsers map[string]*hostBrowser
}

type hostBrowser struct {
	host Host
	id   string
	done chan struct{}
	once sync.Once
}

func (b *hostBrowser) Wait() error { <-b.done; return nil }
func (b *hostBrowser) Kill() error { b.host.KillBrowser(b.id); return nil }
func (b *hostBrowser) Pid() int    { return 0 }

func (a *App) hostReady() error {
	a.hosted.mu.Lock()
	var open string
	for id := range a.hosted.browsers {
		open = id
	}
	a.hosted.mu.Unlock()
	if open == "" {
		return nil
	}
	name := "the open profile"
	if p, err := a.Store.Get(open); err == nil {
		name = p.Name
	}
	return fmt.Errorf("Lock %s first. On Android one profile is open at a time.", name)
}

func (a *App) startInHost(opt firefox.LaunchOptions) (session.Browser, error) {
	env := map[string]string{}
	for _, kv := range opt.Env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	id := env["KEYCAT_PROFILE"]
	if id == "" {
		return nil, errors.New("no profile to open")
	}
	b := &hostBrowser{host: a.Options.Host, id: id, done: make(chan struct{})}
	a.hosted.mu.Lock()
	if a.hosted.browsers == nil {
		a.hosted.browsers = map[string]*hostBrowser{}
	}
	a.hosted.browsers[id] = b
	a.hosted.mu.Unlock()
	if err := a.Options.Host.StartBrowser(id, opt.ProfileDir, env["KEYCAT_API"], env["KEYCAT_TOKEN"]); err != nil {
		a.BrowserExited(id)
		return nil, err
	}
	return b, nil
}

func (a *App) BrowserExited(profileID string) {
	a.hosted.mu.Lock()
	b := a.hosted.browsers[profileID]
	delete(a.hosted.browsers, profileID)
	a.hosted.mu.Unlock()
	if b != nil {
		b.once.Do(func() { close(b.done) })
	}
}

func (a *App) RunHosted() error {
	if a.Options.Host == nil {
		return errors.New("no host")
	}
	a.log.Printf("Keycat %s starting in the app (data %s)", a.Options.Version, a.Paths.DataDir)
	if err := a.startServer(); err != nil {
		return err
	}
	a.Sessions.Recover()
	go a.watchIdle()
	return nil
}
