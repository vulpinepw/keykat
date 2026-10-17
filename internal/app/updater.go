package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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

const versionsURL = "https://product-details.mozilla.org/1.0/firefox_versions.json"

func (u *Updater) stagedDir() string { return u.app.Paths.CoreDir + ".new" }

func (u *Updater) Info() UpdateInfo {
	u.mu.Lock()
	info := u.info
	u.mu.Unlock()
	if c, err := u.app.Core(); err == nil {
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
	product := "firefox-" + version + "-ssl"
	url := "https://download.mozilla.org/?product=" + product + "&os=win64&lang=en-US"
	work := filepath.Join(u.app.Paths.LocalDir, "Update")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(work)
	installer := filepath.Join(work, "firefox-setup.exe")

	u.update(func(i *UpdateInfo) { i.Phase = "download"; i.Progress = 0 })
	if err := download(url, installer, func(done, total int64) {
		if total > 0 {
			u.update(func(i *UpdateInfo) { i.Progress = float64(done) / float64(total) })
		}
	}); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	u.update(func(i *UpdateInfo) { i.Phase = "verify"; i.Progress = 0 })
	if err := VerifyMozillaSignature(installer); err != nil {
		return err
	}

	u.update(func(i *UpdateInfo) { i.Phase = "extract" })
	extracted := filepath.Join(work, "extracted")
	cmd := exec.Command(installer, "/ExtractDir="+extracted)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extracting the installer failed: %w", err)
	}
	src := filepath.Join(extracted, "core")
	if _, err := firefox.DetectCore(src); err != nil {
		src = extracted
	}
	core, err := firefox.DetectCore(src)
	if err != nil {
		return errors.New("the downloaded installer did not contain Firefox")
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

func VerifyMozillaSignature(path string) error {
	script := `$s = Get-AuthenticodeSignature -LiteralPath $env:KEYCAT_VERIFY_PATH; ` +
		`[pscustomobject]@{ Status = $s.Status.ToString(); Subject = $s.SignerCertificate.Subject } | ConvertTo-Json -Compress`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "KEYCAT_VERIFY_PATH="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("could not check the installer signature: %w", err)
	}
	var res struct{ Status, Subject string }
	if err := json.Unmarshal(out, &res); err != nil {
		return fmt.Errorf("could not check the installer signature: %w", err)
	}
	if res.Status != "Valid" {
		return fmt.Errorf("the installer signature is not valid (%s)", res.Status)
	}
	if !strings.Contains(res.Subject, "O=Mozilla Corporation") {
		return fmt.Errorf("the installer is not signed by Mozilla (%s)", res.Subject)
	}
	return nil
}
