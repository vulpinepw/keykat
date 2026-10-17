package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	big := make([]byte, 3*DefaultChunkSize+12345)
	rand.Read(big)
	files := map[string][]byte{
		"prefs.js":      []byte(`user_pref("browser.startup.page", 3);`),
		"places.sqlite": bytes.Repeat([]byte("SQLite format 3\x00"), 5000),
		"empty.txt":     {},
		"storage/default/https+++example.com/idb/1.sqlite": big,
		"extensions/ünïcødé ✓.json":                        []byte(`{"ok":true}`),
		"cache2/entries/ABCDEF":                            []byte("cached response"),
		"parent.lock":                                      []byte("lock"),
	}
	for rel, data := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "bookmarkbackups"), 0o700); err != nil {
		t.Fatal(err)
	}
	return files
}

func testExclude(rel string, isDir bool) bool {
	return rel == "cache2" || rel == "parent.lock"
}

func readTree(t *testing.T, root string) (map[string][]byte, []string) {
	t.Helper()
	files := map[string][]byte{}
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel := filepath.ToSlash(mustRel(root, p))
		if d.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		b, err := os.ReadFile(p)
		files[rel] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, dirs
}

func sealTestVault(t *testing.T, cipherID CipherID, compression string) (string, *SecretKey, map[string][]byte) {
	t.Helper()
	src := t.TempDir()
	want := makeTree(t, src)
	mk := NewMasterKey()
	vaultPath := filepath.Join(t.TempDir(), "vault.kcv")
	res, err := Seal(src, vaultPath, mk, SealOptions{
		ProfileID:   "p1",
		Cipher:      cipherID,
		Compression: compression,
		Exclude:     testExclude,
	})
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if res.Archive.Files != int64(len(want)-2) {
		t.Fatalf("archived %d files, want %d", res.Archive.Files, len(want)-2)
	}
	delete(want, "cache2/entries/ABCDEF")
	delete(want, "parent.lock")
	return vaultPath, mk, want
}

func TestSealUnsealRoundTrip(t *testing.T) {
	for _, c := range CipherIDs() {
		for _, comp := range []string{CompressionZstd, CompressionNone} {
			t.Run(string(c)+"/"+comp, func(t *testing.T) {
				vaultPath, mk, want := sealTestVault(t, c, comp)
				defer mk.Destroy()

				dst := t.TempDir()
				var lastDone int64
				st, err := Unseal(vaultPath, dst, "p1", mk, func(phase string, done, total int64) {
					lastDone = done
				})
				if err != nil {
					t.Fatalf("unseal: %v", err)
				}
				if lastDone == 0 {
					t.Error("no progress reported")
				}
				got, dirs := readTree(t, dst)
				if len(got) != len(want) {
					t.Fatalf("got %d files, want %d", len(got), len(want))
				}
				for rel, data := range want {
					if !bytes.Equal(got[rel], data) {
						t.Errorf("content mismatch for %s", rel)
					}
				}
				hasEmptyDir := false
				for _, d := range dirs {
					if d == "bookmarkbackups" {
						hasEmptyDir = true
					}
					if strings.HasPrefix(d, "cache2") {
						t.Errorf("excluded dir %s was restored", d)
					}
				}
				if !hasEmptyDir {
					t.Error("empty directory was not preserved")
				}
				if st.Files != int64(len(want)) {
					t.Errorf("stats files = %d", st.Files)
				}
			})
		}
	}
}

func TestWrongKeyAndProfile(t *testing.T) {
	vaultPath, mk, _ := sealTestVault(t, CipherXChaCha20Poly1305, CompressionZstd)
	defer mk.Destroy()
	other := NewMasterKey()
	defer other.Destroy()
	if _, err := Unseal(vaultPath, t.TempDir(), "p1", other, nil); !errors.Is(err, ErrAuth) {
		t.Fatalf("wrong key: got %v, want ErrAuth", err)
	}
	if _, err := Unseal(vaultPath, t.TempDir(), "p2", mk, nil); err == nil {
		t.Fatal("vault opened under a different profile id")
	}
}

func chunkOffsets(t *testing.T, data []byte) []int {
	t.Helper()
	hlen := int(binary.LittleEndian.Uint32(data[8:12]))
	off := 12 + hlen
	var offs []int
	for off < len(data) {
		offs = append(offs, off)
		size := int(binary.LittleEndian.Uint32(data[off : off+4]))
		off += chunkHeaderSize + size
	}
	return offs
}

