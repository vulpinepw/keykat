package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestEngineShortcutsRemoved(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Keycat")
	other := filepath.Join(root, "Mozilla Firefox")
	folder := filepath.Join(root, "Programs")
	for _, d := range []string{filepath.Join(install, "core"), other, folder} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"Firefox.lnk":                 filepath.Join(install, "core", "firefox.exe"),
		"Keycat Private Browsing.lnk": filepath.Join(install, "core", "private_browsing.exe"),
		"Keycat.lnk":                  filepath.Join(install, "Keycat.exe"),
		"Real Firefox.lnk":            filepath.Join(other, "firefox.exe"),
	}
	for name, target := range links {
		if err := createShortcut(filepath.Join(folder, name), target, filepath.Dir(target), name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got, err := shortcutTarget(filepath.Join(folder, "Firefox.lnk")); err != nil || !strings.EqualFold(got, links["Firefox.lnk"]) {
		t.Fatalf("shortcut target = %q, %v", got, err)
	}
	if removed := removeEngineShortcutsIn(install, folder); len(removed) != 2 {
		t.Errorf("removed %v, want the two engine shortcuts", removed)
	}
	for name := range links {
		_, err := os.Stat(filepath.Join(folder, name))
		engine := strings.Contains(links[name], `\core\`)
		if engine && err == nil {
			t.Errorf("%s points at the engine and was kept", name)
		}
		if !engine && err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
}

func TestFirefoxShortcutRestored(t *testing.T) {
	root := t.TempDir()
	engine := filepath.Join(root, "Keycat", "core")
	firefox := filepath.Join(root, "Mozilla Firefox")
	programs := filepath.Join(root, "Programs")
	common := filepath.Join(root, "Common")
	for _, d := range []string{engine, firefox, programs, common} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, exe := range []string{"firefox.exe", "private_browsing.exe"} {
		if err := os.WriteFile(filepath.Join(firefox, exe), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(common, "Firefox Private Browsing.lnk"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	removed := map[string]string{
		filepath.Join(programs, "Firefox.lnk"):                  filepath.Join(engine, "firefox.exe"),
		filepath.Join(programs, "Firefox Private Browsing.lnk"): filepath.Join(engine, "private_browsing.exe"),
		filepath.Join(programs, "Keycat Private Browsing.lnk"):  filepath.Join(engine, "private_browsing.exe"),
	}
	restoreFirefoxShortcuts(removed, programs, common, firefox, "308046B0AF4A39CB")
	if got, err := shortcutTarget(filepath.Join(programs, "Firefox.lnk")); err != nil || !strings.EqualFold(got, filepath.Join(firefox, "firefox.exe")) {
		t.Fatalf("Firefox.lnk = %q, %v; want it pointing at the real Firefox", got, err)
	}
	for _, name := range []string{"Firefox Private Browsing.lnk", "Keycat Private Browsing.lnk"} {
		if _, err := os.Stat(filepath.Join(programs, name)); err == nil {
			t.Errorf("%s was restored", name)
		}
	}
}

func TestEngineRecordsRemoved(t *testing.T) {
	base := fmt.Sprintf(`Software\KeycatTest-%d`, time.Now().UnixNano())
	t.Cleanup(func() { deleteTree(base) })
	install := filepath.Join(t.TempDir(), "Keycat")
	engine := filepath.Join(install, "core")
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	cacheDir := filepath.Join(appData, "Mozilla", "Firefox")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ours := filepath.Join(cacheDir, "blocklist-OURS")
	theirs := filepath.Join(cacheDir, "blocklist-THEIRS")
	for _, f := range []string{ours, theirs} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	real := `C:\Program Files\Mozilla Firefox`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, base+`\Launcher`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	_ = k.SetStringValue(engine+`\firefox.exe|Blocklist`, ours)
	_ = k.SetQWordValue(engine+`\firefox.exe|Browser`, 1)
	_ = k.SetStringValue(real+`\firefox.exe|Blocklist`, theirs)
	_ = k.SetQWordValue(real+`\firefox.exe|Browser`, 1)
	k.Close()
	k, _, err = registry.CreateKey(registry.CURRENT_USER, base+`\Default Browser Agent`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	_ = k.SetDWordValue(engine+`|DisableTelemetry`, 1)
	_ = k.SetDWordValue(real+`|DisableTelemetry`, 1)
	_ = k.SetStringValue("CurrentDefault", "edge-chrome")
	k.Close()

	removeEngineRecordsIn(base, install)

	for key, want := range map[string][]string{
		`\Launcher`:              {real + `\firefox.exe|Blocklist`, real + `\firefox.exe|Browser`},
		`\Default Browser Agent`: {real + `|DisableTelemetry`, "CurrentDefault"},
	} {
		k, err := registry.OpenKey(registry.CURRENT_USER, base+key, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		names, _ := k.ReadValueNames(-1)
		k.Close()
		got := map[string]bool{}
		for _, n := range names {
			got[n] = true
		}
		if len(got) != len(want) {
			t.Errorf("%s keeps %v, want %v", key, names, want)
		}
		for _, n := range want {
			if !got[n] {
				t.Errorf("%s lost %s", key, n)
			}
		}
	}
	if _, err := os.Stat(ours); err == nil {
		t.Error("the engine's blocklist cache was kept")
	}
	if _, err := os.Stat(theirs); err != nil {
		t.Error("the real Firefox's blocklist cache was removed")
	}
}

func TestBrowserRegistration(t *testing.T) {
	base := fmt.Sprintf(`Software\KeycatTest-%d`, time.Now().UnixNano())
	t.Cleanup(func() { deleteTree(base) })
	dir := filepath.Join(t.TempDir(), "Keycat")
	exe := filepath.Join(dir, "Keycat.exe")
	if err := registerBrowserIn(base, dir); err != nil {
		t.Fatal(err)
	}
	read := func(path, name string) string {
		k, err := registry.OpenKey(registry.CURRENT_USER, base+`\`+path, registry.QUERY_VALUE)
		if err != nil {
			return ""
		}
		defer k.Close()
		v, _, _ := k.GetStringValue(name)
		return v
	}
	if got := read(`Classes\KeycatURL\shell\open\command`, ""); got != `"`+exe+`" --url "%1"` {
		t.Errorf("link command = %q", got)
	}
	if got := read(`Classes\KeycatHTML\shell\open\command`, ""); got != `"`+exe+`" --url "%1"` {
		t.Errorf("page command = %q", got)
	}
	for _, scheme := range []string{"http", "https"} {
		if got := read(`Clients\StartMenuInternet\Keycat\Capabilities\URLAssociations`, scheme); got != "KeycatURL" {
			t.Errorf("%s = %q", scheme, got)
		}
	}
	if got := read(`Clients\StartMenuInternet\Keycat\Capabilities\FileAssociations`, ".html"); got != "KeycatHTML" {
		t.Errorf(".html = %q", got)
	}
	if got := read(`RegisteredApplications`, "Keycat"); got != `Software\Clients\StartMenuInternet\Keycat\Capabilities` {
		t.Errorf("registered application = %q", got)
	}
	unregisterBrowserIn(base)
	for _, path := range []string{`Classes\KeycatURL`, `Classes\KeycatHTML`, `Clients\StartMenuInternet\Keycat`} {
		if k, err := registry.OpenKey(registry.CURRENT_USER, base+`\`+path, registry.QUERY_VALUE); err == nil {
			k.Close()
			t.Errorf("%s kept after unregistering", path)
		}
	}
	if read(`RegisteredApplications`, "Keycat") != "" {
		t.Error("registered application kept after unregistering")
	}
}

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.1.0", "1.0.0", true},
		{"1.0.0", "1.1.0", false},
		{"1.0.0", "1.0.0", false},
		{"2.0.0", "1.9.9", true},
		{"1.10.0", "1.9.0", true},
		{"1.1.0-dev", "1.0.9", true},
	}
	for _, c := range cases {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
