import { api } from "./api.js";
import { refresh, store } from "./store.js";
import { avatar, confirmDialog, drawer, fmtAgo, fmtBytes, focusFirst, h, icon, inlinePanel, mount, switchRow, toast } from "./dom.js";
import { securityLine, section } from "./views.js";
import { identityEditor, passwordFields, themePicker } from "./wizard.js";
import { privacyEditor } from "./privacy.js";
import { openUnlock } from "./unlock.js";
import { identityTab, passwordsTab } from "./passwords.js";
import { importData } from "./main.js";

export function openEditor(id, initialTab = "profile", options = {}) {
  const state = store.state;
  const p = store.profile(id);
  if (!p) {
    return;
  }
  const encrypted = p.security.mode === "encrypted";
  let tabs = [
    ["profile", "Profile"],
    ["privacy", "Privacy"],
    ...(encrypted ? [["security", "Security"]] : []),
    ...(encrypted ? [["passwords", "Passwords"]] : []),
    ["identity", "Fake identity"],
  ];
  if (options.only?.length) {
    tabs = tabs.filter(([t]) => options.only.includes(t));
  }
  if (!tabs.length) {
    const host = options.inline || document.body;
    mount(host, h("div", { class: "callout" }, icon("info"),
      h("span", { text: "Keycat Passwords is available in encrypted profiles." })));
    return;
  }
  let tab = tabs.some(([t]) => t === initialTab) ? initialTab : tabs[0][0];
  let cleanup = () => {};
  let pendingCode = null;
  const dlg = options.inline
    ? inlinePanel(options.inline, `Settings of ${p.name}`)
    : drawer({ label: `Edit ${p.name}`, onClose: () => cleanup() });

  const head = h("div", { class: "drawer-head" });
  const tabBar = h("div", { class: "tabs", role: "tablist", hidden: tabs.length < 2 });
  const body = h("div", { class: "drawer-body" });
  const foot = h("div", { class: "drawer-foot" });
  mount(dlg.panel, head, tabBar, body, foot);

  function renderHead() {
    const cur = store.profile(id) || p;
    mount(head,
      avatar(cur, "sm"),
      h("div", {}, h("h2", { text: tabs.length === 1 && tab === "passwords" ? "Keycat Passwords" : cur.name }), securityLine(cur)),
      dlg.inline ? null : h("button", { class: "btn btn-ghost btn-icon close", "aria-label": "Close", onclick: () => dlg.close() }, icon("x"))
    );
  }

  function renderTabs() {
    mount(tabBar, ...tabs.map(([key, label]) => h("button", {
      role: "tab", "aria-selected": String(key === tab),
      onclick: () => {
        tab = key;
        renderTabs();
        renderBody();
      },
    }, label)));
  }

  function renderBody() {
    cleanup();
    cleanup = () => {};
    mount(foot);
    foot.hidden = true;
    const ctx = { dlg, body, foot, setCleanup: fn => (cleanup = fn), rerenderHead: renderHead };
    const view = { profile: profileTab, privacy: privacyTab, security: securityTab, passwords: passwordsTab, identity: identityTab }[tab];
    mount(body, view(store.profile(id) || p, ctx));
    body.scrollTop = 0;
  }

  function profileTab(cur, ctx) {
    const draft = { name: cur.name, color: cur.color, avatar: cur.avatar };
    let theme = cur.appearance.theme;
    let keepBackup = cur.security.keepBackup;
    let keepCache = cur.security.keepCache;
    const themes = themePicker(theme, id => (theme = id));

    showFooter(ctx, "Save changes", async () => {
      const body = { name: draft.name, color: draft.color, avatar: draft.avatar, appearance: { theme } };
      if (encrypted) {
        body.keepBackup = keepBackup;
        body.keepCache = keepCache;
      }
      await api("PATCH", `/api/profiles/${id}`, body);
      await refresh();
      renderHead();
      toast("good", "Profile saved", "");
    });

    return h("div", {},
      h("section", { class: "section" }, h("h3", { class: "section-title", text: "Identity" }),
        identityEditor(state, draft, () => {})),
      h("section", { class: "section" }, h("h3", { class: "section-title", text: "Browser theme" }), themes,
        theme === "firefox" ? h("p", { class: "hint", text: "This profile uses a theme chosen in the add-ons manager. Pick one above to switch back to a built-in theme." }) : null),
      section("Import data",
        h("div", { class: "toggle-row" },
          h("span", { class: "text" }, h("div", { class: "title", text: "Import from another browser" }),
            h("div", {
              class: "hint",
              text: encrypted
                ? "Bookmarks (favorites), history, saved form data and passwords from Edge, Chrome, Brave, Opera, Vivaldi or another browser on this PC, or from a passwords or bookmarks file. Everything is encrypted with this profile."
                : "Bookmarks (favorites), history, saved form data and passwords from Edge, Chrome, Brave, Opera, Vivaldi or another browser on this PC, or from a passwords or bookmarks file.",
            })),
          h("button", { class: "btn btn-sm", type: "button", onclick: () => importData(cur) }, icon("download"), "Import"))
      ),
      encrypted ? section("Vault",
        switchRow({ title: "Keep a backup of the previous vault", checked: keepBackup, onchange: on => (keepBackup = on) }),
        switchRow({ title: "Keep the disk cache between sessions", hint: "Faster page loads, larger vault. Off means the cache is erased on close.", checked: keepCache, onchange: on => (keepCache = on) })
      ) : null,
      h("section", { class: "section danger-zone" },
        h("h3", { class: "section-title", text: "Danger zone" }),
        h("div", { class: "panel" }, h("div", { class: "toggle-row" },
          h("span", { class: "text" }, h("div", { class: "title", text: "Delete this profile" }),
            h("div", { class: "hint", text: "Wipes its vault, passwords, history and settings for good." })),
          h("button", { class: "btn btn-danger btn-sm", type: "button", onclick: () => deleteProfile(cur) }, icon("trash"), "Delete")
        ))
      )
    );
  }

  async function deleteProfile(cur) {
    if (cur.status?.state && cur.status.state !== "locked") {
      toast("info", "Close the profile first", `${cur.name} must be closed before it can be deleted.`);
      return;
    }
    const typed = await confirmDialog({
      title: `Delete ${cur.name}?`,
      text: "Its vault, bookmarks, passwords, history and settings are permanently erased. This cannot be undone.",
      confirmLabel: "Delete forever",
      danger: true,
      typeToConfirm: cur.name,
    });
    if (!typed) {
      return;
    }
    try {
      await api("DELETE", `/api/profiles/${id}`, { confirm: typed });
      dlg.close();
      toast("good", `${cur.name} was deleted`, "All of its data was wiped.");
    } catch (err) {
      toast("bad", "The profile could not be deleted", err.message);
    }
  }

  function privacyTab(cur, ctx) {
    let privacy = structuredClone(cur.privacy);
    const running = cur.status?.state === "running";
    showFooter(ctx, "Save privacy settings", async () => {
      const result = await api("PATCH", `/api/profiles/${id}`, { privacy });
      await refresh();
      toast("good", "Privacy settings saved",
        result.restartRequired ? "Private mode, the password manager and identity autofill change the next time the profile opens. Everything else applies now."
          : running ? "Applied to the open browser." : "");
    });
    return h("div", {},
      running ? h("div", { class: "callout" }, icon("info"), h("span", { text: "This profile is open. Saved changes apply to it right away, except private mode, the password manager and identity autofill, which change the next time it opens." })) : null,
      privacyEditor({ value: privacy, presets: state.presets, mode: cur.security.mode, onChange: v => (privacy = v) })
    );
  }

  function securityTab(cur) {
    const s = cur.security;
    const has = slot => s.unlock.includes(slot);
    const actionHost = h("div");
    const overview = h("dl", { class: "kv" },
      h("dt", { text: "Cipher" }), h("dd", { text: s.cipherName }),
      h("dt", { text: "Key derivation" }), h("dd", { text: s.kdfName || "None (Windows account only)" }),
      h("dt", { text: "Unlock with" }), h("dd", { text: [has("password") && "Password", has("recovery") && "Recovery code", has("windows") && "Windows account"].filter(Boolean).join(", ") }),
      h("dt", { text: "Vault size" }), h("dd", { text: cur.stats?.vaultBytes ? fmtBytes(cur.stats.vaultBytes) : "Not created yet" }),
      h("dt", { text: "Last sealed" }), h("dd", { text: cur.stats?.lastSealed ? `${fmtAgo(cur.stats.lastSealed)} (${cur.stats.lastSealMs} ms)` : "Never" }),
      h("dt", { text: "Last unlock" }), h("dd", { text: cur.stats?.lastUnlockMs ? `${cur.stats.lastUnlockMs} ms to derive the key` : "Not opened yet" })
    );
    const actionRow = (title, hint, buttons) => h("div", { class: "toggle-row" },
      h("span", { class: "text" }, h("div", { class: "title", text: title }), h("div", { class: "hint", text: hint })),
      h("div", { class: "row-buttons" }, ...buttons));
    const btn = (label, iconName, onclick, cls = "") => h("button", { class: `btn btn-sm ${cls}`, type: "button", onclick }, icon(iconName), label);

    if (pendingCode) {
      const code = pendingCode;
      pendingCode = null;
      queueMicrotask(() => showCode(code));
    }

    return h("div", {},
      section("Encryption", overview),
      section("Unlock methods",
        actionRow("Password", "Change the password. The vault is not re-encrypted, so this is instant.",
          [btn("Change", "key", () => showAction("password"))]),
        actionRow("Recovery code", has("recovery") ? "A recovery code exists. Creating a new one invalidates the old one." : "No recovery code. Without one, a forgotten password means losing the profile.",
          has("recovery")
            ? [btn("New code", "refresh", () => showAction("recovery-new")), btn("Remove", "trash", () => showAction("recovery-remove"))]
            : [btn("Create", "plus", () => showAction("recovery-new"))]),
        (has("windows") || state.app.windowsUnlock) &&
          actionRow("Windows account", has("windows") ? "This profile opens without a password while you are signed in to Windows." : "Open this profile without typing a password while signed in to Windows.",
            has("windows")
              ? [btn("Turn off", "power", () => showAction("windows-off"))]
              : [btn("Turn on", "windows", () => showAction("windows-on"))])
      ),
      section("Cipher",
        actionRow("Change encryption", "Re-encrypts the vault with another cipher or key derivation. Takes a few seconds.",
          [btn("Change", "lock", () => showAction("encryption"))]),
        actionRow("Verify vault", "Decrypts every block in memory to prove the vault is intact.",
          [btn("Verify", "shieldCheck", () => afterDialog(openUnlock(id, "verify")))])
      ),
      rescueSection(cur),
      actionHost
    );

    function afterDialog(done) {
      done.then(ok => ok && refresh().then(() => {
        renderHead();
        renderBody();
      })).catch(() => {});
    }

    function rescueSection(cur) {
      const closed = (cur.status?.state || "locked") === "locked";
      const rescue = cur.rescue;
      const backup = cur.backup;
      return section("Rescue and backups",
        actionRow("Automatic rescue copy",
          rescue
            ? `Kept since ${fmtAgo(rescue.created)} because an operation on this profile did not finish. The next unlock checks the vault and deletes the copy, or puts it back if the vault is damaged.`
            : "Before Keycat decrypts or re-encrypts this profile, it keeps a copy of the vault and its keys, and deletes it once the profile is safely encrypted again. None is needed right now.",
          rescue && closed
            ? [btn("Check", "shieldCheck", () => afterDialog(openUnlock(id, "verify"))), btn("Restore", "history", () => afterDialog(openUnlock(id, "restore-rescue")))]
            : []),
        actionRow("Previous vault",
          backup
            ? `Saved ${fmtAgo(backup.modified)} · ${fmtBytes(backup.bytes)}. Restoring swaps it with the current vault, so it can be undone.${closed ? "" : " Close the profile to restore it."}`
            : cur.security.keepBackup
              ? "Kept from the next time the profile is encrypted."
              : "Turn on \"Keep a backup of the previous vault\" in the Profile tab to keep one.",
          backup && closed ? [btn("Restore", "history", () => afterDialog(openUnlock(id, "restore-backup")))] : [])
      );
    }

    function showAction(action) {
      const titles = {
        password: "Change password",
        "recovery-new": "Create a new recovery code",
        "recovery-remove": "Remove the recovery code",
        "windows-on": "Turn on Windows account unlock",
        "windows-off": "Turn off Windows account unlock",
        encryption: "Change encryption",
      };
      const cred = credentialFields(cur, action === "encryption");
      const extra = h("div");
      let newPw = null;
      let cipher = s.cipher;
      let kdf = s.kdf && s.kdf !== "custom" ? s.kdf : "argon2id-standard";
      if (action === "password") {
        newPw = passwordFields({ label: "New password" });
        extra.append(newPw.node);
      }
      if (action === "encryption") {
        const sel = (label, options, value, set) => {
          const el = h("select", { class: "select" }, options.map(([v, t]) => h("option", { value: v, text: t, selected: v === value })));
          el.addEventListener("change", () => set(el.value));
          return h("div", { class: "field" }, h("span", { class: "label", text: label }), el);
        };
        extra.append(
          sel("Cipher", state.presets.ciphers.map(c => [c.id, c.name]), cipher, v => (cipher = v)),
          sel("Key derivation", state.presets.kdfs.map(k => [k.id, k.name]), kdf, v => (kdf = v)));
      }
      const error = h("div", { class: "error-text", role: "alert" });
      const submit = h("button", { class: "btn btn-primary", type: "submit" }, titles[action]);
      const form = h("form", { class: "card action-card" },
        h("h3", { class: "action-title", text: titles[action] }),
        cred.node, extra, error,
        h("div", { class: "row-buttons" },
          h("button", { class: "btn btn-ghost", type: "button", text: "Cancel", onclick: () => mount(actionHost) }),
          submit));
      form.addEventListener("submit", async event => {
        event.preventDefault();
        error.textContent = "";
        if (newPw && (newPw.value.length < 8 || !newPw.confirmed)) {
          error.textContent = newPw.value.length < 8 ? "The new password must be at least 8 characters." : "The new passwords do not match.";
          return;
        }
        submit.disabled = true;
        mount(submit, action === "encryption" ? "Re-encrypting" : "Working");
        try {
          const result = await api("POST", `/api/profiles/${id}/security`, {
            credentials: cred.value(),
            action,
            newPassword: newPw?.value || "",
            cipher: action === "encryption" ? cipher : "",
            kdf: action === "encryption" ? kdf : "",
          });
          await refresh();
          if (result.recoveryCode) {
            pendingCode = result.recoveryCode;
          } else {
            toast("good", titles[action], "Done.");
          }
          renderHead();
          renderBody();
        } catch (err) {
          submit.disabled = false;
          mount(submit, titles[action]);
          error.textContent = err.code === "wrong_password" ? "That password is not correct." : err.message;
        }
      });
      mount(actionHost, form);
      form.scrollIntoView({ behavior: "smooth", block: "nearest" });
      focusFirst(form);
    }

    function showCode(code) {
      mount(actionHost, h("div", { class: "card action-card" },
        h("h3", { class: "action-title", text: "Your new recovery code" }),
        h("p", { class: "hint", text: "Store it somewhere safe and offline. The previous code no longer works." }),
        h("div", { class: "recovery-code" }, ...code.split("-").map(g => h("span", { text: g }))),
        h("div", { class: "row-buttons" },
          h("button", {
            class: "btn", type: "button",
            onclick: () => navigator.clipboard.writeText(code).then(() => toast("good", "Recovery code copied", "")),
          }, icon("copy"), "Copy"),
          h("button", { class: "btn btn-primary", type: "button", text: "Done", onclick: () => mount(actionHost) }))
      ));
    }
  }

  renderHead();
  renderTabs();
  renderBody();
}

