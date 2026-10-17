package session

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"keycat/internal/profile"
	"keycat/internal/vault"
)

type Credentials struct {
	Method       string `json:"method"`
	Password     string `json:"password,omitempty"`
	RecoveryCode string `json:"recoveryCode,omitempty"`
}

func (c Credentials) Unlock(p *profile.Profile) (*vault.SecretKey, error) {
	cr := p.Security.Crypto
	if cr == nil {
		return nil, errors.New("this profile is not encrypted")
	}
	switch c.Method {
	case "", "password":
		return cr.UnlockPassword(p.ID, []byte(c.Password))
	case "recovery":
		return cr.UnlockRecovery(p.ID, c.RecoveryCode)
	case "windows":
		return cr.UnlockWindows(p.ID)
	}
	return nil, fmt.Errorf("unknown unlock method %q", c.Method)
}

type SecurityRequest struct {
	Mode          string `json:"mode"`
	Preset        string `json:"preset"`
	Cipher        string `json:"cipher"`
	KDF           string `json:"kdf"`
	Password      string `json:"password"`
	Recovery      bool   `json:"recovery"`
	WindowsUnlock bool   `json:"windowsUnlock"`
	Compression   string `json:"compression"`
	KeepBackup    bool   `json:"keepBackup"`
	KeepCache     bool   `json:"keepCache"`
}

type CreateRequest struct {
	Name       string             `json:"name"`
	Color      string             `json:"color"`
	Avatar     string             `json:"avatar"`
	Security   SecurityRequest    `json:"security"`
	Privacy    profile.Privacy    `json:"privacy"`
	Appearance profile.Appearance `json:"appearance"`
	Open       bool               `json:"open"`
	Import     bool               `json:"import"`
}

const MinPasswordLength = 8

func ValidatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordLength {
		return fmt.Errorf("the password must be at least %d characters", MinPasswordLength)
	}
	if utf8.RuneCountInString(pw) > 1024 {
		return errors.New("the password is too long")
	}
	return nil
}

func resolveCrypto(sec SecurityRequest) (vault.CipherID, string, string, error) {
	if sec.Preset != "" && sec.Preset != "custom" {
		sp, err := profile.SecurityPresetByID(sec.Preset)
		if err != nil {
			return "", "", "", err
		}
		return sp.Cipher, sp.KDF, sp.ID, nil
	}
	c := vault.CipherID(sec.Cipher)
	if !c.Valid() {
		return "", "", "", fmt.Errorf("unknown cipher %q", sec.Cipher)
	}
	if _, err := vault.KDFPreset(sec.KDF); err != nil {
		return "", "", "", err
	}
	for _, sp := range profile.SecurityPresets {
		if sp.Cipher == c && sp.KDF == sec.KDF {
			return c, sec.KDF, sp.ID, nil
		}
	}
	return c, sec.KDF, "custom", nil
}

