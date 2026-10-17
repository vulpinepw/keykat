package firefox

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/openpgp"
)

func TestVerifyRelease(t *testing.T) {
	dir, name := os.Getenv("KEYCAT_RELEASE_DIR"), os.Getenv("KEYCAT_RELEASE_FILE")
	if dir == "" || name == "" {
		t.Skip("KEYCAT_RELEASE_DIR and KEYCAT_RELEASE_FILE are not set")
	}
	read := func(f string) []byte {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	sums, sig, key := read("SHA512SUMS"), read("SHA512SUMS.asc"), read("KEY")
	if keys, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(key)); err == nil {
		for _, k := range keys {
			t.Logf("key %X", k.PrimaryKey.Fingerprint)
		}
	}
	tarball := filepath.Join(dir, filepath.Base(name))
	if err := VerifyRelease(tarball, name, sums, sig, key); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRelease(tarball, name+".other", sums, sig, key); err == nil {
		t.Fatal("a file outside the release passed")
	}
	tampered := bytes.Replace(sums, []byte(hex.EncodeToString([]byte{0})), []byte("11"), 1)
	if err := VerifyRelease(tarball, name, tampered, sig, key); err == nil {
		t.Fatal("tampered checksums passed")
	}
}
