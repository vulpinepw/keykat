package main

import (
	"archive/tar"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/gzip"

	"keycat"
	"keycat/internal/firefox"
)

func main() {
	tarball := flag.String("firefox", "", "Firefox for Linux, the .tar.xz from archive.mozilla.org")
	program := flag.String("keycat", "", "the keycat program built for Linux")
	root := flag.String("root", ".", "the Keycat source folder")
	dir := flag.String("dir", "", "the folder to put Keycat together in (replaced)")
	out := flag.String("out", "", "the .tar.gz to write")
	flag.Parse()
	if *tarball == "" || *program == "" || *dir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: mktarball -firefox FIREFOX.tar.xz -keycat PROGRAM -root DIR -dir DIR -out FILE.tar.gz")
		os.Exit(2)
	}
	if err := assemble(*tarball, *program, *root, *dir); err != nil {
		fmt.Fprintln(os.Stderr, "mktarball:", err)
		os.Exit(1)
	}
	fmt.Println("packing", *out)
	if err := pack(*dir, *out); err != nil {
		fmt.Fprintln(os.Stderr, "mktarball:", err)
		os.Exit(1)
	}
}

func assemble(tarball, program, root, dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "keycat-tarball-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	fmt.Println("unpacking", filepath.Base(tarball))
	if msg, err := exec.Command("tar", "-xf", tarball, "-C", work).CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %v %s", err, bytes.TrimSpace(msg))
	}
	core := filepath.Join(dir, "core")
	if err := firefox.CopyCore(filepath.Join(work, "firefox"), core, nil); err != nil {
		return err
	}
	c, err := firefox.InspectCore(core, "firefox")
	if err != nil {
		return fmt.Errorf("%s is not Firefox for Linux", tarball)
	}
	fmt.Printf("installing the Keycat layer into Firefox %s (%s)\n", c.Version, c.BuildID)
	if err := firefox.ApplyLayer(c, keycat.LayerFS()); err != nil {
		return err
	}

	icons, err := filepath.Glob(filepath.Join(root, "linux", "icons", "keycat-*.png"))
	if err != nil || len(icons) == 0 {
		return fmt.Errorf("no icons in %s", filepath.Join(root, "linux", "icons"))
	}
	copies := map[string]string{
		"keycat":           program,
		"install.sh":       filepath.Join(root, "linux", "install.sh"),
		"uninstall.sh":     filepath.Join(root, "linux", "uninstall.sh"),
		"icons/keycat.svg": filepath.Join(root, "assets", "brand", "keycat-tile.svg"),
	}
	for _, p := range icons {
		copies["icons/"+filepath.Base(p)] = p
	}
	for name, src := range copies {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, name), data); err != nil {
			return err
		}
	}
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "LICENSE.txt"), unixText(license)); err != nil {
		return err
	}
	notices, err := thirdPartyNotices(filepath.Join(root, "licenses"))
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, "THIRD-PARTY-NOTICES.txt"), notices)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, fs.FileMode(mode(data)))
}

func pack(dir, out string) error {
	var names []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && p != dir {
			rel, _ := filepath.Rel(dir, p)
			names = append(names, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		return err
	}
	sort.Strings(names)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out + ".tmp")
	if err != nil {
		return err
	}
	defer os.Remove(out + ".tmp")
	if err := writeArchive(f, dir, names); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(out+".tmp", out)
}

func writeArchive(w io.Writer, dir string, names []string) error {
	gz, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(gz)
	now := time.Now().Truncate(time.Second)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "keycat/", Mode: 0o755, ModTime: now}); err != nil {
		return err
	}
	for _, name := range names {
		p := filepath.Join(dir, filepath.FromSlash(name))
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "keycat/" + name + "/", Mode: 0o755, ModTime: now}); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		hdr := &tar.Header{Typeflag: tar.TypeReg, Name: "keycat/" + name, Mode: mode(data), Size: int64(len(data)), ModTime: now}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func mode(data []byte) int64 {
	if bytes.HasPrefix(data, []byte("\x7fELF")) || bytes.HasPrefix(data, []byte("#!")) {
		return 0o755
	}
	return 0o644
}

func thirdPartyNotices(dir string) ([]byte, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("no licenses in %s", dir)
	}
	sort.Strings(paths)
	var parts []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(p), ".txt")
		parts = append(parts, "==== "+name+" ====\n\n"+string(unixText(data)))
	}
	return []byte(strings.Join(parts, "\n\n")), nil
}

func unixText(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}
