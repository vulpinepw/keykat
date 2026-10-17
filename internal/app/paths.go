package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"keycat/internal/profile"
)

type Paths struct {
	ExeDir   string `json:"exeDir"`
	CoreDir  string `json:"coreDir"`
	DataDir  string `json:"dataDir"`
	LocalDir string `json:"localDir"`
	Profiles string `json:"profiles"`
	Sessions string `json:"sessions"`
	Hub      string `json:"hub"`
	Logs     string `json:"logs"`
	Portable bool   `json:"portable"`
}

func ResolvePaths() (*Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exe)
	p := &Paths{ExeDir: exeDir, CoreDir: filepath.Join(exeDir, "core")}
	if v := os.Getenv("KEYCAT_CORE_DIR"); v != "" {
		p.CoreDir = v
	}
	_, statErr := os.Stat(filepath.Join(exeDir, "portable"))
	switch {
	case os.Getenv("KEYCAT_DATA_DIR") != "":
		p.DataDir = os.Getenv("KEYCAT_DATA_DIR")
		p.LocalDir = filepath.Join(p.DataDir, "Local")
	case statErr == nil:
		p.Portable = true
		p.DataDir = filepath.Join(exeDir, "Data")
		p.LocalDir = filepath.Join(p.DataDir, "Local")
	default:
		roaming, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		local, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		p.DataDir = filepath.Join(roaming, "Keycat")
		p.LocalDir = filepath.Join(local, "Keycat")
	}
	p.Profiles = filepath.Join(p.DataDir, "Profiles")
	p.Sessions = filepath.Join(p.LocalDir, "Sessions")
	p.Hub = filepath.Join(p.LocalDir, "Hub")
	p.Logs = filepath.Join(p.LocalDir, "Logs")
	for _, d := range []string{p.DataDir, p.LocalDir, p.Profiles, p.Sessions, p.Hub, p.Logs} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	return p, nil
}

const settingsVersion = 2

type Settings struct {
	Version       int    `json:"version"`
	Theme         string `json:"theme"`
	AfterOpen     string `json:"afterOpen"`
	SecureWipe    bool   `json:"secureWipe"`
	UpdateChannel string `json:"updateChannel"`
	CheckUpdates  bool   `json:"checkUpdates"`
	Onboarded     bool   `json:"onboarded"`
}

func DefaultSettings() Settings {
	return Settings{Version: settingsVersion, Theme: "dark", AfterOpen: "close", SecureWipe: true, UpdateChannel: "release", CheckUpdates: true}
}

func (s *Settings) Validate() error {
	switch s.Theme {
	case "dark", "light", "system":
	default:
		return errors.New("theme must be dark, light or system")
	}
	switch s.AfterOpen {
	case "keep", "minimize", "close":
	default:
		return errors.New("afterOpen must be keep, minimize or close")
	}
	switch s.UpdateChannel {
	case "release", "esr":
	default:
		return errors.New("updateChannel must be release or esr")
	}
	return nil
}

type SettingsStore struct {
	path string
	mu   sync.Mutex
	cur  Settings
}

func LoadSettings(dataDir string) *SettingsStore {
	s := &SettingsStore{path: filepath.Join(dataDir, "settings.json"), cur: DefaultSettings()}
	data, err := os.ReadFile(s.path)
	if err == nil {
		loaded := DefaultSettings()
		loaded.Version = 0
		if json.Unmarshal(data, &loaded) == nil && loaded.Validate() == nil {
			if loaded.Version < 2 {
				loaded.AfterOpen = "close"
			}
			loaded.Version = settingsVersion
			s.cur = loaded
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		_ = err
	}
	return s
}

func (s *SettingsStore) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func (s *SettingsStore) Set(v Settings) error {
	if err := v.Validate(); err != nil {
		return err
	}
	v.Version = settingsVersion
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := profile.WriteFileAtomic(s.path, data); err != nil {
		return err
	}
	s.cur = v
	return nil
}
