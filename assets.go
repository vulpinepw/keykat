package keycat

import (
	"embed"
	"io/fs"
	"path"
	"testing/fstest"
)

//go:embed all:hub
var hubFiles embed.FS

//go:embed all:browser
var browserFiles embed.FS

func HubFS() fs.FS {
	sub, err := fs.Sub(hubFiles, "hub")
	if err != nil {
		panic(err)
	}
	return sub
}

func LayerFS() fs.FS {
	browser, err := fs.Sub(browserFiles, "browser")
	if err != nil {
		panic(err)
	}
	layer := fstest.MapFS{}
	add := func(src fs.FS, prefix string) {
		err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(src, p)
			if err != nil {
				return err
			}
			layer[path.Join(prefix, p)] = &fstest.MapFile{Data: data, Mode: 0o644}
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	add(browser, ".")
	add(HubFS(), "keycat/content/hub")
	return layer
}
