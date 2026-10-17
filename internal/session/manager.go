package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"keycat/internal/events"
	"keycat/internal/firefox"
	"keycat/internal/keychain"
	"keycat/internal/platform"
	"keycat/internal/profile"
	"keycat/internal/vault"
)

type State string

const (
	StateLocked    State = "locked"
	StateUnlocking State = "unlocking"
	StateRunning   State = "running"
	StateClosing   State = "closing"
	StateSealing   State = "sealing"
	StateUnsealed  State = "unsealed"
	StateError     State = "error"
)

type Status struct {
	ProfileID string    `json:"profileId"`
	State     State     `json:"state"`
	Phase     string    `json:"phase,omitempty"`
	Progress  float64   `json:"progress"`
	PID       int       `json:"pid,omitempty"`
	Since     time.Time `json:"since"`
	Error     string    `json:"error,omitempty"`
	Notice    string    `json:"notice,omitempty"`
}

type Command struct {
	Type   string                 `json:"type"`
	Theme  string                 `json:"theme,omitempty"`
	Config *profile.BrowserConfig `json:"config,omitempty"`
	Prefs  map[string]any         `json:"prefs,omitempty"`
	Reset  []string               `json:"reset,omitempty"`
}

var (
	ErrBusy       = errors.New("this profile is busy, try again in a moment")
	ErrNotRunning = errors.New("this profile is not open")
	ErrInUse      = errors.New("the profile is still open in another browser window")
)

type Config struct {
	Store       *profile.Store
	Keychain    *keychain.Service
	SessionsDir string
	Core        func() (*firefox.Core, error)
	APIURL      func() string
	Bus         *events.Bus
	Log         *log.Logger
	Overwrite   func() bool
	Headless    bool
	Marionette  bool
}

type session struct {
	id      string
	mode    string
	dir     string
	mk      *vault.SecretKey
	token   string
	cmd     *exec.Cmd
	pid     int
	cmds    chan Command
	exited  chan struct{}
	started time.Time
}

type Manager struct {
	cfg      Config
	mu       sync.Mutex
	sessions map[string]*session
	byToken  map[string]*session
	status   map[string]*Status
	busy     map[string]bool
	wg       sync.WaitGroup
}

func New(cfg Config) *Manager {
	if cfg.Overwrite == nil {
		cfg.Overwrite = func() bool { return true }
	}
	return &Manager{
		cfg:      cfg,
		sessions: map[string]*session{},
		byToken:  map[string]*session{},
		status:   map[string]*Status{},
		busy:     map[string]bool{},
	}
}

func (m *Manager) logf(format string, args ...any) {
	if m.cfg.Log != nil {
		m.cfg.Log.Printf(format, args...)
	}
}

func (m *Manager) setStatus(id string, st State, phase string, progress float64, errMsg, notice string) {
	m.mu.Lock()
	cur := m.status[id]
	if cur == nil || cur.State != st {
		cur = &Status{ProfileID: id, Since: time.Now()}
		m.status[id] = cur
	}
	cur.State, cur.Phase, cur.Progress, cur.Error = st, phase, progress, errMsg
	if notice != "" || st == StateLocked {
		cur.Notice = notice
	}
	if s := m.sessions[id]; s != nil {
		cur.PID = s.pid
	} else {
		cur.PID = 0
	}
	snapshot := *cur
	m.mu.Unlock()
	if m.cfg.Bus != nil {
		m.cfg.Bus.Publish("status", snapshot)
	}
}

func (m *Manager) progressFunc(id string, st State) vault.Progress {
	var last time.Time
	return func(phase string, done, total int64) {
		if time.Since(last) < 120*time.Millisecond && done < total {
			return
		}
		last = time.Now()
		frac := 0.0
		if total > 0 {
			frac = float64(done) / float64(total)
			if frac > 1 {
				frac = 1
			}
		}
		m.setStatus(id, st, phase, frac, "", "")
	}
}

func (m *Manager) Status(id string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st := m.status[id]; st != nil {
		return *st
	}
	return Status{ProfileID: id, State: StateLocked}
}

