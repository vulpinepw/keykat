package profile

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
)

type PrefSet struct {
	Set   map[string]any `json:"set"`
	Reset []string       `json:"reset"`
}

const (
	cookieRejectForeign    = 1
	cookieReject           = 2
	cookiePartitionForeign = 5
)

func CookieBehavior(option string) int {
	switch option {
	case "block-third-party":
		return cookieRejectForeign
	case "block-all":
		return cookieReject
	}
	return cookiePartitionForeign
}

func BuildPrefs(p *Profile) PrefSet {
	ps := PrefSet{Set: map[string]any{}}
	set := func(k string, v any) { ps.Set[k] = v }
	reset := func(keys ...string) { ps.Reset = append(ps.Reset, keys...) }
	pv := p.Privacy

	set("browser.profiles.enabled", false)
	set("browser.shell.checkDefaultBrowser", false)
	set("browser.startup.homepage_override.mstone", "ignore")
	set("browser.aboutwelcome.enabled", false)
	set("datareporting.policy.dataSubmissionPolicyBypassNotification", true)
	set("browser.download.start_downloads_in_tmp_dir", true)
	set("browser.helperApps.deleteTempFileOnExit", true)
	set("browser.safebrowsing.downloads.remote.enabled", false)
	set("network.IDN_show_punycode", true)
	set("signon.firefoxRelay.feature", "disabled")
	set("signon.management.page.breach-alerts.enabled", false)

	set("browser.contentblocking.category", pv.TrackingProtection)

	switch pv.Cache {
	case "disk":
		set("browser.cache.disk.enable", true)
		set("browser.cache.memory.enable", true)
	case "memory":
		set("browser.cache.disk.enable", false)
		set("browser.cache.memory.enable", true)
	case "none":
		set("browser.cache.disk.enable", false)
		set("browser.cache.memory.enable", false)
		set("browser.cache.disk_cache_ssl", false)
	}

	set("browser.privatebrowsing.autostart", pv.AlwaysPrivate)
	set("places.history.enabled", pv.History != "off")
	if pv.History == "keep" {
		set("browser.sessionstore.privacy_level", 0)
	} else {
		set("browser.sessionstore.privacy_level", 2)
	}
	if pv.RestoreSession && pv.History == "keep" && !pv.AlwaysPrivate {
		set("browser.startup.page", 3)
	} else {
		set("browser.startup.page", 1)
	}

	clearCookies := pv.Cookies == "session"
	clearHistory := pv.History != "keep"
	clearCache := !(pv.Cache == "disk" && p.Security.KeepCache)
	set("privacy.sanitize.sanitizeOnShutdown", clearCookies || clearHistory || clearCache)
	set("privacy.clearOnShutdown_v2.cookiesAndStorage", clearCookies)
	set("privacy.clearOnShutdown_v2.browsingHistoryAndDownloads", clearHistory)
	set("privacy.clearOnShutdown_v2.historyFormDataAndDownloads", clearHistory)
	set("privacy.clearOnShutdown_v2.formdata", clearHistory)
	set("privacy.clearOnShutdown_v2.cache", clearCache)
	set("privacy.clearOnShutdown_v2.siteSettings", false)
	set("privacy.clearOnShutdown.cookies", clearCookies)
	set("privacy.clearOnShutdown.offlineApps", clearCookies)
	set("privacy.clearOnShutdown.sessions", clearCookies)
	set("privacy.clearOnShutdown.history", clearHistory)
	set("privacy.clearOnShutdown.downloads", clearHistory)
	set("privacy.clearOnShutdown.formdata", clearHistory)
	set("privacy.clearOnShutdown.cache", clearCache)
	set("privacy.clearOnShutdown.siteSettings", false)

	set("dom.security.https_only_mode", pv.HTTPSOnly)
	set("dom.security.https_only_mode_pbm", pv.HTTPSOnly)
	set("dom.security.https_only_mode_send_http_background_request", false)
	switch pv.DNS {
	case "system":
		set("network.trr.mode", 5)
		reset("network.trr.uri", "network.trr.custom_uri")
	case "doh", "doh-strict":
		mode := 2
		if pv.DNS == "doh-strict" {
			mode = 3
		}
		uri := pv.DNSCustomURL
		if pv.DNSProvider != "custom" {
			uri = dohProviders[pv.DNSProvider].URL
		}
		set("network.trr.mode", mode)
		set("network.trr.uri", uri)
		set("network.trr.custom_uri", uri)
		set("network.dns.echconfig.enabled", true)
		set("network.dns.http3_echconfig.enabled", true)
	}

	switch pv.WebRTC {
	case "default":
		reset("media.peerconnection.enabled", "media.peerconnection.ice.default_address_only",
			"media.peerconnection.ice.no_host", "media.peerconnection.ice.proxy_only_if_behind_proxy")
	case "no-leak":
		set("media.peerconnection.enabled", true)
		set("media.peerconnection.ice.default_address_only", true)
		set("media.peerconnection.ice.no_host", true)
		set("media.peerconnection.ice.proxy_only_if_behind_proxy", true)
	case "off":
		set("media.peerconnection.enabled", false)
	}

	set("privacy.resistFingerprinting", pv.Fingerprinting == "resist")
	set("privacy.resistFingerprinting.letterboxing", pv.Fingerprinting == "resist" && pv.Letterboxing)
	set("javascript.enabled", pv.JavaScript)
	set("webgl.disabled", !pv.WebGL)

	prefetchPrefs := []string{
		"network.prefetch-next", "network.dns.disablePrefetch", "network.dns.disablePrefetchFromHTTPS",
		"network.predictor.enabled", "network.predictor.enable-prefetch",
		"network.http.speculative-parallel-limit", "browser.urlbar.speculativeConnect.enabled",
		"browser.places.speculativeConnect.enabled",
	}
	if pv.Prefetch {
		reset(prefetchPrefs...)
	} else {
		set("network.prefetch-next", false)
		set("network.dns.disablePrefetch", true)
		set("network.dns.disablePrefetchFromHTTPS", true)
		set("network.predictor.enabled", false)
		set("network.predictor.enable-prefetch", false)
		set("network.http.speculative-parallel-limit", 0)
		set("browser.urlbar.speculativeConnect.enabled", false)
		set("browser.places.speculativeConnect.enabled", false)
	}

	set("privacy.globalprivacycontrol.enabled", pv.GPC)
	set("privacy.globalprivacycontrol.functionality.enabled", true)

	switch pv.Referrer {
	case "default":
		reset("network.http.referer.XOriginTrimmingPolicy", "network.http.referer.XOriginPolicy")
	case "trim":
		set("network.http.referer.XOriginTrimmingPolicy", 2)
		reset("network.http.referer.XOriginPolicy")
	case "strict":
		set("network.http.referer.XOriginTrimmingPolicy", 2)
		set("network.http.referer.XOriginPolicy", 1)
	}

	set("browser.search.suggest.enabled", pv.SearchSuggestions)
	set("browser.urlbar.suggest.searches", pv.SearchSuggestions)
	set("browser.search.separatePrivateDefault", false)

	firefoxPasswords := pv.Passwords == "firefox"
	set("signon.rememberSignons", firefoxPasswords)
	set("signon.autofillForms", firefoxPasswords)
	set("signon.generation.enabled", firefoxPasswords)
	set("signon.formlessCapture.enabled", firefoxPasswords)
	set("browser.contextual-password-manager.enabled", firefoxPasswords)
	set("browser.formfill.enable", pv.FormAutofill && pv.History != "off")
	set("extensions.formautofill.addresses.enabled", pv.FormAutofill)
	set("extensions.formautofill.creditCards.enabled", pv.FormAutofill)

	set("browser.safebrowsing.malware.enabled", pv.SafeBrowsing)
	set("browser.safebrowsing.phishing.enabled", pv.SafeBrowsing)
	set("browser.safebrowsing.downloads.enabled", pv.SafeBrowsing)
	set("network.captive-portal-service.enabled", pv.ConnectivityChecks)
	set("network.connectivity-service.enabled", pv.ConnectivityChecks)
	set("privacy.userContext.enabled", pv.Containers)
	set("privacy.userContext.ui.enabled", pv.Containers)

	perm := func(v string) int {
		if v == "block" {
			return 2
		}
		return 0
	}
	set("permissions.default.geo", perm(pv.Location))
	set("permissions.default.desktop-notification", perm(pv.Notifications))
	set("permissions.default.camera", perm(pv.MediaDevices))
	set("permissions.default.microphone", perm(pv.MediaDevices))
	set("media.autoplay.default", map[string]int{"allow": 0, "block-audio": 1, "block-all": 5}[pv.Autoplay])

	switch pv.Proxy.Mode {
	case "none":
		set("network.proxy.type", 0)
	case "system":
		set("network.proxy.type", 5)
	case "socks5", "tor":
		host, port := pv.Proxy.Host, pv.Proxy.Port
		if pv.Proxy.Mode == "tor" {
			host, port = "127.0.0.1", 9050
			if pv.Proxy.Port != 0 {
				port = pv.Proxy.Port
			}
		}
		set("network.proxy.type", 1)
		set("network.proxy.socks", host)
		set("network.proxy.socks_port", port)
		set("network.proxy.socks_version", 5)
		set("network.proxy.socks_remote_dns", pv.Proxy.RemoteDNS || pv.Proxy.Mode == "tor")
		set("network.proxy.share_proxy_settings", false)
		set("network.proxy.http", "")
		set("network.proxy.ssl", "")
	case "http":
		set("network.proxy.type", 1)
		set("network.proxy.http", pv.Proxy.Host)
		set("network.proxy.http_port", pv.Proxy.Port)
		set("network.proxy.ssl", pv.Proxy.Host)
		set("network.proxy.ssl_port", pv.Proxy.Port)
		set("network.proxy.share_proxy_settings", true)
		set("network.proxy.socks", "")
	}

	sort.Strings(ps.Reset)
	return ps
}