function showFooter(ctx, label, save) {
  const error = h("span", { class: "error-text", role: "alert" });
  const button = h("button", { class: "btn btn-primary", type: "button" }, icon("check"), label);
  button.addEventListener("click", async () => {
    button.disabled = true;
    error.textContent = "";
    try {
      await save();
    } catch (err) {
      error.textContent = err.message;
    } finally {
      button.disabled = false;
    }
  });
  ctx.foot.hidden = false;
  mount(ctx.foot, error, h("span", { class: "spacer" }), button);
}

export function credentialFields(p, passwordOnly = false) {
  let method = p.security.unlock.includes("password") ? "password" : "recovery";
  const host = h("div");
  const input = h("input", { class: "input", autocomplete: "current-password", spellcheck: "false" });
  const render = () => {
    input.type = method === "password" ? "password" : "text";
    input.classList.toggle("input-mono", method === "recovery");
    input.value = "";
    const canSwitch = !passwordOnly && p.security.unlock.includes("recovery") && p.security.unlock.includes("password");
    mount(host, h("div", { class: "field" },
      h("span", { class: "label", text: method === "password" ? "Current password" : "Recovery code" }),
      input,
      canSwitch ? h("button", {
        type: "button", class: "link-btn hint-link",
        text: method === "password" ? "Use the recovery code instead" : "Use the password instead",
        onclick: () => {
          method = method === "password" ? "recovery" : "password";
          render();
          input.focus();
        },
      }) : null));
  };
  render();
  return {
    node: host,
    value: () => (method === "password" ? { method, password: input.value } : { method, recoveryCode: input.value }),
  };
}
