package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"keycat/internal/profile"
	"keycat/internal/vault"
)

type SecurityChange struct {
	Credentials Credentials `json:"credentials"`
	Action      string      `json:"action"`
	NewPassword string      `json:"newPassword,omitempty"`
	Cipher      string      `json:"cipher,omitempty"`
	KDF         string      `json:"kdf,omitempty"`
}

func sessionDirFor(sessionsDir, id string) string { return filepath.Join(sessionsDir, id) }

func (m *Manager) ChangeSecurity(id string, ch SecurityChange) (string, error) {
	if err := m.claim(id); err != nil {
		return "", err
	}
	defer m.release(id)
	p, err := m.cfg.Store.Get(id)
	if err != nil {
		return "", err
	}
	if !p.IsEncrypted() {
		return "", errors.New("this profile is not encrypted")
	}
	mk, err := ch.Credentials.Unlock(p)
	if err != nil {
		return "", err
	}
	defer mk.Destroy()
	cr := p.Security.Crypto
	if err := cr.VerifyMAC(id, mk); err != nil {
		return "", err
	}

	reason := RescueKeys
	if ch.Action == "encryption" {
		reason = RescueReencrypt
	}
	madeRescue, err := m.createRescue(p, reason)
	if err != nil {
		return "", err
	}
	oldCrypto, err := cloneCrypto(cr)
	if err != nil {
		return "", err
	}
	oldPreset := p.Security.Preset
	undo := func(cause error) (string, error) {
		m.logf("profile %s: security change %q undone: %v", id, ch.Action, cause)
		p.Security.Crypto, p.Security.Preset = oldCrypto, oldPreset
		if err := m.cfg.Store.Save(p); err != nil {
			return "", fmt.Errorf("%v; restoring the previous keys also failed: %w", cause, err)
		}
		if ch.Action == "encryption" && madeRescue {
			rescueVault := filepath.Join(m.rescuePath(id), "vault.kcv")
			if _, err := os.Stat(rescueVault); err == nil {
				if err := m.putVaultBack(id, rescueVault); err != nil {
					return "", fmt.Errorf("%v; restoring the previous vault also failed: %w", cause, err)
				}
			}
		}
		if madeRescue {
			m.dropRescue(id)
		}
		return "", fmt.Errorf("the change could not be verified and was undone: %w", cause)
	}

	var recovery string
	switch ch.Action {
	case "password":
		if err := ValidatePassword(ch.NewPassword); err != nil {
			return "", err
		}
		kdf, err := m.kdfFor(cr, ch.KDF)
		if err != nil {
			return "", err
		}
		if err := cr.SetPassword(id, mk, []byte(ch.NewPassword), kdf); err != nil {
			return "", err
		}
	case "recovery-new":
		if recovery, err = cr.SetRecovery(id, mk); err != nil {
			return "", err
		}
	case "recovery-remove":
		cr.RemoveSlotType(vault.SlotRecovery)
	case "windows-on":
		if err := cr.SetWindows(id, mk); err != nil {
			return "", err
		}
	case "windows-off":
		cr.RemoveSlotType(vault.SlotWindows)
	case "encryption":
		if recovery, err = m.changeEncryption(p, mk, ch); err != nil {
			return undo(err)
		}
		cr = p.Security.Crypto
	default:
		return "", fmt.Errorf("unknown action %q", ch.Action)
	}
	if !cr.HasSlot(vault.SlotPassword) && !cr.HasSlot(vault.SlotWindows) {
		return undo(errors.New("a profile must keep a password or Windows account unlock"))
	}
	if err := cr.UpdateMAC(id, mk); err != nil {
		return undo(err)
	}
	if err := m.cfg.Store.Save(p); err != nil {
		return undo(err)
	}

	if err := m.verifyKeys(id, mk, keyTests(ch, recovery, cr)); err != nil {
		return undo(err)
	}
	if ch.Action == "encryption" {
		if _, err := os.Stat(m.cfg.Store.VaultPath(id)); err == nil {
			if _, err := vault.Verify(m.cfg.Store.VaultPath(id), id, mk); err != nil {
				return undo(fmt.Errorf("the re-encrypted vault does not decrypt: %w", err))
			}
		}
	}
	if madeRescue {
		m.dropRescue(id)
	} else {
		m.refreshRescueKeys(id)
	}
	m.logf("profile %s: security change %q applied and verified", id, ch.Action)
	if m.cfg.Bus != nil {
		m.cfg.Bus.Publish("profiles", nil)
	}
	return recovery, nil
}