func (m *Manager) Statuses() map[string]Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]Status, len(m.status))
	for id, st := range m.status {
		out[id] = *st
	}
	return out
}

func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions) + len(m.busy)
}

func (m *Manager) OpenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, st := range m.status {
		if st.State == StateRunning || st.State == StateUnlocking {
			n++
		}
	}
	return n
}

func (m *Manager) claim(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy[id] {
		return ErrBusy
	}
	m.busy[id] = true
	return nil
}

func (m *Manager) release(id string) {
	m.mu.Lock()
	delete(m.busy, id)
	m.mu.Unlock()
}

func (m *Manager) Recover() {
	entries, err := os.ReadDir(m.cfg.SessionsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(m.cfg.SessionsDir, e.Name())
		id, suffix, _ := strings.Cut(e.Name(), "-")
		p, err := m.cfg.Store.Get(id)
		switch {
		case firefox.ProfileInUse(dir):
			if err == nil && p.IsEncrypted() && suffix == "" {
				m.setStatus(id, StateUnsealed, "", 0, "",
					"This profile is still open in a browser window from a previous Keycat session. Close that window, then unlock the profile to encrypt it again.")
			}
		case err != nil || suffix != "" || !p.IsEncrypted():
			m.logf("wiping leftover session directory %s", e.Name())
			_ = vault.WipeDir(dir, cacheSkip)
		case dirEmpty(dir):
			_ = os.Remove(dir)
		default:
			m.logf("profile %s has unsealed data from an interrupted session", id)
			m.setStatus(id, StateUnsealed, "", 0, "",
				"Keycat was interrupted while this profile was open. Its latest data is still decrypted on disk. Unlock the profile to encrypt it again.")
		}
	}
	if list, err := m.cfg.Store.List(); err == nil {
		for _, p := range list {
			if _, ok := m.Rescue(p.ID); ok && m.Status(p.ID).State == StateLocked {
				m.logf("profile %s: rescue copy kept from an unfinished operation", p.ID)
				m.setStatus(p.ID, StateLocked, "", 0, "",
					"The last operation on this profile did not finish. Keycat kept a rescue copy and checks the profile when you next unlock it.")
			}
		}
	}
}

func dirEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) == 0
}

func (m *Manager) Open(id string, cred Credentials) error {
	m.mu.Lock()
	if s := m.sessions[id]; s != nil {
		st := m.status[id]
		m.mu.Unlock()
		if st != nil && st.State == StateRunning {
			m.Focus(id)
			return nil
		}
		return ErrBusy
	}
	m.mu.Unlock()

	if err := m.claim(id); err != nil {
		return err
	}
	defer m.release(id)

	p, err := m.cfg.Store.Get(id)
	if err != nil {
		return err
	}
	prev := m.Status(id)
	var mk *vault.SecretKey
	if p.IsEncrypted() {
		m.setStatus(id, StateUnlocking, "key", 0, "", "")
		start := time.Now()
		mk, err = cred.Unlock(p)
		if err == nil {
			if verr := p.Security.Crypto.VerifyMAC(p.ID, mk); verr != nil {
				mk.Destroy()
				mk, err = nil, verr
			}
		}
		if err != nil {
			if rmk, rerr := m.unlockWithRescueKeys(p, cred); rerr == nil {
				mk, err = rmk, nil
			}
		}
		if err != nil {
			m.restoreStatus(id, prev)
			return err
		}
		p.Stats.LastUnlockMs = time.Since(start).Milliseconds()
	}
	return m.start(p, mk, prev)
}

func (m *Manager) OpenWithKey(p *profile.Profile, mk *vault.SecretKey) error {
	if err := m.claim(p.ID); err != nil {
		mk.Destroy()
		return err
	}
	defer m.release(p.ID)
	return m.start(p, mk, m.Status(p.ID))
}

func (m *Manager) restoreStatus(id string, prev Status) {
	if prev.State == "" {
		prev.State = StateLocked
	}
	m.setStatus(id, prev.State, "", 0, prev.Error, prev.Notice)
}

