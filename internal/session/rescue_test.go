package session

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keycat/internal/firefox"
	"keycat/internal/profile"
	"keycat/internal/vault"
)

const testPassword = "correct horse battery staple"

func newTestManager(t *testing.T) (*Manager, *profile.Store, string) {
	t.Helper()
	root := t.TempDir()
	store, err := profile.NewStore(filepath.Join(root, "Profiles"))
	if err != nil {
		t.Fatal(err)
	}
	sessions := filepath.Join(root, "Sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Store:       store,
		SessionsDir: sessions,
		Start:       func(firefox.LaunchOptions) (Browser, error) { return nil, errors.New("no engine in tests") },
		APIURL:      func() string { return "" },
		Overwrite:   func() bool { return false },
	})
	return m, store, sessions
}

func newEncryptedProfile(t *testing.T, m *Manager, keepBackup bool) (*profile.Profile, *vault.SecretKey) {
	t.Helper()
	privacy, err := profile.PrivacyPreset("balanced")
	if err != nil {
		t.Fatal(err)
	}
	p, _, mk, err := m.Create(CreateRequest{
		Name: "Rescue test", Color: "#8B6CF6", Avatar: "cat",
		Security: SecurityRequest{
			Mode: profile.ModeEncrypted, Preset: "custom", Cipher: "aes-256-gcm", KDF: "pbkdf2-sha512",
			Password: testPassword, KeepBackup: keepBackup,
		},
		Privacy: privacy,
		Open:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p, mk
}

func sealContent(t *testing.T, m *Manager, p *profile.Profile, mk *vault.SecretKey, content string) {
	t.Helper()
	dir := filepath.Join(m.cfg.SessionsDir, p.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "places.sqlite"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.seal(p, dir, mk); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("session folder still there after sealing: %v", err)
	}
	if _, err := os.Stat(dir + "-wiping"); !os.IsNotExist(err) {
		t.Fatalf("wiping folder still there after sealing: %v", err)
	}
}

func vaultContent(t *testing.T, path string, p *profile.Profile, mk *vault.SecretKey) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := vault.Unseal(path, dir, p.ID, mk, nil); err != nil {
		t.Fatalf("unseal %s: %v", filepath.Base(path), err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "places.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func corrupt(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append([]byte(nil), data...)
	data[len(data)/2] ^= 0xFF
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRescueCopyKeepsTheOldVaultAcrossASeal(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "version one")

	made, err := m.createRescue(p, RescueOpen)
	if err != nil || !made {
		t.Fatalf("createRescue: %v %v", made, err)
	}
	if again, _ := m.createRescue(p, RescueKeys); again {
		t.Fatal("an existing rescue copy was replaced")
	}
	sealContent(t, m, p, mk, "version two")

	rescueVault := filepath.Join(m.rescuePath(p.ID), "vault.kcv")
	if got := vaultContent(t, rescueVault, p, mk); got != "version one" {
		t.Fatalf("rescue copy holds %q", got)
	}
	if got := vaultContent(t, store.VaultPath(p.ID), p, mk); got != "version two" {
		t.Fatalf("vault holds %q", got)
	}
	info, ok := m.Rescue(p.ID)
	if !ok || info.Reason != RescueOpen {
		t.Fatalf("rescue info: %+v %v", info, ok)
	}
	m.dropRescue(p.ID)
	if _, ok := m.Rescue(p.ID); ok {
		t.Fatal("rescue copy not deleted")
	}
	if got := vaultContent(t, store.VaultPath(p.ID), p, mk); got != "version two" {
		t.Fatalf("deleting the rescue copy changed the vault: %q", got)
	}
}

func TestCheckRescueRestoresADamagedVault(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "good data")
	if _, err := m.createRescue(p, RescueOpen); err != nil {
		t.Fatal(err)
	}
	corrupt(t, store.VaultPath(p.ID))

	notice, err := m.checkRescue(p, mk)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notice, "rescue copy") {
		t.Fatalf("notice: %q", notice)
	}
	if got := vaultContent(t, store.VaultPath(p.ID), p, mk); got != "good data" {
		t.Fatalf("restored vault holds %q", got)
	}
	if _, err := os.Stat(store.VaultPath(p.ID) + ".damaged"); err != nil {
		t.Fatalf("damaged vault not kept aside: %v", err)
	}
}

func TestCheckRescueDropsTheCopyWhenTheVaultIsFine(t *testing.T) {
	m, _, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "fine")
	if _, err := m.createRescue(p, RescueOpen); err != nil {
		t.Fatal(err)
	}
	if notice, err := m.checkRescue(p, mk); err != nil || notice != "" {
		t.Fatalf("checkRescue: %q %v", notice, err)
	}
	if _, ok := m.Rescue(p.ID); ok {
		t.Fatal("rescue copy kept although the vault is intact")
	}
}

