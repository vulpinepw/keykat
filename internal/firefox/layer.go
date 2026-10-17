package firefox

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const stampFile = "layer.json"

type layerStamp struct {
	Hash    string `json:"hash"`
	BuildID string `json:"buildId"`
}

func LayerHash(layer fs.FS) (string, error) {
	var files []string
	err := fs.WalkDir(layer, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		data, err := fs.ReadFile(layer, f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", f, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

func LayerCurrent(c *Core, layer fs.FS) bool {
	hash, err := LayerHash(layer)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(c.Dir, "keycat", stampFile))
	if err != nil {
		return false
	}
	var st layerStamp
	return json.Unmarshal(data, &st) == nil && st.Hash == hash && st.BuildID == c.BuildID
}

func ApplyLayer(c *Core, layer fs.FS) error {
	hash, err := LayerHash(layer)
	if err != nil {
		return err
	}
	if err := copyTree(layer, "core", c.Dir); err != nil {
		return fmt.Errorf("installing autoconfig: %w", err)
	}
	pkg := filepath.Join(c.Dir, "keycat")
	if err := os.RemoveAll(pkg); err != nil {
		return fmt.Errorf("removing old Keycat package: %w", err)
	}
	if err := copyTree(layer, "keycat", pkg); err != nil {
		return fmt.Errorf("installing Keycat package: %w", err)
	}
	icons := filepath.Join(c.Dir, "browser", "chrome", "icons", "default")
	if err := copyTree(layer, "branding/icons", icons); err != nil {
		return fmt.Errorf("installing window icons: %w", err)
	}
	art, err := LoadArt(layer, "branding/art")
	if err != nil {
		return fmt.Errorf("reading the artwork rules: %w", err)
	}
	if err := PatchOmni(filepath.Join(c.Dir, "browser", "omni.ja"), layer, "branding/omni", art.For("browser")); err != nil {
		return fmt.Errorf("applying branding: %w", err)
	}
	if err := PatchOmni(filepath.Join(c.Dir, "omni.ja"), layer, "branding/gre", art.For("gre")); err != nil {
		return fmt.Errorf("applying branding to the engine: %w", err)
	}
	stamp, _ := json.Marshal(layerStamp{Hash: hash, BuildID: c.BuildID})
	return os.WriteFile(filepath.Join(pkg, stampFile), stamp, 0o644)
}

type Art struct {
	rules []artRule
	files map[string][]byte
}

type artRule struct {
	archive, kind string
	re            *regexp.Regexp
}

func LoadArt(layer fs.FS, root string) (*Art, error) {
	art := &Art{files: map[string][]byte{}}
	data, err := fs.ReadFile(layer, path.Join(root, "rules.txt"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return art, nil
		}
		return nil, err
	}
	for n, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("rules.txt line %d: want archive, kind and pattern", n+1)
		}
		re, err := regexp.Compile(fields[2])
		if err != nil {
			return nil, fmt.Errorf("rules.txt line %d: %w", n+1, err)
		}
		art.rules = append(art.rules, artRule{archive: fields[0], kind: fields[1], re: re})
	}
	entries, err := fs.ReadDir(layer, root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "rules.txt" {
			continue
		}
		b, err := fs.ReadFile(layer, path.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		art.files[e.Name()] = b
	}
	return art, nil
}

func (a *Art) For(archive string) func(name string) []byte {
	return func(name string) []byte {
		for _, r := range a.rules {
			if r.archive != archive || !r.re.MatchString(name) {
				continue
			}
			var file string
			switch ext := strings.ToLower(path.Ext(name)); ext {
			case ".svg":
				file = r.kind + ".svg"
			case ".png", ".gif", ".webp", ".jpg", ".jpeg", ".avif":
				file = r.kind + ".png"
			case ".ico":
				file = "icon.ico"
			case ".webm", ".mp4":
				return []byte{}
			default:
				return nil
			}
			if data, ok := a.files[file]; ok {
				return data
			}
			return nil
		}
		return nil
	}
}

func copyTree(src fs.FS, root, dst string) error {
	if _, err := fs.Stat(src, root); err != nil {
		return nil
	}
	return fs.WalkDir(src, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func PatchOmni(omniPath string, src fs.FS, root string, art func(name string) []byte) error {
	replacements := map[string][]byte{}
	overrides := map[string][]byte{}
	if _, err := fs.Stat(src, root); err == nil {
		err := fs.WalkDir(src, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(src, p)
			if err != nil {
				return err
			}
			name := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
			if target, ok := strings.CutSuffix(name, ".override"); ok {
				overrides[target] = data
			} else {
				replacements[name] = data
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if art == nil {
		if len(replacements)+len(overrides) == 0 {
			return nil
		}
		art = func(string) []byte { return nil }
	}

	r, err := zip.OpenReader(omniPath)
	if err != nil {
		return err
	}
	tmp := omniPath + ".keycat-tmp"
	out, err := os.Create(tmp)
	if err != nil {
		r.Close()
		return err
	}
	cleanup := func(e error) error {
		out.Close()
		r.Close()
		os.Remove(tmp)
		return e
	}
	w := zip.NewWriter(out)
	used := map[string]bool{}
	writeEntry := func(name string, modified time.Time, data []byte) error {
		fw, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified})
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	}
	for _, f := range r.File {
		if entries, _ := matchReplacement(f.Name, overrides); entries != nil {
			original, err := readZipFile(f)
			if err != nil {
				return cleanup(err)
			}
			merged := MergeFTL(original, entries)
			if strings.HasSuffix(f.Name, ".properties") {
				merged = MergeProperties(original, entries)
			}
			if err := writeEntry(f.Name, f.Modified, merged); err != nil {
				return cleanup(err)
			}
			continue
		}
		if data, pattern := matchReplacement(f.Name, replacements); data != nil {
			used[pattern] = true
			if err := writeEntry(f.Name, f.Modified, data); err != nil {
				return cleanup(err)
			}
			continue
		}
		if data := art(f.Name); data != nil {
			if err := writeEntry(f.Name, f.Modified, data); err != nil {
				return cleanup(err)
			}
			continue
		}
		raw, err := f.OpenRaw()
		if err != nil {
			return cleanup(err)
		}
		fw, err := w.CreateRaw(&f.FileHeader)
		if err != nil {
			return cleanup(err)
		}
		if _, err := io.Copy(fw, raw); err != nil {
			return cleanup(err)
		}
	}
	for pattern, data := range replacements {
		if used[pattern] || strings.Contains(pattern, "_locale_") {
			continue
		}
		fw, err := w.CreateHeader(&zip.FileHeader{Name: pattern, Method: zip.Deflate})
		if err != nil {
			return cleanup(err)
		}
		if _, err := fw.Write(data); err != nil {
			return cleanup(err)
		}
	}
	if err := w.Close(); err != nil {
		return cleanup(err)
	}
	if err := out.Close(); err != nil {
		r.Close()
		os.Remove(tmp)
		return err
	}
	r.Close()
	return os.Rename(tmp, omniPath)
}

func MergeProperties(original, overrides []byte) []byte {
	repl := map[string]string{}
	var order []string
	for _, e := range propertyEntries(overrides) {
		if e.key != "" {
			if _, seen := repl[e.key]; !seen {
				order = append(order, e.key)
			}
			repl[e.key] = e.text
		}
	}
	var b strings.Builder
	used := map[string]bool{}
	for _, e := range propertyEntries(original) {
		if text, ok := repl[e.key]; ok && e.key != "" {
			b.WriteString(text)
			used[e.key] = true
			continue
		}
		b.WriteString(e.text)
	}
	for _, key := range order {
		if !used[key] {
			if s := b.String(); s != "" && !strings.HasSuffix(s, "\n") {
				b.WriteString("\n")
			}
			b.WriteString(repl[key])
		}
	}
	return []byte(b.String())
}

type propertyEntry struct {
	key  string
	text string
}

func propertyEntries(src []byte) []propertyEntry {
	lines := strings.SplitAfter(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	var out []propertyEntry
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == '!' {
			out = append(out, propertyEntry{text: line})
			continue
		}
		end := strings.IndexAny(trimmed, "=:")
		if end < 0 {
			out = append(out, propertyEntry{text: line})
			continue
		}
		key := strings.TrimSpace(trimmed[:end])
		text := line
		for strings.HasSuffix(strings.TrimRight(lines[i], "\r\n"), "\\") && i+1 < len(lines) {
			i++
			text += lines[i]
		}
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		out = append(out, propertyEntry{key: key, text: text})
	}
	return out
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

var ftlEntryStart = regexp.MustCompile(`^(-?[A-Za-z][A-Za-z0-9_-]*)[ \t]*=`)

func ftlEntries(src []byte) []ftlEntry {
	lines := strings.SplitAfter(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	blank := func(l string) bool { return strings.TrimSpace(l) == "" }
	indented := func(l string) bool { return !blank(l) && (l[0] == ' ' || l[0] == '\t') }
	var out []ftlEntry
	for i := 0; i < len(lines); i++ {
		m := ftlEntryStart.FindStringSubmatch(lines[i])
		if m == nil {
			out = append(out, ftlEntry{text: lines[i]})
			continue
		}
		end := i + 1
		for {
			for end < len(lines) && indented(lines[end]) {
				end++
			}
			j := end
			for j < len(lines) && blank(lines[j]) {
				j++
			}
			if j > end && j < len(lines) && indented(lines[j]) {
				end = j
				continue
			}
			break
		}
		text := strings.Join(lines[i:end], "")
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		out = append(out, ftlEntry{id: m[1], text: text})
		i = end - 1
	}
	return out
}

type ftlEntry struct {
	id   string
	text string
}

func MergeFTL(original, overrides []byte) []byte {
	repl := map[string]string{}
	var order []string
	for _, e := range ftlEntries(overrides) {
		if e.id != "" {
			if _, seen := repl[e.id]; !seen {
				order = append(order, e.id)
			}
			repl[e.id] = e.text
		}
	}
	var b strings.Builder
	used := map[string]bool{}
	for _, e := range ftlEntries(original) {
		if text, ok := repl[e.id]; ok && e.id != "" {
			b.WriteString(text)
			used[e.id] = true
			continue
		}
		b.WriteString(e.text)
	}
	for _, id := range order {
		if !used[id] {
			if s := b.String(); s != "" && !strings.HasSuffix(s, "\n\n") {
				b.WriteString("\n")
			}
			b.WriteString(repl[id])
		}
	}
	return []byte(b.String())
}

func matchReplacement(name string, repl map[string][]byte) ([]byte, string) {
	if data, ok := repl[name]; ok {
		return data, name
	}
	for pattern, data := range repl {
		if !strings.Contains(pattern, "_locale_") {
			continue
		}
		if ok, _ := path.Match(strings.ReplaceAll(pattern, "_locale_", "*"), name); ok {
			return data, pattern
		}
	}
	return nil, ""
}
