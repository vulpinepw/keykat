package main

import (
	"archive/tar"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type Manifest struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
}

const manifestName = ".keycat-payload.json"

func main() {
	src := flag.String("src", "", "folder to pack")
	out := flag.String("out", "", "payload file to write")
	version := flag.String("version", "", "Keycat version")
	flag.Parse()
	if *src == "" || *out == "" || *version == "" {
		fmt.Fprintln(os.Stderr, "usage: mkpayload -src DIR -out FILE -version X.Y.Z")
		os.Exit(2)
	}
	start := time.Now()
	m, size, err := pack(*src, *out, *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkpayload:", err)
		os.Exit(1)
	}
	fmt.Printf("payload: %d files, %d MB -> %d MB in %s\n", m.Files, m.Bytes>>20, size>>20, time.Since(start).Round(time.Second))
}

func skip(rel string, isDir bool) bool {
	top := strings.ToLower(strings.SplitN(rel, "/", 2)[0])
	if top == "data" || (!isDir && top == "portable") {
		return true
	}
	return !isDir && strings.HasSuffix(strings.ToLower(rel), ".log")
}

func pack(src, outPath, version string) (Manifest, int64, error) {
	m := Manifest{Product: "Keycat", Version: version}
	type entry struct {
		rel  string
		path string
		info fs.FileInfo
	}
	var entries []entry
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == src {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skip(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, entry{rel, p, info})
		if !d.IsDir() {
			m.Files++
			m.Bytes += info.Size()
		}
		return nil
	})
	if err != nil {
		return m, 0, err
	}
	if _, err := os.Stat(filepath.Join(src, "Keycat.exe")); err != nil {
		return m, 0, fmt.Errorf("%s has no Keycat.exe", src)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return m, 0, err
	}
	defer f.Close()
	zw, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.SpeedBetterCompression), zstd.WithEncoderConcurrency(runtime.NumCPU()))
	if err != nil {
		return m, 0, err
	}
	tw := tar.NewWriter(zw)
	js, _ := json.Marshal(m)
	if err := tw.WriteHeader(&tar.Header{Name: manifestName, Mode: 0o644, Size: int64(len(js)), ModTime: time.Now()}); err != nil {
		return m, 0, err
	}
	if _, err := tw.Write(js); err != nil {
		return m, 0, err
	}
	for _, e := range entries {
		hdr := &tar.Header{Name: e.rel, ModTime: e.info.ModTime(), Mode: 0o644}
		if e.info.IsDir() {
			hdr.Typeflag, hdr.Name, hdr.Mode = tar.TypeDir, e.rel+"/", 0o755
			if err := tw.WriteHeader(hdr); err != nil {
				return m, 0, err
			}
			continue
		}
		hdr.Typeflag, hdr.Size = tar.TypeReg, e.info.Size()
		if err := tw.WriteHeader(hdr); err != nil {
			return m, 0, err
		}
		in, err := os.Open(e.path)
		if err != nil {
			return m, 0, err
		}
		_, err = io.Copy(tw, in)
		in.Close()
		if err != nil {
			return m, 0, err
		}
	}
	if err := tw.Close(); err != nil {
		return m, 0, err
	}
	if err := zw.Close(); err != nil {
		return m, 0, err
	}
	if err := f.Sync(); err != nil {
		return m, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		return m, 0, err
	}
	return m, st.Size(), nil
}
