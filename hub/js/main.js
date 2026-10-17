import { android, api, browser, currentProfileId, hasToken, inBrowser, listen } from "./api.js";
import { activity, isBusy, refresh, store } from "./store.js";
import { avatar, confirmDialog, fmtAgo, fmtBytes, h, icon, mount, openMenu, setDrawings, toast } from "./dom.js";
import { applyTheme, THEME_CYCLE, THEME_ICON, THEME_LABEL } from "./theme.js";
import { openUnlock } from "./unlock.js";
import { openWizard } from "./wizard.js";
import { openEditor } from "./editor.js";
import { openSettings } from "./settings.js";
import { securityLine } from "./views.js";
import { unlockProgress } from "./snake.js";
import { askForLinks } from "./links.js";

const root = document.getElementById("app");
const cards = new Map();
const params = new URLSearchParams(location.search);
const embed = inBrowser ? params.get("embed") || "" : "";
let shell;
let embeddedShown = false;

function buildShell() {
  const page = h("div", { class: "page" });
  const content = h("main", { class: "content" }, page);
  mount(root, h("div", { class: "app" }, content));
  shell = { page, content };
}

function render(state) {
  applyTheme(inBrowser ? "system" : state.settings.theme);
  setDrawings(state.presets.avatars);
  renderPage(state);
}


function tools(state) {
  const open = state.profiles.filter(p => p.status?.state === "running").length;
  const theme = state.settings.theme;
  return [
    open && !inBrowser
      ? h("span", { class: "status status-open", title: `${open} profile${open === 1 ? " is" : "s are"} open` },
          h("span", { class: "dot" }), `${open} open`)
      : null,
    inBrowser ? null : h("button", {
      class: "btn btn-ghost btn-icon",
      title: `Theme: ${THEME_LABEL[theme]} (click to change)`,
      "aria-label": `Theme: ${THEME_LABEL[theme]}. Switch to ${THEME_LABEL[THEME_CYCLE[theme]]}`,
      onclick: () => setTheme(THEME_CYCLE[theme]),
    }, icon(THEME_ICON[theme])),
    h("button", { class: "btn btn-ghost btn-icon", title: "Keycat settings", "aria-label": "Keycat settings", onclick: () => openSettings() },
      icon("settings")),
  ];
}

async function setTheme(theme) {
  applyTheme(theme);
  try {
    store.state.settings = await api("PUT", "/api/settings", { ...store.state.settings, theme });
    store.emit({ type: "settings" });
  } catch (err) {
    toast("bad", "Could not save the theme", err.message);
  }
}


function renderPage(state) {
  cards.clear();
  const children = [...banners(state)];
  if (!state.profiles.length) {
    children.push(h("div", { class: "page-tools" }, ...tools(state)), welcome());
  } else {
    children.push(
      h("div", { class: "page-head" },
        h("div", {},
          h("h1", { text: "Profiles" }),
          h("p", { text: "Each profile is a separate browser with its own encryption and privacy settings." })
        ),
        h("span", { class: "spacer" }),
        ...tools(state),
        h("button", { class: "btn btn-primary", onclick: () => openWizard() }, icon("plus"), "New profile")
      )
    );
    const grid = h("section", { class: "grid", "aria-label": "Profiles" });
    for (const p of state.profiles) {
      const c = card(p);
      cards.set(p.id, c);
      grid.append(c);
    }
    grid.append(
      h("button", { class: "new-card", onclick: () => openWizard() },
        h("span", { class: "plus" }, icon("plus", "icon-lg")),
        "New profile"
      )
    );
    children.push(grid);
  }
  children.push(footer(state));
  mount(shell.page, ...children);
}

