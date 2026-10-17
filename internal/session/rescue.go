package session

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"keycat/internal/profile"
	"keycat/internal/vault"
)

const rescueDirName = "rescue"

const (
	RescueOpen      = "open"
	RescueSecure    = "secure"
	RescueReencrypt = "reencrypt"
	RescueKeys      = "keys"
)

type RescueInfo struct {
	Reason     string    `json:"reason"`
	Created    time.Time `json:"created"`
	VaultBytes int64     `json:"vaultBytes,omitempty"`
}

func (m *Manager) rescuePath(id string) string {
	return filepath.Join(m.cfg.Store.Dir(id), rescueDirName)
}

func (m *Manager) Rescue(id string) (*RescueInfo, bool) {
	data, err := os.ReadFile(filepath.Join(m.rescuePath(id), "rescue.json"))
	if err != nil {
		return nil, false
	}
	var info RescueInfo
	if json.Unmarshal(data, &info) != nil {
		return nil, false
	}
	return &info, true
}

func (m *Manager) createRescue(p *profile.Profile, reason string) (bool, error) {
	dir := m.rescuePath(p.ID)
	if _, ok := m.Rescue(p.ID); ok {
		return false, nil
	}
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return false, err
	}
	fail := func(err error) (bool, error) {
		_ = os.RemoveAll(tmp)
		return false, fmt.Errorf("could not make the rescue copy: %w", err)
	}
	if err := vault.CopyFile(m.cfg.Store.ManifestPath(p.ID), filepath.Join(tmp, "profile.json")); err != nil {
		return fail(err)
	}
	info := RescueInfo{Reason: reason, Created: time.Now().UTC()}
	vaultPath := m.cfg.Store.VaultPath(p.ID)
	if fi, err := os.Stat(vaultPath); err == nil {
		if err := vault.LinkOrCopy(vaultPath, filepath.Join(tmp, "vault.kcv")); err != nil {
			return fail(err)
		}
		info.VaultBytes = fi.Size()
	}
	data, err := json.Marshal(info)
	if err != nil {
		return fail(err)
	}
	if err := vault.WriteFileSynced(filepath.Join(tmp, "rescue.json"), data); err != nil {
		return fail(err)
	}
	if err := vault.RenameDurable(tmp, dir); err != nil {
		return fail(err)
	}
	m.logf("profile %s: rescue copy made (%s)", p.ID, reason)
	return true, nil
}

func (m *Manager) dropRescue(id string) {
	dir := m.rescuePath(id)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		m.logf("profile %s: could not delete the rescue copy: %v", id, err)
		return
	}
	m.logf("profile %s: rescue copy deleted, the profile is safely encrypted", id)
}

func (m *Manager) refreshRescueKeys(id string) {
	if _, ok := m.Rescue(id); !ok {
		return
	}
	dst := filepath.Join(m.rescuePath(id), "profile.json")
	tmp := dst + ".tmp"
	if err := vault.CopyFile(m.cfg.Store.ManifestPath(id), tmp); err != nil {
		m.logf("profile %s: could not update the rescue copy: %v", id, err)
		return
	}
	if err := vault.RenameDurable(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		m.logf("profile %s: could not update the rescue copy: %v", id, err)
	}
}

func (m *Manager) rescueManifest(id string) (*profile.Profile, error) {
	return profile.ReadManifest(filepath.Join(m.rescuePath(id), "profile.json"), id)
}

func (m *Manager) unlockWithRescueKeys(p *profile.Profile, cred Credentials) (*vault.SecretKey, error) {
	rp, err := m.rescueManifest(p.ID)
	if err != nil || rp.Security.Crypto == nil {
		return nil, errors.New("no rescue keys")
	}
	mk, err := cred.Unlock(rp)
	if err != nil {
		return nil, err
	}
	if err := rp.Security.Crypto.VerifyMAC(p.ID, mk); err != nil {
		mk.Destroy()
		return nil, err
	}
	p.Security.Crypto = rp.Security.Crypto
	p.Security.Preset = rp.Security.Preset
	if err := m.cfg.Store.Save(p); err != nil {
		mk.Destroy()
		return nil, err
	}
	m.logf("profile %s: unlock methods restored from the rescue copy", p.ID)
	return mk, nil
}

func (m *Manager) checkRescue(p *profile.Profile, mk *vault.SecretKey) (string, error) {
	info, ok := m.Rescue(p.ID)
	if !ok {
		return "", nil
	}
	vaultPath := m.cfg.Store.VaultPath(p.ID)
	if _, err := vault.RecoverInterruptedSeal(vaultPath, p.ID, mk); err != nil && !errors.Is(err, fs.ErrNotExist) {
		m.logf("profile %s: repairing an interrupted seal: %v", p.ID, err)
	}
	rescueVault := filepath.Join(m.rescuePath(p.ID), "vault.kcv")
	_, rescueErr := os.Stat(rescueVault)
	_, vaultErr := os.Stat(vaultPath)
	if errors.Is(vaultErr, fs.ErrNotExist) && errors.Is(rescueErr, fs.ErrNotExist) {
		m.dropRescue(p.ID)
		return "", nil
	}
	m.setStatus(p.ID, StateUnlocking, vault.PhaseVerify, 0, "", "")
	var verr error
	if vaultErr == nil {
		_, verr = vault.VerifyProgress(vaultPath, p.ID, mk, m.progressFunc(p.ID, StateUnlocking))
		if verr == nil {
			m.dropRescue(p.ID)
			return "", nil
		}
	} else {
		verr = vaultErr
	}
	m.logf("profile %s: vault check after an interrupted operation failed: %v", p.ID, verr)
	if rescueErr != nil {
		return "", fmt.Errorf("the vault is damaged (%v) and its rescue copy is missing", verr)
	}
	if _, err := vault.Verify(rescueVault, p.ID, mk); err != nil {
		return "", fmt.Errorf("the vault is damaged (%v) and so is its rescue copy (%v)", verr, err)
	}
	if err := m.putVaultBack(p.ID, rescueVault); err != nil {
		return "", err
	}
	return fmt.Sprintf("Keycat restored this profile from its automatic rescue copy of %s, because the last operation on it did not finish.",
		info.Created.Local().Format("2 Jan 2006 15:04")), nil
}