func (m *Manager) start(p *profile.Profile, mk *vault.SecretKey, prev Status) error {
	id := p.ID
	core, err := m.cfg.Core()
	if err != nil {
		mk.Destroy()
		m.restoreStatus(id, prev)
		return err
	}
	dir, notice, reused, err := m.prepareDir(p, mk)
	if err != nil {
		mk.Destroy()
		m.restoreStatus(id, prev)
		return err
	}
	if err := writeProfileFiles(dir, p); err != nil {
		m.abandonDir(p, dir, reused)
		mk.Destroy()
		m.restoreStatus(id, prev)
		return err
	}
	if p.IsEncrypted() && m.cfg.Keychain != nil {
		if _, err := m.cfg.Keychain.Attach(p, mk, 0); err != nil {
			m.logf("profile %s: keychain unavailable: %v", id, err)
		}
	}

	m.setStatus(id, StateUnlocking, "launch", 1, "", notice)
	token := randomToken()
	cmd, err := core.Launch(firefox.LaunchOptions{
		ProfileDir: dir,
		Headless:   m.cfg.Headless,
		Marionette: m.cfg.Marionette,
		Env: []string{
			"KEYCAT_MODE=profile",
			"KEYCAT_PROFILE=" + id,
			"KEYCAT_API=" + m.cfg.APIURL(),
			"KEYCAT_TOKEN=" + token,
		},
	})
	if err != nil {
		m.abandonDir(p, dir, reused)
		mk.Destroy()
		m.restoreStatus(id, prev)
		return fmt.Errorf("could not start the browser: %w", err)
	}

	s := &session{
		id: id, mode: p.Security.Mode, dir: dir, mk: mk, token: token, cmd: cmd,
		pid: cmd.Process.Pid, cmds: make(chan Command, 8), exited: make(chan struct{}), started: time.Now(),
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.byToken[token] = s
	m.mu.Unlock()
	m.setStatus(id, StateRunning, "", 0, "", notice)
	m.logf("profile %s opened (mode %s)", id, p.Security.Mode)

	now := time.Now().UTC()
	p.LastUsed = &now
	_ = m.cfg.Store.Save(p)
	if m.cfg.Bus != nil {
		m.cfg.Bus.Publish("profiles", nil)
	}

	m.wg.Add(1)
	go m.supervise(s)
	return nil
}

func (m *Manager) prepareDir(p *profile.Profile, mk *vault.SecretKey) (dir, notice string, reused bool, err error) {
	switch p.Security.Mode {
	case profile.ModePlain:
		dir = m.cfg.Store.PlainDataDir(p.ID)
		if firefox.ProfileInUse(dir) {
			return "", "", false, ErrInUse
		}
		return dir, "", true, os.MkdirAll(dir, 0o700)

	case profile.ModeEphemeral:
		dir = filepath.Join(m.cfg.SessionsDir, p.ID+"-eph"+randomToken()[:8])
		return dir, "", false, os.MkdirAll(dir, 0o700)
	}

	dir = filepath.Join(m.cfg.SessionsDir, p.ID)
	if entries, rerr := os.ReadDir(dir); rerr == nil && len(entries) > 0 {
		if firefox.ProfileInUse(dir) {
			return "", "", false, ErrInUse
		}
		return dir, "Recovered changes from the interrupted session.", true, nil
	}
	_ = os.Remove(dir)

	notice, err = m.checkRescue(p, mk)
	if err != nil {
		return "", "", false, err
	}
	vaultPath := m.cfg.Store.VaultPath(p.ID)
	msg, rerr := vault.RecoverInterruptedSeal(vaultPath, p.ID, mk)
	if errors.Is(rerr, fs.ErrNotExist) {
		return dir, notice, false, os.MkdirAll(dir, 0o700)
	}
	if rerr != nil {
		return "", "", false, rerr
	}
	if notice == "" && msg != "" {
		notice = "Keycat " + msg + "."
	}
	if err := vault.EnsureFreeSpace(m.cfg.SessionsDir, m.unsealSpace(p)); err != nil {
		return "", "", false, fmt.Errorf("could not open the profile: %w", err)
	}
	if _, err := m.createRescue(p, RescueOpen); err != nil {
		return "", "", false, err
	}

	tmp := dir + "-unsealing"
	_ = vault.WipeDir(tmp, nil)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return "", "", false, err
	}
	m.setStatus(p.ID, StateUnlocking, vault.PhaseDecrypt, 0, "", "")
	unsealStart := time.Now()
	if _, err := vault.Unseal(vaultPath, tmp, p.ID, mk, m.progressFunc(p.ID, StateUnlocking)); err != nil {
		_ = vault.WipeDir(tmp, nil)
		hint := ""
		if _, ok := m.Backup(p.ID); ok {
			hint = ". If this keeps happening, restore the previous vault in Encryption and unlock"
		}
		return "", "", false, fmt.Errorf("could not decrypt the profile: %w%s", err, hint)
	}
	if err := vault.RenameDurable(tmp, dir); err != nil {
		_ = vault.WipeDir(tmp, nil)
		return "", "", false, err
	}
	p.Stats.LastUnsealMs = time.Since(unsealStart).Milliseconds()
	return dir, notice, false, nil
}