function banners(state) {
  const out = [];
  const up = state.update || {};
  if (up.busy) {
    const label = { download: "Downloading the browser engine", verify: "Checking the engine's signature", extract: "Unpacking", prepare: "Preparing the engine" }[up.phase] || "Updating the browser engine";
    out.push(h("div", { class: "banner info" }, icon("refresh"),
      h("div", { class: "text" }, h("strong", { text: label }),
        h("div", { class: "progress" + (up.phase === "download" ? "" : " indeterminate") },
          h("span", { vars: { "--p": `${Math.round((up.progress || 0) * 100)}%` } })))));
  } else if (up.staged) {
    out.push(h("div", { class: "banner" }, icon("check"),
      h("div", { class: "text" }, h("strong", { text: `Browser engine ${up.staged} is ready.` }), " Restart Keycat to start using it."),
      h("button", { class: "btn btn-primary btn-sm", onclick: restartKeycat }, "Restart Keycat")));
  } else if (up.available) {
    out.push(h("div", { class: "banner info" }, icon("info"),
      h("div", { class: "text" }, h("strong", { text: `Browser engine ${up.latest} is available.` }), ` You have ${up.current}. Updates include security fixes.`),
      h("button", { class: "btn btn-sm", onclick: startUpdate }, icon("download"), "Update engine")));
  }
  return out;
}

export async function startUpdate() {
  try {
    await api("POST", "/api/core/update", {});
  } catch (err) {
    toast("bad", "The update could not start", err.message);
  }
}

export async function restartKeycat() {
  try {
    await api("POST", "/api/restart", {});
  } catch (err) {
    toast("bad", "Keycat could not restart", err.message);
  }
}

function welcome() {
  const point = (iconName, cls, title, text) =>
    h("div", { class: "point" },
      h("span", { class: `choice-icon ${cls}` }, icon(iconName)),
      h("div", {}, h("strong", { text: title }), h("span", { text }))
    );
  return h("section", { class: "welcome" },
    h("div", {},
      h("h1", { text: "Welcome to Keycat" }),
      h("p", { text: "Every profile is a separate browser kept in its own encrypted vault. Cookies, history, passwords and site data stay locked until you open the profile." }),
      h("button", { class: "btn btn-primary", onclick: () => openWizard() }, icon("plus"), "Create your first profile")
    ),
    h("div", { class: "points" },
      point("lock", "", "Encrypted at rest", "AES-256-GCM, XChaCha20-Poly1305 or both in a cascade, with Argon2id key derivation."),
      point("shieldCheck", "good", "Private by default", "Strict tracking protection, HTTPS-Only, encrypted DNS and no telemetry."),
      point("layers", "info", "Separate identities", "Profiles never share cookies, history or extensions, and share logins only when you choose to.")
    )
  );
}

function footer(state) {
  return h("footer", { class: "footer" },
    android ? null : h("button", {
      class: "link-btn item",
      onclick: () => api("POST", "/api/open-folder", { target: "data" }).catch(err => toast("bad", "Could not open the folder", err.message)),
    }, icon("folder"), "Data folder"),
    state.app.portable ? h("span", { class: "item" }, icon("bolt"), "Portable mode") : null,
    h("span", { class: "item credit" }, `Keycat ${state.app.version} by ${state.app.author || "tordev"}`)
  );
}


function card(p) {
  const st = p.status || { state: "locked" };
  const busy = isBusy(st.state);
  const current = inBrowser && p.id === currentProfileId;
  return h("article", {
    class: `card profile ${st.state === "running" ? "is-open" : ""} ${current ? "is-current" : ""}`,
    vars: { "--pc": p.color },
    "aria-label": p.name,
    dataset: { id: p.id },
  },
    h("div", { class: "profile-top" }, avatar(p), statusPill(p, st)),
    h("div", {}, h("h3", { class: "profile-name", text: p.name, title: p.name }), securityLine(p)),
    h("div", { class: "chips" }, ...chips(p)),
    h("div", { class: "profile-bottom" },
      busy ? unlockProgress(p, st) : h("div", { class: "profile-meta", text: meta(p, st), title: meta(p, st) }),
      st.state === "error" ? h("div", { class: "error-text", text: st.error }) : null,
      actions(p, st)
    )
  );
}

