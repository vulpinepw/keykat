package keychain

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"

	"keycat/internal/identity"
	"keycat/internal/profile"
	"keycat/internal/vault"
)

const (
	fileName      = "keychain.kck"
	sharedSuffix  = ".kcs"
	purposeBody   = "keychain"
	purposeShared = "shared-entry"
	purposeSlot   = "share-slot"
)

var (
	ErrLocked      = errors.New("this profile's passwords are locked")
	ErrNoEntry     = errors.New("password entry not found")
	ErrUnavailable = errors.New("Keycat Passwords needs an encrypted profile")
	ErrDamaged     = errors.New("the password store could not be decrypted")
)

type Entry struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Origins    []string   `json:"origins"`
	Username   string     `json:"username"`
	Password   string     `json:"password,omitempty"`
	Notes      string     `json:"notes,omitempty"`
	Created    time.Time  `json:"created"`
	Updated    time.Time  `json:"updated"`
	LastUsed   *time.Time `json:"lastUsed,omitempty"`
	Owner      string     `json:"owner,omitempty"`
	SharedWith []string   `json:"sharedWith,omitempty"`
}

type body struct {
	Version          int                `json:"version"`
	PrivateKey       []byte             `json:"privateKey"`
	Entries          []Entry            `json:"entries"`
	Identity         *identity.Identity `json:"identity,omitempty"`
	IdentitySettings identity.Settings  `json:"identitySettings"`
	Generator        GeneratorOptions   `json:"generator"`
	NeverSave        []string           `json:"neverSave,omitempty"`
}

type envelope struct {
	Format  string         `json:"format"`
	Version int            `json:"version"`
	Profile string         `json:"profile"`
	Cipher  vault.CipherID `json:"cipher"`
	Sealed  []byte         `json:"sealed"`
}

type shareSlot struct {
	EPK     []byte `json:"epk"`
	Wrapped []byte `json:"wrapped"`
}

type sharedFile struct {
	Format     string               `json:"format"`
	Version    int                  `json:"version"`
	ID         string               `json:"id"`
	Owner      string               `json:"owner"`
	Recipients map[string]shareSlot `json:"recipients"`
	Sealed     []byte               `json:"sealed"`
}

type sharedRef struct {
	file  *sharedFile
	key   []byte
	entry Entry
}

type unlocked struct {
	id      string
	cipher  vault.CipherID
	root    *vault.SecretKey
	priv    *ecdh.PrivateKey
	body    *body
	expires time.Time
}

type Service struct {
	store     *profile.Store
	sharedDir string
	TempTTL   time.Duration
	mu        sync.Mutex
	open      map[string]*unlocked
	stop      chan struct{}
}

func NewService(store *profile.Store, sharedDir string) (*Service, error) {
	if err := os.MkdirAll(sharedDir, 0o700); err != nil {
		return nil, err
	}
	s := &Service{store: store, sharedDir: sharedDir, TempTTL: 10 * time.Minute, open: map[string]*unlocked{}, stop: make(chan struct{})}
	go s.janitor()
	return s, nil
}

func (s *Service) Close() {
	close(s.stop)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, u := range s.open {
		u.root.Destroy()
		delete(s.open, id)
	}
}