func (m *Manager) unsealSpace(p *profile.Profile) uint64 {
	need := p.Stats.DataBytes
	if need <= 0 {
		vaultBytes := p.Stats.VaultBytes
		if fi, err := os.Stat(m.cfg.Store.VaultPath(p.ID)); err == nil {
			vaultBytes = fi.Size()
		}
		need = 3 * vaultBytes
	}
	return uint64(need) + 256<<20
}

func sealSpace(p *profile.Profile, total int64) uint64 {
	need := float64(total)
	if p.Stats.DataBytes > 0 && p.Stats.VaultBytes > 0 && p.Stats.VaultBytes < p.Stats.DataBytes {
		need *= 1.25 * float64(p.Stats.VaultBytes) / float64(p.Stats.DataBytes)
	}
	return uint64(need) + 64<<20
}

func (m *Manager) abandonDir(p *profile.Profile, dir string, reused bool) {
	if reused {
		return
	}
	_ = vault.WipeDir(dir, cacheSkip)
}

func writeProfileFiles(dir string, p *profile.Profile) error {
	ps := profile.BuildPrefs(p)
	header := "Keycat: generated for profile \"" + p.Name + "\" on every launch.\n" +
		"Do not edit; change these settings in Keycat instead."
	if err := os.WriteFile(filepath.Join(dir, "user.js"), []byte(ps.UserJS(header)), 0o600); err != nil {
		return err
	}
	cfg, err := json.MarshalIndent(profile.NewBrowserConfig(p, ps), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, profile.BrowserConfigFile), cfg, 0o600)
}

func (m *Manager) supervise(s *session) {
	defer m.wg.Done()
	_ = s.cmd.Wait()
	firefox.WaitProfileReleased(s.dir, nil)
	close(s.exited)
	m.logf("profile %s: Firefox exited after %s", s.id, time.Since(s.started).Round(time.Second))
	m.finish(s)
}

func (m *Manager) finish(s *session) {
	id := s.id
	p, err := m.cfg.Store.Get(id)
	if err != nil {
		_ = vault.WipeDir(s.dir, cacheSkip)
		s.mk.Destroy()
		m.dropSession(s)
		return
	}
	switch s.mode {
	case profile.ModeEncrypted:
		if err := m.seal(p, s.dir, s.mk); err != nil {
			m.logf("profile %s: sealing failed: %v", id, err)
			m.mu.Lock()
			delete(m.byToken, s.token)
			s.cmd = nil
			m.mu.Unlock()
			m.setStatus(id, StateError, "", 0,
				"Keycat could not encrypt this profile: "+err.Error()+". Your data is still on disk and the previous vault is kept; retry when the problem is fixed.", "")
			return
		}
		s.mk.Destroy()
		m.dropRescue(id)
	case profile.ModeEphemeral:
		m.setStatus(id, StateSealing, "wipe", 0, "", "")
		_ = vault.WipeDir(s.dir, cacheSkip)
	}
	m.dropSession(s)
	m.setStatus(id, StateLocked, "", 0, "", "")
	if m.cfg.Bus != nil {
		m.cfg.Bus.Publish("profiles", nil)
	}
}

