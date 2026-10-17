package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"

	"keycat/internal/platform"
	"keycat/internal/profile"
	"keycat/internal/session"
	"keycat/internal/vault"
)

type ctxKey int

const instanceKey ctxKey = 1

type scope int

const (
	scopeHub scope = iota
	scopeInstance
)

type instance struct {
	hub     bool
	session *session.Instance
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	hub := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, a.guard(scopeHub, h)) }
	inst := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, a.guard(scopeInstance, h)) }

	hub("GET /api/state", a.handleState)
	hub("GET /api/events", a.handleEvents)
	hub("PUT /api/settings", a.handleSettings)
	hub("POST /api/profiles", a.handleCreate)
	hub("PATCH /api/profiles/{id}", a.handleUpdate)
	hub("DELETE /api/profiles/{id}", a.handleDelete)
	hub("POST /api/profiles/{id}/open", a.handleOpen)
	hub("POST /api/profiles/{id}/lock", a.handleLock)
	hub("POST /api/profiles/{id}/force-close", a.handleForceClose)
	hub("POST /api/profiles/{id}/retry-seal", a.handleRetrySeal)
	hub("POST /api/profiles/{id}/secure", a.handleSecureLeftover)
	hub("POST /api/profiles/{id}/security", a.handleSecurity)
	hub("POST /api/profiles/{id}/verify", a.handleVerify)
	hub("POST /api/profiles/{id}/restore", a.handleRestore)
	hub("POST /api/profiles/{id}/import", a.handleImport)
	hub("POST /api/hub/show", a.handleHubShow)
	hub("POST /api/links", a.handleLinks)
	hub("POST /api/links/{id}/open", a.handleLinkOpen)
	hub("POST /api/links/{id}/dismiss", a.handleLinkDismiss)
	hub("GET /api/default-browser", a.handleDefaultBrowser)
	hub("POST /api/default-browser", a.handleMakeDefaultBrowser)
	hub("POST /api/open-folder", a.handleOpenFolder)
	hub("POST /api/core/check", a.handleCoreCheck)
	hub("POST /api/core/update", a.handleCoreUpdate)
	hub("POST /api/quit", a.handleQuit)

	inst("POST /api/instance/hello", a.handleInstanceHello)
	inst("GET /api/instance/commands", a.handleInstanceCommands)
	inst("POST /api/instance/settings", a.handleInstanceSettings)
	inst("POST /api/instance/hub", a.handleInstanceHub)
	inst("POST /api/instance/closing", a.handleInstanceClosing)

	a.keychainRoutes(hub, inst)

	mux.HandleFunc("GET /", a.handleStatic)
	return a.secureHeaders(mux)
}

func (a *App) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != strings.TrimPrefix(a.baseURL, "http://") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) guard(sc scope, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "expected application/json")
				return
			}
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		var in *instance
		if token != "" {
			if a.Hub.authenticate(token) {
				in = &instance{hub: true}
			} else if si, ok := a.Sessions.InstanceByToken(token); ok {
				in = &instance{session: si}
			}
		}
		origin := r.Header.Get("Origin")
		if origin != "" && origin != a.baseURL && !(in != nil && origin == "null") {
			writeError(w, http.StatusForbidden, "forbidden", "cross-origin request refused")
			return
		}
		switch sc {
		case scopeHub:
			if in == nil && !constantTimeEqual(token, a.token) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
				return
			}
		case scopeInstance:
			if in == nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
				return
			}
		}
		if in != nil {
			r = r.WithContext(context.WithValue(r.Context(), instanceKey, in))
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrWrongPassword):
		writeError(w, http.StatusUnauthorized, "wrong_password", "That password or recovery code is not correct.")
	case errors.Is(err, vault.ErrNoSuchSlot):
		writeError(w, http.StatusBadRequest, "no_such_slot", err.Error())
	case errors.Is(err, vault.ErrTampered):
		writeError(w, http.StatusConflict, "tampered", "This profile's security settings were changed outside Keycat. It was not opened.")
	case errors.Is(err, profile.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Profile not found.")
	case errors.Is(err, session.ErrBusy), errors.Is(err, session.ErrInUse):
		writeError(w, http.StatusConflict, "busy", err.Error())
	case errors.Is(err, session.ErrNotRunning):
		writeError(w, http.StatusConflict, "not_running", err.Error())
	default:
		writeError(w, http.StatusBadRequest, "error", err.Error())
	}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	return nil
}

