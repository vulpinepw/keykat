package vault

import (
	"crypto/hkdf"
	"crypto/sha256"
)

func SealBlob(cipherID CipherID, secret []byte, purpose string, plaintext, aad []byte) ([]byte, error) {
	s, err := newSuite(cipherID, secret, nil, purpose)
	if err != nil {
		return nil, err
	}
	return s.sealRandom(plaintext, aad), nil
}

func OpenBlob(cipherID CipherID, secret []byte, purpose string, sealed, aad []byte) ([]byte, error) {
	s, err := newSuite(cipherID, secret, nil, purpose)
	if err != nil {
		return nil, err
	}
	return s.openRandom(sealed, aad)
}

func DeriveKey(mk *SecretKey, purpose string) (*SecretKey, error) {
	raw, err := hkdf.Key(sha256.New, mk.Bytes(), nil, "keycat/v1/derive/"+purpose, KeySize)
	if err != nil {
		return nil, err
	}
	defer zero(raw)
	k := NewSecretKey(KeySize)
	copy(k.Bytes(), raw)
	return k, nil
}