func TestCheckRescueRefusesWhenBothCopiesAreDamaged(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "data")
	if _, err := m.createRescue(p, RescueOpen); err != nil {
		t.Fatal(err)
	}
	corrupt(t, store.VaultPath(p.ID))
	corrupt(t, filepath.Join(m.rescuePath(p.ID), "vault.kcv"))
	if _, err := m.checkRescue(p, mk); err == nil {
		t.Fatal("checkRescue accepted two damaged vaults")
	}
}

func TestRecoverWipesUnfinishedDecryptAndWipe(t *testing.T) {
	m, _, sessions := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	for _, name := range []string{p.ID + "-unsealing", p.ID + "-wiping"} {
		dir := filepath.Join(sessions, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "partial"), []byte("half"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m.Recover()
	for _, name := range []string{p.ID + "-unsealing", p.ID + "-wiping"} {
		if _, err := os.Stat(filepath.Join(sessions, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived recovery", name)
		}
	}
	if st := m.Status(p.ID).State; st != StateLocked {
		t.Fatalf("an unfinished decrypt or wipe was treated as profile data: %s", st)
	}
}

func TestPasswordChangeIsVerifiedAndCleansUp(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "data")
	_, err := m.ChangeSecurity(p.ID, SecurityChange{
		Credentials: Credentials{Method: "password", Password: testPassword},
		Action:      "password",
		NewPassword: "a brand new passphrase",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Rescue(p.ID); ok {
		t.Fatal("rescue copy left after a verified change")
	}
	cur, _ := store.Get(p.ID)
	if _, err := (Credentials{Method: "password", Password: testPassword}).Unlock(cur); err == nil {
		t.Fatal("the old password still unlocks the profile")
	}
	k, err := (Credentials{Method: "password", Password: "a brand new passphrase"}).Unlock(cur)
	if err != nil || !sameKey(k, mk) {
		t.Fatalf("new password: %v", err)
	}
}

func TestCipherChangeReencryptsAndVerifies(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, true)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "survives re-encryption")
	if _, err := m.ChangeSecurity(p.ID, SecurityChange{
		Credentials: Credentials{Method: "password", Password: testPassword},
		Action:      "encryption",
		Cipher:      "aes-256-gcm+xchacha20-poly1305",
		KDF:         "pbkdf2-sha512",
	}); err != nil {
		t.Fatal(err)
	}
	cur, _ := store.Get(p.ID)
	if cur.Security.Crypto.Cipher != "aes-256-gcm+xchacha20-poly1305" {
		t.Fatalf("cipher is %s", cur.Security.Crypto.Cipher)
	}
	if got := vaultContent(t, store.VaultPath(p.ID), cur, mk); got != "survives re-encryption" {
		t.Fatalf("vault holds %q", got)
	}
	if _, ok := m.Rescue(p.ID); ok {
		t.Fatal("rescue copy left after a verified re-encryption")
	}
}

func TestInterruptedKeyChangeIsUndoneWithTheRescueKeys(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "data")
	if _, err := m.createRescue(p, RescueKeys); err != nil {
		t.Fatal(err)
	}
	broken, _ := store.Get(p.ID)
	if err := broken.Security.Crypto.SetPassword(p.ID, mk, []byte("half-finished password"), mustKDF(t)); err != nil {
		t.Fatal(err)
	}
	for i := range broken.Security.Crypto.Keyslots {
		if broken.Security.Crypto.Keyslots[i].Type == vault.SlotPassword {
			broken.Security.Crypto.Keyslots[i].Wrapped[0] ^= 0xFF
		}
	}
	if err := store.Save(broken); err != nil {
		t.Fatal(err)
	}
	cur, _ := store.Get(p.ID)
	if _, err := (Credentials{Method: "password", Password: testPassword}).Unlock(cur); err == nil {
		t.Fatal("test setup: the old password should no longer match the saved keys")
	}
	k, err := m.unlockWithRescueKeys(cur, Credentials{Method: "password", Password: testPassword})
	if err != nil || !sameKey(k, mk) {
		t.Fatalf("unlock with rescue keys: %v", err)
	}
	restored, _ := store.Get(p.ID)
	if _, err := (Credentials{Method: "password", Password: testPassword}).Unlock(restored); err != nil {
		t.Fatalf("keys were not restored: %v", err)
	}
}

func TestRestorePreviousVaultSwapsAndCanBeUndone(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, true)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "older")
	sealContent(t, m, p, mk, "newer")
	if _, ok := m.Backup(p.ID); !ok {
		t.Fatal("no previous vault kept")
	}
	cred := Credentials{Method: "password", Password: testPassword}
	if _, err := m.Restore(p.ID, "backup", cred); err != nil {
		t.Fatal(err)
	}
	if got := vaultContent(t, store.VaultPath(p.ID), p, mk); got != "older" {
		t.Fatalf("after restore the vault holds %q", got)
	}
	if _, err := m.Restore(p.ID, "backup", cred); err != nil {
		t.Fatal(err)
	}
	if got := vaultContent(t, store.VaultPath(p.ID), p, mk); got != "newer" {
		t.Fatalf("after undoing the restore the vault holds %q", got)
	}
}