func TestTamperDetection(t *testing.T) {
	for _, comp := range []string{CompressionNone, CompressionZstd} {
		t.Run(comp, func(t *testing.T) { testTamperDetection(t, comp) })
	}
}

func testTamperDetection(t *testing.T, compression string) {
	vaultPath, mk, _ := sealTestVault(t, CipherCascade, compression)
	defer mk.Destroy()
	orig, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	offs := chunkOffsets(t, orig)
	if len(offs) < 3 {
		t.Fatalf("expected several chunks, got %d", len(offs))
	}

	cases := map[string]func([]byte) []byte{
		"flip ciphertext byte": func(b []byte) []byte { b[offs[1]+100] ^= 1; return b },
		"flip header byte": func(b []byte) []byte {
			i := bytes.Index(b, []byte(`"chunkSize"`))
			b[i+1] = 'C'
			return b
		},
		"truncate tail":       func(b []byte) []byte { return b[:len(b)-10] },
		"drop final chunk":    func(b []byte) []byte { return b[:offs[len(offs)-1]] },
		"append garbage":      func(b []byte) []byte { return append(b, 1, 2, 3) },
		"mark middle as last": func(b []byte) []byte { b[offs[0]+4] = flagLast; return b },
		"swap chunks": func(b []byte) []byte {
			a := append([]byte(nil), b[offs[0]:offs[1]]...)
			c := append([]byte(nil), b[offs[1]:offs[2]]...)
			out := append([]byte(nil), b[:offs[0]]...)
			out = append(out, c...)
			out = append(out, a...)
			return append(out, b[offs[2]:]...)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "tampered.kcv")
			if err := os.WriteFile(p, mutate(append([]byte(nil), orig...)), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(p, "p1", mk); err == nil {
				t.Fatal("tampered vault verified successfully")
			}
			if _, err := Unseal(p, t.TempDir(), "p1", mk, nil); err == nil {
				t.Fatal("tampered vault unsealed successfully")
			}
		})
	}
}

func fastKDF() KDFParams {
	return KDFParams{Algorithm: KDFArgon2id, Salt: RandomBytes(32), Time: 1, MemoryKiB: 19 * 1024, Threads: 1}
}

func TestKeyslots(t *testing.T) {
	for _, c := range CipherIDs() {
		t.Run(string(c), func(t *testing.T) {
			mk := NewMasterKey()
			defer mk.Destroy()
			cr := &Crypto{Cipher: c}
			if err := cr.SetPassword("p1", mk, []byte("correct horse battery staple"), fastKDF()); err != nil {
				t.Fatal(err)
			}
			code, err := cr.SetRecovery("p1", mk)
			if err != nil {
				t.Fatal(err)
			}
			if err := cr.UpdateMAC("p1", mk); err != nil {
				t.Fatal(err)
			}

			got, err := cr.UnlockPassword("p1", []byte("correct horse battery staple"))
			if err != nil {
				t.Fatalf("unlock: %v", err)
			}
			if !bytes.Equal(got.Bytes(), mk.Bytes()) {
				t.Fatal("unlocked key differs")
			}
			if err := cr.VerifyMAC("p1", got); err != nil {
				t.Fatalf("mac: %v", err)
			}
			got.Destroy()

			if _, err := cr.UnlockPassword("p1", []byte("Correct horse battery staple")); !errors.Is(err, ErrWrongPassword) {
				t.Fatalf("wrong password: %v", err)
			}
			if _, err := cr.UnlockPassword("p2", []byte("correct horse battery staple")); !errors.Is(err, ErrWrongPassword) {
				t.Fatalf("cross-profile unlock: %v", err)
			}

			messy := strings.ToLower(strings.ReplaceAll(code, "-", " "))
			rk, err := cr.UnlockRecovery("p1", messy)
			if err != nil {
				t.Fatalf("recovery unlock: %v", err)
			}
			if !bytes.Equal(rk.Bytes(), mk.Bytes()) {
				t.Fatal("recovery key differs")
			}
			rk.Destroy()

			tampered := *cr
			tampered.Cipher = CipherAES256GCM
			if c == CipherAES256GCM {
				tampered.Cipher = CipherChaCha20Poly1305
			}
			if err := tampered.VerifyMAC("p1", mk); !errors.Is(err, ErrTampered) {
				t.Fatalf("tampered manifest: %v", err)
			}
		})
	}
}

