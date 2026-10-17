package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"keycat/internal/firefox"
	"keycat/internal/profile"
	"keycat/internal/session"
	"keycat/internal/winutil"
)

type Hub struct {
	app     *App
	mu      sync.Mutex
	cmd     *exec.Cmd
	pid     int
	token   string
	cmds    chan session.Command
	exited  chan struct{}
	leaving bool
}

func (h *Hub) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cmd != nil
}

func (h *Hub) Show() error {
	h.mu.Lock()
	if h.cmd != nil && h.leaving {
		exited := h.exited
		h.mu.Unlock()
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
		}
		h.mu.Lock()
	}
	defer h.mu.Unlock()
	if h.cmd != nil {
		if h.pid != 0 {
			pid := uint32(h.pid)
			go func() {
				for i := 0; i < 20 && !winutil.FocusProcess(pid); i++ {
					time.Sleep(150 * time.Millisecond)
				}
			}()
		}
		select {
		case h.cmds <- session.Command{Type: "focus"}:
		default:
		}
		return nil
	}
	core, err := h.app.Core()
	if err != nil {
		return err
	}
	dir := h.app.Paths.Hub
	if firefox.ProfileInUse(dir) {
		return errors.New("the Keycat hub is already open in another window")
	}
	if err := writeHubPrefs(dir, h.app.Settings.Get()); err != nil {
		return err
	}
	h.token = randomHex(32)
	h.cmds = make(chan session.Command, 8)
	h.exited = make(chan struct{})
	cmd, err := core.Launch(firefox.LaunchOptions{
		ProfileDir: dir,
		Args:       []string{h.app.HubURL()},
		Headless:   h.app.Options.Headless,
		Env: []string{
			"KEYCAT_MODE=hub",
			"KEYCAT_API=" + h.app.BaseURL(),
			"KEYCAT_TOKEN=" + h.token,
			"KEYCAT_HUB_THEME=" + h.app.Settings.Get().Theme,
		},
	})
	if err != nil {
		return err
	}
	h.cmd, h.pid, h.leaving = cmd, 0, false
	h.app.log.Printf("hub window opened")
	exited := h.exited
	go func() {
		_ = cmd.Wait()
		firefox.WaitProfileReleased(dir, nil)
		h.mu.Lock()
		h.cmd, h.pid, h.token, h.leaving = nil, 0, "", false
		h.mu.Unlock()
		close(exited)
		h.app.log.Printf("hub window closed")
		h.app.checkExit()
	}()
	return nil
}

func (h *Hub) StepAside() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cmd == nil {
		return
	}
	h.leaving = true
	h.app.log.Printf("hub window making way for a profile")
	select {
	case h.cmds <- session.Command{Type: "quit"}:
	default:
	}
}

func (h *Hub) Send(c session.Command) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cmd == nil {
		return
	}
	select {
	case h.cmds <- c:
	default:
	}
}

func (h *Hub) authenticate(token string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cmd != nil && token != "" && constantTimeEqual(token, h.token)
}

func (h *Hub) hello(pid int) {
	h.mu.Lock()
	h.pid = pid
	h.mu.Unlock()
	go func() {
		for i := 0; i < 30 && !winutil.FocusProcess(uint32(pid)); i++ {
			time.Sleep(150 * time.Millisecond)
		}
	}()
}

func (h *Hub) nextCommands(timeout time.Duration, done <-chan struct{}) []session.Command {
	h.mu.Lock()
	cmds, exited := h.cmds, h.exited
	h.mu.Unlock()
	if cmds == nil {
		return nil
	}
	var out []session.Command
	select {
	case c := <-cmds:
		out = append(out, c)
	case <-time.After(timeout):
		return nil
	case <-done:
		return nil
	case <-exited:
		return nil
	}
	for {
		select {
		case c := <-cmds:
			out = append(out, c)
		default:
			return out
		}
	}
}

func writeHubPrefs(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	prefs := map[string]any{
		"browser.startup.page":                     0,
		"browser.sessionstore.resume_from_crash":   false,
		"browser.sessionstore.max_resumed_crashes": 0,
		"browser.shell.checkDefaultBrowser":        false,
		"browser.tabs.inTitlebar":                  1,

		"browser.tabs.warnOnClose":                         false,
		"browser.warnOnQuit":                               false,
		"browser.toolbars.bookmarks.visibility":            "never",
		"browser.profiles.enabled":                         false,
		"browser.aboutwelcome.enabled":                     false,
		"places.history.enabled":                           false,
		"browser.cache.disk.enable":                        false,
		"signon.rememberSignons":                           false,
		"browser.formfill.enable":                          false,
		"extensions.formautofill.addresses.enabled":        false,
		"extensions.formautofill.creditCards.enabled":      false,
		"browser.translations.enable":                      false,
		"browser.translations.automaticallyPopup":          false,
		"network.trr.mode":                                 5,
		"network.prefetch-next":                            false,
		"network.dns.disablePrefetch":                      true,
		"network.captive-portal-service.enabled":           false,
		"network.connectivity-service.enabled":             false,
		"browser.safebrowsing.malware.enabled":             false,
		"browser.safebrowsing.phishing.enabled":            false,
		"browser.safebrowsing.downloads.enabled":           false,
		"ui.key.menuAccessKeyFocuses":                      false,
		"sidebar.revamp":                                   false,
		"sidebar.verticalTabs":                             false,
		"media.autoplay.default":                           5,
		"permissions.default.geo":                          2,
		"permissions.default.desktop-notification":         2,
		"permissions.default.camera":                       2,
		"permissions.default.microphone":                   2,
		"privacy.resistFingerprinting":                     false,
		"identity.fxaccounts.enabled":                      false,
		"browser.download.useDownloadDir":                  true,
		"layout.css.prefers-color-scheme.content-override": map[string]int{"dark": 0, "light": 1, "system": 2}[s.Theme],
	}
	ps := profile.PrefSet{Set: prefs}
	return os.WriteFile(filepath.Join(dir, "user.js"),
		[]byte(ps.UserJS("Keycat hub profile: generated on every launch.")), 0o600)
}

func hubThemeID(theme string) string {
	switch t := strings.ToLower(theme); t {
	case "light", "system":
		return t
	}
	return "dark"
}