func (m *Manager) dropSession(s *session) {
	m.mu.Lock()
	delete(m.sessions, s.id)
	delete(m.byToken, s.token)
	m.mu.Unlock()
	if m.cfg.Keychain != nil {
		m.cfg.Keychain.Detach(s.id)
	}
}

func (m *Manager) seal(p *profile.Profile, dir string, mk *vault.SecretKey) error {
	id := p.ID
	m.setStatus(id, StateSealing, vault.PhaseEncrypt, 0, "", "")
	exclude := vaultExclude(p.Security.KeepCache)
	if total, err := vault.ScanSize(dir, exclude); err == nil {
		if err := vault.EnsureFreeSpace(m.cfg.Store.Dir(id), sealSpace(p, total)); err != nil {
			return err
		}
	}
	res, err := vault.Seal(dir, m.cfg.Store.VaultPath(id), mk, vault.SealOptions{
		ProfileID:   id,
		Cipher:      p.Security.Crypto.Cipher,
		Compression: p.Security.Compression,
		Exclude:     exclude,
		Progress:    m.progressFunc(id, StateSealing),
		KeepBackup:  p.Security.KeepBackup,
	})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	p.Stats.VaultBytes = res.VaultBytes
	p.Stats.DataBytes = res.Archive.Bytes
	p.Stats.Files = res.Archive.Files
	p.Stats.LastSealed = &now
	p.Stats.LastSealMs = res.Duration.Milliseconds()
	if err := m.cfg.Store.Save(p); err != nil {
		return err
	}
	m.setStatus(id, StateSealing, "wipe", 1, "", "")
	wiping := dir + "-wiping"
	_ = vault.WipeDir(wiping, nil)
	if err := vault.RenameDurable(dir, wiping); err == nil {
		dir = wiping
	} else {
		m.logf("profile %s: could not move the session folder aside: %v", id, err)
	}
	skip := cacheSkip
	if !m.cfg.Overwrite() {
		skip = func(string) bool { return true }
	}
	if err := vault.WipeDir(dir, skip); err != nil {
		m.logf("profile %s: wiping session directory: %v", id, err)
	}
	m.logf("profile %s sealed: %d files, %d bytes in %s", id, res.Archive.Files, res.VaultBytes, res.Duration.Round(time.Millisecond))
	return nil
}

func vaultExclude(keepCache bool) vault.ExcludeFunc {
	return func(rel string, isDir bool) bool {
		lower := strings.ToLower(rel)
		if !isDir {
			switch lower {
			case "parent.lock", "lock", ".parentlock", "user.js", strings.ToLower(profile.BrowserConfigFile):
				return true
			}
			return false
		}
		switch lower {
		case "startupcache", "shader-cache", "thumbnails", "jumplistcache", "minidumps", "crashes",
			"saved-telemetry-pings", "datareporting", "safebrowsing-updating", "offlinecache", "storage/temporary":
			return true
		case "cache2":
			return !keepCache
		}
		return false
	}
}

func cacheSkip(rel string) bool {
	top := strings.ToLower(strings.SplitN(rel, "/", 2)[0])
	return top == "cache2" || top == "startupcache" || top == "shader-cache" || top == "safebrowsing"
}

func (m *Manager) Lock(id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil || s.cmd == nil {
		return ErrNotRunning
	}
	select {
	case s.cmds <- Command{Type: "quit"}:
	default:
	}
	m.setStatus(id, StateClosing, "", 0, "", "")
	go func() {
		select {
		case <-s.exited:
		case <-time.After(25 * time.Second):
			if m.Status(id).State == StateClosing {
				m.setStatus(id, StateRunning, "", 0, "",
					"The browser did not close. It may be waiting for you to confirm closing tabs.")
			}
		}
	}()
	return nil
}

func (m *Manager) ForceClose(id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil || s.cmd == nil {
		return ErrNotRunning
	}
	if s.pid != 0 && s.pid != s.cmd.Process.Pid {
		if proc, err := os.FindProcess(s.pid); err == nil {
			_ = proc.Kill()
		}
	}
	return s.cmd.Process.Kill()
}

