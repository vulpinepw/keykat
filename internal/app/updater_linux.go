//go:build linux

package app

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"time"

	"keycat/internal/firefox"
)

func (u *Updater) fetch(version, work string) (string, error) {
	release := "https://archive.mozilla.org/pub/firefox/releases/" + version + "/"
	name := "linux-x86_64/en-US/firefox-" + version + ".tar.xz"
	tarball := filepath.Join(work, "firefox.tar.xz")
	if err := u.download(release+name, tarball); err != nil {
		return "", err
	}
	u.update(func(i *UpdateInfo) { i.Phase = "verify"; i.Progress = 0 })
	var files [3][]byte
	for i, f := range []string{"SHA512SUMS", "SHA512SUMS.asc", "KEY"} {
		data, err := fetchSmall(release + f)
		if err != nil {
			return "", fmt.Errorf("could not get the release's %s: %w", f, err)
		}
		files[i] = data
	}
	if err := firefox.VerifyRelease(tarball, name, files[0], files[1], files[2]); err != nil {
		return "", err
	}
	u.update(func(i *UpdateInfo) { i.Phase = "extract" })
	if out, err := exec.Command("tar", "-xJf", tarball, "-C", work).CombinedOutput(); err != nil {
		return "", fmt.Errorf("unpacking Firefox failed: %v %s", err, bytes.TrimSpace(out))
	}
	return filepath.Join(work, "firefox"), nil
}

func fetchSmall(url string) ([]byte, error) {
	client := &http.Client{Timeout: time.Minute}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Keycat-Updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}
