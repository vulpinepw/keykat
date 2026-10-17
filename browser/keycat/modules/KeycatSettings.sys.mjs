const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  KeycatAbout: "resource://keycat/modules/KeycatAbout.sys.mjs",
  LoginHelper: "resource://gre/modules/LoginHelper.sys.mjs",
});

const HTML_NS = "http://www.w3.org/1999/xhtml";
const ICONS = "chrome://keycat/content/icons/";

function settingsDocument(doc) {
  const url = doc?.documentURI || "";
  return url.startsWith("about:preferences") || url.startsWith("about:settings");
}

export const KeycatSettings = {
  boot: null,

  init(boot) {
    this.boot = boot;
    Services.obs.addObserver(this, "document-element-inserted");
    this.patchPasswordManager();
  },

  get usesKeycatPasswords() {
    return this.boot?.config?.privacy?.passwords === "keycat";
  },

  panes() {
    const config = this.boot.config;
    const panes = [
      {
        view: "paneKeycat",
        id: "category-keycat",
        label: "Keycat",
        icon: ICONS + "keycat.svg",
        src: `about:keycat?embed=profile&id=${encodeURIComponent(config.id)}&tabs=profile,privacy,security,identity`,
      },
    ];
    if (this.usesKeycatPasswords) {
      panes.push({
        view: "paneKeycatPasswords",
        id: "category-keycat-passwords",
        label: "Passwords",
        icon: "chrome://browser/skin/login.svg",
        src: `about:keycat?embed=profile&id=${encodeURIComponent(config.id)}&tabs=passwords`,
      });
    }
    panes.push({
      view: "paneKeycatProfiles",
      id: "category-keycat-profiles",
      label: "Profiles",
      icon: ICONS + "profiles.svg",
      src: "about:keycat?embed=profiles",
    });
    return panes;
  },

  observe(doc, topic) {
    if (topic !== "document-element-inserted") {
      return;
    }
    if ((doc.documentURI || "").startsWith("about:support")) {
      this.renameApplication(doc);
      return;
    }
    if (!settingsDocument(doc) || !this.boot?.config) {
      return;
    }
    const win = doc.defaultView;
    if (!win) {
      return;
    }
    doc.addEventListener("DOMContentLoaded", () => {
      try {
        this.inject(win);
      } catch (ex) {
        console.error("Keycat settings:", ex);
      }
    }, { once: true });
  },

  inject(win) {
    const doc = win.document;
    const nav = doc.getElementById("categories");
    const main = doc.getElementById("mainPrefPane");
    if (!nav || !main || doc.getElementById("category-keycat") || typeof win.register_module !== "function") {
      return;
    }
    doc.documentElement.setAttribute("keycat-settings", this.usesKeycatPasswords ? "passwords" : "");

    const first = nav.firstElementChild;
    for (const pane of this.panes()) {
      const button = doc.createElementNS(HTML_NS, "moz-page-nav-button");
      button.id = pane.id;
      button.setAttribute("view", pane.view);
      button.setAttribute("iconsrc", pane.icon);
      button.textContent = pane.label;
      nav.insertBefore(button, first);

      const box = doc.createElementNS(HTML_NS, "div");
      box.className = "keycat-pane";
      box.dataset.category = pane.view;
      box.hidden = true;
      const frame = doc.createElementNS(HTML_NS, "iframe");
      frame.className = "keycat-frame";
      frame.setAttribute("title", pane.label);
      box.append(frame);
      main.append(box);

      win.register_module(pane.view, {
        init: () => {
          if (!frame.getAttribute("src")) {
            frame.src = pane.src;
          }
          this.fit(win, frame);
        },
      });
    }
    const separator = doc.createElementNS(HTML_NS, "hr");
    separator.className = "nav-separator";
    separator.setAttribute("role", "presentation");
    nav.insertBefore(separator, first);

    if (!win.location.hash) {
      win.history.replaceState(null, "", "#keycat");
    }

    const refit = () => {
      for (const frame of doc.querySelectorAll(".keycat-pane:not([hidden]) .keycat-frame")) {
        this.fit(win, frame);
      }
    };
    win.addEventListener("resize", refit);
    const scroller = doc.querySelector(".main-content");
    if (scroller) {
      new win.ResizeObserver(refit).observe(scroller);
    }
    doc.addEventListener("paneshown", event => {
      const category = event.detail?.category || "";
      if (category.startsWith("paneKeycat")) {
        doc.documentElement.setAttribute("keycat-pane-shown", category);
      } else {
        doc.documentElement.removeAttribute("keycat-pane-shown");
      }
      win.requestAnimationFrame(refit);
      win.requestAnimationFrame(() => this.addNotes(win));
    });
  },

  addNotes(win) {
    const doc = win.document;
    const notes = {
      panePrivacy: "Tracking protection, cookies, history, HTTPS-Only, encrypted DNS, fingerprinting protection and default permissions are set in Keycat for this profile.",
      paneSearch: "The default search engine and search suggestions are set in Keycat for this profile.",
    };
    for (const [category, text] of Object.entries(notes)) {
      const header = doc.querySelector(`setting-pane[data-category="${category}"] > section > moz-page-header`);
      if (!header || header.parentNode.querySelector(".keycat-note")) {
        continue;
      }
      const note = doc.createElementNS(HTML_NS, "div");
      note.className = "keycat-note";
      const icon = doc.createElementNS(HTML_NS, "img");
      icon.src = ICONS + "keycat.svg";
      icon.alt = "";
      const body = doc.createElementNS(HTML_NS, "div");
      body.className = "text";
      const title = doc.createElementNS(HTML_NS, "strong");
      title.textContent = "Keycat privacy settings";
      const span = doc.createElementNS(HTML_NS, "span");
      span.textContent = text;
      body.append(title, span);
      const button = doc.createElementNS(HTML_NS, "moz-button");
      button.setAttribute("label", "Open");
      button.addEventListener("click", () => this.showKeycatTab(win, "privacy"));
      note.append(icon, body, button);
      header.after(note);
    }
  },

  showKeycatTab(win, tab) {
    const frame = win.document.querySelector('.keycat-pane[data-category="paneKeycat"] .keycat-frame');
    const pane = this.panes().find(p => p.view === "paneKeycat");
    if (frame && pane) {
      frame.src = `${pane.src}&tab=${encodeURIComponent(tab)}`;
    }
    win.gotoPref("paneKeycat");
  },

  renameApplication(doc) {
    doc.addEventListener("DOMContentLoaded", () => {
      const win = doc.defaultView;
      const fix = () => {
        const box = doc.getElementById("application-box");
        if (box && box.textContent.trim() === Services.appinfo.name) {
          box.textContent = "Keycat";
        }
      };
      new win.MutationObserver(fix).observe(doc.body, { childList: true, subtree: true });
      fix();
    }, { once: true });
  },

  fit(win, frame) {
    win.requestAnimationFrame(() => {
      const scroller = frame.ownerDocument.querySelector(".main-content");
      if (!scroller || !frame.isConnected) {
        return;
      }
      const top = frame.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;
      const bottom = scroller.getBoundingClientRect().bottom - scroller.getBoundingClientRect().top;
      frame.style.height = `${Math.max(360, Math.floor(bottom - top - 2))}px`;
    });
  },

  patchPasswordManager() {
    const helper = lazy.LoginHelper;
    if (helper.keycatPatched) {
      return;
    }
    const original = helper.openPasswordManager;
    const self = this;
    helper.openPasswordManager = function (window, options) {
      if (!self.usesKeycatPasswords) {
        return original.call(this, window, options);
      }
      return self.openPasswords(window);
    };
    helper.keycatPatched = true;
  },

  openPasswords(win) {
    win = win?.gBrowser ? win : Services.wm.getMostRecentWindow("navigator:browser");
    win?.openPreferences?.("keycatPasswords");
  },

  watchWindow(win) {
    if (!this.usesKeycatPasswords || !win.gBrowser) {
      return;
    }
    win.gBrowser.addTabsProgressListener({
      onLocationChange: (browser, webProgress, request, location) => {
        if (!webProgress?.isTopLevel || !location || location.scheme !== "about" || !location.spec.startsWith("about:logins")) {
          return;
        }
        Services.tm.dispatchToMainThread(() => {
          browser.fixupAndLoadURIString("about:preferences#keycatPasswords", {
            triggeringPrincipal: Services.scriptSecurityManager.getSystemPrincipal(),
            loadFlags: Ci.nsIWebNavigation.LOAD_FLAGS_REPLACE_HISTORY,
          });
        });
      },
    });
  },

  open(win, pane = "keycat") {
    win = win || Services.wm.getMostRecentWindow("navigator:browser");
    win?.openPreferences?.(pane);
  },

  openProfiles(win) {
    lazy.KeycatAbout.open(win);
  },
};
