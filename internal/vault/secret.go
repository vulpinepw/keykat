package vault

import "sync"

type SecretKey struct {
	mu   sync.Mutex
	b    []byte
	free func()
}

func NewSecretKey(n int) *SecretKey {
	b, free := allocLocked(n)
	return &SecretKey{b: b, free: free}
}

func (k *SecretKey) Bytes() []byte {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.b
}

func (k *SecretKey) Clone() *SecretKey {
	c := NewSecretKey(len(k.Bytes()))
	copy(c.b, k.Bytes())
	return c
}

func (k *SecretKey) Destroy() {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.b == nil {
		return
	}
	zero(k.b)
	if k.free != nil {
		k.free()
	}
	k.b, k.free = nil, nil
}

func NormalizePassword(p []byte) []byte { return p }