func (m *Manager) RetrySeal(id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil || s.cmd != nil || m.Status(id).State != StateError {
		return ErrBusy
	}
	if err := m.claim(id); err != nil {
		return err
	}
	go func() {
		defer m.release(id)
		m.finish(s)
	}()
	return nil
}

func (m *Manager) SecureLeftover(id string, cred Credentials) error {
	if st := m.Status(id); st.State != StateUnsealed {
		return errors.New("there is no unsealed data for this profile")
	}
	if err := m.claim(id); err != nil {
		return err
	}
	defer m.release(id)
	p, err := m.cfg.Store.Get(id)
	if err != nil {
		return err
	}
	dir := filepath.Join(m.cfg.SessionsDir, id)
	if firefox.ProfileInUse(dir) {
		return ErrInUse
	}
	mk, err := cred.Unlock(p)
	if err != nil {
		return err
	}
	defer mk.Destroy()
	if err := p.Security.Crypto.VerifyMAC(id, mk); err != nil {
		return err
	}
	if _, err := vault.RecoverInterruptedSeal(m.cfg.Store.VaultPath(id), id, mk); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := m.createRescue(p, RescueSecure); err != nil {
		return err
	}
	if err := m.seal(p, dir, mk); err != nil {
		m.setStatus(id, StateUnsealed, "", 0, err.Error(), "")
		return err
	}
	m.dropRescue(id)
	m.setStatus(id, StateLocked, "", 0, "", "")
	return nil
}

func (m *Manager) Focus(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil || s.pid == 0 {
		return
	}
	if !platform.RaisesWindows {
		m.Send(id, Command{Type: "focus"})
		return
	}
	go focusWithRetry(uint32(s.pid))
}

func focusWithRetry(pid uint32) {
	for i := 0; i < 40; i++ {
		if platform.FocusProcess(pid) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (m *Manager) ApplySettings(p *profile.Profile) bool {
	m.mu.Lock()
	s := m.sessions[p.ID]
	m.mu.Unlock()
	if s == nil || s.cmd == nil {
		return false
	}
	if err := writeProfileFiles(s.dir, p); err != nil {
		m.logf("profile %s: could not update its settings files: %v", p.ID, err)
	}
	ps := profile.BuildPrefs(p)
	cfg := profile.NewBrowserConfig(p, ps)
	m.Send(p.ID, Command{Type: "settings", Config: &cfg, Prefs: ps.Set, Reset: ps.Reset})
	return true
}

func (m *Manager) Send(id string, c Command) {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s != nil {
		select {
		case s.cmds <- c:
		default:
		}
	}
}

type Instance struct {
	ProfileID string
	session   *session
}

func (m *Manager) InstanceByToken(token string) (*Instance, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.byToken[token]
	if s == nil {
		return nil, false
	}
	return &Instance{ProfileID: s.id, session: s}, true
}

func (m *Manager) Hello(in *Instance, pid int) {
	m.mu.Lock()
	in.session.pid = pid
	m.mu.Unlock()
	m.setStatus(in.ProfileID, m.Status(in.ProfileID).State, "", 0, "", m.Status(in.ProfileID).Notice)
	if platform.RaisesWindows {
		go focusWithRetry(uint32(pid))
	}
}

func (m *Manager) Closing(in *Instance) {
	if m.Status(in.ProfileID).State == StateRunning {
		m.logf("profile %s: browser closing", in.ProfileID)
		m.setStatus(in.ProfileID, StateClosing, "", 0, "", "")
	}
}

func (m *Manager) NextCommands(in *Instance, timeout time.Duration, done <-chan struct{}) []Command {
	var out []Command
	select {
	case c := <-in.session.cmds:
		out = append(out, c)
	case <-time.After(timeout):
		return nil
	case <-done:
		return nil
	case <-in.session.exited:
		return nil
	}
	for {
		select {
		case c := <-in.session.cmds:
			out = append(out, c)
		default:
			return out
		}
	}
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	var ids []string
	for id, s := range m.sessions {
		if s.cmd != nil {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Lock(id)
	}
	m.wg.Wait()
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
