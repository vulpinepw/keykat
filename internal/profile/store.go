package profile

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	"keycat/internal/vault"
)

const ManifestVersion = 1

var ErrNotFound = errors.New("profile not found")

var idRe = regexp.MustCompile(`^p[0-9a-f]{12}$`)

func NewID() string { return "p" + hex.EncodeToString(vault.RandomBytes(6)) }

func ValidID(id string) bool { return idRe.MatchString(id) }

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) Dir(id string) string { return filepath.Join(s.root, id) }

func (s *Store) VaultPath(id string) string { return filepath.Join(s.Dir(id), "vault.kcv") }

func (s *Store) PlainDataDir(id string) string { return filepath.Join(s.Dir(id), "data") }

func (s *Store) ManifestPath(id string) string { return filepath.Join(s.Dir(id), "profile.json") }

func (s *Store) manifestPath(id string) string { return s.ManifestPath(id) }

func (s *Store) List() ([]*Profile, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var out []*Profile
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		p, err := s.Get(e.Name())
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

func (s *Store) Get(id string) (*Profile, error) {
	if !ValidID(id) {
		return nil, ErrNotFound
	}
	path := s.manifestPath(id)
	p, err := ReadManifest(path, id)
	if err == nil {
		return p, nil
	}
	if prev, perr := ReadManifest(path+".bak", id); perr == nil {
		return prev, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return nil, err
}

func ReadManifest(path, id string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("profile %s: corrupt manifest: %w", id, err)
	}
	if p.ID != id {
		return nil, fmt.Errorf("profile %s: manifest id mismatch", id)
	}
	p.Appearance.Normalize()
	return &p, nil
}

func (s *Store) Save(p *Profile) error {
	if !ValidID(p.ID) {
		return fmt.Errorf("invalid profile id %q", p.ID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.Dir(p.ID), 0o700); err != nil {
		return err
	}
	p.Version = ManifestVersion
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	path := s.manifestPath(p.ID)
	if _, err := ReadManifest(path, p.ID); err == nil {
		_ = os.Remove(path + ".bak")
		_ = vault.LinkOrCopy(path, path+".bak")
	}
	return WriteFileAtomic(path, data)
}

func (s *Store) Delete(id string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	return vault.WipeDir(s.Dir(id), nil)
}

func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := vault.WriteFileSynced(tmp, data); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := vault.RenameDurable(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
