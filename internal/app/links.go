package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"keycat/internal/platform"
	"keycat/internal/profile"
	"keycat/internal/session"
)

type Link struct {
	ID      string    `json:"id"`
	URL     string    `json:"url"`
	Profile string    `json:"profile,omitempty"`
	Time    time.Time `json:"time"`
}

const maxLinks = 20

type linkQueue struct {
	mu      sync.Mutex
	pending []Link
}

func (q *linkQueue) add(l Link) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(q.pending, l)
	if len(q.pending) > maxLinks {
		q.pending = q.pending[len(q.pending)-maxLinks:]
	}
}

func (q *linkQueue) take(id string) (Link, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, l := range q.pending {
		if l.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return l, true
		}
	}
	return Link{}, false
}

func (q *linkQueue) list() []Link {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Link{}, q.pending...)
}

var errBadLink = errors.New("not a web address or a file")

func ParseLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 32<<10 || strings.ContainsAny(raw, "\x00\r\n") {
		return "", errBadLink
	}
	if u, err := url.Parse(raw); err == nil && len(u.Scheme) > 1 {
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			if u.Host == "" {
				return "", errBadLink
			}
			return u.String(), nil
		case "file":
			return u.String(), nil
		}
		return "", fmt.Errorf("Keycat does not open %s: addresses", u.Scheme)
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", errBadLink
	}
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String(), nil
}

func (a *App) ReceiveLinks(raw []string) error {
	var urls []string
	for _, r := range raw {
		u, err := ParseLink(r)
		if err != nil {
			return err
		}
		urls = append(urls, u)
	}
	for _, u := range urls {
		a.log.Printf("link from another app")
		a.routeLink(Link{ID: randomHex(8), URL: u, Time: time.Now().UTC()})
	}
	return nil
}

func (a *App) routeLink(l Link) {
	target := a.Settings.Get().OpenLinks
	switch target {
	case "auto":
		target = ""
		if running := a.Sessions.Running(); len(running) == 1 {
			target = running[0]
		}
	case "ask":
		target = ""
	}
	if target != "" {
		if err := a.openLinkIn(target, l); err == nil {
			return
		} else if !errors.Is(err, errNeedsUnlock) {
			a.log.Printf("link: %v", err)
		} else {
			l.Profile = target
		}
	}
	a.links.add(l)
	a.Bus.Publish("links", a.links.list())
	if !a.Options.NoHub {
		if err := a.Hub.Show(); err != nil {
			a.log.Printf("profiles window: %v", err)
		}
	}
}

var errNeedsUnlock = errors.New("the profile has to be unlocked first")

func (a *App) openLinkIn(id string, l Link) error {
	p, err := a.Store.Get(id)
	if err != nil {
		return err
	}
	switch a.Sessions.Status(id).State {
	case session.StateRunning:
		a.Sessions.Send(id, session.Command{Type: "open", URL: l.URL})
		a.Sessions.Focus(id)
		return nil
	case session.StateLocked:
		if p.IsEncrypted() {
			return errNeedsUnlock
		}
		if err := a.Sessions.Open(id, session.Credentials{}); err != nil {
			return err
		}
		a.Sessions.Send(id, session.Command{Type: "open", URL: l.URL})
		a.afterOpen()
		return nil
	}
	return fmt.Errorf("%s is busy", p.Name)
}

func validOpenLinks(v string) bool {
	return v == "auto" || v == "ask" || profile.ValidID(v)
}

func (a *App) handleLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URLs []string `json:"urls"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if len(req.URLs) == 0 || len(req.URLs) > 10 {
		writeError(w, http.StatusBadRequest, "link", "expected one to ten links")
		return
	}
	if err := a.ReceiveLinks(req.URLs); err != nil {
		writeError(w, http.StatusBadRequest, "link", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleLinkOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	l, ok := a.links.take(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "link", "This link is no longer waiting.")
		return
	}
	if err := a.openLinkIn(req.Profile, l); err != nil {
		a.links.add(l)
		writeErr(w, err)
		return
	}
	a.Bus.Publish("links", a.links.list())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleLinkDismiss(w http.ResponseWriter, r *http.Request) {
	a.links.take(r.PathValue("id"))
	a.Bus.Publish("links", a.links.list())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func defaultBrowserStatus() map[string]bool {
	exe, _ := os.Executable()
	registered, isDefault := platform.DefaultBrowser(exe)
	return map[string]bool{"registered": registered, "isDefault": isDefault}
}

func (a *App) handleDefaultBrowser(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, defaultBrowserStatus())
}

func (a *App) handleMakeDefaultBrowser(w http.ResponseWriter, r *http.Request) {
	exe, _ := os.Executable()
	if err := platform.MakeDefaultBrowser(exe); err != nil {
		writeError(w, http.StatusConflict, "default_browser", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, defaultBrowserStatus())
}
