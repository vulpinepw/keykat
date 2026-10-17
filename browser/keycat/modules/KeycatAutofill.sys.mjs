const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  KeycatLink: "resource://keycat/modules/KeycatLink.sys.mjs",
  KeycatSettings: "resource://keycat/modules/KeycatSettings.sys.mjs",
});

const HTML_NS = "http://www.w3.org/1999/xhtml";
const PANEL_ID = "keycat-autofill-panel";
const ICONS = {
  login: "chrome://browser/skin/login.svg",
  identity: "chrome://browser/skin/personal-info-16.svg",
  another: "chrome://global/skin/icons/reload.svg",
  manage: "chrome://global/skin/icons/settings.svg",
};

let current = null;
let sequence = 0;

function el(doc, tag, props = {}, ...children) {
  const node = doc.createElementNS(HTML_NS, tag);
  for (const [k, v] of Object.entries(props)) {
    if (v === undefined || v === null) {
      continue;
    }
    if (k === "class") {
      node.className = v;
    } else if (k === "text") {
      node.textContent = v;
    } else if (k.startsWith("on")) {
      node.addEventListener(k.slice(2), v);
    } else {
      node.setAttribute(k, v);
    }
  }
  for (const child of children) {
    if (child) {
      node.append(child);
    }
  }
  return node;
}

function hostOf(origin) {
  try {
    return new URL(origin).host;
  } catch {
    return origin;
  }
}

const api = (method, path, body) => lazy.KeycatLink.call(method, path, body);

function send(actor, name, data) {
  try {
    return actor.sendQuery(name, data).catch(() => null);
  } catch (ex) {
    return Promise.resolve(null);
  }
}