function chips(p) {
  const pv = p.privacy;
  const label = { balanced: "Balanced", strict: "Strict", paranoid: "Paranoid" }[pv.preset] || "Custom";
  const out = [h("span", { class: "chip" }, icon("shield"), label)];
  if (pv.alwaysPrivate) {
    out.push(h("span", { class: "chip", text: "Private mode" }));
  }
  if (pv.proxy?.mode === "tor") {
    out.push(h("span", { class: "chip", text: "Tor" }));
  } else if (pv.proxy && pv.proxy.mode !== "none") {
    out.push(h("span", { class: "chip", text: "Proxy" }));
  }
  const spoofed = { windows: "Spoofs Windows", macos: "Spoofs macOS", linux: "Spoofs Linux", random: "Random system" }[pv.userAgent];
  if (spoofed && pv.fingerprinting !== "resist") {
    out.push(h("span", { class: "chip", text: spoofed }));
  }
  if (pv.dns !== "system") {
    out.push(h("span", { class: "chip", text: "Encrypted DNS" }));
  }
  if (pv.ublockOrigin) {
    out.push(h("span", { class: "chip", text: "uBlock Origin" }));
  }
  if (p.security.unlock.includes("windows")) {
    out.push(h("span", { class: "chip", text: "Windows unlock" }));
  }
  return out.slice(0, 4);
}

function meta(p, st) {
  if (st.notice) {
    return st.notice;
  }
  const parts = [p.lastUsed ? `Opened ${fmtAgo(p.lastUsed)}` : "Never opened"];
  if (p.security.mode === "encrypted" && p.stats?.vaultBytes) {
    parts.push(`${fmtBytes(p.stats.vaultBytes)} vault`);
  }
  return parts.join(" · ");
}

function statusPill(p, st) {
  const pill = (cls, text, dot = true) =>
    h("span", { class: `status ${cls}` }, dot ? h("span", { class: "dot" }) : icon("lock"), text);
  switch (st.state) {
    case "running":
      return pill("status-open", inBrowser && p.id === currentProfileId ? "This window" : "Open");
    case "unlocking":
      return pill("status-busy", "Unlocking");
    case "sealing":
      return pill("status-busy", st.phase === "wipe" ? "Wiping" : "Encrypting");
    case "closing":
      return pill("status-busy", "Closing");
    case "unsealed":
      return pill("status-alert", "Unsealed data");
    case "error":
      return pill("status-alert", "Needs attention");
  }
  if (p.security.mode === "encrypted") {
    return pill("", "Locked", false);
  }
  return h("span", { class: "status" }, p.security.mode === "ephemeral" ? "Ready" : "Closed");
}

function actions(p, st) {
  const more = h("button", {
    class: "btn btn-icon btn-outline",
    "aria-label": `More actions for ${p.name}`,
    "aria-haspopup": "menu",
    dataset: { action: "more" },
    onclick: event => openMenu(event.currentTarget, menuItems(p, st)),
  }, icon("more"));
  const button = (label, iconName, onclick, primary = false, action = label) =>
    h("button", { class: `btn ${primary ? "btn-primary" : ""}`, dataset: { action }, onclick }, icon(iconName), label);
  const row = (...items) => h("div", { class: "profile-actions" }, ...items);
  const closeLabel = p.security.mode === "encrypted" ? "Lock" : p.security.mode === "ephemeral" ? "Erase" : "Close";

  switch (st.state) {
    case "running":
      if (inBrowser && p.id === currentProfileId) {
        return row(button("Settings", "settings", () => browser.openSettings("keycat"), false, "Show"),
          button(closeLabel, p.security.mode === "encrypted" ? "lock" : "power", () => lockProfile(p), true), more);
      }
      return row(button("Show", "focus", () => focusProfile(p)), button(closeLabel, p.security.mode === "encrypted" ? "lock" : "power", () => lockProfile(p), true), more);
    case "closing":
      return row(button("Force close", "power", () => forceClose(p)), more);
    case "unsealed":
      return row(button("Secure now", "lock", () => openUnlock(p.id, "secure"), true), more);
    case "error":
      return row(button("Retry", "refresh", () => retrySeal(p), true), more);
    case "unlocking":
    case "sealing": {
      const b = button("Open", "unlock", () => {}, true);
      b.disabled = true;
      more.disabled = true;
      return row(b, more);
    }
  }
  return row(button("Open", p.security.mode === "encrypted" ? "unlock" : "chevronRight", () => openProfile(p), true), more);
}