func (s *Service) janitor() {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-t.C:
			s.mu.Lock()
			for id, u := range s.open {
				if !u.expires.IsZero() && now.After(u.expires) {
					u.root.Destroy()
					delete(s.open, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) path(id string) string { return filepath.Join(s.store.Dir(id), fileName) }

func bodyAAD(id string) []byte { return []byte("keycat/v1/keychain\x00" + id) }

func newEntryID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Service) Attach(p *profile.Profile, mk *vault.SecretKey, ttl time.Duration) (bool, error) {
	if !p.IsEncrypted() || p.Security.Crypto == nil {
		return false, ErrUnavailable
	}
	root, err := vault.DeriveKey(mk, "keychain")
	if err != nil {
		return false, err
	}
	b, created, err := s.load(p.ID, root)
	if err != nil {
		root.Destroy()
		return false, err
	}
	priv, err := ecdh.X25519().NewPrivateKey(b.PrivateKey)
	if err != nil {
		root.Destroy()
		return false, ErrDamaged
	}
	u := &unlocked{id: p.ID, cipher: p.Security.Crypto.Cipher, root: root, priv: priv, body: b}
	if ttl > 0 {
		u.expires = time.Now().Add(ttl)
	}

	s.mu.Lock()
	if old := s.open[p.ID]; old != nil {
		if old.expires.IsZero() {
			u.expires = time.Time{}
		}
		old.root.Destroy()
	}
	s.open[p.ID] = u
	if created {
		err = s.save(u)
	}
	s.mu.Unlock()
	if err != nil {
		return false, err
	}

	pub := priv.PublicKey().Bytes()
	if !bytes.Equal(p.Security.Crypto.SharePublic, pub) {
		p.Security.Crypto.SharePublic = pub
		if err := p.Security.Crypto.UpdateMAC(p.ID, mk); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *Service) load(id string, root *vault.SecretKey) (*body, bool, error) {
	b, created, err := s.loadFile(id, s.path(id), root)
	if err == nil && !created {
		return b, false, nil
	}
	if prev, _, perr := s.loadFile(id, s.path(id)+".bak", root); perr == nil && prevExists(s.path(id)+".bak") {
		return prev, false, nil
	}
	return b, created, err
}

func prevExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *Service) loadFile(id, path string, root *vault.SecretKey) (*body, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		priv, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return nil, false, err
		}
		return &body{
			Version:          1,
			PrivateKey:       priv.Bytes(),
			Generator:        DefaultGenerator(),
			IdentitySettings: identity.DefaultSettings(),
		}, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil || env.Format != "keycat-keychain" || env.Profile != id {
		return nil, false, ErrDamaged
	}
	plain, err := vault.OpenBlob(env.Cipher, root.Bytes(), purposeBody, env.Sealed, bodyAAD(id))
	if err != nil {
		return nil, false, ErrDamaged
	}
	defer clear(plain)
	var b body
	if err := json.Unmarshal(plain, &b); err != nil {
		return nil, false, ErrDamaged
	}
	if b.Generator.Length == 0 {
		b.Generator = DefaultGenerator()
	}
	if b.IdentitySettings.Country == "" {
		b.IdentitySettings = identity.DefaultSettings()
	}
	return &b, false, nil
}

func (s *Service) save(u *unlocked) error {
	plain, err := json.Marshal(u.body)
	if err != nil {
		return err
	}
	defer clear(plain)
	sealed, err := vault.SealBlob(u.cipher, u.root.Bytes(), purposeBody, plain, bodyAAD(u.id))
	if err != nil {
		return err
	}
	check, err := vault.OpenBlob(u.cipher, u.root.Bytes(), purposeBody, sealed, bodyAAD(u.id))
	if err != nil || !bytes.Equal(check, plain) {
		return errors.New("keychain: the encrypted data could not be verified, nothing was written")
	}
	clear(check)
	data, err := json.Marshal(envelope{Format: "keycat-keychain", Version: 1, Profile: u.id, Cipher: u.cipher, Sealed: sealed})
	if err != nil {
		return err
	}
	path := s.path(u.id)
	if _, created, err := s.loadFile(u.id, path, u.root); err == nil && !created {
		_ = os.Remove(path + ".bak")
		_ = vault.LinkOrCopy(path, path+".bak")
	}
	return profile.WriteFileAtomic(path, data)
}

func (s *Service) Detach(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.open[id]; u != nil {
		u.root.Destroy()
		delete(s.open, id)
	}
}

func (s *Service) LockTemporary(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.open[id]; u != nil && !u.expires.IsZero() {
		u.root.Destroy()
		delete(s.open, id)
	}
}

func (s *Service) IsOpen(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.getLocked(id)
	return err == nil
}

func (s *Service) getLocked(id string) (*unlocked, error) {
	u := s.open[id]
	if u == nil {
		return nil, ErrLocked
	}
	if !u.expires.IsZero() {
		if time.Now().After(u.expires) {
			u.root.Destroy()
			delete(s.open, id)
			return nil, ErrLocked
		}
		u.expires = time.Now().Add(s.TempTTL)
	}
	return u, nil
}

func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty website address")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("%q is not a website address", raw)
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	origin := u.Scheme + "://" + host
	if port != "" {
		origin += ":" + port
	}
	return origin, nil
}

func originHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func site(host string) string {
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return host
	}
	if d, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return d
	}
	return host
}