export const KeycatAutofill = {
  attachWindow(win) {
    const doc = win.document;
    const menu = doc.getElementById("contentAreaContextMenu");
    if (!menu || doc.getElementById("keycat-context-autofill")) {
      return;
    }
    const item = doc.createXULElement("menuitem");
    item.id = "keycat-context-autofill";
    item.setAttribute("label", "Fill with Keycat");
    item.setAttribute("accesskey", "K");
    const separator = doc.createXULElement("menuseparator");
    separator.id = "keycat-context-autofill-separator";
    item.addEventListener("command", () => {
      const actor = win.gContextMenu?.actor?.manager?.getActor("KeycatAutofill");
      if (actor) {
        send(actor, "KeycatAutofill:ContextShow", {});
      }
    });
    menu.addEventListener("popupshowing", event => {
      if (event.target !== menu) {
        return;
      }
      const ctx = win.gContextMenu;
      const show = !!ctx && !!(ctx.onTextInput || ctx.onPassword);
      item.hidden = separator.hidden = !show;
    });
    const anchor = doc.getElementById("context-undo");
    menu.insertBefore(item, anchor || menu.firstChild);
    menu.insertBefore(separator, anchor || item.nextSibling);
  },

  async show(actor, origin, field) {
    const win = actor.browser?.ownerDocument?.defaultView;
    if (!win || !lazy.KeycatLink.available) {
      return;
    }
    const token = ++sequence;
    const wantLogins = field.kind === "password" || field.kind === "username";
    const wantIdentity = !field.auto && (field.kind === "identity" || field.signup ||
      (field.kind === "username" && !field.hasPasswords));
    const [keychain, identity] = await Promise.all([
      api("GET", "/api/instance/keychain?origin=" + encodeURIComponent(origin)).catch(() => null),
      wantIdentity ? api("GET", "/api/instance/identity").catch(() => null) : null,
    ]);
    if (token !== sequence) {
      return;
    }
    const passwordsOn = !!keychain?.enabled;
    let generated = null;
    if (passwordsOn && field.kind === "password" && field.newPassword && !field.auto) {
      generated = (await api("POST", "/api/instance/generate", {}).catch(() => null))?.password || null;
      if (token !== sequence) {
        return;
      }
    }
    const logins = passwordsOn && wantLogins && !field.newPassword ? keychain.entries || [] : [];
    const fake = identity && identity.mode !== "off" && identity.identity ? identity : null;
    if (!logins.length && !generated && !fake) {
      send(actor, "KeycatAutofill:PanelClosed", {});
      return;
    }
    await this.open(win, actor, field.rect, this.build(win.document, actor, origin, field, { logins, generated, fake, passwordsOn }));
  },

  build(doc, actor, origin, field, { logins, generated, fake, passwordsOn }) {
    const close = () => this.hide(actor);
    const item = (icon, title, sub, onclick, mono = false) => {
      const button = el(doc, "button", { class: "kca-item", type: "button" },
        el(doc, "img", { class: "kca-icon", src: icon, alt: "" }),
        el(doc, "span", { class: "kca-title", text: title }),
        sub ? el(doc, "span", { class: mono ? "kca-sub kca-mono" : "kca-sub", text: sub }) : null);
      button.addEventListener("mousedown", event => event.preventDefault());
      button.addEventListener("click", () => {
        close();
        onclick().catch(ex => console.error("Keycat autofill:", ex));
      });
      return button;
    };
    const separator = () => el(doc, "div", { class: "kca-separator", role: "separator" });
    const content = el(doc, "div", { class: "kca", role: "menu" });
    content.append(el(doc, "div", { class: "kca-head" },
      el(doc, "span", { text: "Keycat" }),
      el(doc, "span", { text: hostOf(origin) })));

    for (const entry of logins.slice(0, 8)) {
      content.append(item(ICONS.login, entry.username || "(no user name)", entry.title || hostOf(origin), async () => {
        await send(actor, "KeycatAutofill:FillLogin", { username: entry.username, password: entry.password });
        api("POST", "/api/instance/keychain/used", { id: entry.id }).catch(() => {});
      }));
    }

    if (generated) {
      content.append(item(ICONS.login, "Use a strong password", generated, async () => {
        await send(actor, "KeycatAutofill:FillPassword", { password: generated });
      }, true));
    }

    if (fake) {
      if (content.childElementCount > 1) {
        content.append(separator());
      }
      const id = fake.identity;
      const random = fake.mode === "random";
      const fill = async identity => {
        let password = "";
        if (passwordsOn && field.signup && field.hasPasswords) {
          password = (await api("POST", "/api/instance/generate", {}))?.password || "";
        }
        await send(actor, "KeycatAutofill:FillIdentity", { identity, password });
      };
      content.append(
        item(ICONS.identity, random ? "Fill with a random identity" : "Fill with fake identity",
          [id.fullName, id.email].filter(Boolean).join(" · "), () => fill(id)),
        item(ICONS.another, "Use another random identity", "A new name, e-mail, phone and address", async () => {
          const fresh = await api("GET", "/api/instance/identity?fresh=1");
          if (fresh?.identity) {
            await fill(fresh.identity);
          }
        })
      );
    }

    const win = doc.defaultView;
    content.append(separator(),
      item(ICONS.manage, passwordsOn ? "Manage Keycat Passwords" : "Keycat settings", "", async () => {
        if (passwordsOn) {
          lazy.KeycatSettings.openPasswords(win);
        } else {
          lazy.KeycatSettings.open(win);
        }
      }));
    return content;
  },

  panelFor(win) {
    const doc = win.document;
    let panel = doc.getElementById(PANEL_ID);
    if (!panel) {
      panel = doc.createXULElement("panel");
      panel.id = PANEL_ID;
      panel.setAttribute("type", "arrow");
      panel.setAttribute("noautofocus", "true");
      panel.setAttribute("consumeoutsideclicks", "never");
      panel.setAttribute("role", "dialog");
      panel.setAttribute("aria-label", "Keycat autofill");
      panel.addEventListener("popuphidden", event => {
        if (event.target !== panel) {
          return;
        }
        if (current?.panel === panel) {
          send(current.actor, "KeycatAutofill:PanelClosed", {});
          current = null;
        }
      });
      doc.getElementById("mainPopupSet").appendChild(panel);
    }
    return panel;
  },

  async open(win, actor, rect, content) {
    const panel = this.panelFor(win);
    if (panel.state !== "closed") {
      const hidden = new Promise(resolve => panel.addEventListener("popuphidden", resolve, { once: true }));
      panel.hidePopup();
      await hidden;
    }
    panel.replaceChildren(content);
    current = { actor, panel };
    panel.openPopupAtScreenRect("after_start", rect.left, rect.top, rect.width, rect.height, false, false);
  },

  hide(actor) {
    if (current && current.actor === actor) {
      const { panel } = current;
      if (panel.state !== "closed") {
        panel.hidePopup();
      }
    }
  },

  async offerSave(actor, origin, { username, password }) {
    if (!password || !lazy.KeycatLink.available) {
      return;
    }
    const keychain = await api("GET", "/api/instance/keychain?origin=" + encodeURIComponent(origin)).catch(() => null);
    if (!keychain?.enabled || keychain.neverSave) {
      return;
    }
    const existing = (keychain.entries || []).find(e => e.username === username);
    if (existing && existing.password === password) {
      return;
    }
    const browser = actor.browser;
    const win = browser?.ownerDocument?.defaultView;
    if (!win?.PopupNotifications) {
      return;
    }
    const host = hostOf(origin);
    const save = () => api("POST", "/api/instance/keychain", { origin, username, password })
      .catch(ex => console.error("Keycat: could not save the login", ex));
    const secondary = [{ label: "Not now", accessKey: "N", callback() {} }];
    if (!existing) {
      secondary.push({
        label: "Never save for this site",
        accessKey: "e",
        callback: () => api("POST", "/api/instance/keychain/never", { origin }).catch(() => {}),
      });
    }
    win.PopupNotifications.show(
      browser,
      "keycat-password",
      existing ? `Update the password of ${username || "this login"} for <>?` : "Save this login for <> in Keycat Passwords?",
      "password-notification-icon",
      { label: existing ? "Update" : "Save", accessKey: existing ? "U" : "S", callback: save },
      secondary,
      {
        name: host,
        popupIconURL: ICONS.login,
        persistWhileVisible: true,
        timeout: Date.now() + 30000,
      }
    );
  },
};
