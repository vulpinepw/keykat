package app

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"keycat/internal/identity"
	"keycat/internal/keychain"
	"keycat/internal/profile"
	"keycat/internal/session"
)

func (a *App) keychainRoutes(hub, inst func(string, http.HandlerFunc)) {
	hub("GET /api/profiles/{id}/keychain", a.handleKeychainList)
	hub("POST /api/profiles/{id}/keychain/unlock", a.handleKeychainUnlock)
	hub("POST /api/profiles/{id}/keychain/lock", a.handleKeychainLock)
	hub("POST /api/profiles/{id}/keychain/entries", a.handleKeychainSave)
	hub("PUT /api/profiles/{id}/keychain/entries/{eid}", a.handleKeychainSave)
	hub("DELETE /api/profiles/{id}/keychain/entries/{eid}", a.handleKeychainDelete)
	hub("GET /api/profiles/{id}/keychain/entries/{eid}/password", a.handleKeychainPassword)
	hub("PUT /api/profiles/{id}/keychain/entries/{eid}/share", a.handleKeychainShare)
	hub("PUT /api/profiles/{id}/keychain/generator", a.handleKeychainGenerator)
	hub("GET /api/profiles/{id}/identity", a.handleIdentityGet)
	hub("POST /api/profiles/{id}/identity/regenerate", a.handleIdentityRegenerate)
	hub("PUT /api/profiles/{id}/identity/settings", a.handleIdentitySettings)
	hub("POST /api/generate", a.handleGenerate)
	hub("POST /api/restart", a.handleRestart)

	inst("GET /api/instance/keychain", a.handleInstanceKeychain)
	inst("POST /api/instance/keychain", a.handleInstanceKeychainSave)
	inst("POST /api/instance/keychain/never", a.handleInstanceKeychainNever)
	inst("POST /api/instance/keychain/used", a.handleInstanceKeychainUsed)
	inst("POST /api/instance/keychain/import", a.handleInstanceKeychainImport)
	inst("GET /api/instance/identity", a.handleInstanceIdentity)
	inst("POST /api/instance/generate", a.handleInstanceGenerate)
}

func writeKeychainErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, keychain.ErrLocked):
		writeError(w, http.StatusLocked, "locked", "Unlock the profile's passwords first.")
	case errors.Is(err, keychain.ErrNoEntry):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, keychain.ErrUnavailable):
		writeError(w, http.StatusBadRequest, "unavailable", err.Error())
	case errors.Is(err, keychain.ErrDamaged):
		writeError(w, http.StatusConflict, "damaged", err.Error())
	default:
		writeErr(w, err)
	}
}

func (a *App) handleKeychainList(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !p.IsEncrypted() {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	entries, err := a.Keychain.List(p.ID)
	if errors.Is(err, keychain.ErrLocked) {
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "open": false})
		return
	}
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	gen, _ := a.Keychain.Generator(p.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true, "open": true, "entries": entries,
		"generator": gen, "entropy": gen.Entropy(),
		"enabled": p.Privacy.Passwords == "keycat",
	})
}