func matchScore(e *Entry, origin string) int {
	best := 0
	host := originHost(origin)
	scheme, _, _ := strings.Cut(origin, "://")
	for _, o := range e.Origins {
		if o == origin {
			return 2
		}
		oScheme, _, _ := strings.Cut(o, "://")
		oHost := originHost(o)
		switch {
		case oScheme == scheme && site(oHost) == site(host):
			best = 1
		case oScheme == "http" && scheme == "https" && strings.TrimPrefix(oHost, "www.") == strings.TrimPrefix(host, "www."):
			best = 1
		}
	}
	return best
}

func (e *Entry) normalize() error {
	e.Title = strings.TrimSpace(e.Title)
	e.Username = strings.TrimSpace(e.Username)
	var origins []string
	for _, o := range e.Origins {
		if strings.TrimSpace(o) == "" {
			continue
		}
		n, err := NormalizeOrigin(o)
		if err != nil {
			return err
		}
		dup := false
		for _, x := range origins {
			dup = dup || x == n
		}
		if !dup {
			origins = append(origins, n)
		}
	}
	e.Origins = origins
	if e.Title == "" && len(origins) > 0 {
		e.Title = strings.TrimPrefix(originHost(origins[0]), "www.")
	}
	switch {
	case e.Title == "":
		return errors.New("give the login a name or a website")
	case e.Password == "":
		return errors.New("the password is empty")
	case len(e.Title) > 200 || len(e.Username) > 500 || len(e.Password) > 4096 || len(e.Notes) > 20000 || len(origins) > 20:
		return errors.New("the login is too large")
	}
	return nil
}

func stripSecrets(e Entry) Entry {
	e.Password = ""
	return e
}

func slotAAD(entryID, recipient string) []byte {
	return []byte("keycat/v1/share-slot\x00" + entryID + "\x00" + recipient)
}

func sharedAAD(entryID string) []byte { return []byte("keycat/v1/shared\x00" + entryID) }

func slotKEK(shared, epk, recipientPub []byte, entryID, recipient string) ([]byte, error) {
	salt := append(append([]byte{}, epk...), recipientPub...)
	return hkdf.Key(sha256.New, shared, salt, "keycat/v1/share/"+entryID+"/"+recipient, vault.KeySize)
}

func wrapSlot(recipientPub []byte, entryID, recipient string, entryKey []byte) (shareSlot, error) {
	pub, err := ecdh.X25519().NewPublicKey(recipientPub)
	if err != nil {
		return shareSlot{}, err
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return shareSlot{}, err
	}
	shared, err := eph.ECDH(pub)
	if err != nil {
		return shareSlot{}, err
	}
	epk := eph.PublicKey().Bytes()
	kek, err := slotKEK(shared, epk, recipientPub, entryID, recipient)
	if err != nil {
		return shareSlot{}, err
	}
	defer clear(kek)
	wrapped, err := vault.SealBlob(vault.CipherXChaCha20Poly1305, kek, purposeSlot, entryKey, slotAAD(entryID, recipient))
	return shareSlot{EPK: epk, Wrapped: wrapped}, err
}

func unwrapSlot(priv *ecdh.PrivateKey, slot shareSlot, entryID, recipient string) ([]byte, error) {
	epk, err := ecdh.X25519().NewPublicKey(slot.EPK)
	if err != nil {
		return nil, err
	}
	shared, err := priv.ECDH(epk)
	if err != nil {
		return nil, err
	}
	kek, err := slotKEK(shared, slot.EPK, priv.PublicKey().Bytes(), entryID, recipient)
	if err != nil {
		return nil, err
	}
	defer clear(kek)
	return vault.OpenBlob(vault.CipherXChaCha20Poly1305, kek, purposeSlot, slot.Wrapped, slotAAD(entryID, recipient))
}

func (s *Service) sharedPath(entryID string) string {
	return filepath.Join(s.sharedDir, entryID+sharedSuffix)
}

