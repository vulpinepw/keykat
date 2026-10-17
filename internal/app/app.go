package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"keycat/internal/events"
	"keycat/internal/firefox"
	"keycat/internal/keychain"
	"keycat/internal/profile"
	"keycat/internal/session"
	"keycat/internal/winutil"
)

const Author = "tordev"

type Options struct {
	Version    string
	Headless   bool
	NoHub      bool
	Marionette bool
}

type App struct {
	Options  Options
	Paths    *Paths
	Settings *SettingsStore
	Store    *profile.Store
	Keychain *keychain.Service
	Sessions *session.Manager
	Bus      *events.Bus
	Hub      *Hub
	Updater  *Updater
	Layer    fs.FS
	HubFS    fs.FS
	log      *log.Logger

	coreMu sync.Mutex
	core   *firefox.Core

	returnPending atomic.Bool
	quitting      atomic.Bool

	server  *http.Server
	baseURL string
	token   string
	done    chan struct{}
	once    sync.Once
}

func New(opts Options, layer, hubFS fs.FS) (*App, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	logger := openLog(paths.Logs)
	store, err := profile.NewStore(paths.Profiles)
	if err != nil {
		return nil, err
	}
	kc, err := keychain.NewService(store, filepath.Join(paths.DataDir, "Shared"))
	if err != nil {
		return nil, err
	}
	a := &App{
		Options:  opts,
		Paths:    paths,
		Settings: LoadSettings(paths.DataDir),
		Store:    store,
		Keychain: kc,
		Bus:      events.NewBus(),
		Layer:    layer,
		HubFS:    hubFS,
		log:      logger,
		token:    randomHex(32),
		done:     make(chan struct{}),
	}
	a.Hub = &Hub{app: a}
	a.Updater = &Updater{app: a}
	a.Sessions = session.New(session.Config{
		Store:       store,
		Keychain:    kc,
		SessionsDir: paths.Sessions,
		Core:        a.Core,
		APIURL:      a.BaseURL,
		Bus:         a.Bus,
		Log:         logger,
		Overwrite:   func() bool { return a.Settings.Get().SecureWipe },
		Headless:    opts.Headless,
		Marionette:  opts.Marionette,
	})
	return a, nil
}

func openLog(dir string) *log.Logger {
	path := filepath.Join(dir, "keycat.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return log.New(io.Discard, "", 0)
	}
	return log.New(f, "", log.LstdFlags|log.Lmicroseconds)
}

func (a *App) BaseURL() string { return a.baseURL }

func (a *App) HubURL() string { return a.baseURL + "/#k=" + a.token }

func (a *App) Token() string { return a.token }

func (a *App) Done() <-chan struct{} { return a.done }

func (a *App) Core() (*firefox.Core, error) {
	a.coreMu.Lock()
	defer a.coreMu.Unlock()
	if a.core != nil {
		return a.core, nil
	}
	c, err := firefox.DetectCore(a.Paths.CoreDir)
	if err != nil {
		return nil, err
	}
	a.core = c
	return c, nil
}

func (a *App) resetCore() {
	a.coreMu.Lock()
	a.core = nil
	a.coreMu.Unlock()
}

type launcherInfo struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func (a *App) launcherFile() string { return filepath.Join(a.Paths.LocalDir, "launcher.json") }

func (a *App) Run() error {
	sum := sha256.Sum256([]byte(a.Paths.DataDir))
	release, first, err := winutil.SingleInstance(`Local\Keycat-` + hex.EncodeToString(sum[:8]))
	if err != nil {
		return err
	}
	defer release()
	if !first {
		return a.forwardToRunning()
	}
	a.log.Printf("Keycat %s starting (data %s, portable %v)", a.Options.Version, a.Paths.DataDir, a.Paths.Portable)

	if err := a.Updater.ApplyStaged(); err != nil {
		a.log.Printf("applying staged engine update: %v", err)
	}
	if err := a.ensureCore(); err != nil {
		return err
	}
	if err := a.ensureLayer(); err != nil {
		a.log.Printf("layer: %v", err)
		if !a.Options.NoHub {
			winutil.ShowError("Keycat", "Keycat could not prepare its browser engine:\n\n"+err.Error())
		}
		return err
	}
	if err := a.startServer(); err != nil {
		return err
	}
	a.Sessions.Recover()
	go a.watchIdle()
	if a.Settings.Get().CheckUpdates && !a.Options.NoHub {
		go a.Updater.CheckInBackground()
	}
	if !a.Options.NoHub {
		if err := a.Hub.Show(); err != nil {
			winutil.ShowError("Keycat", "Keycat could not open its window:\n\n"+err.Error())
			a.shutdown()
		}
	}
	<-a.done
	return nil
}