function menuItems(p, st) {
  const encrypted = p.security.mode === "encrypted";
  const closed = st.state === "locked";
  return [
    { label: "Edit profile", icon: "edit", onclick: () => editProfile(p, "profile") },
    { label: "Privacy settings", icon: "shield", onclick: () => editProfile(p, "privacy") },
    encrypted ? { label: "Passwords", icon: "key", onclick: () => editProfile(p, "passwords") } : null,
    { label: "Fake identity", icon: "user", onclick: () => editProfile(p, "identity") },
    { label: android ? "Import from a file" : "Import from another browser", icon: "download", onclick: () => importData(p) },
    encrypted ? { label: "Encryption and unlock", icon: "lock", onclick: () => editProfile(p, "security") } : null,
    encrypted && closed ? { label: "Verify vault", icon: "shieldCheck", onclick: () => openUnlock(p.id, "verify") } : null,
    st.state === "unsealed" ? { label: "Open profile", icon: "unlock", onclick: () => openUnlock(p.id, "open") } : null,
    "-",
    { label: "Delete profile", icon: "trash", danger: true, onclick: () => deleteProfile(p) },
  ].filter(Boolean);
}

function editProfile(p, tab) {
  if (embed && p.id === currentProfileId) {
    browser.openSettings(tab === "passwords" ? "keycatPasswords" : "keycat");
    return;
  }
  openEditor(p.id, tab);
}


export async function openProfile(p, extra = {}) {
  if (p.security.mode === "encrypted") {
    const unlock = p.security.unlock;
    if (!unlock.includes("password") && unlock.includes("windows") && store.state.app.windowsUnlock) {
      try {
        await api("POST", `/api/profiles/${p.id}/open`, { method: "windows", ...extra });
      } catch (err) {
        toast("bad", `${p.name} could not be opened`, err.message);
      }
      return;
    }
    openUnlock(p.id, "open", extra);
    return;
  }
  try {
    await api("POST", `/api/profiles/${p.id}/open`, extra);
  } catch (err) {
    toast("bad", `${p.name} could not be opened`, err.message);
  }
}

export async function importData(p) {
  const p2 = store.profile(p.id) || p;
  if (p2.status?.state === "running") {
    try {
      await api("POST", `/api/profiles/${p.id}/import`, {});
      if (!(inBrowser && p.id === currentProfileId)) {
        toast("info", "Import window opened", `Choose the browser to import from in ${p.name}.`);
      }
    } catch (err) {
      toast("bad", "Could not start the import", err.message);
    }
    return;
  }
  if (isBusy(p2.status?.state || "locked") || p2.status?.state === "unsealed" || p2.status?.state === "error") {
    toast("info", "Not right now", `${p.name} is busy. Try again once it is closed or open.`);
    return;
  }
  openProfile(p2, { then: "import" });
}

async function focusProfile(p) {
  try {
    await api("POST", `/api/profiles/${p.id}/open`, {});
  } catch (err) {
    toast("bad", "Could not show the window", err.message);
  }
}

async function lockProfile(p) {
  try {
    await api("POST", `/api/profiles/${p.id}/lock`, {});
  } catch (err) {
    toast("bad", `${p.name} could not be closed`, err.message);
  }
}

async function forceClose(p) {
  const ok = await confirmDialog({
    title: `Force ${p.name} to close?`,
    text: "The browser is closed immediately. Anything it has not saved yet (for example a form you are typing in) is lost. The profile is encrypted again afterwards.",
    confirmLabel: "Force close",
    danger: true,
  });
  if (ok) {
    await api("POST", `/api/profiles/${p.id}/force-close`, {}).catch(err => toast("bad", "Could not close the browser", err.message));
  }
}

async function retrySeal(p) {
  try {
    await api("POST", `/api/profiles/${p.id}/retry-seal`, {});
  } catch (err) {
    toast("bad", "Encryption failed again", err.message);
  }
}

async function deleteProfile(p) {
  if (p.status?.state && p.status.state !== "locked") {
    toast("info", "Close the profile first", `${p.name} must be closed before it can be deleted.`);
    return;
  }
  const typed = await confirmDialog({
    title: `Delete ${p.name}?`,
    text: "Its vault, bookmarks, passwords, history and settings are permanently erased. This cannot be undone.",
    confirmLabel: "Delete forever",
    danger: true,
    typeToConfirm: p.name,
  });
  if (!typed) {
    return;
  }
  try {
    await api("DELETE", `/api/profiles/${p.id}`, { confirm: typed });
    toast("good", `${p.name} was deleted`, "All of its data was wiped.");
  } catch (err) {
    toast("bad", "The profile could not be deleted", err.message);
  }
}


