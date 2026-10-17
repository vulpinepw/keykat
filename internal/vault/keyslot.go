package vault

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	SlotPassword = "password"
	SlotRecovery = "recovery"
	SlotWindows  = "windows"
)

type Keyslot struct {
	ID      string     `json:"id"`
	Type    string     `json:"type"`
	KDF     *KDFParams `json:"kdf,omitempty"`
	Wrapped []byte     `json:"wrapped"`
	Created time.Time  `json:"created"`
}

type Crypto struct {
	Cipher      CipherID  `json:"cipher"`
	Keyslots    []Keyslot `json:"keyslots"`
	SharePublic []byte    `json:"sharePublic,omitempty"`
	MAC         []byte    `json:"mac,omitempty"`
}

var (
	ErrWrongPassword = errors.New("wrong password")
	ErrNoSuchSlot    = errors.New("this unlock method is not set up for the profile")
	ErrTampered      = errors.New("profile security settings were modified outside Keycat")
)

func NewMasterKey() *SecretKey {
	k := NewSecretKey(KeySize)
	copy(k.Bytes(), RandomBytes(KeySize))
	return k
}

func keyslotAAD(profileID string, ks *Keyslot) []byte {
	return []byte("keycat/v1/keyslot\x00" + profileID + "\x00" + ks.ID + "\x00" + ks.Type)
}

func (c *Crypto) slot(id string) *Keyslot {
	for i := range c.Keyslots {
		if c.Keyslots[i].ID == id {
			return &c.Keyslots[i]
		}
	}
	return nil
}

func (c *Crypto) HasSlot(slotType string) bool {
	for _, ks := range c.Keyslots {
		if ks.Type == slotType {
			return true
		}
	}
	return false
}

func (c *Crypto) RemoveSlotType(slotType string) {
	kept := c.Keyslots[:0]
	for _, ks := range c.Keyslots {
		if ks.Type != slotType {
			kept = append(kept, ks)
		}
	}
	c.Keyslots = kept
}

func (c *Crypto) putSlot(ks Keyslot) {
	if existing := c.slot(ks.ID); existing != nil {
		*existing = ks
		return
	}
	c.Keyslots = append(c.Keyslots, ks)
}

func wrapWithKEK(cipherID CipherID, kek []byte, mk *SecretKey, aad []byte) ([]byte, error) {
	s, err := newSuite(cipherID, kek, nil, "keyslot")
	if err != nil {
		return nil, err
	}
	return s.sealRandom(mk.Bytes(), aad), nil
}

func unwrapWithKEK(cipherID CipherID, kek []byte, wrapped, aad []byte) (*SecretKey, error) {
	s, err := newSuite(cipherID, kek, nil, "keyslot")
	if err != nil {
		return nil, err
	}
	raw, err := s.openRandom(wrapped, aad)
	if err != nil {
		return nil, err
	}
	defer zero(raw)
	if len(raw) != KeySize {
		return nil, ErrAuth
	}
	k := NewSecretKey(KeySize)
	copy(k.Bytes(), raw)
	return k, nil
}

func (c *Crypto) SetPassword(profileID string, mk *SecretKey, password []byte, kdf KDFParams) error {
	if len(password) == 0 {
		return errors.New("password must not be empty")
	}
	if err := kdf.Validate(); err != nil {
		return err
	}
	if kdf.Algorithm == KDFHKDFSHA256 {
		return errors.New("hkdf is not a password KDF")
	}
	kek, err := kdf.Derive(NormalizePassword(password))
	if err != nil {
		return err
	}
	defer zero(kek)
	ks := Keyslot{ID: SlotPassword, Type: SlotPassword, KDF: &kdf, Created: time.Now().UTC()}
	if ks.Wrapped, err = wrapWithKEK(c.Cipher, kek, mk, keyslotAAD(profileID, &ks)); err != nil {
		return err
	}
	c.putSlot(ks)
	return nil
}

func (c *Crypto) UnlockPassword(profileID string, password []byte) (*SecretKey, error) {
	ks := c.slot(SlotPassword)
	if ks == nil || ks.KDF == nil {
		return nil, ErrNoSuchSlot
	}
	kek, err := ks.KDF.Derive(NormalizePassword(password))
	if err != nil {
		return nil, err
	}
	defer zero(kek)
	mk, err := unwrapWithKEK(c.Cipher, kek, ks.Wrapped, keyslotAAD(profileID, ks))
	if err != nil {
		return nil, ErrWrongPassword
	}
	return mk, nil
}