func (a *App) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	data, err := fs.ReadFile(a.HubFS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	_, _ = w.Write(data)
}

type securityView struct {
	Mode        string   `json:"mode"`
	Preset      string   `json:"preset"`
	Cipher      string   `json:"cipher,omitempty"`
	CipherName  string   `json:"cipherName,omitempty"`
	KDF         string   `json:"kdf,omitempty"`
	KDFName     string   `json:"kdfName,omitempty"`
	Unlock      []string `json:"unlock"`
	Compression string   `json:"compression"`
	KeepBackup  bool     `json:"keepBackup"`
	KeepCache   bool     `json:"keepCache"`
}

type profileView struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	Color      string                `json:"color"`
	Avatar     string                `json:"avatar"`
	Created    time.Time             `json:"created"`
	LastUsed   *time.Time            `json:"lastUsed,omitempty"`
	Security   securityView          `json:"security"`
	Privacy    profile.Privacy       `json:"privacy"`
	Appearance profile.Appearance    `json:"appearance"`
	Stats      profile.Stats         `json:"stats"`
	Summary    []profile.SummaryItem `json:"summary"`
	Status     session.Status        `json:"status"`
	Rescue     *session.RescueInfo   `json:"rescue"`
	Backup     *session.BackupInfo   `json:"backup"`
}

func (a *App) view(p *profile.Profile) profileView {
	sv := securityView{
		Mode: p.Security.Mode, Preset: p.Security.Preset, Unlock: []string{},
		Compression: p.Security.Compression, KeepBackup: p.Security.KeepBackup, KeepCache: p.Security.KeepCache,
	}
	if c := p.Security.Crypto; c != nil {
		sv.Cipher, sv.CipherName = string(c.Cipher), c.Cipher.Describe()
		for _, ks := range c.Keyslots {
			sv.Unlock = append(sv.Unlock, ks.Type)
			if ks.Type == vault.SlotPassword && ks.KDF != nil {
				sv.KDFName = ks.KDF.Describe()
				sv.KDF = kdfPresetName(ks.KDF)
			}
		}
	}
	v := profileView{
		ID: p.ID, Name: p.Name, Color: p.Color, Avatar: p.Avatar, Created: p.Created, LastUsed: p.LastUsed,
		Security: sv, Privacy: p.Privacy, Appearance: p.Appearance, Stats: p.Stats,
		Summary: profile.Summary(p), Status: a.Sessions.Status(p.ID),
	}
	if p.IsEncrypted() {
		v.Rescue, _ = a.Sessions.Rescue(p.ID)
		v.Backup, _ = a.Sessions.Backup(p.ID)
	}
	return v
}

func kdfPresetName(k *vault.KDFParams) string {
	for _, name := range vault.KDFPresetNames() {
		pr, _ := vault.KDFPreset(name)
		if pr.Algorithm == k.Algorithm && pr.Time == k.Time && pr.MemoryKiB == k.MemoryKiB &&
			pr.LogN == k.LogN && pr.Iterations == k.Iterations {
			return name
		}
	}
	return "custom"
}