func (m *Manager) putVaultBack(id, src string) error {
	vaultPath := m.cfg.Store.VaultPath(id)
	if _, err := os.Stat(vaultPath); err == nil {
		_ = os.Remove(vaultPath + ".damaged")
		if err := vault.RenameDurable(vaultPath, vaultPath+".damaged"); err != nil {
			return err
		}
	}
	tmp := vaultPath + ".restore"
	_ = os.Remove(tmp)
	if err := vault.LinkOrCopy(src, tmp); err != nil {
		return err
	}
	if err := vault.RenameDurable(tmp, vaultPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	m.logf("profile %s: vault restored from %s", id, filepath.Base(filepath.Dir(src)))
	return nil
}

func (m *Manager) VerifiedIntact(id string) {
	m.mu.Lock()
	_, running := m.sessions[id]
	m.mu.Unlock()
	if running || m.Status(id).State == StateUnsealed {
		return
	}
	if _, ok := m.Rescue(id); ok {
		m.dropRescue(id)
		m.setStatus(id, StateLocked, "", 0, "", "")
	}
}

func sameKey(a, b *vault.SecretKey) bool {
	return subtle.ConstantTimeCompare(a.Bytes(), b.Bytes()) == 1
}

type BackupInfo struct {
	Bytes    int64     `json:"bytes"`
	Modified time.Time `json:"modified"`
}

func (m *Manager) Backup(id string) (*BackupInfo, bool) {
	fi, err := os.Stat(m.cfg.Store.VaultPath(id) + ".bak")
	if err != nil {
		return nil, false
	}
	return &BackupInfo{Bytes: fi.Size(), Modified: fi.ModTime().UTC()}, true
}

func (m *Manager) Restore(id, source string, cred Credentials) (string, error) {
	if err := m.claim(id); err != nil {
		return "", err
	}
	defer m.release(id)
	m.mu.Lock()
	_, running := m.sessions[id]
	m.mu.Unlock()
	if running || m.Status(id).State == StateUnsealed {
		return "", errors.New("close the profile and secure its data before restoring a copy")
	}
	p, err := m.cfg.Store.Get(id)
	if err != nil {
		return "", err
	}
	if !p.IsEncrypted() {
		return "", errors.New("this profile is not encrypted")
	}
	mk, err := cred.Unlock(p)
	if err != nil {
		return "", err
	}
	defer mk.Destroy()
	if err := p.Security.Crypto.VerifyMAC(id, mk); err != nil {
		return "", err
	}
	vaultPath := m.cfg.Store.VaultPath(id)
	var src string
	switch source {
	case "backup":
		src = vaultPath + ".bak"
	case "rescue":
		src = filepath.Join(m.rescuePath(id), "vault.kcv")
	default:
		return "", fmt.Errorf("unknown copy %q", source)
	}
	if _, err := os.Stat(src); err != nil {
		return "", errors.New("that copy does not exist")
	}
	m.setStatus(id, StateSealing, vault.PhaseVerify, 0, "", "")
	defer m.setStatus(id, StateLocked, "", 0, "", "")
	if _, err := vault.VerifyProgress(src, id, mk, m.progressFunc(id, StateSealing)); err != nil {
		return "", fmt.Errorf("that copy is damaged and was not used: %w", err)
	}
	swap := vaultPath + ".swap"
	_ = os.Remove(swap)
	if err := vault.LinkOrCopy(src, swap); err != nil {
		return "", err
	}
	if _, err := os.Stat(vaultPath); err == nil {
		if err := vault.RenameDurable(vaultPath, vaultPath+".bak"); err != nil {
			_ = os.Remove(swap)
			return "", err
		}
	}
	if err := vault.RenameDurable(swap, vaultPath); err != nil {
		return "", err
	}
	if source == "rescue" {
		m.dropRescue(id)
	}
	m.logf("profile %s: vault restored from its %s", id, source)
	return "The copy was verified and restored. The vault it replaced is kept as the previous vault.", nil
}

func (m *Manager) verifyKeys(id string, mk *vault.SecretKey, tests []Credentials) error {
	p, err := m.cfg.Store.Get(id)
	if err != nil {
		return err
	}
	if err := p.Security.Crypto.VerifyMAC(id, mk); err != nil {
		return fmt.Errorf("the saved keys do not authenticate: %w", err)
	}
	for _, c := range tests {
		got, err := c.Unlock(p)
		if err != nil {
			return fmt.Errorf("the new %s does not unlock the profile: %w", c.Method, err)
		}
		ok := sameKey(got, mk)
		got.Destroy()
		if !ok {
			return fmt.Errorf("the new %s unlocks a different key", c.Method)
		}
	}
	return nil
}
