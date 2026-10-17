package profile

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"keycat/internal/identity"
	"keycat/internal/vault"
)

const (
	ModeEncrypted = "encrypted"
	ModePlain     = "plain"
	ModeEphemeral = "ephemeral"
)

type Profile struct {
	Version               int                `json:"version"`
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	Color                 string             `json:"color"`
	Avatar                string             `json:"avatar"`
	Created               time.Time          `json:"created"`
	LastUsed              *time.Time         `json:"lastUsed,omitempty"`
	Security              Security           `json:"security"`
	Privacy               Privacy            `json:"privacy"`
	Appearance            Appearance         `json:"appearance"`
	Stats                 Stats              `json:"stats"`
	PlainIdentity         *identity.Identity `json:"plainIdentity,omitempty"`
	PlainIdentitySettings *identity.Settings `json:"plainIdentitySettings,omitempty"`
}

type Security struct {
	Mode        string        `json:"mode"`
	Preset      string        `json:"preset"`
	Crypto      *vault.Crypto `json:"crypto,omitempty"`
	Compression string        `json:"compression"`
	KeepBackup  bool          `json:"keepBackup"`
	KeepCache   bool          `json:"keepCache"`
}

type Privacy struct {
	Preset             string `json:"preset"`
	TrackingProtection string `json:"trackingProtection"`
	Cookies            string `json:"cookies"`
	Cache              string `json:"cache"`
	History            string `json:"history"`
	AlwaysPrivate      bool   `json:"alwaysPrivate"`
	RestoreSession     bool   `json:"restoreSession"`
	HTTPSOnly          bool   `json:"httpsOnly"`
	DNS                string `json:"dns"`
	DNSProvider        string `json:"dnsProvider"`
	DNSCustomURL       string `json:"dnsCustomUrl,omitempty"`
	WebRTC             string `json:"webrtc"`
	Fingerprinting     string `json:"fingerprinting"`
	Letterboxing       bool   `json:"letterboxing"`
	JavaScript         bool   `json:"javascript"`
	WebGL              bool   `json:"webgl"`
	Prefetch           bool   `json:"prefetch"`
	GPC                bool   `json:"gpc"`
	Referrer           string `json:"referrer"`
	SearchEngine       string `json:"searchEngine"`
	SearchSuggestions  bool   `json:"searchSuggestions"`
	Passwords          string `json:"passwords"`
	FakeIdentity       string `json:"fakeIdentity"`
	FormAutofill       bool   `json:"formAutofill"`
	UBlockOrigin       bool   `json:"ublockOrigin"`
	SafeBrowsing       bool   `json:"safeBrowsing"`
	ConnectivityChecks bool   `json:"connectivityChecks"`
	Containers         bool   `json:"containers"`
	Location           string `json:"location"`
	Notifications      string `json:"notifications"`
	MediaDevices       string `json:"mediaDevices"`
	Autoplay           string `json:"autoplay"`
	Proxy              Proxy  `json:"proxy"`
}