func (ps PrefSet) UserJS(header string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(header), "\n") {
		b.WriteString("// " + line + "\n")
	}
	b.WriteString("\n")
	keys := make([]string, 0, len(ps.Set))
	for k := range ps.Set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "user_pref(%s, %s);\n", jsString(k), jsValue(ps.Set[k]))
	}
	return b.String()
}

func jsString(s string) string {
	out, _ := json.Marshal(s)
	return string(out)
}

func jsValue(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", x)
	case string:
		return jsString(x)
	}
	panic(fmt.Sprintf("unsupported pref value %T", v))
}

type SummaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Level string `json:"level"`
}

func Summary(p *Profile) []SummaryItem {
	pv := p.Privacy
	item := func(label, value, level string) SummaryItem { return SummaryItem{label, value, level} }
	var out []SummaryItem

	if pv.TrackingProtection == "strict" {
		out = append(out, item("Tracking protection", "Strict", "good"))
	} else {
		out = append(out, item("Tracking protection", "Standard", "ok"))
	}
	cookies := map[string]SummaryItem{
		"partitioned":       item("Cookies", "Isolated per site", "ok"),
		"block-third-party": item("Cookies", "Third-party blocked", "good"),
		"session":           item("Cookies", "Deleted on close", "good"),
		"block-all":         item("Cookies", "All blocked", "good"),
	}[pv.Cookies]
	out = append(out, cookies)
	switch {
	case pv.Cache == "none":
		out = append(out, item("Cache", "Disabled", "good"))
	case pv.Cache == "memory":
		out = append(out, item("Cache", "Memory only", "good"))
	case p.Security.KeepCache:
		out = append(out, item("Cache", "Disk, kept in vault", "ok"))
	default:
		out = append(out, item("Cache", "Disk, erased on close", "ok"))
	}
	switch {
	case pv.AlwaysPrivate:
		out = append(out, item("History", "Never saved", "good"))
	case pv.History == "off":
		out = append(out, item("History", "Off", "good"))
	case pv.History == "session":
		out = append(out, item("History", "Erased on close", "good"))
	default:
		out = append(out, item("History", "Kept", "ok"))
	}
	if pv.HTTPSOnly {
		out = append(out, item("HTTPS-Only", "On", "good"))
	} else {
		out = append(out, item("HTTPS-Only", "Off", "off"))
	}
	switch pv.DNS {
	case "system":
		out = append(out, item("DNS", "System (unencrypted)", "off"))
	default:
		name := "custom"
		if pv.DNSProvider != "custom" {
			name = dohProviders[pv.DNSProvider].Name
		}
		if pv.DNS == "doh-strict" {
			out = append(out, item("DNS", "Encrypted only · "+name, "good"))
		} else {
			out = append(out, item("DNS", "Encrypted · "+name, "good"))
		}
	}
	out = append(out, map[string]SummaryItem{
		"default": item("WebRTC", "Default", "off"),
		"no-leak": item("WebRTC", "IP leak protection", "good"),
		"off":     item("WebRTC", "Disabled", "good"),
	}[pv.WebRTC])
	switch pv.Fingerprinting {
	case "resist":
		out = append(out, item("Fingerprinting", "Resisted (uniform browser)", "good"))
	case "protect":
		out = append(out, item("Fingerprinting", "Blocked and blurred", "good"))
	default:
		out = append(out, item("Fingerprinting", "Known fingerprinters blocked", "ok"))
	}
	systems := map[string]string{"windows": "Windows", "macos": "macOS", "linux": "Linux"}
	switch {
	case pv.Fingerprinting == "resist":
		out = append(out, item("User agent", "Standard Firefox", "good"))
	case pv.UserAgent == "random":
		out = append(out, item("User agent", "A random system", "good"))
	case pv.UserAgent == runtime.GOOS:
		out = append(out, item("User agent", systems[pv.UserAgent], "ok"))
	case systems[pv.UserAgent] != "":
		out = append(out, item("User agent", systems[pv.UserAgent]+" (spoofed)", "good"))
	default:
		out = append(out, item("User agent", "This system", "ok"))
	}
	if pv.UBlockOrigin {
		out = append(out, item("Ad blocker", "uBlock Origin", "good"))
	} else {
		out = append(out, item("Ad blocker", "Off", "off"))
	}
	switch pv.Passwords {
	case "keycat":
		out = append(out, item("Passwords", "Keycat Passwords (encrypted)", "good"))
	case "firefox":
		out = append(out, item("Passwords", "Built-in password manager", "ok"))
	}
	switch pv.FakeIdentity {
	case "profile":
		out = append(out, item("Autofill identity", "Fake identity", "good"))
	case "random":
		out = append(out, item("Autofill identity", "New fake identity each time", "good"))
	}
	if !pv.JavaScript {
		out = append(out, item("JavaScript", "Disabled", "good"))
	}
	if pv.Proxy.Mode != "none" {
		label := map[string]string{"system": "System proxy", "socks5": "SOCKS5", "http": "HTTP proxy", "tor": "Tor (SOCKS5)"}[pv.Proxy.Mode]
		out = append(out, item("Proxy", label, "good"))
	}
	out = append(out, item("Search", pv.SearchEngine, "ok"))
	return out
}