func (a *App) forwardToRunning() error {
	data, err := os.ReadFile(a.launcherFile())
	if err != nil {
		return errors.New("Keycat is already running")
	}
	var info launcherInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return err
	}
	winutil.AllowAnyForeground()
	req, _ := http.NewRequest("POST", info.URL+"/api/hub/show", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+info.Token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (a *App) startServer() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	a.baseURL = "http://" + ln.Addr().String()
	a.server = &http.Server{
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := a.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Printf("api server: %v", err)
		}
	}()
	info, _ := json.Marshal(launcherInfo{URL: a.baseURL, Token: a.token, PID: os.Getpid()})
	if err := profile.WriteFileAtomic(a.launcherFile(), info); err != nil {
		return err
	}
	a.log.Printf("api listening on %s", a.baseURL)
	return nil
}

func (a *App) watchIdle() {
	ch, unsubscribe := a.Bus.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-a.done:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			st, ok := ev.Data.(session.Status)
			if ev.Type != "status" || !ok {
				continue
			}
			switch st.State {
			case session.StateRunning:
				a.returnPending.Store(true)
			case session.StateClosing, session.StateSealing, session.StateError:
				a.returnToHub(st)
			case session.StateLocked:
				a.returnToHub(st)
				a.checkExit()
			}
		}
	}
}

func (a *App) returnToHub(st session.Status) {
	if a.Options.NoHub || a.Sessions.OpenCount() > 0 || !a.returnPending.Swap(false) {
		return
	}
	if a.quitting.Load() || winutil.ShuttingDown() {
		return
	}
	a.log.Printf("profile %s %s, no profile open: showing the profiles window", st.ProfileID, st.State)
	if err := a.Hub.Show(); err != nil {
		a.log.Printf("profiles window: %v", err)
	}
}

func (a *App) checkExit() {
	if a.Options.NoHub || a.Hub.Running() {
		return
	}
	time.Sleep(300 * time.Millisecond)
	if a.Hub.Running() || a.Sessions.ActiveCount() > 0 {
		return
	}
	a.log.Printf("no profiles window and no open profiles: exiting")
	a.shutdown()
}

func (a *App) Shutdown() {
	a.quitting.Store(true)
	a.Sessions.Shutdown()
	a.Hub.Send(session.Command{Type: "quit"})
	for i := 0; i < 100 && a.Hub.Running(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	a.shutdown()
}

func (a *App) shutdown() {
	a.once.Do(func() {
		if a.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = a.server.Shutdown(ctx)
			cancel()
		}
		_ = os.Remove(a.launcherFile())
		a.Keychain.Close()
		a.log.Printf("Keycat stopped")
		close(a.done)
	})
}

func (a *App) ensureCore() error {
	if _, err := a.Core(); err == nil {
		return nil
	}
	if a.Options.NoHub {
		return fmt.Errorf("%w in %s", firefox.ErrNoCore, a.Paths.CoreDir)
	}
	if sys := firefox.FindSystemFirefox(); sys != "" {
		c, _ := firefox.DetectCore(sys)
		msg := fmt.Sprintf("Keycat runs on the Firefox engine.\n\nSet it up now by copying Firefox %s from:\n%s\n\nYour installed Firefox and its profiles are not changed.", c.Version, sys)
		if winutil.AskYesNo("Keycat setup", msg) {
			if err := a.installCoreFrom(sys); err != nil {
				winutil.ShowError("Keycat setup", "Copying the engine failed:\n\n"+err.Error())
				return err
			}
			return nil
		}
	}
	msg := "Keycat runs on the Firefox engine, which is not installed yet.\n\nDownload the latest Firefox from mozilla.org (about 70 MB)? Its digital signature is checked before it is used."
	if !winutil.AskYesNo("Keycat setup", msg) {
		return firefox.ErrNoCore
	}
	if err := a.Updater.InstallLatest(a.Settings.Get().UpdateChannel); err != nil {
		winutil.ShowError("Keycat setup", "Downloading the engine failed:\n\n"+err.Error())
		return err
	}
	return nil
}

func (a *App) installCoreFrom(src string) error {
	tmp := a.Paths.CoreDir + ".new"
	_ = os.RemoveAll(tmp)
	if err := firefox.CopyCore(src, tmp, nil); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return a.swapCore(tmp)
}

func (a *App) swapCore(newDir string) error {
	old := a.Paths.CoreDir + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(a.Paths.CoreDir); err == nil {
		if err := os.Rename(a.Paths.CoreDir, old); err != nil {
			return fmt.Errorf("the current engine is in use: %w", err)
		}
	}
	if err := os.Rename(newDir, a.Paths.CoreDir); err != nil {
		_ = os.Rename(old, a.Paths.CoreDir)
		return err
	}
	_ = os.RemoveAll(old)
	a.resetCore()
	return a.ensureLayer()
}

func (a *App) ensureLayer() error {
	c, err := a.Core()
	if err != nil {
		return err
	}
	if firefox.LayerCurrent(c, a.Layer) {
		return nil
	}
	a.log.Printf("applying Keycat layer to Firefox %s (%s)", c.Version, c.BuildID)
	return firefox.ApplyLayer(c, a.Layer)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
