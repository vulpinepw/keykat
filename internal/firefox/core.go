package firefox

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Core struct {
	Dir     string `json:"dir"`
	Exe     string `json:"exe"`
	Version string `json:"version"`
	BuildID string `json:"buildId"`
	Channel string `json:"channel"`
}

var ErrNoCore = errors.New("the browser engine is not installed")

func DetectCore(dir string) (*Core, error) {
	exe := filepath.Join(dir, "firefox.exe")
	if _, err := os.Stat(exe); err != nil {
		return nil, ErrNoCore
	}
	c := &Core{Dir: dir, Exe: exe}
	if ini, err := readINI(filepath.Join(dir, "application.ini")); err == nil {
		c.Version = ini["App.Version"]
		c.BuildID = ini["App.BuildID"]
	}
	c.Channel = readChannel(filepath.Join(dir, "defaults", "pref", "channel-prefs.js"))
	return c, nil
}

func readINI(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			out[section+"."+strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out, sc.Err()
}

func readChannel(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(data)
	if i := strings.Index(s, `"app.update.channel"`); i >= 0 {
		rest := s[i+len(`"app.update.channel"`):]
		if a := strings.Index(rest, `"`); a >= 0 {
			if b := strings.Index(rest[a+1:], `"`); b >= 0 {
				return rest[a+1 : a+1+b]
			}
		}
	}
	return ""
}

var skipWhenCopying = map[string]bool{
	"uninstall":                        true,
	"maintenanceservice.exe":           true,
	"maintenanceservice_installer.exe": true,
	"default-browser-agent.exe":        true,
	"install.log":                      true,
	"installation_telemetry.json":      true,
	"update_telemetry.json":            true,
	"distribution":                     true,
	"postSigningData":                  true,
}

func CopyCore(src, dst string, progress func(done, total int64)) error {
	if _, err := DetectCore(src); err != nil {
		return fmt.Errorf("%s does not contain a Firefox installation", src)
	}
	var total int64
	_ = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	var done int64
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		if skipWhenCopying[top] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		n, err := copyFile(p, target)
		done += n
		if progress != nil {
			progress(done, total)
		}
		return err
	})
}

func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return n, err
}
