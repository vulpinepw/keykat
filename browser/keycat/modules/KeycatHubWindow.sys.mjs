const BLOCKED_ACCEL = new Set(["t", "n", "l", "k", "d", "h", "j", "b", "p", "o", "s", "u", "g", "e", "i", "y", "m", "f"]);
const BLOCKED_ACCEL_SHIFT = new Set(["p", "n", "b", "t", "h", "o", "y", "e", "k", "c", "i", "m", "j", "s", "a", "x"]);
const BLOCKED_KEYS = new Set(["F1", "F6", "F7", "F10", "F11"]);

function loadSheet(win, url) {
  try {
    win.windowUtils.loadSheetUsingURIString(url, win.windowUtils.AUTHOR_SHEET);
  } catch (ex) {
    console.error("Keycat: cannot load", url, ex);
  }
}

function blockShortcuts(event) {
  const key = (event.key || "").toLowerCase();
  const devtools = Services.env.get("KEYCAT_DEBUG");
  let block = false;
  if (event.ctrlKey && event.shiftKey) {
    block = BLOCKED_ACCEL_SHIFT.has(key) && !(devtools && (key === "i" || key === "j" || key === "c"));
  } else if (event.ctrlKey && !event.altKey) {
    block = BLOCKED_ACCEL.has(key) || key === "tab" || key === "pageup" || key === "pagedown";
  } else if (event.altKey && !event.ctrlKey) {
    block = key === "home" || key === "arrowleft" || key === "arrowright" || key === "alt";
  } else if (!event.ctrlKey && !event.altKey) {
    block = BLOCKED_KEYS.has(event.key) || (event.key === "F12" && !devtools);
  }
  if (block) {
    event.preventDefault();
    event.stopPropagation();
  }
}

const HTML_NS = "http://www.w3.org/1999/xhtml";

export const KeycatHubWindow = {
  attach(win) {
    const doc = win.document;
    if (doc.documentElement.getAttribute("windowtype") !== "navigator:browser") {
      return;
    }
    doc.documentElement.setAttribute("keycat-hub", "true");
    loadSheet(win, "chrome://keycat/content/hub-window.css");
    this.size(win);
    this.titlebar(win);
    win.addEventListener("keydown", blockShortcuts, true);
    if (win.gBrowser?.getWindowTitleForBrowser) {
      win.gBrowser.getWindowTitleForBrowser = () => "Keycat";
      win.gBrowser.updateTitlebar();
    }

    const gBrowser = win.gBrowser;
    gBrowser?.tabContainer?.addEventListener("TabOpen", event => {
      if (gBrowser.tabs.length > 1) {
        gBrowser.removeTab(event.target);
      }
    });
    win.addEventListener(
      "drop",
      event => {
        event.preventDefault();
        event.stopPropagation();
      },
      true
    );
  },

  titlebar(win) {
    const doc = win.document;
    if (doc.getElementById("keycat-titlebar")) {
      return;
    }
    const ct = win.CustomTitlebar;
    if (ct && !ct.keycatForced) {
      const allowedBy = ct.allowedBy.bind(ct);
      ct.allowedBy = (condition, allow) => allowedBy(condition, condition === "non-popup" ? true : allow);
      delete ct._disallowed?.["non-popup"];
      ct.keycatForced = true;
      ct._update?.();
    }

    const el = (tag, props = {}) => {
      const node = doc.createElementNS(HTML_NS, tag);
      for (const [k, v] of Object.entries(props)) {
        if (k === "text") {
          node.textContent = v;
        } else {
          node.setAttribute(k, v);
        }
      }
      return node;
    };
    const bar = el("div", { id: "keycat-titlebar" });
    const brand = el("div", { class: "kt-brand" });
    brand.append(el("img", { src: "chrome://branding/content/about-logo.svg", alt: "" }), el("span", { text: "Keycat" }));
    const button = (id, label, action) => {
      const b = el("button", { id, class: "kt-button", title: label, "aria-label": label });
      b.addEventListener("click", action);
      return b;
    };
    const toggleMaximize = () => {
      if (win.windowState === win.STATE_MAXIMIZED) {
        win.restore();
      } else {
        win.maximize();
      }
    };
    bar.append(
      brand,
      el("div", { class: "kt-spacer" }),
      button("kt-min", "Minimize", () => win.minimize()),
      button("kt-max", "Maximize", toggleMaximize),
      button("kt-close", "Close", () => win.close())
    );
    bar.addEventListener("dblclick", event => {
      if (!event.target.closest(".kt-button")) {
        toggleMaximize();
      }
    });
    const updateMaxLabel = () => {
      const max = doc.getElementById("kt-max");
      const label = win.windowState === win.STATE_MAXIMIZED ? "Restore" : "Maximize";
      max?.setAttribute("title", label);
      max?.setAttribute("aria-label", label);
    };
    win.addEventListener("sizemodechange", updateMaxLabel);
    const browserBox = doc.getElementById("browser");
    browserBox.parentNode.insertBefore(bar, browserBox);
    updateMaxLabel();
  },

  size(win) {
    const url = "chrome://browser/content/browser.xhtml";
    if (Services.xulStore.hasValue(url, "main-window", "width")) {
      return;
    }
    const screen = win.screen;
    const width = Math.min(1200, screen.availWidth - 64);
    const height = Math.min(800, screen.availHeight - 64);
    win.resizeTo(width, height);
    win.moveTo(
      screen.availLeft + Math.round((screen.availWidth - width) / 2),
      screen.availTop + Math.round((screen.availHeight - height) / 2)
    );
  },
};
