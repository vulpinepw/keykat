import { AppConstants } from "resource://gre/modules/AppConstants.sys.mjs";

const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  KeycatAbout: "resource://keycat/modules/KeycatAbout.sys.mjs",
  KeycatAutofill: "resource://keycat/modules/KeycatAutofill.sys.mjs",
  KeycatLink: "resource://keycat/modules/KeycatLink.sys.mjs",
  KeycatSettings: "resource://keycat/modules/KeycatSettings.sys.mjs",
  KeycatThemes: "resource://keycat/modules/KeycatThemes.sys.mjs",
});

const HTML_NS = "http://www.w3.org/1999/xhtml";
const WIDGET_ID = "keycat-profile-button";
const PANEL_ID = "keycat-panel";

const TOGGLES = [
  {
    key: "httpsOnly",
    label: "HTTPS-Only mode",
    hint: "Upgrade every connection to HTTPS",
    prefs: on => ({ "dom.security.https_only_mode": on, "dom.security.https_only_mode_pbm": on }),
  },
  {
    key: "javascript",
    label: "JavaScript",
    hint: "Applies to pages loaded from now on",
    prefs: on => ({ "javascript.enabled": on }),
  },
  {
    key: "webgl",
    label: "WebGL",
    hint: "3D graphics; a common fingerprinting source",
    prefs: on => ({ "webgl.disabled": !on }),
  },
  {
    key: "searchSuggestions",
    label: "Search suggestions",
    hint: "Sends what you type to your search engine",
    prefs: on => ({ "browser.search.suggest.enabled": on, "browser.urlbar.suggest.searches": on }),
  },
];

const THEME_CHOICES = [
  ["system", "System"],
  ["light", "Light"],
  ["dark", "Dark"],
];

let widgetCreated = false;

function loadSheet(win, url) {
  try {
    win.windowUtils.loadSheetUsingURIString(url, win.windowUtils.AUTHOR_SHEET);
  } catch (ex) {
    console.error("Keycat: cannot load", url, ex);
  }
}

function setPrefs(prefs) {
  for (const [name, value] of Object.entries(prefs)) {
    if (typeof value === "boolean") {
      Services.prefs.setBoolPref(name, value);
    } else if (typeof value === "number") {
      Services.prefs.setIntPref(name, value);
    } else {
      Services.prefs.setStringPref(name, value);
    }
  }
}

function svgURL(svg) {
  return "data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg || "");
}