type Proxy struct {
	Mode      string `json:"mode"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	RemoteDNS bool   `json:"remoteDns"`
}

type Appearance struct {
	Theme string `json:"theme"`
}

var legacyThemes = map[string]string{
	"keycat-dark":  "dark",
	"keycat-light": "light",
	"keycat-auto":  "system",
}

func (a *Appearance) Normalize() {
	if v, ok := legacyThemes[a.Theme]; ok {
		a.Theme = v
	}
}

type Stats struct {
	VaultBytes   int64      `json:"vaultBytes,omitempty"`
	DataBytes    int64      `json:"dataBytes,omitempty"`
	Files        int64      `json:"files,omitempty"`
	LastSealed   *time.Time `json:"lastSealed,omitempty"`
	LastSealMs   int64      `json:"lastSealMs,omitempty"`
	LastUnlockMs int64      `json:"lastUnlockMs,omitempty"`
	LastUnsealMs int64      `json:"lastUnsealMs,omitempty"`
}

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

var Palette = []string{
	"#F5A524", "#F97316", "#EF4444", "#E05AB4", "#8B6CF6", "#6366F1",
	"#4C8DF6", "#22B8CF", "#14B8A6", "#2EB67D", "#84CC16", "#64748B",
}

func oneOf(field, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return fmt.Errorf("%s: invalid value %q", field, v)
}

func (p *Profile) ValidateIdentity() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return errors.New("profile name is required")
	}
	if utf8.RuneCountInString(p.Name) > 40 {
		return errors.New("profile name must be at most 40 characters")
	}
	if strings.ContainsAny(p.Name, "\x00\r\n\t") {
		return errors.New("profile name contains invalid characters")
	}
	if !colorRe.MatchString(p.Color) {
		return errors.New("color must look like #RRGGBB")
	}
	if p.Avatar == "" {
		p.Avatar = AvatarInitials
	}
	if !ValidAvatar(p.Avatar) {
		return fmt.Errorf("unknown profile picture %q", p.Avatar)
	}
	return nil
}

func (pv *Privacy) Validate() error {
	checks := []error{
		oneOf("preset", pv.Preset, "balanced", "strict", "paranoid", "custom"),
		oneOf("trackingProtection", pv.TrackingProtection, "standard", "strict"),
		oneOf("cookies", pv.Cookies, "partitioned", "block-third-party", "session", "block-all"),
		oneOf("cache", pv.Cache, "disk", "memory", "none"),
		oneOf("history", pv.History, "keep", "session", "off"),
		oneOf("dns", pv.DNS, "system", "doh", "doh-strict"),
		oneOf("webrtc", pv.WebRTC, "default", "no-leak", "off"),
		oneOf("fingerprinting", pv.Fingerprinting, "standard", "resist"),
		oneOf("referrer", pv.Referrer, "default", "trim", "strict"),
		oneOf("location", pv.Location, "ask", "block"),
		oneOf("notifications", pv.Notifications, "ask", "block"),
		oneOf("mediaDevices", pv.MediaDevices, "ask", "block"),
		oneOf("autoplay", pv.Autoplay, "allow", "block-audio", "block-all"),
		oneOf("proxy.mode", pv.Proxy.Mode, "none", "system", "socks5", "http", "tor"),
		oneOf("passwords", pv.Passwords, "keycat", "firefox", "off"),
		oneOf("fakeIdentity", pv.FakeIdentity, "off", "profile", "random"),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	if pv.DNS != "system" {
		if _, ok := dohProviders[pv.DNSProvider]; !ok && pv.DNSProvider != "custom" {
			return fmt.Errorf("dnsProvider: invalid value %q", pv.DNSProvider)
		}
		if pv.DNSProvider == "custom" {
			u, err := url.Parse(pv.DNSCustomURL)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				return errors.New("custom DNS-over-HTTPS address must be an https:// URL")
			}
		}
	}
	if _, ok := SearchEngines[pv.SearchEngine]; !ok {
		return fmt.Errorf("searchEngine: unknown engine %q", pv.SearchEngine)
	}
	switch pv.Proxy.Mode {
	case "socks5", "http":
		if pv.Proxy.Host == "" || strings.ContainsAny(pv.Proxy.Host, " /\\\"'") {
			return errors.New("proxy host is required")
		}
		if pv.Proxy.Port < 1 || pv.Proxy.Port > 65535 {
			return errors.New("proxy port must be 1-65535")
		}
	}
	return nil
}

func (a *Appearance) Validate() error {
	a.Normalize()
	return oneOf("theme", a.Theme, "dark", "light", "system", "firefox")
}

func (p *Profile) AdaptToMode() {
	if p.Security.Mode != ModeEncrypted && p.Privacy.Passwords == "keycat" {
		p.Privacy.Passwords = "firefox"
		if p.Security.Mode == ModeEphemeral {
			p.Privacy.Passwords = "off"
		}
	}
}

func (p *Profile) IsEncrypted() bool { return p.Security.Mode == ModeEncrypted }

func (p *Profile) IsEphemeral() bool { return p.Security.Mode == ModeEphemeral }

type EncryptionSummary struct {
	Mode    string   `json:"mode"`
	Cipher  string   `json:"cipher,omitempty"`
	KDF     string   `json:"kdf,omitempty"`
	Unlock  []string `json:"unlock,omitempty"`
	Summary string   `json:"summary"`
}

func (p *Profile) Encryption() EncryptionSummary {
	switch p.Security.Mode {
	case ModeEphemeral:
		return EncryptionSummary{Mode: ModeEphemeral, Summary: "Ephemeral: wiped when closed"}
	case ModePlain:
		return EncryptionSummary{Mode: ModePlain, Summary: "Not encrypted"}
	}
	es := EncryptionSummary{Mode: ModeEncrypted}
	if c := p.Security.Crypto; c != nil {
		es.Cipher = c.Cipher.Describe()
		for _, ks := range c.Keyslots {
			es.Unlock = append(es.Unlock, ks.Type)
			if ks.Type == vault.SlotPassword && ks.KDF != nil {
				es.KDF = ks.KDF.Describe()
			}
		}
	}
	es.Summary = "Encrypted · " + es.Cipher
	return es
}