func (c *Crypto) SetRecovery(profileID string, mk *SecretKey) (string, error) {
	secret := RandomBytes(20)
	defer zero(secret)
	kdf := KDFParams{Algorithm: KDFHKDFSHA256, Salt: RandomBytes(32)}
	kek, err := kdf.Derive(secret)
	if err != nil {
		return "", err
	}
	defer zero(kek)
	ks := Keyslot{ID: SlotRecovery, Type: SlotRecovery, KDF: &kdf, Created: time.Now().UTC()}
	if ks.Wrapped, err = wrapWithKEK(c.Cipher, kek, mk, keyslotAAD(profileID, &ks)); err != nil {
		return "", err
	}
	c.putSlot(ks)
	return FormatRecoveryCode(secret), nil
}

func (c *Crypto) UnlockRecovery(profileID, code string) (*SecretKey, error) {
	ks := c.slot(SlotRecovery)
	if ks == nil || ks.KDF == nil {
		return nil, ErrNoSuchSlot
	}
	secret, err := ParseRecoveryCode(code)
	if err != nil {
		return nil, ErrWrongPassword
	}
	defer zero(secret)
	kek, err := ks.KDF.Derive(secret)
	if err != nil {
		return nil, err
	}
	defer zero(kek)
	mk, err := unwrapWithKEK(c.Cipher, kek, ks.Wrapped, keyslotAAD(profileID, ks))
	if err != nil {
		return nil, ErrWrongPassword
	}
	return mk, nil
}

func (c *Crypto) SetWindows(profileID string, mk *SecretKey) error {
	ks := Keyslot{ID: SlotWindows, Type: SlotWindows, Created: time.Now().UTC()}
	wrapped, err := dpapiProtect(mk.Bytes(), keyslotAAD(profileID, &ks))
	if err != nil {
		return err
	}
	ks.Wrapped = wrapped
	c.putSlot(ks)
	return nil
}

func (c *Crypto) UnlockWindows(profileID string) (*SecretKey, error) {
	ks := c.slot(SlotWindows)
	if ks == nil {
		return nil, ErrNoSuchSlot
	}
	raw, err := dpapiUnprotect(ks.Wrapped, keyslotAAD(profileID, ks))
	if err != nil {
		return nil, fmt.Errorf("Windows could not unlock this profile: %w", err)
	}
	defer zero(raw)
	if len(raw) != KeySize {
		return nil, ErrAuth
	}
	k := NewSecretKey(KeySize)
	copy(k.Bytes(), raw)
	return k, nil
}

type macInput struct {
	Profile     string    `json:"profile"`
	Cipher      CipherID  `json:"cipher"`
	Keyslots    []Keyslot `json:"keyslots"`
	SharePublic []byte    `json:"sharePublic,omitempty"`
}

func (c *Crypto) computeMAC(profileID string, mk *SecretKey) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, mk.Bytes(), nil, "keycat/v1/manifest-mac/"+profileID, KeySize)
	if err != nil {
		return nil, err
	}
	defer zero(key)
	msg, err := json.Marshal(macInput{Profile: profileID, Cipher: c.Cipher, Keyslots: c.Keyslots, SharePublic: c.SharePublic})
	if err != nil {
		return nil, err
	}
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil), nil
}

func (c *Crypto) UpdateMAC(profileID string, mk *SecretKey) error {
	mac, err := c.computeMAC(profileID, mk)
	if err != nil {
		return err
	}
	c.MAC = mac
	return nil
}

func (c *Crypto) VerifyMAC(profileID string, mk *SecretKey) error {
	mac, err := c.computeMAC(profileID, mk)
	if err != nil {
		return err
	}
	if !hmac.Equal(mac, c.MAC) {
		return ErrTampered
	}
	return nil
}

var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func FormatRecoveryCode(secret []byte) string {
	s := recoveryEncoding.EncodeToString(secret)
	var groups []string
	for i := 0; i < len(s); i += 4 {
		end := min(i+4, len(s))
		groups = append(groups, s[i:end])
	}
	return strings.Join(groups, "-")
}

func ParseRecoveryCode(code string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == '-' || r == ' ' || r == '\t':
			return -1
		case r == '0':
			return 'O'
		case r == '1':
			return 'I'
		case r == '8':
			return 'B'
		}
		return r
	}, strings.ToUpper(code))
	secret, err := recoveryEncoding.DecodeString(clean)
	if err != nil || len(secret) != 20 {
		return nil, errors.New("malformed recovery code")
	}
	return secret, nil
}