func (s *Service) loadShared(u *unlocked) []sharedRef {
	files, _ := filepath.Glob(filepath.Join(s.sharedDir, "*"+sharedSuffix))
	var out []sharedRef
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var f sharedFile
		if json.Unmarshal(data, &f) != nil || f.Format != "keycat-shared" {
			continue
		}
		slot, ok := f.Recipients[u.id]
		if !ok {
			continue
		}
		key, err := unwrapSlot(u.priv, slot, f.ID, u.id)
		if err != nil {
			continue
		}
		plain, err := vault.OpenBlob(vault.CipherXChaCha20Poly1305, key, purposeShared, f.Sealed, sharedAAD(f.ID))
		if err != nil {
			clear(key)
			continue
		}
		var e Entry
		err = json.Unmarshal(plain, &e)
		clear(plain)
		if err != nil || e.ID != f.ID {
			clear(key)
			continue
		}
		e.Owner = f.Owner
		e.SharedWith = recipientsOf(&f)
		out = append(out, sharedRef{file: &f, key: key, entry: e})
	}
	return out
}

func recipientsOf(f *sharedFile) []string {
	ids := make([]string, 0, len(f.Recipients))
	for id := range f.Recipients {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func releaseRefs(refs []sharedRef) {
	for _, r := range refs {
		clear(r.key)
	}
}

func (s *Service) writeShared(f *sharedFile, key []byte, e Entry) error {
	e.Owner, e.SharedWith = "", nil
	plain, err := json.Marshal(e)
	if err != nil {
		return err
	}
	defer clear(plain)
	if f.Sealed, err = vault.SealBlob(vault.CipherXChaCha20Poly1305, key, purposeShared, plain, sharedAAD(f.ID)); err != nil {
		return err
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return profile.WriteFileAtomic(s.sharedPath(f.ID), data)
}

func (s *Service) List(id string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(u.body.Entries))
	for _, e := range u.body.Entries {
		out = append(out, stripSecrets(e))
	}
	refs := s.loadShared(u)
	defer releaseRefs(refs)
	for _, r := range refs {
		out = append(out, stripSecrets(r.entry))
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	return out, nil
}

func (s *Service) Get(id, entryID string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return Entry{}, err
	}
	for _, e := range u.body.Entries {
		if e.ID == entryID {
			return e, nil
		}
	}
	refs := s.loadShared(u)
	defer releaseRefs(refs)
	for _, r := range refs {
		if r.entry.ID == entryID {
			return r.entry, nil
		}
	}
	return Entry{}, ErrNoEntry
}

func (s *Service) Save(id string, e Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return Entry{}, err
	}
	now := time.Now().UTC()
	e.Owner, e.SharedWith = "", nil
	if e.ID == "" {
		if err := e.normalize(); err != nil {
			return Entry{}, err
		}
		e.ID, e.Created, e.Updated = newEntryID(), now, now
		u.body.Entries = append(u.body.Entries, e)
		return stripSecrets(e), s.save(u)
	}
	for i := range u.body.Entries {
		if old := u.body.Entries[i]; old.ID == e.ID {
			if e.Password == "" {
				e.Password = old.Password
			}
			if err := e.normalize(); err != nil {
				return Entry{}, err
			}
			e.Created, e.Updated, e.LastUsed = old.Created, now, old.LastUsed
			u.body.Entries[i] = e
			return stripSecrets(e), s.save(u)
		}
	}
	refs := s.loadShared(u)
	defer releaseRefs(refs)
	for _, r := range refs {
		if r.entry.ID == e.ID {
			if e.Password == "" {
				e.Password = r.entry.Password
			}
			if err := e.normalize(); err != nil {
				return Entry{}, err
			}
			e.Created, e.Updated, e.LastUsed = r.entry.Created, now, r.entry.LastUsed
			if err := s.writeShared(r.file, r.key, e); err != nil {
				return Entry{}, err
			}
			e.Owner, e.SharedWith = r.file.Owner, recipientsOf(r.file)
			return stripSecrets(e), nil
		}
	}
	return Entry{}, ErrNoEntry
}

func (s *Service) Delete(id, entryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return err
	}
	for i := range u.body.Entries {
		if u.body.Entries[i].ID == entryID {
			u.body.Entries = append(u.body.Entries[:i], u.body.Entries[i+1:]...)
			return s.save(u)
		}
	}
	refs := s.loadShared(u)
	defer releaseRefs(refs)
	for _, r := range refs {
		if r.entry.ID == entryID {
			return os.Remove(s.sharedPath(entryID))
		}
	}
	return ErrNoEntry
}

