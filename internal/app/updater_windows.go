//go:build windows

package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"keycat/internal/firefox"
)

func (u *Updater) fetch(version, work string) (string, error) {
	installer := filepath.Join(work, "firefox-setup.exe")
	if err := u.download("https://download.mozilla.org/?product=firefox-"+version+"-ssl&os=win64&lang=en-US", installer); err != nil {
		return "", err
	}
	u.update(func(i *UpdateInfo) { i.Phase = "verify"; i.Progress = 0 })
	if err := VerifyMozillaSignature(installer); err != nil {
		return "", err
	}
	u.update(func(i *UpdateInfo) { i.Phase = "extract" })
	extracted := filepath.Join(work, "extracted")
	cmd := exec.Command(installer, "/ExtractDir="+extracted)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("extracting the installer failed: %w", err)
	}
	if _, err := firefox.DetectCore(filepath.Join(extracted, "core")); err == nil {
		return filepath.Join(extracted, "core"), nil
	}
	return extracted, nil
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