function el(doc, tag, props = {}, ...children) {
  const node = doc.createElementNS(HTML_NS, tag);
  for (const [k, v] of Object.entries(props)) {
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

export const KeycatUI = {
  attach(win, boot) {
    const doc = win.document;
    if (doc.documentElement.getAttribute("windowtype") !== "navigator:browser") {
      return;
    }
    loadSheet(win, "chrome://keycat/content/browser.css");
    const root = doc.documentElement;
    root.setAttribute("keycat-mode", boot.mode);

    if (boot.mode === "profile" && boot.config) {
      this.patchTitle(win, boot.config.name);
      this.ensureWidget(win, boot);
      if (boot.autofill) {
        lazy.KeycatAutofill.attachWindow(win);
      }
    } else if (boot.mode === "unmanaged") {
      this.showUnmanagedNotice(win);
    }
  },

  patchTitle(win, name) {
    const gBrowser = win.gBrowser;
    if (!gBrowser || gBrowser.keycatTitlePatched || !gBrowser.getWindowTitleForBrowser) {
      return;
    }
    const original = gBrowser.getWindowTitleForBrowser;
    gBrowser.getWindowTitleForBrowser = function (browser) {
      const parts = original.call(this, browser).split(" \\u2014 ");
      parts.splice(Math.max(parts.length - 1, 0), 0, KeycatUI.profileName || name);
      return parts.join(" - ");
    };
    gBrowser.keycatTitlePatched = true;
    gBrowser.updateTitlebar();
  },

  refresh(boot) {
    const config = boot.config;
    this.profileName = config.name;
    for (const win of Services.wm.getEnumerator("navigator:browser")) {
      const button = win.document.getElementById(WIDGET_ID);
      if (button) {
        this.decorateButton(button, config);
      }
      win.gBrowser?.updateTitlebar?.();
    }
  },

  ensureWidget(win, boot) {
    if (widgetCreated) {
      return;
    }
    widgetCreated = true;
    const CUI = win.CustomizableUI;
    CUI.createWidget({
      id: WIDGET_ID,
      type: "custom",
      defaultArea: CUI.AREA_NAVBAR,
      onBuild: doc => {
        const button = doc.createXULElement("toolbarbutton");
        button.id = WIDGET_ID;
        button.className = "toolbarbutton-1 chromeclass-toolbar-additional";
        button.setAttribute("label", "Keycat profile");
        button.setAttribute("removable", "true");
        this.decorateButton(button, boot.config);
        button.addEventListener("command", () => this.openPanel(doc.defaultView, button, boot));
        return button;
      },
    });
    if (!CUI.getPlacementOfWidget(WIDGET_ID)) {
      CUI.addWidgetToArea(WIDGET_ID, CUI.AREA_NAVBAR);
    }
  },

  decorateButton(button, config) {
    button.style.listStyleImage = `url("${svgURL(config.badgeSvg)}")`;
    button.setAttribute("tooltiptext", `${config.name}: ${config.encryption?.summary || "Keycat profile"}`);
  },

  openPanel(win, anchor, boot) {
    const doc = win.document;
    let panel = doc.getElementById(PANEL_ID);
    if (!panel) {
      panel = doc.createXULElement("panel");
      panel.id = PANEL_ID;
      panel.setAttribute("type", "arrow");
      panel.setAttribute("role", "dialog");
      panel.setAttribute("aria-label", "Keycat profile");
      panel.setAttribute("noautofocus", "true");
      doc.getElementById("mainPopupSet").appendChild(panel);
    }
    panel.replaceChildren(this.buildPanelContent(win, panel, boot));
    panel.openPopup(anchor, "bottomright topright", 0, 0, false, false);
  },

  buildPanelContent(win, panel, boot) {
    const doc = win.document;
    const config = boot.config;
    const enc = config.encryption || {};

    const head = el(doc, "div", { class: "kc-head" },
      el(doc, "img", { class: "kc-avatar", src: svgURL(config.avatarSvg), alt: "" }),
      el(doc, "div", { class: "kc-id" },
        el(doc, "div", { class: "kc-name", text: config.name }),
        el(doc, "div", { class: `kc-sub kc-${enc.mode || "plain"}` },
          el(doc, "span", { class: "kc-sub-icon" }),
          el(doc, "span", { text: enc.mode === "encrypted" ? `Encrypted · ${enc.cipher}` : enc.summary || "" })
        ),
        enc.kdf ? el(doc, "div", { class: "kc-kdf", text: `Key derivation: ${enc.kdf}` }) : null
      )
    );

    const summary = el(doc, "ul", { class: "kc-summary" });
    const renderSummary = () => {
      summary.replaceChildren(
        ...(config.summary || []).map(item =>
          el(doc, "li", { class: `kc-row kc-${item.level}` },
            el(doc, "span", { class: "kc-dot" }),
            el(doc, "span", { class: "kc-label", text: item.label }),
            el(doc, "span", { class: "kc-value", text: item.value })
          )
        )
      );
    };
    renderSummary();

    const toggles = el(doc, "div", { class: "kc-toggles" });
    for (const toggle of TOGGLES) {
      const control = el(doc, "moz-toggle", { label: toggle.label, description: toggle.hint });
      control.pressed = !!config.privacy[toggle.key];
      control.addEventListener("toggle", async () => {
        const on = control.pressed;
        setPrefs(toggle.prefs(on));
        config.privacy[toggle.key] = on;
        try {
          const result = await lazy.KeycatLink.saveSettings({ privacy: { [toggle.key]: on } });
          config.summary = result.summary;
          config.privacy = result.privacy;
          renderSummary();
        } catch (ex) {
          console.error("Keycat: could not save setting", ex);
        }
      });
      toggles.append(control);
    }

    const themes = el(doc, "moz-segmented-control", { "aria-label": "Theme" });
    const current = lazy.KeycatThemes.currentKey();
    for (const [key, label] of THEME_CHOICES) {
      const item = el(doc, "moz-segmented-control-item", { label, value: key });
      if (key === current) {
        item.setAttribute("checked", "");
      }
      themes.append(item);
    }
    themes.value = current;
    themes.addEventListener("change", async () => {
      const key = themes.value;
      await lazy.KeycatThemes.activate(key);
      config.appearance.theme = key;
      lazy.KeycatLink.saveSettings({ theme: key }).catch(console.error);
    });

    const profilesButton = el(doc, "moz-button", { label: "Profiles", iconsrc: "chrome://keycat/content/icons/profiles.svg" });
    profilesButton.addEventListener("click", () => {
      panel.hidePopup();
      lazy.KeycatAbout.open(win);
    });
    const settingsButton = el(doc, "moz-button", { label: "Settings", iconsrc: "chrome://global/skin/icons/settings.svg" });
    settingsButton.addEventListener("click", () => {
      panel.hidePopup();
      lazy.KeycatSettings.open(win);
    });
    const closeButton = el(doc, "moz-button", {
      type: "primary",
      label: enc.mode === "encrypted" ? "Lock profile" : enc.mode === "ephemeral" ? "Close and erase" : "Close profile",
    });
    closeButton.addEventListener("click", () => {
      panel.hidePopup();
      Services.startup.quit(Ci.nsIAppStartup.eAttemptQuit);
    });

    const separator = () => el(doc, "div", { class: "kc-separator", role: "separator" });
    return el(doc, "div", { class: "kc-panel" },
      head,
      separator(),
      el(doc, "h2", { class: "kc-heading", text: "Protection" }),
      summary,
      separator(),
      el(doc, "h2", { class: "kc-heading", text: "Quick settings" }),
      toggles,
      separator(),
      el(doc, "div", { class: "kc-theme" }, el(doc, "h2", { class: "kc-heading", text: "Theme" }), themes),
      separator(),
      el(doc, "div", { class: "kc-actions" },
        profilesButton, settingsButton, el(doc, "span", { class: "kc-spacer" }), closeButton)
    );
  },

  showUnmanagedNotice(win) {
    const box = win.gNotificationBox;
    if (!box || box.getNotificationWithValue?.("keycat-unmanaged")) {
      return;
    }
    const launch = () => {
      try {
        const exe = Services.dirsvc.get("GreD", Ci.nsIFile).parent;
        exe.append(AppConstants.platform === "win" ? "Keycat.exe" : "keycat");
        if (exe.exists()) {
          const proc = Cc["@mozilla.org/process/util;1"].createInstance(Ci.nsIProcess);
          proc.init(exe);
          proc.runw(false, [], 0);
        }
      } catch (ex) {
        console.error("Keycat: cannot start Keycat", ex);
      }
      return false;
    };
    Promise.resolve(
      box.appendNotification(
        "keycat-unmanaged",
        {
          label: "This is the Keycat engine running on its own, without your encrypted profiles. Start Keycat to use them.",
          priority: box.PRIORITY_WARNING_MEDIUM,
        },
        [{ label: "Open Keycat", accessKey: "O", callback: launch }]
      )
    ).catch(ex => console.error("Keycat: notice failed", ex));
  },
};