func TestRestoreRefusesADamagedCopy(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, true)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "older")
	sealContent(t, m, p, mk, "newer")
	corrupt(t, store.VaultPath(p.ID)+".bak")
	if _, err := m.Restore(p.ID, "backup", Credentials{Method: "password", Password: testPassword}); err == nil {
		t.Fatal("a damaged copy was restored")
	}
	if got := vaultContent(t, store.VaultPath(p.ID), p, mk); got != "newer" {
		t.Fatalf("a refused restore changed the vault: %q", got)
	}
}

func TestDamagedManifestFallsBackToThePreviousVersion(t *testing.T) {
	m, store, _ := newTestManager(t)
	p, mk := newEncryptedProfile(t, m, false)
	defer mk.Destroy()
	sealContent(t, m, p, mk, "data")
	if err := os.WriteFile(store.ManifestPath(p.ID), []byte("{ damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(p.ID)
	if err != nil {
		t.Fatalf("damaged manifest: %v", err)
	}
	if _, err := (Credentials{Method: "password", Password: testPassword}).Unlock(got); err != nil {
		t.Fatalf("keys from the previous manifest do not work: %v", err)
	}
}

func TestSealFailsCleanlyWithoutSpace(t *testing.T) {
	err := vault.EnsureFreeSpace(t.TempDir(), 1<<62)
	var ns *vault.NoSpaceError
	if !errors.As(err, &ns) {
		t.Skipf("free space is not available here: %v", err)
	}
	if !strings.Contains(err.Error(), "free") || bytes.Contains([]byte(err.Error()), []byte("%!")) {
		t.Fatalf("message: %v", err)
	}
}

func mustKDF(t *testing.T) vault.KDFParams {
	t.Helper()
	k, err := vault.KDFPreset("pbkdf2-sha512")
	if err != nil {
		t.Fatal(err)
	}
	return k
}