func (s *Service) Share(id, entryID string, recipients []string) (Entry, error) {
	set := map[string][]byte{}
	for _, r := range recipients {
		if r == id || r == "" {
			continue
		}
		p, err := s.store.Get(r)
		if err != nil {
			return Entry{}, fmt.Errorf("cannot share with %s: %w", r, err)
		}
		if !p.IsEncrypted() || p.Security.Crypto == nil || len(p.Security.Crypto.SharePublic) != 32 {
			return Entry{}, fmt.Errorf("cannot share with %q: logins can only be shared with encrypted profiles", p.Name)
		}
		set[r] = p.Security.Crypto.SharePublic
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return Entry{}, err
	}
	var entry Entry
	personal := -1
	var shared *sharedRef
	for i := range u.body.Entries {
		if u.body.Entries[i].ID == entryID {
			entry, personal = u.body.Entries[i], i
		}
	}
	refs := s.loadShared(u)
	defer releaseRefs(refs)
	if personal < 0 {
		for i := range refs {
			if refs[i].entry.ID == entryID {
				entry, shared = refs[i].entry, &refs[i]
			}
		}
	}
	if personal < 0 && shared == nil {
		return Entry{}, ErrNoEntry
	}
	entry.Updated = time.Now().UTC()

	if len(set) == 0 {
		if shared != nil {
			entry.Owner, entry.SharedWith = "", nil
			u.body.Entries = append(u.body.Entries, entry)
			if err := s.save(u); err != nil {
				return Entry{}, err
			}
			if err := os.Remove(s.sharedPath(entryID)); err != nil {
				return Entry{}, err
			}
		}
		entry.Owner, entry.SharedWith = "", nil
		return stripSecrets(entry), nil
	}

	key := vault.RandomBytes(vault.KeySize)
	defer clear(key)
	owner := id
	if shared != nil {
		owner = shared.file.Owner
	}
	f := &sharedFile{Format: "keycat-shared", Version: 1, ID: entryID, Owner: owner, Recipients: map[string]shareSlot{}}
	set[id] = u.priv.PublicKey().Bytes()
	for r, pub := range set {
		slot, err := wrapSlot(pub, entryID, r, key)
		if err != nil {
			return Entry{}, err
		}
		f.Recipients[r] = slot
	}
	if err := s.writeShared(f, key, entry); err != nil {
		return Entry{}, err
	}
	if personal >= 0 {
		u.body.Entries = append(u.body.Entries[:personal], u.body.Entries[personal+1:]...)
		if err := s.save(u); err != nil {
			return Entry{}, err
		}
	}
	entry.Owner, entry.SharedWith = owner, recipientsOf(f)
	return stripSecrets(entry), nil
}

func (s *Service) Match(id, origin string) ([]Entry, error) {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return nil, err
	}
	type scored struct {
		e     Entry
		score int
	}
	var hits []scored
	for _, e := range u.body.Entries {
		if sc := matchScore(&e, origin); sc > 0 {
			hits = append(hits, scored{e, sc})
		}
	}
	refs := s.loadShared(u)
	defer releaseRefs(refs)
	for _, r := range refs {
		if sc := matchScore(&r.entry, origin); sc > 0 {
			hits = append(hits, scored{r.entry, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		ti, tj := hits[i].e.Updated, hits[j].e.Updated
		if hits[i].e.LastUsed != nil {
			ti = *hits[i].e.LastUsed
		}
		if hits[j].e.LastUsed != nil {
			tj = *hits[j].e.LastUsed
		}
		return ti.After(tj)
	})
	out := make([]Entry, len(hits))
	for i, h := range hits {
		out[i] = h.e
	}
	return out, nil
}

func (s *Service) MarkUsed(id, entryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return
	}
	for i := range u.body.Entries {
		if u.body.Entries[i].ID == entryID {
			now := time.Now().UTC()
			u.body.Entries[i].LastUsed = &now
			_ = s.save(u)
			return
		}
	}
}

type ImportedLogin struct {
	Origin   string `json:"origin"`
	Username string `json:"username"`
	Password string `json:"password"`
	Title    string `json:"title,omitempty"`
}

type ImportResult struct {
	Created   int   `json:"created"`
	Unchanged int   `json:"unchanged"`
	Skipped   []int `json:"skipped"`
}