func (a *App) handleKeychainUnlock(w http.ResponseWriter, r *http.Request) {
	var cred session.Credentials
	if err := decode(r, &cred); err != nil {
		writeErr(w, err)
		return
	}
	p, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !p.IsEncrypted() {
		writeKeychainErr(w, keychain.ErrUnavailable)
		return
	}
	mk, err := cred.Unlock(p)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer mk.Destroy()
	if err := p.Security.Crypto.VerifyMAC(p.ID, mk); err != nil {
		writeErr(w, err)
		return
	}
	changed, err := a.Keychain.Attach(p, mk, a.Keychain.TempTTL)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	if changed {
		_ = a.Store.Save(p)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleKeychainLock(w http.ResponseWriter, r *http.Request) {
	a.Keychain.LockTemporary(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleKeychainSave(w http.ResponseWriter, r *http.Request) {
	var e keychain.Entry
	if err := decode(r, &e); err != nil {
		writeErr(w, err)
		return
	}
	e.ID = r.PathValue("eid")
	saved, err := a.Keychain.Save(r.PathValue("id"), e)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	a.Bus.Publish("keychain", map[string]string{"profileId": r.PathValue("id")})
	writeJSON(w, http.StatusOK, saved)
}

func (a *App) handleKeychainDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.Keychain.Delete(r.PathValue("id"), r.PathValue("eid")); err != nil {
		writeKeychainErr(w, err)
		return
	}
	a.Bus.Publish("keychain", map[string]string{"profileId": r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleKeychainPassword(w http.ResponseWriter, r *http.Request) {
	e, err := a.Keychain.Get(r.PathValue("id"), r.PathValue("eid"))
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": e.Password})
}

func (a *App) handleKeychainShare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Recipients []string `json:"recipients"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	for _, id := range req.Recipients {
		if !profile.ValidID(id) {
			writeError(w, http.StatusBadRequest, "error", "invalid profile id")
			return
		}
	}
	e, err := a.Keychain.Share(r.PathValue("id"), r.PathValue("eid"), req.Recipients)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	a.Bus.Publish("keychain", map[string]string{"profileId": r.PathValue("id")})
	writeJSON(w, http.StatusOK, e)
}

func (a *App) handleKeychainGenerator(w http.ResponseWriter, r *http.Request) {
	var o keychain.GeneratorOptions
	if err := decode(r, &o); err != nil {
		writeErr(w, err)
		return
	}
	saved, err := a.Keychain.SetGenerator(r.PathValue("id"), o)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generator": saved, "entropy": saved.Entropy()})
}

func (a *App) handleGenerate(w http.ResponseWriter, r *http.Request) {
	o := keychain.DefaultGenerator()
	if err := decode(r, &o); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"password": keychain.GeneratePassword(o), "entropy": o.Entropy()})
}

var ephemeral = struct {
	sync.Mutex
	ids map[string]identity.Identity
}{ids: map[string]identity.Identity{}}

func (a *App) identityFor(p *profile.Profile, fresh, regenerate bool) (identity.Identity, identity.Settings, error) {
	switch p.Security.Mode {
	case profile.ModeEncrypted:
		if regenerate {
			if _, err := a.Keychain.RegenerateIdentity(p.ID); err != nil {
				return identity.Identity{}, identity.Settings{}, err
			}
		}
		return a.Keychain.Identity(p.ID, fresh)
	case profile.ModeEphemeral:
		settings := identity.DefaultSettings()
		if p.PlainIdentitySettings != nil {
			settings = *p.PlainIdentitySettings
		}
		if fresh {
			return identity.Generate(settings), settings, nil
		}
		ephemeral.Lock()
		defer ephemeral.Unlock()
		id, ok := ephemeral.ids[p.ID]
		if !ok || regenerate {
			id = identity.Generate(settings)
			ephemeral.ids[p.ID] = id
		}
		return id, settings, nil
	}
	settings := identity.DefaultSettings()
	if p.PlainIdentitySettings != nil {
		settings = *p.PlainIdentitySettings
	}
	if fresh {
		return identity.Generate(settings), settings, nil
	}
	if p.PlainIdentity == nil || regenerate {
		id := identity.Generate(settings)
		p.PlainIdentity = &id
		if err := a.Store.Save(p); err != nil {
			return id, settings, err
		}
	}
	return *p.PlainIdentity, settings, nil
}

func (a *App) handleIdentityGet(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	id, settings, err := a.identityFor(p, false, false)
	if errors.Is(err, keychain.ErrLocked) {
		writeJSON(w, http.StatusOK, map[string]any{"locked": true, "mode": p.Privacy.FakeIdentity, "countries": identity.Countries()})
		return
	}
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"identity": id, "settings": settings, "mode": p.Privacy.FakeIdentity, "countries": identity.Countries(),
	})
}

func (a *App) handleIdentityRegenerate(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	id, settings, err := a.identityFor(p, false, true)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"identity": id, "settings": settings})
}

func (a *App) handleIdentitySettings(w http.ResponseWriter, r *http.Request) {
	var s identity.Settings
	if err := decode(r, &s); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Validate(); err != nil {
		writeErr(w, err)
		return
	}
	p, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if p.IsEncrypted() {
		id, err := a.Keychain.SetIdentitySettings(p.ID, s)
		if err != nil {
			writeKeychainErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"identity": id, "settings": s})
		return
	}
	p.PlainIdentitySettings = &s
	id, settings, err := a.identityFor(p, false, true)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"identity": id, "settings": settings})
}

func (a *App) instanceProfile(w http.ResponseWriter, r *http.Request) *profile.Profile {
	in := instanceOf(r)
	if in.hub {
		writeError(w, http.StatusBadRequest, "error", "not a profile")
		return nil
	}
	p, err := a.Store.Get(in.session.ProfileID)
	if err != nil {
		writeErr(w, err)
		return nil
	}
	return p
}

func (a *App) handleInstanceKeychain(w http.ResponseWriter, r *http.Request) {
	p := a.instanceProfile(w, r)
	if p == nil {
		return
	}
	if p.Privacy.Passwords != "keycat" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	origin := r.URL.Query().Get("origin")
	entries, err := a.Keychain.Match(p.ID, origin)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true, "entries": entries, "neverSave": a.Keychain.IsNeverSave(p.ID, origin),
	})
}

func (a *App) handleInstanceKeychainSave(w http.ResponseWriter, r *http.Request) {
	p := a.instanceProfile(w, r)
	if p == nil {
		return
	}
	var req struct {
		Origin   string `json:"origin"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if p.Privacy.Passwords != "keycat" {
		writeKeychainErr(w, keychain.ErrUnavailable)
		return
	}
	entry, action, err := a.Keychain.SaveFromBrowser(p.ID, req.Origin, req.Username, req.Password)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	a.Bus.Publish("keychain", map[string]string{"profileId": p.ID})
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry, "action": action})
}

func (a *App) handleInstanceKeychainNever(w http.ResponseWriter, r *http.Request) {
	p := a.instanceProfile(w, r)
	if p == nil {
		return
	}
	var req struct {
		Origin string `json:"origin"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := a.Keychain.NeverSave(p.ID, req.Origin); err != nil {
		writeKeychainErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleInstanceKeychainUsed(w http.ResponseWriter, r *http.Request) {
	p := a.instanceProfile(w, r)
	if p == nil {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	a.Keychain.MarkUsed(p.ID, req.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleInstanceKeychainImport(w http.ResponseWriter, r *http.Request) {
	p := a.instanceProfile(w, r)
	if p == nil {
		return
	}
	if p.Privacy.Passwords != "keycat" {
		writeKeychainErr(w, keychain.ErrUnavailable)
		return
	}
	var req struct {
		Logins []keychain.ImportedLogin `json:"logins"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if len(req.Logins) > 1000 {
		writeError(w, http.StatusBadRequest, "error", "send at most 1000 logins at a time")
		return
	}
	res, err := a.Keychain.ImportLogins(p.ID, req.Logins)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	a.log.Printf("profile %s: imported logins: %d new, %d already there, %d skipped",
		p.ID, res.Created, res.Unchanged, len(res.Skipped))
	a.Bus.Publish("keychain", map[string]string{"profileId": p.ID})
	writeJSON(w, http.StatusOK, res)
}

func (a *App) handleInstanceIdentity(w http.ResponseWriter, r *http.Request) {
	p := a.instanceProfile(w, r)
	if p == nil {
		return
	}
	mode := p.Privacy.FakeIdentity
	if mode == "off" || mode == "" {
		writeJSON(w, http.StatusOK, map[string]any{"mode": "off"})
		return
	}
	fresh := mode == "random" || r.URL.Query().Get("fresh") == "1"
	id, _, err := a.identityFor(p, fresh, false)
	if err != nil {
		writeKeychainErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": mode, "identity": id})
}

func (a *App) handleInstanceGenerate(w http.ResponseWriter, r *http.Request) {
	p := a.instanceProfile(w, r)
	if p == nil {
		return
	}
	o := keychain.DefaultGenerator()
	if p.IsEncrypted() {
		if g, err := a.Keychain.Generator(p.ID); err == nil {
			o = g
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"password": keychain.GeneratePassword(o)})
}

func (a *App) handleRestart(w http.ResponseWriter, r *http.Request) {
	if a.Sessions.ActiveCount() > 0 {
		writeError(w, http.StatusConflict, "busy", "Close all profiles before restarting Keycat.")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	go func() {
		a.Hub.Send(session.Command{Type: "quit"})
		for i := 0; i < 100 && a.Hub.Running(); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		cmd := exec.Command(exe, "--wait-pid", strconv.Itoa(os.Getpid()))
		if err := cmd.Start(); err != nil {
			a.log.Printf("restart: %v", err)
			return
		}
		a.shutdown()
	}()
}