type option struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (a *App) presets() map[string]any {
	privacy := map[string]profile.Privacy{}
	for _, name := range append(profile.PrivacyPresetNames, "custom") {
		privacy[name], _ = profile.PrivacyPreset(name)
	}
	var ciphers []option
	for _, c := range vault.CipherIDs() {
		ciphers = append(ciphers, option{ID: string(c), Name: c.Describe()})
	}
	var kdfs []option
	for _, name := range vault.KDFPresetNames() {
		k, _ := vault.KDFPreset(name)
		kdfs = append(kdfs, option{ID: name, Name: k.Describe()})
	}
	var engines []profile.SearchEngine
	for _, name := range profile.SearchEngineOrder {
		engines = append(engines, profile.SearchEngines[name])
	}
	return map[string]any{
		"security": profile.SecurityPresets,
		"privacy":  privacy,
		"ciphers":  ciphers,
		"kdfs":     kdfs,
		"dns":      profile.DoHProviders(),
		"search":   engines,
		"palette":  profile.Palette,
		"avatars":  profile.AvatarDrawings,
	}
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	profiles, err := a.Store.List()
	if err != nil {
		writeErr(w, err)
		return
	}
	views := make([]profileView, 0, len(profiles))
	for _, p := range profiles {
		views = append(views, a.view(p))
	}
	core := map[string]any{"installed": false}
	if c, err := a.Core(); err == nil {
		core = map[string]any{"installed": true, "version": c.Version, "buildId": c.BuildID, "channel": c.Channel, "dir": c.Dir}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app": map[string]any{
			"version":       a.Options.Version,
			"author":        Author,
			"platform":      runtime.GOOS,
			"portable":      a.Paths.Portable,
			"dataDir":       a.Paths.DataDir,
			"sessionsDir":   a.Paths.Sessions,
			"windowsUnlock": vault.WindowsUnlockAvailable(),
		},
		"settings": a.Settings.Get(),
		"core":     core,
		"update":   a.Updater.Info(),
		"profiles": views,
		"presets":  a.presets(),
		"links":    a.links.list(),
	})
}

func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "error", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	ch, unsubscribe := a.Bus.Subscribe()
	defer unsubscribe()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-a.done:
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	s := a.Settings.Get()
	if err := decode(r, &s); err != nil {
		writeErr(w, err)
		return
	}
	prevTheme := a.Settings.Get().Theme
	if err := a.Settings.Set(s); err != nil {
		writeErr(w, err)
		return
	}
	if s.Theme != prevTheme {
		a.Hub.Send(session.Command{Type: "theme", Theme: hubThemeID(s.Theme)})
	}
	a.Bus.Publish("settings", s)
	writeJSON(w, http.StatusOK, s)
}

func (a *App) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req session.CreateRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	p, recovery, mk, err := a.Sessions.Create(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.Open {
		var openErr error
		if p.IsEncrypted() {
			openErr = a.Sessions.OpenWithKey(p, mk)
		} else {
			openErr = a.Sessions.Open(p.ID, session.Credentials{})
		}
		if openErr != nil {
			a.log.Printf("open after create: %v", openErr)
			a.Keychain.Detach(p.ID)
		} else {
			if req.Import {
				a.Sessions.Send(p.ID, session.Command{Type: "import"})
			}
			a.afterOpen()
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": a.view(p), "recoveryCode": recovery})
}

type updateRequest struct {
	Name       *string             `json:"name"`
	Color      *string             `json:"color"`
	Avatar     *string             `json:"avatar"`
	Privacy    *profile.Privacy    `json:"privacy"`
	Appearance *profile.Appearance `json:"appearance"`
	KeepBackup *bool               `json:"keepBackup"`
	KeepCache  *bool               `json:"keepCache"`
}

func (a *App) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	p, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	themeChanged := false
	before := p.Privacy
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Color != nil {
		p.Color = *req.Color
	}
	if req.Avatar != nil {
		p.Avatar = *req.Avatar
	}
	if err := p.ValidateIdentity(); err != nil {
		writeErr(w, err)
		return
	}
	if req.Privacy != nil {
		if err := req.Privacy.Validate(); err != nil {
			writeErr(w, err)
			return
		}
		p.Privacy = *req.Privacy
		p.AdaptToMode()
	}
	if req.Appearance != nil {
		if err := req.Appearance.Validate(); err != nil {
			writeErr(w, err)
			return
		}
		themeChanged = req.Appearance.Theme != p.Appearance.Theme
		p.Appearance = *req.Appearance
	}
	if req.KeepBackup != nil {
		p.Security.KeepBackup = *req.KeepBackup
	}
	if req.KeepCache != nil {
		p.Security.KeepCache = *req.KeepCache
	}
	if err := a.Store.Save(p); err != nil {
		writeErr(w, err)
		return
	}
	running := a.Sessions.ApplySettings(p)
	if running && themeChanged {
		a.Sessions.Send(p.ID, session.Command{Type: "theme", Theme: p.Appearance.Theme})
	}
	a.Bus.Publish("profiles", nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":         a.view(p),
		"restartRequired": running && needsRestart(before, p.Privacy),
	})
}