func keyTests(ch SecurityChange, recovery string, cr *vault.Crypto) []Credentials {
	var tests []Credentials
	switch ch.Action {
	case "password":
		tests = append(tests, Credentials{Method: "password", Password: ch.NewPassword})
	case "recovery-new":
		tests = append(tests, Credentials{Method: "recovery", RecoveryCode: recovery})
	case "windows-on":
		tests = append(tests, Credentials{Method: "windows"})
	case "encryption":
		tests = append(tests, Credentials{Method: "password", Password: ch.Credentials.Password})
		if recovery != "" {
			tests = append(tests, Credentials{Method: "recovery", RecoveryCode: recovery})
		}
		if cr.HasSlot(vault.SlotWindows) {
			tests = append(tests, Credentials{Method: "windows"})
		}
	}
	slot := map[string]string{"password": vault.SlotPassword, "recovery": vault.SlotRecovery, "windows": vault.SlotWindows}[ch.Credentials.Method]
	if slot != "" && cr.HasSlot(slot) && ch.Action != "password" && ch.Action != "encryption" {
		tests = append(tests, ch.Credentials)
	}
	return tests
}

func cloneCrypto(c *vault.Crypto) (*vault.Crypto, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var out vault.Crypto
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (m *Manager) kdfFor(cr *vault.Crypto, name string) (vault.KDFParams, error) {
	if name != "" {
		return vault.KDFPreset(name)
	}
	for _, ks := range cr.Keyslots {
		if ks.Type == vault.SlotPassword && ks.KDF != nil {
			k := *ks.KDF
			k.Salt = vault.RandomBytes(32)
			return k, nil
		}
	}
	return vault.KDFPreset("argon2id-standard")
}

func (m *Manager) changeEncryption(p *profile.Profile, mk *vault.SecretKey, ch SecurityChange) (string, error) {
	if ch.Credentials.Method != "" && ch.Credentials.Method != "password" {
		return "", errors.New("changing the encryption requires the profile password")
	}
	old := p.Security.Crypto
	cipherID, kdfName, presetID, err := resolveCrypto(SecurityRequest{Preset: "custom", Cipher: ch.Cipher, KDF: ch.KDF})
	if err != nil {
		return "", err
	}
	kdf, err := vault.KDFPreset(kdfName)
	if err != nil {
		return "", err
	}
	next := &vault.Crypto{Cipher: cipherID}
	if err := next.SetPassword(p.ID, mk, []byte(ch.Credentials.Password), kdf); err != nil {
		return "", err
	}
	var recovery string
	if old.HasSlot(vault.SlotRecovery) {
		if recovery, err = next.SetRecovery(p.ID, mk); err != nil {
			return "", err
		}
	}
	if old.HasSlot(vault.SlotWindows) {
		if err := next.SetWindows(p.ID, mk); err != nil {
			return "", err
		}
	}
	if err := next.UpdateMAC(p.ID, mk); err != nil {
		return "", err
	}

	m.mu.Lock()
	_, running := m.sessions[p.ID]
	m.mu.Unlock()
	vaultPath := m.cfg.Store.VaultPath(p.ID)
	if _, statErr := os.Stat(vaultPath); !running && statErr == nil && old.Cipher != cipherID {
		scratch := filepath.Join(m.cfg.SessionsDir, p.ID+"-reencrypt")
		_ = vault.WipeDir(scratch, nil)
		if err := os.MkdirAll(scratch, 0o700); err != nil {
			return "", err
		}
		defer vault.WipeDir(scratch, nil)
		m.setStatus(p.ID, StateSealing, vault.PhaseDecrypt, 0, "", "")
		if _, err := vault.Unseal(vaultPath, scratch, p.ID, mk, m.progressFunc(p.ID, StateSealing)); err != nil {
			m.setStatus(p.ID, StateLocked, "", 0, "", "")
			return "", err
		}
		p.Security.Crypto = next
		if err := m.seal(p, scratch, mk); err != nil {
			p.Security.Crypto = old
			m.setStatus(p.ID, StateLocked, "", 0, "", "")
			return "", err
		}
		m.setStatus(p.ID, StateLocked, "", 0, "", "")
	}
	p.Security.Crypto = next
	p.Security.Preset = presetID
	return recovery, nil
}