func (s *Service) ImportLogins(id string, logins []ImportedLogin) (ImportResult, error) {
	res := ImportResult{Skipped: []int{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return res, err
	}
	now := time.Now().UTC()
	changed := false
	for i, l := range logins {
		origin, err := NormalizeOrigin(l.Origin)
		if err != nil || l.Password == "" {
			res.Skipped = append(res.Skipped, i)
			continue
		}
		username := strings.TrimSpace(l.Username)
		if hasLogin(u.body.Entries, origin, username, l.Password) {
			res.Unchanged++
			continue
		}
		e := Entry{Title: l.Title, Origins: []string{origin}, Username: username, Password: l.Password}
		if err := e.normalize(); err != nil {
			res.Skipped = append(res.Skipped, i)
			continue
		}
		e.ID, e.Created, e.Updated = newEntryID(), now, now
		u.body.Entries = append(u.body.Entries, e)
		res.Created++
		changed = true
	}
	if !changed {
		return res, nil
	}
	return res, s.save(u)
}

func hasLogin(entries []Entry, origin, username, password string) bool {
	for _, e := range entries {
		if e.Username == username && e.Password == password && containsString(e.Origins, origin) {
			return true
		}
	}
	return false
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Service) SaveFromBrowser(id, origin, username, password string) (Entry, string, error) {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return Entry{}, "", err
	}
	matches, err := s.Match(id, origin)
	if err != nil {
		return Entry{}, "", err
	}
	for _, m := range matches {
		if m.Username == username {
			if m.Password == password {
				return stripSecrets(m), "unchanged", nil
			}
			m.Password = password
			saved, err := s.Save(id, m)
			return saved, "updated", err
		}
	}
	saved, err := s.Save(id, Entry{Origins: []string{origin}, Username: username, Password: password})
	return saved, "created", err
}

func (s *Service) NeverSave(id, origin string) error {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return err
	}
	for _, o := range u.body.NeverSave {
		if o == origin {
			return nil
		}
	}
	u.body.NeverSave = append(u.body.NeverSave, origin)
	return s.save(u)
}

func (s *Service) IsNeverSave(id, origin string) bool {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return false
	}
	for _, o := range u.body.NeverSave {
		if o == origin {
			return true
		}
	}
	return false
}

func (s *Service) Generator(id string) (GeneratorOptions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return DefaultGenerator(), err
	}
	return u.body.Generator, nil
}

func (s *Service) SetGenerator(id string, o GeneratorOptions) (GeneratorOptions, error) {
	o.normalize()
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return o, err
	}
	u.body.Generator = o
	return o, s.save(u)
}

func (s *Service) Identity(id string, fresh bool) (identity.Identity, identity.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return identity.Identity{}, identity.Settings{}, err
	}
	if fresh {
		return identity.Generate(u.body.IdentitySettings), u.body.IdentitySettings, nil
	}
	if u.body.Identity == nil {
		ident := identity.Generate(u.body.IdentitySettings)
		u.body.Identity = &ident
		if err := s.save(u); err != nil {
			return identity.Identity{}, identity.Settings{}, err
		}
	}
	return *u.body.Identity, u.body.IdentitySettings, nil
}

func (s *Service) RegenerateIdentity(id string) (identity.Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return identity.Identity{}, err
	}
	ident := identity.Generate(u.body.IdentitySettings)
	u.body.Identity = &ident
	return ident, s.save(u)
}

func (s *Service) SetIdentitySettings(id string, settings identity.Settings) (identity.Identity, error) {
	if err := settings.Validate(); err != nil {
		return identity.Identity{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, err := s.getLocked(id)
	if err != nil {
		return identity.Identity{}, err
	}
	u.body.IdentitySettings = settings
	ident := identity.Generate(settings)
	u.body.Identity = &ident
	return ident, s.save(u)
}

func (s *Service) ForgetProfile(id string) {
	s.Detach(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	files, _ := filepath.Glob(filepath.Join(s.sharedDir, "*"+sharedSuffix))
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var f sharedFile
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if _, ok := f.Recipients[id]; !ok {
			continue
		}
		delete(f.Recipients, id)
		if len(f.Recipients) == 0 {
			_ = vault.WipeDir(path, nil)
			_ = os.Remove(path)
			continue
		}
		if out, err := json.Marshal(f); err == nil {
			_ = profile.WriteFileAtomic(path, out)
		}
	}
}