func (m *Manager) Create(req CreateRequest) (*profile.Profile, string, *vault.SecretKey, error) {
	p := &profile.Profile{
		ID:         profile.NewID(),
		Name:       req.Name,
		Color:      req.Color,
		Avatar:     req.Avatar,
		Created:    time.Now().UTC(),
		Privacy:    req.Privacy,
		Appearance: req.Appearance,
	}
	if err := p.ValidateIdentity(); err != nil {
		return nil, "", nil, err
	}
	if err := p.Privacy.Validate(); err != nil {
		return nil, "", nil, err
	}
	if p.Appearance.Theme == "" {
		p.Appearance.Theme = "system"
	}
	if err := p.Appearance.Validate(); err != nil {
		return nil, "", nil, err
	}
	sec := req.Security
	if sec.Compression == "" {
		sec.Compression = vault.CompressionZstd
	}
	if sec.Compression != vault.CompressionZstd && sec.Compression != vault.CompressionNone {
		return nil, "", nil, errors.New("invalid compression")
	}
	p.Security = profile.Security{
		Mode: sec.Mode, Compression: sec.Compression, KeepBackup: sec.KeepBackup, KeepCache: sec.KeepCache,
	}
	p.AdaptToMode()

	var recovery string
	var mk *vault.SecretKey
	switch sec.Mode {
	case profile.ModePlain, profile.ModeEphemeral:
		p.Security.Preset = sec.Mode
	case profile.ModeEncrypted:
		cipherID, kdfName, presetID, err := resolveCrypto(sec)
		if err != nil {
			return nil, "", nil, err
		}
		if sec.Password == "" && !sec.WindowsUnlock {
			if !vault.WindowsUnlockAvailable() {
				return nil, "", nil, errors.New("an encrypted profile needs a password")
			}
			return nil, "", nil, errors.New("an encrypted profile needs a password or Windows account unlock")
		}
		if sec.Password != "" {
			if err := ValidatePassword(sec.Password); err != nil {
				return nil, "", nil, err
			}
		}
		if sec.WindowsUnlock && !vault.WindowsUnlockAvailable() {
			return nil, "", nil, errors.New("Windows account unlock is not available on this system")
		}
		p.Security.Preset = presetID
		cr := &vault.Crypto{Cipher: cipherID}
		mk = vault.NewMasterKey()
		fail := func(err error) (*profile.Profile, string, *vault.SecretKey, error) {
			mk.Destroy()
			return nil, "", nil, err
		}
		if sec.Password != "" {
			kdf, err := vault.KDFPreset(kdfName)
			if err != nil {
				return fail(err)
			}
			if err := cr.SetPassword(p.ID, mk, []byte(sec.Password), kdf); err != nil {
				return fail(err)
			}
		}
		if sec.Recovery {
			if recovery, err = cr.SetRecovery(p.ID, mk); err != nil {
				return fail(err)
			}
		}
		if sec.WindowsUnlock {
			if err := cr.SetWindows(p.ID, mk); err != nil {
				return fail(err)
			}
		}
		if err := cr.UpdateMAC(p.ID, mk); err != nil {
			return fail(err)
		}
		p.Security.Crypto = cr
	default:
		return nil, "", nil, fmt.Errorf("unknown security mode %q", sec.Mode)
	}

	if err := m.cfg.Store.Save(p); err != nil {
		mk.Destroy()
		return nil, "", nil, err
	}
	if p.IsEncrypted() && m.cfg.Keychain != nil {
		if changed, err := m.cfg.Keychain.Attach(p, mk, 0); err != nil {
			m.logf("profile %s: creating keychain: %v", p.ID, err)
		} else if changed {
			if err := m.cfg.Store.Save(p); err != nil {
				mk.Destroy()
				return nil, "", nil, err
			}
		}
		if !req.Open {
			m.cfg.Keychain.Detach(p.ID)
		}
	}
	m.logf("profile %s created (mode %s, preset %s)", p.ID, p.Security.Mode, p.Security.Preset)
	if m.cfg.Bus != nil {
		m.cfg.Bus.Publish("profiles", nil)
	}
	if !req.Open {
		mk.Destroy()
		mk = nil
	}
	return p, recovery, mk, nil
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	_, running := m.sessions[id]
	m.mu.Unlock()
	if running {
		return errors.New("close the profile before deleting it")
	}
	if err := m.claim(id); err != nil {
		return err
	}
	defer m.release(id)
	_ = vault.WipeDir(sessionDirFor(m.cfg.SessionsDir, id), nil)
	if m.cfg.Keychain != nil {
		m.cfg.Keychain.ForgetProfile(id)
	}
	if err := m.cfg.Store.Delete(id); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.status, id)
	m.mu.Unlock()
	m.logf("profile %s deleted", id)
	if m.cfg.Bus != nil {
		m.cfg.Bus.Publish("profiles", nil)
	}
	return nil
}
