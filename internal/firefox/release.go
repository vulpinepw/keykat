package firefox

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/openpgp"
)

const MozillaKeyFingerprint = "14F26682D0916CDD81E37B6D61B7B526D98F0353"

func VerifyRelease(path, name string, sums, signature, keyFile []byte) error {
	keys, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(keyFile))
	if err != nil {
		return fmt.Errorf("reading Mozilla's release key: %w", err)
	}
	var trusted openpgp.EntityList
	for _, k := range keys {
		if strings.EqualFold(hex.EncodeToString(k.PrimaryKey.Fingerprint[:]), MozillaKeyFingerprint) {
			trusted = append(trusted, k)
		}
	}
	if len(trusted) == 0 {
		return errors.New("Mozilla's release key is missing from the release")
	}
	if _, err := openpgp.CheckArmoredDetachedSignature(trusted, bytes.NewReader(sums), bytes.NewReader(signature)); err != nil {
		return fmt.Errorf("Mozilla's signature on the release checksums is not valid: %w", err)
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return fmt.Errorf("%s is not part of the release", name)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("the download does not match the checksum Mozilla published")
	}
	return nil
}