function updateCard(id) {
  const old = cards.get(id);
  const p = store.profile(id);
  if (!old || !p) {
    return;
  }
  const focused = old.contains(document.activeElement) ? document.activeElement.dataset?.action : null;
  const next = card(p);
  old.replaceWith(next);
  cards.set(id, next);
  if (focused) {
    next.querySelector(`[data-action="${focused}"]`)?.focus();
  }
}

let lastStates = {};

function notifyTransitions(state) {
  for (const p of state.profiles) {
    const prev = lastStates[p.id];
    const now = p.status?.state || "locked";
    if (prev === "sealing" && now === "locked" && p.security.mode === "encrypted") {
      toast("good", `${p.name} is locked`, "The profile was encrypted and the decrypted copy wiped.");
    } else if (prev && prev !== "error" && now === "error") {
      toast("bad", `${p.name} needs attention`, p.status.error);
    }
    lastStates[p.id] = now;
  }
}

function renderEmbedded(state) {
  applyTheme("system");
  setDrawings(state.presets.avatars);
  if (embed === "profile") {
    const tabs = (params.get("tabs") || "").split(",").filter(Boolean);
    const id = params.get("id") || currentProfileId;
    if (!store.profile(id)) {
      mount(shell.page, h("div", { class: "callout" }, icon("info"), h("span", { text: "This profile was not found." })));
      return;
    }
    openEditor(id, params.get("tab") || tabs[0] || "profile", { inline: shell.page, only: tabs });
  } else if (embed === "settings") {
    openSettings({ inline: shell.page });
  }
}

function openFromHash() {
  const hash = new URLSearchParams(location.hash.slice(1));
  if (hash.has("settings")) {
    openSettings();
  } else if (hash.get("profile") && store.profile(hash.get("profile"))) {
    openEditor(hash.get("profile"), hash.get("tab") || "profile");
  }
}

function boot() {
  for (const type of ["dragover", "drop"]) {
    document.addEventListener(type, event => event.preventDefault());
  }
  document.addEventListener("contextmenu", event => {
    if (!event.target.closest("input, textarea")) {
      event.preventDefault();
    }
  });
  document.addEventListener("auxclick", event => event.button === 1 && event.preventDefault());
  if (inBrowser) {
    document.documentElement.dataset.inbrowser = "1";
    applyTheme("system");
  }
  if (embed) {
    document.documentElement.dataset.embed = embed;
  }

  buildShell();
  if (!hasToken()) {
    mount(shell.page, h("div", { class: "welcome" },
      h("div", {}, h("h1", { text: "Open Keycat from its shortcut" }),
        h("p", { text: "This page only works inside Keycat." }))));
    return;
  }
  let first = true;
  store.on((state, change) => {
    if (!state) {
      return;
    }
    if (change.type === "keychain") {
      return;
    }
    if (change.type === "links") {
      if (!inBrowser) {
        askForLinks();
      }
      return;
    }
    notifyTransitions(state);
    if (embed === "profile" || embed === "settings") {
      if (!embeddedShown) {
        embeddedShown = true;
        renderEmbedded(state);
      }
      return;
    }
    if (change.type === "status" && cards.has(change.id)) {
      updateCard(change.id);
    } else {
      render(state);
    }
    if (first) {
      first = false;
      openFromHash();
    }
    if (!inBrowser) {
      askForLinks();
    }
  });
  window.addEventListener("hashchange", () => openFromHash());
  listen(event => {
    switch (event.type) {
      case "status":
        store.setStatus(event.data);
        break;
      case "profiles":
        refresh().catch(() => {});
        break;
      case "settings":
        store.state.settings = event.data;
        store.emit({ type: "settings" });
        break;
      case "core":
        store.state.update = event.data;
        store.emit({ type: "core" });
        break;
      case "keychain":
        store.emit({ type: "keychain", id: event.data?.profileId });
        break;
      case "links":
        store.state.links = event.data || [];
        store.emit({ type: "links" });
        break;
    }
  }, () => refresh().catch(() => {}));
  refresh().catch(err => {
    mount(shell.page, h("div", { class: "banner" }, icon("alert"), h("div", { class: "text", text: err.message })));
  });
}

boot();