func needsRestart(before, after profile.Privacy) bool {
	identityOn := func(pv profile.Privacy) bool { return pv.FakeIdentity != "off" && pv.FakeIdentity != "" }
	return before.AlwaysPrivate != after.AlwaysPrivate ||
		before.Passwords != after.Passwords ||
		identityOn(before) != identityOn(after) ||
		before.Containers != after.Containers
}

func (a *App) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	p, err := a.Store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.Confirm != p.Name {
		writeError(w, http.StatusBadRequest, "confirm", "Type the profile name to confirm deletion.")
		return
	}
	if err := a.Sessions.Delete(p.ID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": p.ID})
}

func (a *App) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		session.Credentials
		Then string `json:"then,omitempty"`
		Link string `json:"link,omitempty"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	id := r.PathValue("id")
	wasRunning := a.Sessions.Status(id).State == session.StateRunning
	if err := a.Sessions.Open(id, req.Credentials); err != nil {
		writeErr(w, err)
		return
	}
	if req.Then == "import" {
		a.Sessions.Send(id, session.Command{Type: "import"})
	}
	if l, ok := a.links.take(req.Link); ok {
		a.Sessions.Send(id, session.Command{Type: "open", URL: l.URL})
		a.Bus.Publish("links", a.links.list())
	}
	if !wasRunning {
		a.afterOpen()
	}
	writeJSON(w, http.StatusOK, a.Sessions.Status(id))
}

func (a *App) afterOpen() {
	if !a.Hub.Running() {
		return
	}
	switch a.Settings.Get().AfterOpen {
	case "minimize":
		a.Hub.Send(session.Command{Type: "minimize"})
	case "close":
		a.Hub.StepAside()
	}
}

func (a *App) handleLock(w http.ResponseWriter, r *http.Request) {
	if err := a.Sessions.Lock(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleForceClose(w http.ResponseWriter, r *http.Request) {
	if err := a.Sessions.ForceClose(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleRetrySeal(w http.ResponseWriter, r *http.Request) {
	if err := a.Sessions.RetrySeal(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleSecureLeftover(w http.ResponseWriter, r *http.Request) {
	var cred session.Credentials
	if err := decode(r, &cred); err != nil {
		writeErr(w, err)
		return
	}
	if err := a.Sessions.SecureLeftover(r.PathValue("id"), cred); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleSecurity(w http.ResponseWriter, r *http.Request) {
	var ch session.SecurityChange
	if err := decode(r, &ch); err != nil {
		writeErr(w, err)
		return
	}
	code, err := a.Sessions.ChangeSecurity(r.PathValue("id"), ch)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, _ := a.Store.Get(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"profile": a.view(p), "recoveryCode": code})
}

func (a *App) handleVerify(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "error", "This profile is not encrypted.")
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
	start := time.Now()
	st, err := vault.Verify(a.Store.VaultPath(p.ID), p.ID, mk)
	if errors.Is(err, fs.ErrNotExist) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "empty": true})
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "corrupt", "The vault failed verification: "+err.Error())
		return
	}
	a.Sessions.VerifiedIntact(p.ID)
	a.Bus.Publish("profiles", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "files": st.Files, "bytes": st.Bytes, "ms": time.Since(start).Milliseconds()})
}

func (a *App) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Credentials session.Credentials `json:"credentials"`
		Source      string              `json:"source"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	msg, err := a.Sessions.Restore(r.PathValue("id"), req.Source, req.Credentials)
	if err != nil {
		writeErr(w, err)
		return
	}
	a.Bus.Publish("profiles", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (a *App) handleImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if a.Sessions.Status(id).State != session.StateRunning {
		writeError(w, http.StatusConflict, "not_open", "Open the profile first.")
		return
	}
	a.Sessions.Send(id, session.Command{Type: "import"})
	a.Sessions.Focus(id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleHubShow(w http.ResponseWriter, r *http.Request) {
	if err := a.Hub.Show(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string `json:"target"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	dirs := map[string]string{"data": a.Paths.DataDir, "sessions": a.Paths.Sessions, "logs": a.Paths.Logs, "core": a.Paths.CoreDir}
	dir, ok := dirs[req.Target]
	if !ok {
		writeError(w, http.StatusBadRequest, "error", "unknown folder")
		return
	}
	if err := platform.OpenFolder(dir, false); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleCoreCheck(w http.ResponseWriter, r *http.Request) {
	info, err := a.Updater.Check()
	if err != nil {
		writeError(w, http.StatusBadGateway, "update_check", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *App) handleCoreUpdate(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := a.Updater.Stage(); err != nil {
			a.log.Printf("engine update: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, a.Updater.Info())
}

func (a *App) handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	go a.Shutdown()
}

func instanceOf(r *http.Request) *instance {
	in, _ := r.Context().Value(instanceKey).(*instance)
	return in
}

func (a *App) handleInstanceHello(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PID     int    `json:"pid"`
		Version string `json:"version"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	in := instanceOf(r)
	if in.hub {
		a.Hub.hello(req.PID)
		writeJSON(w, http.StatusOK, map[string]any{"mode": "hub", "theme": hubThemeID(a.Settings.Get().Theme)})
		return
	}
	a.Sessions.Hello(in.session, req.PID)
	writeJSON(w, http.StatusOK, map[string]any{"mode": "profile", "profile": in.session.ProfileID})
}

func (a *App) handleInstanceCommands(w http.ResponseWriter, r *http.Request) {
	in := instanceOf(r)
	var cmds []session.Command
	if in.hub {
		cmds = a.Hub.nextCommands(25*time.Second, r.Context().Done())
	} else {
		cmds = a.Sessions.NextCommands(in.session, 25*time.Second, r.Context().Done())
	}
	if cmds == nil {
		cmds = []session.Command{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"commands": cmds})
}

func (a *App) handleInstanceSettings(w http.ResponseWriter, r *http.Request) {
	in := instanceOf(r)
	if in.hub {
		writeError(w, http.StatusBadRequest, "error", "not a profile")
		return
	}
	var req struct {
		Theme   string         `json:"theme"`
		Privacy map[string]any `json:"privacy"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	p, err := a.Store.Get(in.session.ProfileID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.Theme != "" {
		ap := profile.Appearance{Theme: req.Theme}
		if err := ap.Validate(); err != nil {
			writeErr(w, err)
			return
		}
		p.Appearance = ap
	}
	if len(req.Privacy) > 0 {
		merged, err := mergePrivacy(p.Privacy, req.Privacy)
		if err != nil {
			writeErr(w, err)
			return
		}
		p.Privacy = merged
		p.AdaptToMode()
	}
	if err := a.Store.Save(p); err != nil {
		writeErr(w, err)
		return
	}
	a.Sessions.ApplySettings(p)
	a.Bus.Publish("profiles", nil)
	writeJSON(w, http.StatusOK, map[string]any{"summary": profile.Summary(p), "privacy": p.Privacy})
}

func mergePrivacy(base profile.Privacy, patch map[string]any) (profile.Privacy, error) {
	raw, _ := json.Marshal(base)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for k, v := range patch {
		if _, ok := m[k]; !ok || k == "preset" {
			return base, fmt.Errorf("unknown privacy option %q", k)
		}
		m[k] = v
	}
	m["preset"] = "custom"
	raw, _ = json.Marshal(m)
	var out profile.Privacy
	if err := json.Unmarshal(raw, &out); err != nil {
		return base, err
	}
	return out, out.Validate()
}

func (a *App) handleInstanceClosing(w http.ResponseWriter, r *http.Request) {
	if in := instanceOf(r); !in.hub {
		a.Sessions.Closing(in.session)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleInstanceHub(w http.ResponseWriter, r *http.Request) {
	if err := a.Hub.Show(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
