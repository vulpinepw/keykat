package vault

import (
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/scrypt"
)

const (
	KDFArgon2id     = "argon2id"
	KDFScrypt       = "scrypt"
	KDFPBKDF2SHA512 = "pbkdf2-sha512"
	KDFHKDFSHA256   = "hkdf-sha256"
)

type KDFParams struct {
	Algorithm string `json:"alg"`
	Salt      []byte `json:"salt"`

	Time      uint32 `json:"time,omitempty"`
	MemoryKiB uint32 `json:"memoryKiB,omitempty"`
	Threads   uint8  `json:"threads,omitempty"`

	LogN uint8  `json:"logN,omitempty"`
	R    uint32 `json:"r,omitempty"`
	P    uint32 `json:"p,omitempty"`

	Iterations uint32 `json:"iterations,omitempty"`
}

var kdfPresets = map[string]KDFParams{
	"argon2id-standard": {Algorithm: KDFArgon2id, Time: 3, MemoryKiB: 256 * 1024, Threads: 4},
	"argon2id-strong":   {Algorithm: KDFArgon2id, Time: 4, MemoryKiB: 512 * 1024, Threads: 4},
	"argon2id-paranoid": {Algorithm: KDFArgon2id, Time: 8, MemoryKiB: 1024 * 1024, Threads: 4},
	"scrypt":            {Algorithm: KDFScrypt, LogN: 17, R: 8, P: 1},
	"pbkdf2-sha512":     {Algorithm: KDFPBKDF2SHA512, Iterations: 650_000},
}

func KDFPreset(name string) (KDFParams, error) {
	p, ok := kdfPresets[name]
	if !ok {
		return KDFParams{}, fmt.Errorf("unknown KDF preset %q", name)
	}
	p.Salt = RandomBytes(32)
	return p, nil
}

func KDFPresetNames() []string {
	return []string{"argon2id-standard", "argon2id-strong", "argon2id-paranoid", "scrypt", "pbkdf2-sha512"}
}

func (p *KDFParams) Validate() error {
	if len(p.Salt) < 16 || len(p.Salt) > 64 {
		return errors.New("kdf: salt must be 16-64 bytes")
	}
	switch p.Algorithm {
	case KDFArgon2id:
		if p.Time < 1 || p.Time > 64 {
			return errors.New("kdf: argon2id time must be 1-64")
		}
		if p.MemoryKiB < 19*1024 || p.MemoryKiB > 4*1024*1024 {
			return errors.New("kdf: argon2id memory must be 19 MiB - 4 GiB")
		}
		if p.Threads < 1 || p.Threads > 64 {
			return errors.New("kdf: argon2id threads must be 1-64")
		}
	case KDFScrypt:
		if p.LogN < 15 || p.LogN > 22 {
			return errors.New("kdf: scrypt logN must be 15-22")
		}
		if p.R < 1 || p.R > 32 || p.P < 1 || p.P > 16 {
			return errors.New("kdf: scrypt r must be 1-32 and p 1-16")
		}
		if uint64(128)*uint64(p.R)*(uint64(1)<<p.LogN) > 2<<30 {
			return errors.New("kdf: scrypt parameters need more than 2 GiB")
		}
	case KDFPBKDF2SHA512:
		if p.Iterations < 210_000 || p.Iterations > 50_000_000 {
			return errors.New("kdf: pbkdf2 iterations must be 210k-50M")
		}
	case KDFHKDFSHA256:
	default:
		return fmt.Errorf("kdf: unknown algorithm %q", p.Algorithm)
	}
	return nil
}

func (p *KDFParams) Derive(secret []byte) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	switch p.Algorithm {
	case KDFArgon2id:
		return argon2.IDKey(secret, p.Salt, p.Time, p.MemoryKiB, p.Threads, KeySize), nil
	case KDFScrypt:
		return scrypt.Key(secret, p.Salt, 1<<p.LogN, int(p.R), int(p.P), KeySize)
	case KDFPBKDF2SHA512:
		return pbkdf2.Key(sha512.New, string(secret), p.Salt, int(p.Iterations), KeySize)
	case KDFHKDFSHA256:
		return hkdf.Key(sha256.New, secret, p.Salt, "keycat/v1/recovery", KeySize)
	}
	return nil, fmt.Errorf("kdf: unknown algorithm %q", p.Algorithm)
}

func (p *KDFParams) Describe() string {
	switch p.Algorithm {
	case KDFArgon2id:
		return fmt.Sprintf("Argon2id · %d MiB · %d passes", p.MemoryKiB/1024, p.Time)
	case KDFScrypt:
		return fmt.Sprintf("scrypt · N=2^%d · r=%d", p.LogN, p.R)
	case KDFPBKDF2SHA512:
		return fmt.Sprintf("PBKDF2-SHA512 · %dk iterations", p.Iterations/1000)
	case KDFHKDFSHA256:
		return "HKDF-SHA256"
	}
	return p.Algorithm
}
