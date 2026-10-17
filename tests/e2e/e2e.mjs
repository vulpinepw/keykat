import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { Marionette } from "./marionette.mjs";

const root = path.resolve(import.meta.dirname, "..", "..");
const windows = process.platform === "win32";
const exe = process.argv[2] || (windows ? path.join(root, "dist", "Keycat", "Keycat.exe") : path.join(root, "dist", "linux", "keycat", "keycat"));
const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), "keycat-e2e-"));
const outDir = path.join(root, "tests", "e2e", "output");
fs.mkdirSync(outDir, { recursive: true });

const PASSWORD = "correct horse battery staple";
let base, token, launcher;
const results = [];

function check(name, ok, detail = "") {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  (" + detail + ")" : ""}`);
}

const sleep = ms => new Promise(r => setTimeout(r, ms));

async function waitFor(fn, timeoutMs, label) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await fn().catch(() => undefined);
    if (value) {
      return value;
    }
    if (Date.now() > deadline) {
      throw new Error(`timed out waiting for ${label}`);
    }
    await sleep(250);
  }
}

async function api(method, p, body) {
  const res = await fetch(base + p, {
    method,
    headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: res.status, data: await res.json().catch(() => ({})) };
}

async function profileStatus(id) {
  const { data } = await api("GET", "/api/state");
  return data.profiles.find(p => p.id === id)?.status;
}

async function connectMarionette() {
  const m = new Marionette();
  await m.connect(90000);
  await m.session();
  return m;
}

async function inspect(m) {
  await waitFor(
    () => m.exec(`return Services.prefs.getStringPref("extensions.activeThemeID", "") == "firefox-compact-dark@mozilla.org";`),
    30000,
    "Firefox Dark theme"
  ).catch(() => {});
  return m.exec(`return (async () => {
    const P = Services.prefs;
    const get = n => {
      switch (P.getPrefType(n)) {
        case P.PREF_BOOL: return P.getBoolPref(n);
        case P.PREF_INT: return P.getIntPref(n);
        case P.PREF_STRING: return P.getStringPref(n);
      }
      return null;
    };
    const names = ["browser.contentblocking.category", "network.cookie.cookieBehavior",
      "browser.contentblocking.features.strict", "network.trr.mode", "network.trr.uri",
      "browser.cache.disk.enable", "browser.cache.memory.enable", "privacy.sanitize.sanitizeOnShutdown",
      "dom.security.https_only_mode", "browser.profiles.enabled", "network.prefetch-next",
      "media.peerconnection.ice.no_host", "permissions.default.geo", "keycat.mode",
      "extensions.activeThemeID", "privacy.trackingprotection.enabled", "toolkit.telemetry.unified",
      "keycat.test.marker", "browser.privatebrowsing.autostart", "privacy.resistFingerprinting",
      "identity.fxaccounts.enabled", "signon.firefoxRelay.feature", "privacy.fingerprintingProtection",
      "privacy.trackingprotection.fingerprinting.enabled", "general.useragent.override"];
    const prefs = {};
    for (const n of names) prefs[n] = get(n);
    const brand = Services.strings.createBundle("chrome://branding/locale/brand.properties")
      .GetStringFromName("brandShortName");
    const fluentBrand = await document.l10n.formatValue("browser-main-window-default-title");
    const { SearchService } = ChromeUtils.importESModule("moz-src:///toolkit/components/search/SearchService.sys.mjs");
    await SearchService.init();
    const engine = (await SearchService.getDefault()).name;
    const policies = Services.policies.getActivePolicies() || {};
    return {
      prefs,
      locked: {
        profiles: P.prefIsLocked("browser.profiles.enabled"),
        telemetry: P.prefIsLocked("toolkit.telemetry.unified"),
        accounts: P.prefIsLocked("identity.fxaccounts.enabled"),
      },
      brand, fluentBrand, engine,
      policiesActive: Services.policies.status == Ci.nsIEnterprisePolicies.ACTIVE,
      policyNames: Object.keys(policies),
      badge: !!document.getElementById("keycat-profile-button"),
      accountButton: (() => { const b = document.getElementById("fxa-toolbar-menu-button"); return !!b && b.checkVisibility(); })(),
      title: document.title,
    };
  })();`);
}

async function main() {
  console.log("Keycat e2e: exe", exe);
  console.log("data dir", dataDir);
  launcher = spawn(exe, ["--no-hub", "--headless", "--marionette"], {
    env: { ...process.env, KEYCAT_DATA_DIR: dataDir },
    stdio: "ignore",
  });
  const info = await waitFor(async () => {
    const f = path.join(dataDir, "Local", "launcher.json");
    return fs.existsSync(f) ? JSON.parse(fs.readFileSync(f, "utf8")) : undefined;
  }, 60000, "launcher.json");
  base = info.url;
  token = info.token;
  check("launcher started and wrote launcher.json", !!base);

  const noAuth = await fetch(base + "/api/state");
  check("API rejects requests without a token", noAuth.status === 401, `status ${noAuth.status}`);
  const wrongType = await fetch(base + "/api/profiles", {
    method: "POST",
    headers: { Authorization: "Bearer " + token, "Content-Type": "text/plain" },
    body: "{}",
  });
  check("API rejects non-JSON writes (CSRF guard)", wrongType.status === 415, `status ${wrongType.status}`);

  const { data: state } = await api("GET", "/api/state");
  check("state lists presets", !!state.presets?.privacy?.strict && state.presets.security.length === 3);
  check("engine detected", state.core.installed, `Firefox ${state.core.version}`);

  const created = await api("POST", "/api/profiles", {
    name: "E2E Strict",
    color: "#8B6CF6",
    avatar: "cat",
    security: { mode: "encrypted", preset: "standard", password: PASSWORD, recovery: true, keepBackup: true },
    privacy: state.presets.privacy.strict,
    appearance: { theme: "dark" },
  });
  const profile = created.data.profile;
  check("encrypted profile created", created.status === 201, created.data.error);
  check("recovery code issued", /^[A-Z2-7]{4}(-[A-Z2-7]{4}){7}$/.test(created.data.recoveryCode || ""));
  check("profile uses AES-256-GCM + Argon2id", profile.security.cipher === "aes-256-gcm" && profile.security.kdf === "argon2id-standard");

  const wrong = await api("POST", `/api/profiles/${profile.id}/open`, { method: "password", password: "nope nope nope" });
  check("wrong password is rejected", wrong.status === 401 && wrong.data.code === "wrong_password", `status ${wrong.status}`);

  let t0 = Date.now();
  const opened = await api("POST", `/api/profiles/${profile.id}/open`, { method: "password", password: PASSWORD });
  check("profile opens with the right password", opened.status === 200, opened.data.error);
  await waitFor(async () => (await profileStatus(profile.id))?.pid > 0, 90000, "browser hello");
  check("browser layer connected to the launcher", true, `${Date.now() - t0} ms`);

  let m = await connectMarionette();
  const r = await inspect(m);
  fs.writeFileSync(path.join(outDir, "inspect-strict.json"), JSON.stringify(r, null, 2));
  const p = r.prefs;
  check("branding: brand.properties says Keycat", r.brand === "Keycat", r.brand);
  check("branding: Fluent brand says Keycat", r.fluentBrand === "Keycat", r.fluentBrand);
  check("policies active", r.policiesActive && r.policyNames.includes("DisableTelemetry"), r.policyNames.join(","));
  check("tracking protection: strict", p["browser.contentblocking.category"] === "strict");
  check("cookies: third-party blocked inside strict mode", p["network.cookie.cookieBehavior"] === 1 && /cookieBehavior1/.test(p["browser.contentblocking.features.strict"]), `${p["network.cookie.cookieBehavior"]}`);
  check("trackers blocked in normal windows", p["privacy.trackingprotection.enabled"] === true);
  check("DNS over HTTPS: strict, Quad9", p["network.trr.mode"] === 3 && p["network.trr.uri"] === "https://dns.quad9.net/dns-query");
  check("cache: memory only", p["browser.cache.disk.enable"] === false && p["browser.cache.memory.enable"] === true);
  check("history erased on close", p["privacy.sanitize.sanitizeOnShutdown"] === true);
  check("HTTPS-Only on", p["dom.security.https_only_mode"] === true);
  check("prefetching off", p["network.prefetch-next"] === false);
  check("WebRTC IP leak protection", p["media.peerconnection.ice.no_host"] === true);
  check("fingerprinting: known fingerprinters blocked, the rest blurred", p["privacy.trackingprotection.fingerprinting.enabled"] === true && p["privacy.fingerprintingProtection"] === true);
  check("sites are told the real system by default", p["general.useragent.override"] === null);
  check("location blocked by default", p["permissions.default.geo"] === 2);
  check("Firefox profile switcher disabled and locked", p["browser.profiles.enabled"] === false && r.locked.profiles);
  check("telemetry off and locked", p["toolkit.telemetry.unified"] === false && r.locked.telemetry);
  check("no Mozilla account: accounts off and locked, policy active", p["identity.fxaccounts.enabled"] === false && r.locked.accounts && r.policyNames.includes("DisableFirefoxAccounts"));
  check("no Mozilla account: Relay masks off, no account button", p["signon.firefoxRelay.feature"] === "disabled" && !r.accountButton);
  check("Keycat layer running in profile mode", p["keycat.mode"] === "profile");
  check("Firefox Dark theme active", p["extensions.activeThemeID"] === "firefox-compact-dark@mozilla.org", p["extensions.activeThemeID"]);
  check("search engine: DuckDuckGo", r.engine === "DuckDuckGo", r.engine);
  check("profile badge in toolbar", r.badge);
  const panelOpened = await m.exec(`return (async () => {
    document.getElementById("keycat-profile-button").doCommand();
    for (let i = 0; i < 40 && document.getElementById("keycat-panel")?.state !== "open"; i++) await new Promise(r => setTimeout(r, 100));
    const panel = document.getElementById("keycat-panel");
    const ok = panel?.state === "open" && !!panel.querySelector("moz-toggle") && !!panel.querySelector("moz-segmented-control");
    panel?.hidePopup();
    return ok;
  })();`);
  check("badge opens the Keycat panel (Firefox components)", panelOpened);
  await m.send("Marionette:SetContext", { value: "content" });
  await m.send("WebDriver:Navigate", { url: "about:preferences" });
  await sleep(1500);
  const prefsPage = (await m.send("WebDriver:ExecuteScript", {
    script: `return {
      general: document.getElementById("category-sync")?.textContent.trim(),
      accountVisible: [...document.querySelectorAll('setting-group[groupid="account"], setting-group[groupid="accountDisabled"], setting-group[groupid="sync"]')].some(g => g.checkVisibility()),
      managedNotice: !!document.getElementById("policies-container-content")?.checkVisibility(),
    };`,
    args: [],
  })).value;
  await m.send("Marionette:SetContext", { value: "chrome" });
  check("Settings: no account or sync section, no managed notice", prefsPage.general === "General" && !prefsPage.accountVisible && !prefsPage.managedNotice, JSON.stringify(prefsPage));
  check("window title shows profile", r.title.includes("E2E Strict"), r.title);
  check("window title uses plain separators", !r.title.includes("\u2014") && r.title.endsWith(" - Keycat"), r.title);
  fs.writeFileSync(path.join(outDir, "browser-strict.png"), await m.screenshot());

  const contentExec = async script => {
    await m.send("Marionette:SetContext", { value: "content" });
    try {
      return (await m.send("WebDriver:ExecuteScript", { script, args: [] })).value;
    } finally {
      await m.send("Marionette:SetContext", { value: "chrome" });
    }
  };
  const contentNavigate = async url => {
    await m.send("Marionette:SetContext", { value: "content" });
    try {
      await m.send("WebDriver:Navigate", { url });
    } finally {
      await m.send("Marionette:SetContext", { value: "chrome" });
    }
  };
  await contentNavigate("about:preferences");
  const keycatPane = await waitFor(() => contentExec(`
    const frame = document.querySelector('.keycat-pane[data-category="paneKeycat"] iframe');
    const doc = frame?.contentDocument;
    const tabs = doc ? [...doc.querySelectorAll(".drawer.inline .tabs button")].map(b => b.textContent) : [];
    if (!tabs.length) return null;
    return {
      hash: location.hash,
      categories: ["category-keycat", "category-keycat-passwords", "category-keycat-profiles"].map(id => !!document.getElementById(id)),
      tabs,
      name: doc.querySelector(".drawer.inline .drawer-head h2")?.textContent,
      firefoxCategories: ["category-about-firefox", "category-more-from-mozilla"].map(id => document.getElementById(id)?.checkVisibility() || false),
    };`), 20000, "Keycat settings pane").catch(err => ({ error: String(err) }));
  check("Settings open on the Keycat category", keycatPane.hash === "#keycat" && keycatPane.categories?.every(Boolean), JSON.stringify(keycatPane));
  check("Keycat category shows this profile's settings", keycatPane.name === "E2E Strict" && keycatPane.tabs?.join(",") === "Profile,Privacy,Security,Fake identity", JSON.stringify(keycatPane.tabs));
  check("Firefox-only categories hidden", keycatPane.firefoxCategories?.every(v => !v), JSON.stringify(keycatPane.firefoxCategories));
  fs.writeFileSync(path.join(outDir, "settings-keycat.png"), await m.screenshot());

  await contentNavigate("about:logins");
  const redirected = await waitFor(async () => {
    const uri = await m.exec(`return gBrowser.selectedBrowser.currentURI.spec;`);
    return uri.startsWith("about:preferences#keycatPasswords") ? uri : null;
  }, 15000, "password redirect").catch(() => "");
  check("about:logins leads to Keycat Passwords", !!redirected, redirected);
  const passwordsPane = await waitFor(() => contentExec(`
    const doc = document.querySelector('.keycat-pane[data-category="paneKeycatPasswords"] iframe')?.contentDocument;
    return doc?.querySelector(".passwords .pw-toolbar") ? doc.querySelector(".drawer-head h2")?.textContent : null;`), 20000, "Keycat Passwords pane").catch(() => "");
  check("Keycat Passwords shown in the browser's settings", passwordsPane === "Keycat Passwords", passwordsPane);
  const menuRedirect = await m.exec(`return (async () => {
    const { LoginHelper } = ChromeUtils.importESModule("resource://gre/modules/LoginHelper.sys.mjs");
    gBrowser.selectedTab = gBrowser.addTrustedTab("about:blank");
    LoginHelper.openPasswordManager(window, { entryPoint: "mainmenu" });
    for (let i = 0; i < 50; i++) {
      const spec = gBrowser.selectedBrowser.currentURI.spec;
      if (spec.startsWith("about:preferences#keycatPasswords")) return spec;
      await new Promise(r => setTimeout(r, 100));
    }
    return gBrowser.selectedBrowser.currentURI.spec;
  })();`);
  check("the browser's Passwords menu opens Keycat Passwords", menuRedirect.startsWith("about:preferences#keycatPasswords"), menuRedirect);

  await contentNavigate("about:keycat");
  const aboutKeycat = await waitFor(() => contentExec(`
    const cards = [...document.querySelectorAll(".card.profile")];
    if (!cards.length) return null;
    return {
      names: cards.map(c => c.querySelector(".profile-name")?.textContent),
      current: document.querySelector(".card.profile.is-current .status")?.textContent,
      text: document.body.innerText,
    };`), 20000, "about:keycat").catch(err => ({ error: String(err) }));
  check("about:keycat lists the profiles inside the browser", aboutKeycat.names?.includes("E2E Strict") && aboutKeycat.current === "This window", JSON.stringify({ ...aboutKeycat, text: undefined }));
  check("about:keycat names no other browser", !!aboutKeycat.text && !/firefox/i.test(aboutKeycat.text));
  fs.writeFileSync(path.join(outDir, "about-keycat.png"), await m.screenshot());

  const before = (await api("GET", "/api/state")).data.profiles.find(x => x.id === profile.id).privacy;
  const patched = await api("PATCH", `/api/profiles/${profile.id}`, { privacy: { ...before, httpsOnly: false, preset: "custom" } });
  const liveOff = await waitFor(() => m.exec(`return Services.prefs.getBoolPref("dom.security.https_only_mode") === false;`), 10000, "live setting").catch(() => false);
  check("privacy change applied to the open browser without a restart", patched.status === 200 && !patched.data.restartRequired && liveOff, patched.data.error);
  await api("PATCH", `/api/profiles/${profile.id}`, { privacy: before });
  const liveOn = await waitFor(() => m.exec(`return Services.prefs.getBoolPref("dom.security.https_only_mode") === true;`), 10000, "live setting back").catch(() => false);
  check("privacy change undone live too", liveOn);

  const httpAgent = `const http = Cc["@mozilla.org/network/protocol;1?name=http"].getService(Ci.nsIHttpProtocolHandler);`;
  const spoof = await api("PATCH", `/api/profiles/${profile.id}`, { privacy: { ...before, preset: "custom", fingerprinting: "block", userAgent: "macos" } });
  const spoofed = await waitFor(() => m.exec(`${httpAgent}
    const P = Services.prefs;
    const version = parseInt(Services.appinfo.version, 10) + ".0";
    const ok = http.userAgent === "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:" + version + ") Gecko/20100101 Firefox/" + version &&
      P.getBoolPref("privacy.fingerprintingProtection") === false && P.getBoolPref("privacy.trackingprotection.fingerprinting.enabled") === true;
    return ok ? http.userAgent : null;`), 10000, "spoofed system").catch(() => null);
  check("sites are told another system, without a restart", spoof.status === 200 && !!spoofed, spoofed || JSON.stringify(spoof.data));
  await contentNavigate("about:blank");
  const seen = await contentExec(`return { ua: navigator.userAgent, platform: navigator.platform, oscpu: navigator.oscpu };`);
  check("page scripts see the same system", seen.ua === spoofed && seen.platform === "MacIntel" && seen.oscpu === "Intel Mac OS X 10.15", JSON.stringify(seen));
  await api("PATCH", `/api/profiles/${profile.id}`, { privacy: { ...before, preset: "custom", userAgent: "random" } });
  const random = await waitFor(() => m.exec(`${httpAgent}
    return /\\((Windows NT 10\\.0; Win64; x64|Macintosh; Intel Mac OS X 10\\.15|X11; Linux x86_64); rv:/.test(http.userAgent) &&
      Services.prefs.prefHasUserValue("general.useragent.override") ? http.userAgent : null;`), 10000, "random system").catch(() => null);
  check("a random system is one of the three", !!random, random);
  await api("PATCH", `/api/profiles/${profile.id}`, { privacy: before });
  const real = await waitFor(() => m.exec(`${httpAgent}
    const P = Services.prefs;
    const ok = !P.prefHasUserValue("general.useragent.override") && !P.prefHasUserValue("general.platform.override") &&
      P.getBoolPref("privacy.fingerprintingProtection") === true && !http.userAgent.includes("Macintosh");
    return ok ? http.userAgent : null;`), 10000, "real system").catch(() => null);
  check("back to the real system and to blurring", !!real && (windows ? real.includes("Windows NT") : real.includes("X11; Linux")), real);

  const branding = await m.exec(`return (async () => {
    const read = async url => (await (await fetch(url)).text());
    const l10n = new Localization(["toolkit/branding/brandings.ftl", "browser/browser.ftl", "browser/appExtensionFields.ftl"], true);
    const [alpenglow] = await l10n.formatValues(["extension-firefox-alpenglow-name"]);
    const P = Services.prefs;
    const { AppConstants } = ChromeUtils.importESModule("resource://gre/modules/AppConstants.sys.mjs");
    const { ShellService } = ChromeUtils.importESModule("moz-src:///browser/components/shell/ShellService.sys.mjs");
    const provider = name => JSON.parse(P.getStringPref("browser.newtabpage.activity-stream.asrouter.providers." + name));
    const { ASRouter } = ChromeUtils.importESModule("resource:///modules/asrouter/ASRouter.sys.mjs");
    await Promise.race([ASRouter.waitForInitialized, new Promise(r => setTimeout(r, 15000))]);
    const { PlacesUtils } = ChromeUtils.importESModule("resource://gre/modules/PlacesUtils.sys.mjs");
    const bookmarks = [];
    for (const guid of [PlacesUtils.bookmarks.menuGuid, PlacesUtils.bookmarks.toolbarGuid]) {
      const tree = await PlacesUtils.promiseBookmarksTree(guid);
      bookmarks.push(...(tree.children || []).map(c => c.title));
    }
    return {
      bookmarks,
      routerMessages: ASRouter.initialized ? ASRouter.state.messages.map(msg => msg.provider + ":" + msg.id) : null,
      pin: AppConstants.platform === "win"
        ? (await ShellService.doesAppNeedPin()) === false && !String(ShellService.pinToTaskbar).includes("doesAppNeedPin")
        : P.getBoolPref("browser.gnome-search-provider.enabled") === false && P.prefIsLocked("browser.gnome-search-provider.enabled"),
      messages: P.getBoolPref("browser.bypassAutoTriggerActions") === true && P.prefIsLocked("browser.bypassAutoTriggerActions") &&
        ["onboarding", "cfr", "message-groups", "messaging-experiments"].every(name => provider(name).enabled === false),
      privateLogo: (await read("chrome://browser/skin/privatebrowsing/pbm-logo.svg")).includes("kc-pb"),
      mascot: (await read("chrome://global/skin/illustrations/kit-concerned.svg")).includes("kc-ill"),
      alpenglow,
      support: P.getStringPref("app.support.baseURL"),
      customIcon: P.getBoolPref("browser.shell.customIcon.enabled") === false && P.prefIsLocked("browser.shell.customIcon.enabled"),
      shortcut: P.getBoolPref("browser.privacySegmentation.createdShortcut") === true && P.getBoolPref("browser.privateWindowSeparation.enabled") === false,
      jumpList: P.getBoolPref("browser.taskbar.lists.enabled") === false,
      appstrings: Services.strings.createBundle("chrome://browser/locale/appstrings.properties").GetStringFromName("connectionFailure"),
    };
  })();`);
  check("Firefox mascot and private browsing logo replaced with Keycat art", branding.privateLogo && branding.mascot, JSON.stringify(branding));
  check("built-in theme and error messages without the Firefox name", branding.alpenglow === "Alpenglow" && branding.appstrings.startsWith("Keycat"), `${branding.alpenglow} / ${branding.appstrings}`);
  check("help links lead to Keycat's page", branding.support === "https://github.com/vulpinepw#", branding.support);
  check("the engine creates no Start menu shortcuts or jump lists", branding.customIcon && branding.shortcut && branding.jumpList);
  check(windows ? "the engine never pins itself to the taskbar" : "the desktop's search cannot read the history", branding.pin);
  check("no Mozilla default bookmarks", !branding.bookmarks.some(t => /firefox|mozilla/i.test(t || "")), JSON.stringify(branding.bookmarks));
  check("no Firefox onboarding, tours or silent first-start actions", branding.messages && Array.isArray(branding.routerMessages) && branding.routerMessages.length === 0,
    JSON.stringify(branding.routerMessages));
  if (windows) {
    const visual = path.join(path.dirname(exe), "core", "browser", "VisualElements", "VisualElements_70.png");
    check("notification icon is Keycat's", fs.existsSync(visual) && fs.readFileSync(visual).equals(fs.readFileSync(path.join(root, "browser", "core", "browser", "VisualElements", "VisualElements_70.png"))));
  } else {
    const icon = path.join(path.dirname(exe), "core", "browser", "chrome", "icons", "default", "default48.png");
    check("window icon is Keycat's", fs.existsSync(icon) && fs.readFileSync(icon).equals(fs.readFileSync(path.join(root, "browser", "branding", "icons", "default48.png"))));
  }

  await m.exec(`Services.prefs.setStringPref("keycat.test.marker", "survives-encryption"); Services.prefs.savePrefFile(null);`);
  m.close();

  const sessionDir = path.join(dataDir, "Local", "Sessions", profile.id);
  check("decrypted session folder exists while open", fs.existsSync(sessionDir));

  t0 = Date.now();
  const locked = await api("POST", `/api/profiles/${profile.id}/lock`, {});
  check("lock accepted", locked.status === 200, locked.data.error);
  await waitFor(async () => (await profileStatus(profile.id))?.state === "locked", 90000, "sealed state");
  check("profile sealed after Firefox exit", true, `${Date.now() - t0} ms`);
  const vaultPath = path.join(dataDir, "Profiles", profile.id, "vault.kcv");
  check("vault written", fs.existsSync(vaultPath), `${fs.existsSync(vaultPath) ? fs.statSync(vaultPath).size : 0} bytes`);
  check("decrypted session folder wiped", !fs.existsSync(sessionDir));
  const vaultBytes = fs.readFileSync(vaultPath);
  check("vault has Keycat magic and no plaintext prefs", vaultBytes.subarray(0, 7).toString() === "KCVAULT" && !vaultBytes.includes("survives-encryption") && !vaultBytes.includes("user_pref"));

  const verify = await api("POST", `/api/profiles/${profile.id}/verify`, { method: "password", password: PASSWORD });
  check("vault verifies end to end", verify.status === 200 && verify.data.ok, `${verify.data.files} files`);

  await api("POST", `/api/profiles/${profile.id}/open`, { method: "password", password: PASSWORD });
  await waitFor(async () => (await profileStatus(profile.id))?.pid > 0, 90000, "browser hello (2)");
  m = await connectMarionette();
  const marker = await m.exec(`return Services.prefs.getStringPref("keycat.test.marker", "");`);
  check("data survived seal and unseal", marker === "survives-encryption", marker);
  const rescueDir = path.join(dataDir, "Profiles", profile.id, "rescue");
  check("rescue copy of vault and keys kept while the profile is open",
    fs.existsSync(path.join(rescueDir, "rescue.json")) && fs.existsSync(path.join(rescueDir, "vault.kcv")) && fs.existsSync(path.join(rescueDir, "profile.json")));
  check("no partial decrypt folder left behind", !fs.existsSync(path.join(dataDir, "Local", "Sessions", profile.id + "-unsealing")));
  m.close();
  await api("POST", `/api/profiles/${profile.id}/lock`, {});
  await waitFor(async () => (await profileStatus(profile.id))?.state === "locked", 90000, "sealed state (2)");
  check("backup of previous vault kept", fs.existsSync(vaultPath + ".bak"));
  check("rescue copy deleted once the profile is safely encrypted again", !fs.existsSync(rescueDir));
  check("decrypted copy fully wiped (no wiping folder left)", !fs.existsSync(path.join(dataDir, "Local", "Sessions", profile.id + "-wiping")));

  const restored = await api("POST", `/api/profiles/${profile.id}/restore`, { credentials: { method: "password", password: PASSWORD }, source: "backup" });
  check("previous vault verified and restored", restored.status === 200 && restored.data.ok, restored.data.error);
  const afterRestore = await api("POST", `/api/profiles/${profile.id}/verify`, { method: "password", password: PASSWORD });
  check("restored vault verifies", afterRestore.status === 200 && afterRestore.data.ok, afterRestore.data.error);
  const undone = await api("POST", `/api/profiles/${profile.id}/restore`, { credentials: { method: "password", password: PASSWORD }, source: "backup" });
  check("restore can be undone", undone.status === 200 && undone.data.ok, undone.data.error);

  const changed = await api("POST", `/api/profiles/${profile.id}/security`, {
    credentials: { method: "password", password: PASSWORD },
    action: "password",
    newPassword: "a brand new passphrase",
  });
  check("password changed without re-encrypting", changed.status === 200, changed.data.error);
  const oldPw = await api("POST", `/api/profiles/${profile.id}/verify`, { method: "password", password: PASSWORD });
  check("old password no longer works", oldPw.status === 401);
  const viaRecovery = await api("POST", `/api/profiles/${profile.id}/verify`, { method: "recovery", recoveryCode: created.data.recoveryCode.toLowerCase() });
  check("recovery code unlocks the vault", viaRecovery.status === 200, viaRecovery.data.error);

  const recipher = await api("POST", `/api/profiles/${profile.id}/security`, {
    credentials: { method: "password", password: "a brand new passphrase" },
    action: "encryption",
    cipher: "aes-256-gcm+xchacha20-poly1305",
    kdf: "scrypt",
  });
  check("cipher changed to cascade (vault re-encrypted)", recipher.status === 200 && recipher.data.profile.security.cipher === "aes-256-gcm+xchacha20-poly1305", recipher.data.error);
  const verify2 = await api("POST", `/api/profiles/${profile.id}/verify`, { method: "password", password: "a brand new passphrase" });
  check("re-encrypted vault verifies", verify2.status === 200 && verify2.data.ok, verify2.data.error);

  const eph = await api("POST", "/api/profiles", {
    name: "E2E Ephemeral",
    color: "#22B8CF",
    avatar: "initials",
    security: { mode: "ephemeral" },
    privacy: state.presets.privacy.paranoid,
    appearance: { theme: "light" },
  });
  check("ephemeral profile created", eph.status === 201, eph.data.error);
  const ephId = eph.data.profile.id;
  await api("POST", `/api/profiles/${ephId}/open`, {});
  await waitFor(async () => (await profileStatus(ephId))?.pid > 0, 90000, "ephemeral hello");
  m = await connectMarionette();
  const er = await m.exec(`return {
    pbm: Services.prefs.getBoolPref("browser.privatebrowsing.autostart"),
    rfp: Services.prefs.getBoolPref("privacy.resistFingerprinting"),
    webrtc: Services.prefs.getBoolPref("media.peerconnection.enabled", true),
    cache: Services.prefs.getBoolPref("browser.cache.memory.enable"),
    profileDir: Services.dirsvc.get("ProfD", Ci.nsIFile).path,
  };`);
  check("paranoid preset: always private, RFP, no WebRTC, no cache", er.pbm && er.rfp && !er.webrtc && !er.cache, JSON.stringify(er));
  fs.writeFileSync(path.join(outDir, "browser-ephemeral.png"), await m.screenshot());
  m.close();
  await api("POST", `/api/profiles/${ephId}/lock`, {});
  await waitFor(async () => (await profileStatus(ephId))?.state === "locked", 60000, "ephemeral closed");
  check("ephemeral session wiped", !fs.existsSync(er.profileDir), er.profileDir);
  check("ephemeral profile has no vault", !fs.existsSync(path.join(dataDir, "Profiles", ephId, "vault.kcv")));

  const del = await api("DELETE", `/api/profiles/${ephId}`, { confirm: "E2E Ephemeral" });
  check("profile deleted", del.status === 200 && !fs.existsSync(path.join(dataDir, "Profiles", ephId)));
}

main()
  .catch(err => {
    check("unexpected error", false, err.stack || String(err));
  })
  .finally(async () => {
    if (base) {
      await api("POST", "/api/quit", {}).catch(() => {});
    }
    await sleep(1500);
    launcher?.kill();
    const failed = results.filter(r => !r.ok).length;
    console.log(`\n${results.length - failed}/${results.length} checks passed`);
    try {
      fs.rmSync(dataDir, { recursive: true, force: true });
    } catch {}
    process.exit(failed ? 1 : 0);
  });
