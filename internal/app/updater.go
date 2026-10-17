package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"keycat/internal/firefox"
)

type UpdateInfo struct {
	Channel   string    `json:"channel"`
	Current   string    `json:"current"`
	Latest    string    `json:"latest,omitempty"`
	Available bool      `json:"available"`
	CheckedAt time.Time `json:"checkedAt,omitempty"`
	Staged    string    `json:"staged,omitempty"`
	Busy      bool      `json:"busy"`
	Phase     string    `json:"phase,omitempty"`
	Progress  float64   `json:"progress"`
	Error     string    `json:"error,omitempty"`
}

type Updater struct {
	app  *App
	mu   sync.Mutex
	info UpdateInfo
}

var errEngineInApp = errors.New("the browser engine is updated with the Keycat app")

const versionsURL = "https://product-details.mozilla.org/1.0/firefox_versions.json"

func (u *Updater) stagedDir() string { return u.app.Paths.CoreDir + ".new" }

func (u *Updater) Info() UpdateInfo {
	u.mu.Lock()
	info := u.info
	u.mu.Unlock()
	if u.app.Options.Host != nil {
		info.Current = u.app.Options.Engine.Version
	} else if c, err := u.app.Core(); err == nil {
		info.Current = c.Version
	}
	if info.Channel == "" {
		info.Channel = u.app.Settings.Get().UpdateChannel
	}
	if st := u.readStaged(); st != "" {
		info.Staged = st
	}
	return info
}

func (u *Updater) update(fn func(*UpdateInfo)) {
	u.mu.Lock()
	fn(&u.info)
	u.mu.Unlock()
	u.app.Bus.Publish("core", u.Info())
}

func (u *Updater) CheckInBackground() {
	time.Sleep(20 * time.Second)
	if _, err := u.Check(); err != nil {
		u.app.log.Printf("update check: %v", err)
	}
}

func (u *Updater) Check() (UpdateInfo, error) {
	if u.app.Options.Host != nil {
		return u.Info(), errEngineInApp
	}
	channel := u.app.Settings.Get().UpdateChannel
	latest, err := latestVersion(channel)
	u.update(func(i *UpdateInfo) {
		i.Channel = channel
		i.CheckedAt = time.Now()
		i.Error = ""
		if err != nil {
			i.Error = err.Error()
			return
		}
		i.Latest = latest
		i.Available = false
		if c, cerr := u.app.Core(); cerr == nil {
			i.Available = compareVersions(latest, c.Version) > 0
		}
	})
	return u.Info(), err
}

func latestVersion(channel string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", versionsURL, nil)
	req.Header.Set("User-Agent", "Keycat-Updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("version service returned %s", resp.Status)
	}
	var v map[string]string
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err != nil {
		return "", err
	}
	key := "LATEST_FIREFOX_VERSION"
	if channel == "esr" {
		key = "FIREFOX_ESR"
	}
	if v[key] == "" {
		return "", errors.New("version service did not report a version")
	}
	return v[key], nil
}

func compareVersions(a, b string) int {
	pa := strings.Split(strings.TrimSuffix(a, "esr"), ".")
	pb := strings.Split(strings.TrimSuffix(b, "esr"), ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &y)
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

func (u *Updater) Stage() error {
	if u.app.Options.Host != nil {
		return errEngineInApp
	}
	u.mu.Lock()
	if u.info.Busy {
		u.mu.Unlock()
		return errors.New("an update is already in progress")
	}
	u.info.Busy = true
	u.mu.Unlock()
	defer u.update(func(i *UpdateInfo) { i.Busy = false; i.Phase = "" })

	channel := u.app.Settings.Get().UpdateChannel
	version, err := latestVersion(channel)
	if err != nil {
		u.update(func(i *UpdateInfo) { i.Error = err.Error() })
		return err
	}
	if err := u.prepare(version, channel); err != nil {
		u.update(func(i *UpdateInfo) { i.Error = err.Error() })
		return err
	}
	u.update(func(i *UpdateInfo) { i.Error = ""; i.Available = false; i.Staged = version })
	return nil
}

func (u *Updater) InstallLatest(channel string) error {
	version, err := latestVersion(channel)
	if err != nil {
		return err
	}
	if err := u.prepare(version, channel); err != nil {
		return err
	}
	return u.ApplyStaged()
}

func (u *Updater) prepare(version, channel string) error {
	work := filepath.Join(u.app.Paths.LocalDir, "Update")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(work)
	src, err := u.fetch(version, work)
	if err != nil {
		return err
	}
	core, err := firefox.DetectCore(src)
	if err != nil {
		return errors.New("the download did not contain Firefox")
	}

	u.update(func(i *UpdateInfo) { i.Phase = "prepare" })
	staged := u.stagedDir()
	_ = os.RemoveAll(staged)
	if err := firefox.CopyCore(src, staged, nil); err != nil {
		return err
	}
	stagedCore, err := firefox.DetectCore(staged)
	if err != nil {
		return err
	}
	if err := firefox.ApplyLayer(stagedCore, u.app.Layer); err != nil {
		return err
	}
	marker, _ := json.Marshal(map[string]string{"version": core.Version, "channel": channel})
	return os.WriteFile(filepath.Join(staged, "keycat-staged.json"), marker, 0o644)
}

func (u *Updater) readStaged() string {
	data, err := os.ReadFile(filepath.Join(u.stagedDir(), "keycat-staged.json"))
	if err != nil {
		return ""
	}
	var m map[string]string
	_ = json.Unmarshal(data, &m)
	return m["version"]
}

func (u *Updater) ApplyStaged() error {
	if u.readStaged() == "" {
		_ = os.RemoveAll(u.stagedDir())
		return nil
	}
	_ = os.Remove(filepath.Join(u.stagedDir(), "keycat-staged.json"))
	if err := u.app.swapCore(u.stagedDir()); err != nil {
		return err
	}
	u.app.log.Printf("engine updated")
	return nil
}

func (u *Updater) download(url, dst string) error {
	u.update(func(i *UpdateInfo) { i.Phase = "download"; i.Progress = 0 })
	err := download(url, dst, func(done, total int64) {
		if total > 0 {
			u.update(func(i *UpdateInfo) { i.Progress = float64(done) / float64(total) })
		}
	})
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	return nil
}

func download(url, dst string, progress func(done, total int64)) error {
	client := &http.Client{Timeout: 15 * time.Minute}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Keycat-Updater")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	if !strings.HasSuffix(strings.ToLower(resp.Request.URL.Host), ".mozilla.net") &&
		!strings.HasSuffix(strings.ToLower(resp.Request.URL.Host), ".mozilla.org") {
		return fmt.Errorf("unexpected download host %s", resp.Request.URL.Host)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			progress(done, resp.ContentLength)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return f.Sync()
}
