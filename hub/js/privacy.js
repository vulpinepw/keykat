import { android } from "./api.js";
import { h, icon, segmented, selectRow, switchRow } from "./dom.js";
import { choiceCards, section } from "./views.js";

const PRESET_CARDS = [
  { value: "balanced", title: "Balanced", icon: "shield", iconCls: "info", tag: "Recommended", desc: "Blocks trackers and fingerprinting, HTTPS-Only, encrypted DNS. Sites keep working." },
  { value: "strict", title: "Strict", icon: "shieldCheck", desc: "Also blocks third-party cookies, keeps cache in memory and erases history on close." },
  { value: "paranoid", title: "Paranoid", icon: "fingerprint", iconCls: "good", desc: "Always private, resists fingerprinting, no WebRTC, WebGL or cache. Some sites break." },
];

const userAgentHint = v => v.fingerprinting === "resist"
  ? "Resisting fingerprinting sends the standard Firefox user agent of this system instead"
  : android
    ? "What the browser tells sites about your system. Another system makes sites show their computer version. It still says it is Firefox"
    : "What the browser tells sites about your system, in its user agent and to their scripts. It still says it is Firefox";

export function privacyEditor({ value, presets, mode, onChange }) {
  let v = structuredClone(value);
  const root = h("div", { class: "privacy-editor" });

  const emit = () => onChange(structuredClone(v));
  const set = (key, val) => {
    v[key] = val;
    v.preset = "custom";
    presetCards.setValue("custom");
    emit();
    refresh();
  };
  const setProxy = (key, val) => {
    v.proxy = { ...v.proxy, [key]: val };
    v.preset = "custom";
    presetCards.setValue("custom");
    emit();
    refresh();
  };

  const presetCards = choiceCards(PRESET_CARDS, v.preset, name => {
    v = structuredClone(presets.privacy[name]);
    if (mode !== "encrypted" && v.passwords === "keycat") {
      v.passwords = mode === "ephemeral" ? "off" : "firefox";
    }
    emit();
    build();
  }, "Privacy level");

  const dynamic = {};
  function refresh() {
    dynamic.letterbox.querySelector("input").disabled = v.fingerprinting !== "resist";
    dynamic.userAgent.querySelector("select").disabled = v.fingerprinting === "resist";
    dynamic.userAgent.querySelector(".hint").textContent = userAgentHint(v);
    dynamic.dnsProvider.hidden = v.dns === "system";
    dynamic.dnsCustom.hidden = v.dns === "system" || v.dnsProvider !== "custom";
    const proxyFields = v.proxy.mode === "socks5" || v.proxy.mode === "http";
    dynamic.proxyHost.hidden = !proxyFields;
    dynamic.proxyDNS.hidden = v.proxy.mode !== "socks5";
    dynamic.torNote.hidden = v.proxy.mode !== "tor";
  }

  function build() {
    presetCards.setValue(v.preset);
    const dns = presets.dns.map(d => [d.id, d.name]);
    dns.push(["custom", "Custom address"]);
    dynamic.letterbox = switchRow({
      title: "Letterboxing",
      hint: "Rounds the page size so window size cannot identify you",
      checked: v.letterboxing,
      onchange: on => set("letterboxing", on),
    });
    dynamic.userAgent = selectRow({
      title: "System shown to sites",
      hint: userAgentHint(v),
      value: v.userAgent,
      onchange: val => set("userAgent", val),
      options: [["real", android ? "The real one (Android)" : "The real one"], ["windows", "Windows"], ["macos", "macOS"], ["linux", "Linux"], ["random", "A random one each time the profile opens"]],
    });
    dynamic.dnsProvider = selectRow({
      title: "DNS provider",
      value: v.dnsProvider,
      options: dns,
      onchange: val => set("dnsProvider", val),
    });
    const customInput = h("input", { class: "input", type: "url", placeholder: "https://dns.example/dns-query", value: v.dnsCustomUrl || "", "aria-label": "Custom DNS-over-HTTPS address" });
    customInput.addEventListener("change", () => set("dnsCustomUrl", customInput.value.trim()));
    dynamic.dnsCustom = h("div", { class: "toggle-row" }, h("span", { class: "text" }, h("div", { class: "title", text: "Custom DoH address" })), customInput);

    const host = h("input", { class: "input", placeholder: "127.0.0.1", value: v.proxy.host || "", "aria-label": "Proxy host" });
    host.addEventListener("change", () => setProxy("host", host.value.trim()));
    const port = h("input", { class: "input input-port", type: "number", min: "1", max: "65535", placeholder: "1080", value: v.proxy.port || "", "aria-label": "Proxy port" });
    port.addEventListener("change", () => setProxy("port", parseInt(port.value, 10) || 0));
    dynamic.proxyHost = h("div", { class: "toggle-row" },
      h("span", { class: "text" }, h("div", { class: "title", text: "Proxy address" })),
      h("div", { class: "proxy-fields" }, host, port));
    dynamic.proxyDNS = switchRow({
      title: "Resolve DNS through the proxy",
      hint: "Prevents DNS lookups from leaking outside the proxy",
      checked: v.proxy.remoteDns,
      onchange: on => setProxy("remoteDns", on),
    });
    dynamic.torNote = h("div", { class: "toggle-row" }, h("div", { class: "callout" }, icon("info"),
      h("span", { text: android
        ? "Sends traffic through Tor on 127.0.0.1:9050. Start Orbot first; its SOCKS port is 9050."
        : "Sends traffic through a Tor service on 127.0.0.1:9050 (Tor Browser uses 9150). Tor must be running separately." })));

    const keycatPasswords = mode === "encrypted";
    root.replaceChildren(
      presetCards,
      section("Blocking",
        h("div", { class: "toggle-row" },
          h("span", { class: "text" }, h("div", { class: "title", text: "Tracking protection" }),
            h("div", { class: "hint", text: "Strict also blocks cryptominers and tracking content in every window" })),
          segmented([["standard", "Standard"], ["strict", "Strict"]], v.trackingProtection, val => set("trackingProtection", val), "Tracking protection")
        ),
        switchRow({ title: "Ad blocker", hint: "uBlock Origin, installed from the add-on store and checked before it is installed", checked: v.ublockOrigin, onchange: on => set("ublockOrigin", on) }),
        selectRow({
          title: "Cookies", value: v.cookies, onchange: val => set("cookies", val),
          options: [["partitioned", "Isolate per site"], ["block-third-party", "Block third-party cookies"], ["session", "Delete when the profile closes"], ["block-all", "Block all cookies (breaks most sites)"]],
        }),
        selectRow({
          title: "Fingerprinting",
          hint: "Sites can recognise a browser from its screen, fonts, graphics and settings, without any cookie",
          value: v.fingerprinting, onchange: val => set("fingerprinting", val),
          options: [["block", "Block known fingerprinters"], ["protect", "Block them and blur what others measure"], ["resist", "Resist fingerprinting (uniform browser)"]],
        }),
        dynamic.letterbox,
        dynamic.userAgent,
        switchRow({ title: "Global Privacy Control", hint: "Tells sites not to sell or share your data", checked: v.gpc, onchange: on => set("gpc", on) })
      ),
      section("Storage and history",
        selectRow({
          title: "Cache", value: v.cache, onchange: val => set("cache", val),
          options: [["disk", "On disk, erased when closed"], ["memory", "Memory only"], ["none", "No cache"]],
        }),
        selectRow({
          title: "History", value: v.history, onchange: val => set("history", val),
          options: [["keep", "Keep"], ["session", "Erase when the profile closes"], ["off", "Never save"]],
        }),
        switchRow({ title: "Always use private browsing", hint: "Nothing from browsing is written to the profile", checked: v.alwaysPrivate, onchange: on => set("alwaysPrivate", on) }),
        switchRow({ title: "Restore previous session", hint: "Reopen your tabs when the profile opens", checked: v.restoreSession, onchange: on => set("restoreSession", on) })
      ),
      section("Passwords and autofill",
        selectRow({
          title: "Password manager",
          hint: keycatPasswords ? "Keycat Passwords is encrypted with this profile's key and can share logins with other profiles" : "Keycat Passwords needs an encrypted profile",
          value: v.passwords, onchange: val => set("passwords", val),
          options: [
            ...(keycatPasswords ? [["keycat", "Keycat Passwords (encrypted)"]] : []),
            ["firefox", "Built-in password manager"],
            ["off", "Off"],
          ],
        }),
        selectRow({
          title: "Fake identity autofill",
          hint: "Fills sign-up forms with a generated name, email, phone and address",
          value: v.fakeIdentity, onchange: val => set("fakeIdentity", val),
          options: [["off", "Off"], ["profile", "One identity for this profile"], ["random", "A new identity every time"]],
        }),
        switchRow({ title: "Remember form entries and addresses", checked: v.formAutofill, onchange: on => set("formAutofill", on) })
      ),
      section("Network",
        switchRow({ title: "HTTPS-Only mode", hint: "Never load pages over unencrypted connections", checked: v.httpsOnly, onchange: on => set("httpsOnly", on) }),
        selectRow({
          title: "Encrypted DNS", value: v.dns, onchange: val => set("dns", val),
          options: [["system", "Off (system DNS)"], ["doh", "On, fall back to system DNS"], ["doh-strict", "On, never fall back"]],
        }),
        dynamic.dnsProvider,
        dynamic.dnsCustom,
        selectRow({
          title: "WebRTC", value: v.webrtc, onchange: val => set("webrtc", val),
          options: [["no-leak", "Hide my IP address"], ["off", "Disable WebRTC"], ["default", "Browser default"]],
        }),
        selectRow({
          title: "Referrer", value: v.referrer, onchange: val => set("referrer", val),
          options: [["trim", "Send only the site to other sites"], ["strict", "Only within the same site"], ["default", "Browser default"]],
        }),
        switchRow({ title: "Speculative loading", hint: "Preloads links and DNS before you click, which reveals intent", checked: v.prefetch, onchange: on => set("prefetch", on) }),
        switchRow({ title: "Connectivity checks", hint: "Detects Wi-Fi login pages by contacting Mozilla", checked: v.connectivityChecks, onchange: on => set("connectivityChecks", on) }),
        selectRow({
          title: "Proxy", value: v.proxy.mode, onchange: val => setProxy("mode", val),
          options: [["none", "No proxy"], ["system", "Use system settings"], ["socks5", "SOCKS5"], ["http", "HTTP / HTTPS"], ["tor", "Tor (SOCKS5 on 127.0.0.1)"]],
        }),
        dynamic.proxyHost,
        dynamic.proxyDNS,
        dynamic.torNote
      ),
      section("Site permissions and content",
        switchRow({ title: "JavaScript", hint: "Turning it off breaks most modern sites", checked: v.javascript, onchange: on => set("javascript", on) }),
        switchRow({ title: "WebGL", hint: "3D graphics, a common fingerprinting source", checked: v.webgl, onchange: on => set("webgl", on) }),
        selectRow({ title: "Autoplay", value: v.autoplay, onchange: val => set("autoplay", val), options: [["block-audio", "Block audio"], ["block-all", "Block audio and video"], ["allow", "Allow"]] }),
        selectRow({ title: "Location", value: v.location, onchange: val => set("location", val), options: [["ask", "Ask"], ["block", "Block"]] }),
        selectRow({ title: "Notifications", value: v.notifications, onchange: val => set("notifications", val), options: [["ask", "Ask"], ["block", "Block"]] }),
        selectRow({ title: "Camera and microphone", value: v.mediaDevices, onchange: val => set("mediaDevices", val), options: [["ask", "Ask"], ["block", "Block"]] })
      ),
      section("Search and services",
        selectRow({ title: "Search engine", value: v.searchEngine, onchange: val => set("searchEngine", val), options: presets.search.map(e => [e.name, e.private ? `${e.name} (private)` : e.name]) }),
        switchRow({ title: "Search suggestions", hint: "Sends what you type in the address bar to the search engine", checked: v.searchSuggestions, onchange: on => set("searchSuggestions", on) }),
        switchRow({ title: "Safe Browsing", hint: "Warns about dangerous sites using locally stored lists", checked: v.safeBrowsing, onchange: on => set("safeBrowsing", on) }),
        switchRow({ title: "Containers", hint: "Separate tabs into isolated contexts within the profile", checked: v.containers, onchange: on => set("containers", on) })
      )
    );
    refresh();
  }

  build();
  return root;
}
