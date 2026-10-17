package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const KeySize = 32

type CipherID string

const (
	CipherAES256GCM         CipherID = "aes-256-gcm"
	CipherChaCha20Poly1305  CipherID = "chacha20-poly1305"
	CipherXChaCha20Poly1305 CipherID = "xchacha20-poly1305"
	CipherCascade           CipherID = "aes-256-gcm+xchacha20-poly1305"
)

func CipherIDs() []CipherID {
	return []CipherID{CipherAES256GCM, CipherXChaCha20Poly1305, CipherChaCha20Poly1305, CipherCascade}
}

func (c CipherID) layerKinds() ([]string, error) {
	switch c {
	case CipherAES256GCM:
		return []string{"aes-256-gcm"}, nil
	case CipherChaCha20Poly1305:
		return []string{"chacha20-poly1305"}, nil
	case CipherXChaCha20Poly1305:
		return []string{"xchacha20-poly1305"}, nil
	case CipherCascade:
		return []string{"aes-256-gcm", "xchacha20-poly1305"}, nil
	}
	return nil, fmt.Errorf("unknown cipher %q", string(c))
}

func (c CipherID) Valid() bool {
	_, err := c.layerKinds()
	return err == nil
}

func (c CipherID) Describe() string {
	switch c {
	case CipherAES256GCM:
		return "AES-256-GCM"
	case CipherChaCha20Poly1305:
		return "ChaCha20-Poly1305"
	case CipherXChaCha20Poly1305:
		return "XChaCha20-Poly1305"
	case CipherCascade:
		return "AES-256-GCM, then XChaCha20-Poly1305"
	}
	return string(c)
}

func newPrimitive(kind string, key []byte) (cipher.AEAD, error) {
	switch kind {
	case "aes-256-gcm":
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case "chacha20-poly1305":
		return chacha20poly1305.New(key)
	case "xchacha20-poly1305":
		return chacha20poly1305.NewX(key)
	}
	return nil, fmt.Errorf("unknown primitive %q", kind)
}

type suite struct {
	id     CipherID
	layers []cipher.AEAD
}

func newSuite(id CipherID, secret, salt []byte, purpose string) (*suite, error) {
	kinds, err := id.layerKinds()
	if err != nil {
		return nil, err
	}
	s := &suite{id: id}
	for i, kind := range kinds {
		info := fmt.Sprintf("keycat/v1/%s/%s/layer%d/%s", purpose, id, i, kind)
		key, err := hkdf.Key(sha256.New, secret, salt, info, KeySize)
		if err != nil {
			return nil, err
		}
		aead, err := newPrimitive(kind, key)
		zero(key)
		if err != nil {
			return nil, err
		}
		s.layers = append(s.layers, aead)
	}
	return s, nil
}

func (s *suite) Overhead() int {
	n := 0
	for _, l := range s.layers {
		n += l.Overhead()
	}
	return n
}

func (s *suite) sealRandom(plaintext, aad []byte) []byte {
	var nonces []byte
	data := append([]byte(nil), plaintext...)
	for _, l := range s.layers {
		nonce := RandomBytes(l.NonceSize())
		nonces = append(nonces, nonce...)
		data = l.Seal(nil, nonce, data, aad)
	}
	return append(nonces, data...)
}

func (s *suite) openRandom(sealed, aad []byte) ([]byte, error) {
	nonceLen := 0
	for _, l := range s.layers {
		nonceLen += l.NonceSize()
	}
	if len(sealed) < nonceLen+s.Overhead() {
		return nil, ErrAuth
	}
	nonces, data := sealed[:nonceLen], sealed[nonceLen:]
	offsets := make([]int, len(s.layers))
	off := 0
	for i, l := range s.layers {
		offsets[i] = off
		off += l.NonceSize()
	}
	var err error
	for i := len(s.layers) - 1; i >= 0; i-- {
		l := s.layers[i]
		nonce := nonces[offsets[i] : offsets[i]+l.NonceSize()]
		data, err = l.Open(nil, nonce, data, aad)
		if err != nil {
			return nil, ErrAuth
		}
	}
	return data, nil
}

var ErrAuth = errors.New("vault: authentication failed (wrong key or corrupted data)")

func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("vault: system random source failed: " + err.Error())
	}
	return b
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