func TestWindowsKeyslot(t *testing.T) {
	if !WindowsUnlockAvailable() {
		t.Skip("DPAPI not available")
	}
	mk := NewMasterKey()
	defer mk.Destroy()
	cr := &Crypto{Cipher: CipherXChaCha20Poly1305}
	if err := cr.SetWindows("p1", mk); err != nil {
		t.Fatal(err)
	}
	got, err := cr.UnlockWindows("p1")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Destroy()
	if !bytes.Equal(got.Bytes(), mk.Bytes()) {
		t.Fatal("DPAPI round trip mismatch")
	}
	if _, err := cr.UnlockWindows("p2"); err == nil {
		t.Fatal("DPAPI slot opened for another profile")
	}
}

func TestKDFValidation(t *testing.T) {
	bad := []KDFParams{
		{Algorithm: KDFArgon2id, Salt: RandomBytes(32), Time: 1, MemoryKiB: 1024, Threads: 1},
		{Algorithm: KDFArgon2id, Salt: RandomBytes(4), Time: 3, MemoryKiB: 65536, Threads: 4},
		{Algorithm: KDFScrypt, Salt: RandomBytes(32), LogN: 10, R: 8, P: 1},
		{Algorithm: KDFScrypt, Salt: RandomBytes(32), LogN: 22, R: 32, P: 1},
		{Algorithm: KDFPBKDF2SHA512, Salt: RandomBytes(32), Iterations: 1000},
		{Algorithm: "md5", Salt: RandomBytes(32)},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("case %d: weak or invalid params accepted: %+v", i, p)
		}
	}
	for _, name := range KDFPresetNames() {
		p, err := KDFPreset(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Validate(); err != nil {
			t.Errorf("preset %s invalid: %v", name, err)
		}
	}
	for _, p := range []KDFParams{
		fastKDF(),
		{Algorithm: KDFScrypt, Salt: RandomBytes(32), LogN: 15, R: 8, P: 1},
		{Algorithm: KDFPBKDF2SHA512, Salt: RandomBytes(32), Iterations: 210_000},
	} {
		a, err := p.Derive([]byte("pw"))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := p.Derive([]byte("pw"))
		c, _ := p.Derive([]byte("pw2"))
		if !bytes.Equal(a, b) || bytes.Equal(a, c) || len(a) != KeySize {
			t.Errorf("%s: derivation not deterministic or not distinct", p.Algorithm)
		}
	}
}

func TestValidRelPath(t *testing.T) {
	good := []string{"prefs.js", "a/b/c.txt", "storage/default/https+++x.com/idb"}
	bad := []string{"", "/etc/passwd", "../x", "a/../b", "a//b", "./a", "C:/x", `a\b`, "a/", "a/./b", "x\x00y"}
	for _, p := range good {
		if err := validRelPath(p); err != nil {
			t.Errorf("rejected good path %q: %v", p, err)
		}
	}
	for _, p := range bad {
		if err := validRelPath(p); err == nil {
			t.Errorf("accepted bad path %q", p)
		}
	}
}

func TestRecoverInterruptedSeal(t *testing.T) {
	vaultPath, mk, _ := sealTestVault(t, CipherAES256GCM, CompressionZstd)
	defer mk.Destroy()
	if err := os.Rename(vaultPath, vaultPath+".tmp"); err != nil {
		t.Fatal(err)
	}
	msg, err := RecoverInterruptedSeal(vaultPath, "p1", mk)
	if err != nil || msg == "" {
		t.Fatalf("recover: %q %v", msg, err)
	}
	if _, err := Verify(vaultPath, "p1", mk); err != nil {
		t.Fatalf("recovered vault invalid: %v", err)
	}
}

func TestWipeDir(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	target := filepath.Join(dir, "prefs.js")
	if err := WipeDir(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("wiped directory still exists")
	}
}

func BenchmarkKDFPresets(b *testing.B) {
	for _, name := range KDFPresetNames() {
		p, _ := KDFPreset(name)
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := p.Derive([]byte("benchmark password")); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSeal(b *testing.B) {
	src := b.TempDir()
	data := make([]byte, 64<<20)
	rand.Read(data[:32<<20])
	os.WriteFile(filepath.Join(src, "blob"), data, 0o600)
	mk := NewMasterKey()
	defer mk.Destroy()
	for _, c := range CipherIDs() {
		b.Run(string(c), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				vp := filepath.Join(b.TempDir(), "v.kcv")
				if _, err := Seal(src, vp, mk, SealOptions{ProfileID: "b", Cipher: c, Compression: CompressionZstd}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
