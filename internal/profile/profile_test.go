package profile

import (
	"strings"
	"testing"
	"time"
)

func newTestProfile(t *testing.T, preset string) *Profile {
	t.Helper()
	pv, err := PrivacyPreset(preset)
	if err != nil {
		t.Fatal(err)
	}
	return &Profile{
		ID: NewID(), Name: "Work", Color: "#F5A524", Avatar: "cat", Created: time.Now(),
		Security:   Security{Mode: ModeEncrypted, Preset: "standard"},
		Privacy:    pv,
		Appearance: Appearance{Theme: "dark"},
	}
}

func TestPresetsValidate(t *testing.T) {
	for _, name := range append(PrivacyPresetNames, "custom") {
		p := newTestProfile(t, name)
		if err := p.Privacy.Validate(); err != nil {
			t.Errorf("preset %s invalid: %v", name, err)
		}
		if err := p.ValidateIdentity(); err != nil {
			t.Errorf("identity invalid: %v", err)
		}
	}
	for _, sp := range SecurityPresets {
		if !sp.Cipher.Valid() {
			t.Errorf("security preset %s has invalid cipher", sp.ID)
		}
	}
	for _, name := range SearchEngineOrder {
		if _, ok := SearchEngines[name]; !ok {
			t.Errorf("search engine %s missing from registry", name)
		}
	}
}

func TestBuildPrefsPresets(t *testing.T) {
	type want map[string]any
	cases := map[string]want{
		"balanced": {
			"browser.contentblocking.category":             "strict",
			"dom.security.https_only_mode":                 true,
			"network.trr.mode":                             2,
			"network.trr.uri":                              "https://dns.quad9.net/dns-query",
			"browser.cache.disk.enable":                    true,
			"places.history.enabled":                       true,
			"browser.startup.page":                         3,
			"media.peerconnection.ice.no_host":             true,
			"privacy.resistFingerprinting":                 false,
			"browser.profiles.enabled":                     false,
			"privacy.clearOnShutdown_v2.cookiesAndStorage": false,
		},
		"strict": {
			"browser.cache.disk.enable":                              false,
			"browser.cache.memory.enable":                            true,
			"network.trr.mode":                                       3,
			"privacy.sanitize.sanitizeOnShutdown":                    true,
			"privacy.clearOnShutdown_v2.browsingHistoryAndDownloads": true,
			"permissions.default.geo":                                2,
			"media.autoplay.default":                                 5,
			"browser.search.suggest.enabled":                         false,
		},
		"paranoid": {
			"browser.privatebrowsing.autostart":            true,
			"browser.cache.memory.enable":                  false,
			"places.history.enabled":                       false,
			"media.peerconnection.enabled":                 false,
			"privacy.resistFingerprinting":                 true,
			"privacy.resistFingerprinting.letterboxing":    true,
			"webgl.disabled":                               true,
			"network.http.referer.XOriginPolicy":           1,
			"privacy.clearOnShutdown_v2.cookiesAndStorage": true,
			"signon.rememberSignons":                       false,
			"signon.generation.enabled":                    false,
		},
	}
	for preset, expected := range cases {
		ps := BuildPrefs(newTestProfile(t, preset))
		for k, v := range expected {
			if got, ok := ps.Set[k]; !ok || got != v {
				t.Errorf("%s: %s = %v (set=%v), want %v", preset, k, got, ok, v)
			}
		}
		for _, r := range ps.Reset {
			if _, dup := ps.Set[r]; dup {
				t.Errorf("%s: %s is both set and reset", preset, r)
			}
		}
	}
}

func TestPrefetchAndWebRTCDefaultsUseReset(t *testing.T) {
	p := newTestProfile(t, "custom")
	p.Privacy.Prefetch = true
	p.Privacy.WebRTC = "default"
	p.Privacy.Referrer = "default"
	ps := BuildPrefs(p)
	for _, k := range []string{"network.prefetch-next", "media.peerconnection.enabled", "network.http.referer.XOriginPolicy"} {
		found := false
		for _, r := range ps.Reset {
			if r == k {
				found = true
			}
		}
		if !found {
			t.Errorf("%s should be reset to Firefox's default", k)
		}
		if _, ok := ps.Set[k]; ok {
			t.Errorf("%s should not be written to user.js", k)
		}
	}
}

func TestProxyAndCustomDNS(t *testing.T) {
	p := newTestProfile(t, "custom")
	p.Privacy.Proxy = Proxy{Mode: "tor", RemoteDNS: true}
	p.Privacy.DNSProvider = "custom"
	p.Privacy.DNSCustomURL = "https://doh.example.net/dns-query"
	if err := p.Privacy.Validate(); err != nil {
		t.Fatal(err)
	}
	ps := BuildPrefs(p)
	if ps.Set["network.proxy.socks_port"] != 9050 || ps.Set["network.proxy.socks_remote_dns"] != true {
		t.Errorf("tor proxy prefs wrong: %v %v", ps.Set["network.proxy.socks_port"], ps.Set["network.proxy.socks_remote_dns"])
	}
	if ps.Set["network.trr.uri"] != "https://doh.example.net/dns-query" {
		t.Errorf("custom DoH uri not applied")
	}
	p.Privacy.DNSCustomURL = "http://insecure.example/dns"
	if err := p.Privacy.Validate(); err == nil {
		t.Error("non-https DoH url accepted")
	}
}

func TestUserJSRendering(t *testing.T) {
	ps := PrefSet{Set: map[string]any{
		"a.bool": true, "b.int": 5, "c.string": `quote " and \ backslash`,
	}}
	js := ps.UserJS("Keycat test")
	for _, line := range []string{
		`user_pref("a.bool", true);`,
		`user_pref("b.int", 5);`,
		`user_pref("c.string", "quote \" and \\ backslash");`,
	} {
		if !strings.Contains(js, line) {
			t.Errorf("missing line %s in:\n%s", line, js)
		}
	}
	if !strings.HasPrefix(js, "// Keycat test") {
		t.Error("header missing")
	}
}

func TestIdentityValidation(t *testing.T) {
	p := newTestProfile(t, "balanced")
	for _, bad := range []func(*Profile){
		func(p *Profile) { p.Name = "   " },
		func(p *Profile) { p.Name = strings.Repeat("x", 41) },
		func(p *Profile) { p.Color = "red" },
		func(p *Profile) { p.Avatar = "<script>" },
	} {
		q := *p
		bad(&q)
		if err := q.ValidateIdentity(); err == nil {
			t.Errorf("invalid identity accepted: %+v", q)
		}
	}
	if !ValidID(NewID()) || ValidID("../etc") || ValidID("p123") {
		t.Error("id validation broken")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := newTestProfile(t, "strict")
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(p.ID)
	if err != nil || got.Name != "Work" || got.Privacy.Cache != "memory" {
		t.Fatalf("get: %+v %v", got, err)
	}
	list, _ := s.List()
	if len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(p.ID); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
}
