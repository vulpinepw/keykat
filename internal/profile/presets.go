package profile

import (
	"fmt"

	"keycat/internal/vault"
)

type SecurityPreset struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Cipher      vault.CipherID `json:"cipher"`
	KDF         string         `json:"kdf"`
}

var SecurityPresets = []SecurityPreset{
	{
		ID: "standard", Name: "Standard",
		Description: "AES-256-GCM with Argon2id (256 MiB). Hardware-accelerated and fast to unlock.",
		Cipher:      vault.CipherAES256GCM, KDF: "argon2id-standard",
	},
	{
		ID: "strong", Name: "Strong",
		Description: "XChaCha20-Poly1305 with Argon2id (512 MiB). Extended nonces, heavier key derivation.",
		Cipher:      vault.CipherXChaCha20Poly1305, KDF: "argon2id-strong",
	},
	{
		ID: "paranoid", Name: "Paranoid",
		Description: "Cascade of AES-256-GCM and XChaCha20-Poly1305 with Argon2id (1 GiB). Survives a break of either cipher.",
		Cipher:      vault.CipherCascade, KDF: "argon2id-paranoid",
	},
}

func SecurityPresetByID(id string) (SecurityPreset, error) {
	for _, p := range SecurityPresets {
		if p.ID == id {
			return p, nil
		}
	}
	return SecurityPreset{}, fmt.Errorf("unknown security preset %q", id)
}

type DoHProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Note string `json:"note"`
}

var dohProviders = map[string]DoHProvider{
	"quad9":      {ID: "quad9", Name: "Quad9", URL: "https://dns.quad9.net/dns-query", Note: "Swiss non-profit, blocks malicious domains"},
	"mullvad":    {ID: "mullvad", Name: "Mullvad", URL: "https://dns.mullvad.net/dns-query", Note: "No logging, no filtering"},
	"mullvad-ad": {ID: "mullvad-ad", Name: "Mullvad (ad blocking)", URL: "https://adblock.dns.mullvad.net/dns-query", Note: "Blocks ads and trackers at DNS level"},
	"cloudflare": {ID: "cloudflare", Name: "Cloudflare", URL: "https://mozilla.cloudflare-dns.com/dns-query", Note: "Fast, audited under Mozilla's resolver policy"},
	"nextdns":    {ID: "nextdns", Name: "NextDNS", URL: "https://firefox.dns.nextdns.io/", Note: "Configurable filtering"},
	"adguard":    {ID: "adguard", Name: "AdGuard", URL: "https://dns.adguard-dns.com/dns-query", Note: "Blocks ads and trackers at DNS level"},
}

func DoHProviders() []DoHProvider {
	ids := []string{"quad9", "mullvad", "mullvad-ad", "cloudflare", "nextdns", "adguard"}
	out := make([]DoHProvider, 0, len(ids))
	for _, id := range ids {
		out = append(out, dohProviders[id])
	}
	return out
}

type SearchEngine struct {
	Name    string `json:"name"`
	Private bool   `json:"private"`
	Builtin bool   `json:"builtin"`
}

var SearchEngines = map[string]SearchEngine{
	"DuckDuckGo":     {Name: "DuckDuckGo", Private: true, Builtin: true},
	"Startpage":      {Name: "Startpage", Private: true, Builtin: true},
	"Brave Search":   {Name: "Brave Search", Private: true},
	"Mojeek":         {Name: "Mojeek", Private: true},
	"Qwant":          {Name: "Qwant", Private: true, Builtin: true},
	"Ecosia":         {Name: "Ecosia", Builtin: true},
	"Wikipedia (en)": {Name: "Wikipedia (en)", Builtin: true},
	"Google":         {Name: "Google", Builtin: true},
	"Bing":           {Name: "Bing", Builtin: true},
}

var SearchEngineOrder = []string{"DuckDuckGo", "Startpage", "Brave Search", "Mojeek", "Qwant", "Ecosia", "Google", "Bing"}

func PrivacyPreset(name string) (Privacy, error) {
	balanced := Privacy{
		Preset:             "balanced",
		TrackingProtection: "strict",
		Cookies:            "partitioned",
		Cache:              "disk",
		History:            "keep",
		RestoreSession:     true,
		HTTPSOnly:          true,
		DNS:                "doh",
		DNSProvider:        "quad9",
		WebRTC:             "no-leak",
		Fingerprinting:     "protect",
		UserAgent:          "real",
		JavaScript:         true,
		WebGL:              true,
		Prefetch:           false,
		GPC:                true,
		Referrer:           "trim",
		SearchEngine:       "DuckDuckGo",
		SearchSuggestions:  true,
		Passwords:          "keycat",
		FakeIdentity:       "profile",
		FormAutofill:       true,
		UBlockOrigin:       true,
		SafeBrowsing:       true,
		ConnectivityChecks: true,
		Containers:         true,
		Location:           "ask",
		Notifications:      "ask",
		MediaDevices:       "ask",
		Autoplay:           "block-audio",
		Proxy:              Proxy{Mode: "none", RemoteDNS: true},
	}
	switch name {
	case "balanced", "custom":
		balanced.Preset = name
		return balanced, nil
	case "strict":
		p := balanced
		p.Preset = "strict"
		p.Cookies = "block-third-party"
		p.Cache = "memory"
		p.History = "session"
		p.RestoreSession = false
		p.DNS = "doh-strict"
		p.SearchSuggestions = false
		p.FormAutofill = false
		p.ConnectivityChecks = false
		p.Location = "block"
		p.Notifications = "block"
		p.Autoplay = "block-all"
		return p, nil
	case "paranoid":
		p := balanced
		p.Preset = "paranoid"
		p.Cookies = "session"
		p.Cache = "none"
		p.History = "off"
		p.AlwaysPrivate = true
		p.RestoreSession = false
		p.DNS = "doh-strict"
		p.WebRTC = "off"
		p.Fingerprinting = "resist"
		p.Letterboxing = true
		p.WebGL = false
		p.Referrer = "strict"
		p.SearchSuggestions = false
		p.FakeIdentity = "random"
		p.FormAutofill = false
		p.ConnectivityChecks = false
		p.Location = "block"
		p.Notifications = "block"
		p.MediaDevices = "block"
		p.Autoplay = "block-all"
		return p, nil
	}
	return Privacy{}, fmt.Errorf("unknown privacy preset %q", name)
}

var PrivacyPresetNames = []string{"balanced", "strict", "paranoid"}
