package firefox

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestMergeProperties(t *testing.T) {
	original := "# comment\r\nfileNotFound=Firefox can’t find the file at %S.\r\nlong=First \\\r\n  continued\r\nkeep=Keep me\r\n"
	overrides := "# Keycat\nfileNotFound=Keycat can't find the file at %S.\nlong=Short now\nadded=New\n"
	want := "# comment\nfileNotFound=Keycat can't find the file at %S.\nlong=Short now\nkeep=Keep me\nadded=New\n"
	if got := string(MergeProperties([]byte(original), []byte(overrides))); got != want {
		t.Fatalf("MergeProperties:\n%q\nwant:\n%q", got, want)
	}
}

func TestPatchOmniArtAndOverrides(t *testing.T) {
	dir := t.TempDir()
	omni := filepath.Join(dir, "omni.ja")
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range map[string]string{
		"chrome/toolkit/skin/classic/global/illustrations/kit-happy.svg":    "<svg>fox</svg>",
		"chrome/toolkit/skin/classic/global/illustrations/shield-check.svg": "<svg>shield</svg>",
		"chrome/en-US/locale/en-US/global/appstrings.properties":            "networkProtocolError=Firefox has experienced an error.\nother=Other\n",
		"assets/kit-default.webm":                                           "video",
	} {
		f, _ := w.Create(name)
		f.Write([]byte(data))
	}
	w.Close()
	if err := os.WriteFile(omni, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	layer := fstest.MapFS{
		"branding/art/rules.txt":        {Data: []byte("# rules\ngre illustration ^chrome/toolkit/skin/classic/global/illustrations/kit-[^/]*\\.svg$\ngre illustration ^assets/kit[^/]*\\.webm$\n")},
		"branding/art/illustration.svg": {Data: []byte("<svg>keycat</svg>")},
		"branding/gre/chrome/_locale_/locale/_locale_/global/appstrings.properties.override": {Data: []byte("networkProtocolError=Keycat has experienced an error.\n")},
	}
	art, err := LoadArt(layer, "branding/art")
	if err != nil {
		t.Fatal(err)
	}
	if err := PatchOmni(omni, layer, "branding/gre", art.For("gre")); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(omni)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got := map[string]string{}
	for _, f := range r.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		got[f.Name] = string(data)
	}
	want := map[string]string{
		"chrome/toolkit/skin/classic/global/illustrations/kit-happy.svg":    "<svg>keycat</svg>",
		"chrome/toolkit/skin/classic/global/illustrations/shield-check.svg": "<svg>shield</svg>",
		"chrome/en-US/locale/en-US/global/appstrings.properties":            "networkProtocolError=Keycat has experienced an error.\nother=Other\n",
		"assets/kit-default.webm":                                           "",
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %q, want %q", name, got[name], w)
		}
	}
}

func TestMergeFTL(t *testing.T) {
	original := "## Section\n\n" +
		"pane-general-title = General\n\n" +
		"account-sync-section =\n    .heading = Account and sync\n\n" +
		"long-message =\n    First paragraph.\n\n    Second paragraph.\n\n" +
		"pane-account-sync-title2 = Account and sync\n    .title = Account and sync\n\n" +
		"last = Last\n"
	overrides := "# Keycat\n" +
		"pane-account-sync-title2 = General\n    .title = General\n\n" +
		"account-sync-section =\n    .heading = General\n\n" +
		"long-message = One line now.\n" +
		"keycat-new = Added\n"
	want := "## Section\n\n" +
		"pane-general-title = General\n\n" +
		"account-sync-section =\n    .heading = General\n\n" +
		"long-message = One line now.\n\n" +
		"pane-account-sync-title2 = General\n    .title = General\n\n" +
		"last = Last\n\n" +
		"keycat-new = Added\n"
	if got := string(MergeFTL([]byte(original), []byte(overrides))); got != want {
		t.Fatalf("MergeFTL:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeFTLTermsAndCRLF(t *testing.T) {
	original := "-brand-short-name = Firefox\r\n-vendor-short-name = Mozilla\r\ntrademarkInfo = Firefox is a trademark.\r\n"
	overrides := "-brand-short-name = Keycat\ntrademarkInfo = Keycat is based on Firefox.\n"
	want := "-brand-short-name = Keycat\n-vendor-short-name = Mozilla\ntrademarkInfo = Keycat is based on Firefox.\n"
	if got := string(MergeFTL([]byte(original), []byte(overrides))); got != want {
		t.Fatalf("MergeFTL:\n%q\nwant:\n%q", got, want)
	}
}
