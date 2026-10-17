package keychain

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"keycat/internal/profile"
	"keycat/internal/vault"
)

type fixture struct {
	svc   *Service
	store *profile.Store
	keys  map[string]*vault.SecretKey
	ids   map[string]string
}

func newFixture(t *testing.T, names ...string) *fixture {
	t.Helper()
	root := t.TempDir()
	store, err := profile.NewStore(filepath.Join(root, "Profiles"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(store, filepath.Join(root, "Shared"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	f := &fixture{svc: svc, store: store, keys: map[string]*vault.SecretKey{}, ids: map[string]string{}}
	for _, name := range names {
		mk := vault.NewMasterKey()
		p := &profile.Profile{
			ID: profile.NewID(), Name: name, Color: "#F5A524", Avatar: "cat", Created: time.Now(),
			Security: profile.Security{Mode: profile.ModeEncrypted, Crypto: &vault.Crypto{Cipher: vault.CipherXChaCha20Poly1305}},
		}
		if err := p.Security.Crypto.UpdateMAC(p.ID, mk); err != nil {
			t.Fatal(err)
		}
		if err := store.Save(p); err != nil {
			t.Fatal(err)
		}
		changed, err := svc.Attach(p, mk, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !changed || len(p.Security.Crypto.SharePublic) != 32 {
			t.Fatal("attach did not create a sharing key")
		}
		if err := p.Security.Crypto.VerifyMAC(p.ID, mk); err != nil {
			t.Fatal("manifest MAC not updated after adding the sharing key")
		}
		if err := store.Save(p); err != nil {
			t.Fatal(err)
		}
		f.keys[name], f.ids[name] = mk, p.ID
	}
	return f
}

func TestPersonalEntries(t *testing.T) {
	f := newFixture(t, "A")
	a := f.ids["A"]
	e, err := f.svc.Save(a, Entry{Origins: []string{"accounts.example.com"}, Username: "me", Password: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Password != "" || e.Title != "accounts.example.com" || e.Origins[0] != "https://accounts.example.com" {
		t.Fatalf("unexpected saved entry %+v", e)
	}
	e.Title = "Example"
	if _, err := f.svc.Save(a, e); err != nil {
		t.Fatal(err)
	}
	full, err := f.svc.Get(a, e.ID)
	if err != nil || full.Password != "s3cret" || full.Title != "Example" {
		t.Fatalf("get after update: %+v %v", full, err)
	}
	f.svc.Detach(a)
	if _, err := f.svc.List(a); !errors.Is(err, ErrLocked) {
		t.Fatalf("list while locked: %v", err)
	}
	p, _ := f.store.Get(a)
	if _, err := f.svc.Attach(p, f.keys["A"], 0); err != nil {
		t.Fatal(err)
	}
	list, _ := f.svc.List(a)
	if len(list) != 1 || list[0].Password != "" {
		t.Fatalf("list after reattach: %+v", list)
	}
	f.svc.Detach(a)
	wrong := vault.NewMasterKey()
	defer wrong.Destroy()
	if _, err := f.svc.Attach(p, wrong, 0); !errors.Is(err, ErrDamaged) {
		t.Fatalf("attach with wrong key: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(f.store.Dir(a), fileName))
	if strings.Contains(string(data), "s3cret") || strings.Contains(string(data), "accounts.example.com") {
		t.Fatal("keychain file leaks plaintext")
	}
	if err := f.svc.Delete(a, e.ID); !errors.Is(err, ErrLocked) {
		t.Fatalf("delete while locked: %v", err)
	}
}

func TestDamagedKeychainFallsBackToThePreviousVersion(t *testing.T) {
	f := newFixture(t, "A")
	a := f.ids["A"]
	for _, host := range []string{"one.example", "two.example"} {
		if _, err := f.svc.Save(a, Entry{Origins: []string{host}, Username: "me", Password: "pw-" + host}); err != nil {
			t.Fatal(err)
		}
	}
	f.svc.Detach(a)
	path := filepath.Join(f.store.Dir(a), fileName)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"format":"keycat-keychain", damaged`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, _ := f.store.Get(a)
	if _, err := f.svc.Attach(p, f.keys["A"], 0); err != nil {
		t.Fatalf("attach with a damaged keychain: %v", err)
	}
	list, err := f.svc.List(a)
	if err != nil || len(list) != 1 || list[0].Title != "one.example" {
		t.Fatalf("expected the previous version (one login): %+v %v", list, err)
	}
}

func TestImportLogins(t *testing.T) {
	f := newFixture(t, "A")
	a := f.ids["A"]
	if _, err := f.svc.Save(a, Entry{Origins: []string{"https://mail.example"}, Username: "me", Password: "old"}); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.ImportLogins(a, []ImportedLogin{
		{Origin: "https://mail.example", Username: "me", Password: "new"},
		{Origin: "https://shop.example:443", Username: "buyer", Password: "pw"},
		{Origin: "https://shop.example", Username: "buyer", Password: "pw"},
		{Origin: "file:///C:/notes.txt", Username: "x", Password: "y"},
		{Origin: "https://empty.example", Username: "x", Password: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 2 || res.Unchanged != 1 || len(res.Skipped) != 2 || res.Skipped[0] != 3 || res.Skipped[1] != 4 {
		t.Fatalf("unexpected result %+v", res)
	}
	list, _ := f.svc.List(a)
	if len(list) != 3 {
		t.Fatalf("expected 3 logins, got %+v", list)
	}
	m, _ := f.svc.Match(a, "https://mail.example")
	if len(m) != 2 {
		t.Fatalf("the existing password must be kept next to the imported one: %+v", m)
	}
	again, err := f.svc.ImportLogins(a, []ImportedLogin{{Origin: "https://mail.example", Username: "me", Password: "new"}})
	if err != nil || again.Created != 0 || again.Unchanged != 1 {
		t.Fatalf("importing the same login twice: %+v %v", again, err)
	}
}

func TestMatching(t *testing.T) {
	f := newFixture(t, "A")
	a := f.ids["A"]
	save := func(origin, user string) {
		if _, err := f.svc.Save(a, Entry{Origins: []string{origin}, Username: user, Password: "pw"}); err != nil {
			t.Fatal(err)
		}
	}
	save("https://login.example.com", "exact")
	save("https://www.example.com", "same-site")
	save("http://example.com", "http-only")
	save("https://alice.github.io", "alice")
	save("https://example.org", "other")

	got, err := f.svc.Match(a, "https://login.example.com/path?q=1")
	if err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, e := range got {
		users = append(users, e.Username)
	}
	if strings.Join(users, ",") != "exact,same-site" {
		t.Fatalf("matches = %v", users)
	}
	if got, _ := f.svc.Match(a, "https://bob.github.io"); len(got) != 0 {
		t.Fatalf("github.io pages must not share logins: %v", got)
	}
	save("http://shop.example.net", "upgraded")
	save("https://secure.example.net", "secure-only")
	upgraded := false
	got, _ = f.svc.Match(a, "https://shop.example.net/login")
	for _, e := range got {
		upgraded = upgraded || e.Username == "upgraded"
	}
	if !upgraded {
		t.Fatalf("http login not offered on https: %v", got)
	}
	got, _ = f.svc.Match(a, "http://secure.example.net")
	for _, e := range got {
		if e.Username == "secure-only" {
			t.Fatalf("https login offered on http: %v", got)
		}
	}
	if _, err := NormalizeOrigin("javascript:alert(1)"); err == nil {
		t.Fatal("accepted a javascript: origin")
	}
}

func TestSharing(t *testing.T) {
	f := newFixture(t, "A", "B", "C")
	a, b, c := f.ids["A"], f.ids["B"], f.ids["C"]
	e, err := f.svc.Save(a, Entry{Title: "Netflix", Origins: []string{"netflix.com"}, Username: "family", Password: "popcorn"})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := f.svc.Share(a, e.ID, []string{b})
	if err != nil {
		t.Fatal(err)
	}
	if len(shared.SharedWith) != 2 || shared.Owner != a {
		t.Fatalf("shared entry %+v", shared)
	}
	got, err := f.svc.Get(b, e.ID)
	if err != nil || got.Password != "popcorn" {
		t.Fatalf("B get: %+v %v", got, err)
	}
	if m, _ := f.svc.Match(b, "https://www.netflix.com"); len(m) != 1 {
		t.Fatal("B cannot autofill the shared login")
	}
	if _, err := f.svc.Get(c, e.ID); !errors.Is(err, ErrNoEntry) {
		t.Fatalf("C can read a login not shared with it: %v", err)
	}
	if list, _ := f.svc.List(a); len(list) != 1 || list[0].Owner != a {
		t.Fatalf("A list %+v", list)
	}
	got.Password = "popcorn2"
	if _, err := f.svc.Save(b, got); err != nil {
		t.Fatal(err)
	}
	if g, _ := f.svc.Get(a, e.ID); g.Password != "popcorn2" {
		t.Fatalf("A does not see B's edit: %q", g.Password)
	}
	f.svc.mu.Lock()
	oldFile, _ := os.ReadFile(f.svc.sharedPath(e.ID))
	f.svc.mu.Unlock()
	if _, err := f.svc.Share(a, e.ID, []string{c}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(b, e.ID); !errors.Is(err, ErrNoEntry) {
		t.Fatal("B still sees a login after being removed")
	}
	if g, err := f.svc.Get(c, e.ID); err != nil || g.Password != "popcorn2" {
		t.Fatalf("C after sharing: %+v %v", g, err)
	}
	newFile, _ := os.ReadFile(f.svc.sharedPath(e.ID))
	if string(oldFile) == string(newFile) {
		t.Fatal("shared file not re-encrypted on revoke")
	}
	if _, err := f.svc.Share(a, e.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.svc.sharedPath(e.ID)); !os.IsNotExist(err) {
		t.Fatal("shared file not removed")
	}
	if g, err := f.svc.Get(a, e.ID); err != nil || g.Owner != "" {
		t.Fatalf("A personal again: %+v %v", g, err)
	}
	if _, err := f.svc.Get(c, e.ID); !errors.Is(err, ErrNoEntry) {
		t.Fatal("C still sees an unshared login")
	}
	if _, err := f.svc.Share(a, e.ID, []string{b, c}); err != nil {
		t.Fatal(err)
	}
	f.svc.ForgetProfile(c)
	raw, _ := os.ReadFile(f.svc.sharedPath(e.ID))
	if strings.Contains(string(raw), c) {
		t.Fatal("deleted profile still a recipient")
	}
}

func TestTemporaryUnlockExpires(t *testing.T) {
	f := newFixture(t, "A")
	a := f.ids["A"]
	f.svc.Detach(a)
	f.svc.TempTTL = 60 * time.Millisecond
	p, _ := f.store.Get(a)
	if _, err := f.svc.Attach(p, f.keys["A"], 60*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !f.svc.IsOpen(a) {
		t.Fatal("not open after temporary unlock")
	}
	time.Sleep(150 * time.Millisecond)
	if f.svc.IsOpen(a) {
		t.Fatal("temporary unlock did not expire")
	}
}

func TestIdentityAndGenerator(t *testing.T) {
	f := newFixture(t, "A")
	a := f.ids["A"]
	one, _, err := f.svc.Identity(a, false)
	if err != nil {
		t.Fatal(err)
	}
	again, _, _ := f.svc.Identity(a, false)
	if one.Email != again.Email {
		t.Fatal("stored identity changed between calls")
	}
	fresh, _, _ := f.svc.Identity(a, true)
	if fresh.Email == one.Email && fresh.Phone == one.Phone {
		t.Fatal("fresh identity equals the stored one")
	}
	if stored, _, _ := f.svc.Identity(a, false); stored.Email != one.Email {
		t.Fatal("a fresh identity replaced the stored one")
	}
	re, _ := f.svc.RegenerateIdentity(a)
	if re.Email == one.Email && re.Phone == one.Phone {
		t.Fatal("regenerate did not change the identity")
	}

	o := DefaultGenerator()
	for i := 0; i < 200; i++ {
		pw := GeneratePassword(o)
		if len(pw) != 20 || strings.ContainsAny(pw, "lIO01") {
			t.Fatalf("bad password %q", pw)
		}
		if !strings.ContainsAny(pw, "abcdefghijkmnpqrstuvwxyz") || !strings.ContainsAny(pw, "ABCDEFGHJKLMNPQRSTUVWXYZ") ||
			!strings.ContainsAny(pw, "23456789") || !strings.ContainsAny(pw, "!@#$%^&*-_=+?.:;~") {
			t.Fatalf("password %q misses a class", pw)
		}
	}
	if o.Entropy() < 110 {
		t.Fatalf("entropy %d too low", o.Entropy())
	}
	short := GeneratePassword(GeneratorOptions{Length: 3, Digits: true})
	if len(short) != 8 || strings.Trim(short, "0123456789") != "" {
		t.Fatalf("clamped digits-only password %q", short)
	}
}
