import { api, inBrowser } from "./api.js";
import { store } from "./store.js";
import { drawer, fmtAgo, h, icon, inlinePanel, mount, segmented, selectRow, switchRow, toast } from "./dom.js";
import { applyTheme } from "./theme.js";
import { section } from "./views.js";
import { restartKeycat, startUpdate } from "./main.js";

export function openSettings(options = {}) {
  let unsubscribe = () => {};
  let browserStatus = null;
  const checkBrowser = () => api("GET", "/api/default-browser")
    .then(status => {
      browserStatus = status;
      render();
    })
    .catch(() => {});
  window.addEventListener("focus", checkBrowser);
  const dlg = options.inline
    ? inlinePanel(options.inline, "Keycat settings")
    : drawer({
        label: "Settings",
        onClose: () => {
          unsubscribe();
          window.removeEventListener("focus", checkBrowser);
        },
      });
  const body = h("div", { class: "drawer-body" });
  mount(dlg.panel,
    h("div", { class: "drawer-head" },
      h("span", { class: "choice-icon muted" }, icon("settings")),
      h("div", {}, h("h2", { text: "Keycat settings" }), h("p", { text: "Apply to Keycat and every profile." })),
      dlg.inline ? null : h("button", { class: "btn btn-ghost btn-icon close", "aria-label": "Close", onclick: () => dlg.close() }, icon("x"))
    ),
    h("div"),
    body
  );

  async function save(patch) {
    try {
      store.state.settings = await api("PUT", "/api/settings", { ...store.state.settings, ...patch });
      store.emit({ type: "settings" });
    } catch (err) {
      toast("bad", "Could not save the setting", err.message);
    }
  }

  async function makeDefault(event) {
    event.currentTarget.disabled = true;
    try {
      browserStatus = await api("POST", "/api/default-browser", {});
      render();
    } catch (err) {
      toast("bad", "Keycat could not be made the default browser", err.message);
      render();
    }
  }

  function defaultBrowserRow(app) {
    const linux = app.platform === "linux";
    const s = browserStatus;
    let title = "Default browser";
    let hint = "Checking";
    let button = null;
    if (s?.isDefault) {
      title = "Keycat is your default browser";
      hint = "Links from other apps open in Keycat.";
    } else if (s && !s.registered) {
      hint = linux
        ? "Install Keycat with install.sh to make it your default browser."
        : "Install Keycat with KeycatSetup.exe to make it your default browser. A portable copy cannot be one.";
    } else if (s) {
      hint = linux
        ? "Links from other apps will open in Keycat."
        : "Windows opens Keycat's page in its default apps settings. Choose Set default there.";
      button = h("button", { class: "btn btn-primary btn-sm", type: "button", onclick: makeDefault }, "Make default");
    }
    return h("div", { class: "toggle-row" },
      h("span", { class: "text" }, h("div", { class: "title", text: title }), h("div", { class: "hint", text: hint })),
      button);
  }

  function render() {
    const { settings, app, core, update } = store.state;
    const engineStatus = (() => {
      if (update.busy) {
        return `Updating: ${update.phase || "working"}`;
      }
      if (update.staged) {
        return `Version ${update.staged} is ready and will be used after a restart.`;
      }
      if (update.error) {
        return `Last check failed: ${update.error}`;
      }
      if (update.available) {
        return `Version ${update.latest} is available.`;
      }
      if (update.checkedAt && !update.checkedAt.startsWith("0001")) {
        return `Up to date (checked ${fmtAgo(update.checkedAt)}).`;
      }
      return "Not checked yet.";
    })();
    const folderRow = (title, path, target) => h("div", { class: "toggle-row" },
      h("span", { class: "text" }, h("div", { class: "title", text: title }), h("div", { class: "hint path", text: path })),
      h("button", {
        class: "btn btn-sm", type: "button",
        onclick: () => api("POST", "/api/open-folder", { target }).catch(err => toast("bad", "Could not open the folder", err.message)),
      }, icon("folder"), "Open"));

    mount(body,
      section("Appearance",
        h("div", { class: "toggle-row" },
          h("span", { class: "text" }, h("div", { class: "title", text: "Keycat window theme" }),
            h("div", { class: "hint", text: inBrowser ? "The profiles window's theme, and the default for new profiles. This browser follows its own profile's theme." : "Also the default theme for new profiles" })),
          segmented([["dark", "Dark"], ["light", "Light"], ["system", "System"]], settings.theme, theme => {
            if (!inBrowser) {
              applyTheme(theme);
            }
            save({ theme });
          }, "Keycat theme"))
      ),
      section("Behavior",
        selectRow({
          title: "When a profile opens",
          hint: "The profiles window comes back when the last profile is closed",
          value: settings.afterOpen,
          options: [["close", "Switch to the browser window"], ["minimize", "Minimize the profiles window"], ["keep", "Keep the profiles window open"]],
          onchange: afterOpen => save({ afterOpen }),
        }),
        switchRow({
          title: "Overwrite decrypted files before deleting them",
          hint: `Best effort on SSDs. Full-disk encryption (${app.platform === "linux" ? "LUKS" : "BitLocker"}) is the complete protection.`,
          checked: settings.secureWipe,
          onchange: secureWipe => save({ secureWipe }),
        })
      ),
      section("Default browser",
        defaultBrowserRow(app),
        selectRow({
          title: "Links from other apps",
          hint: "Where a link opens when Keycat is your default browser",
          value: settings.openLinks,
          options: [
            ["auto", "In the open profile, or ask"],
            ["ask", "Ask every time"],
            ...store.state.profiles.map(p => [p.id, `Always in ${p.name}`]),
          ],
          onchange: openLinks => save({ openLinks }),
        })
      ),
      section("Browser engine",
        h("dl", { class: "kv" },
          h("dt", { text: "Version" }), h("dd", { text: core.installed ? core.version : "Not installed" }),
          h("dt", { text: "Channel" }), h("dd", { text: core.channel || "release" }),
          h("dt", { text: "Build" }), h("dd", { text: core.buildId || "Unknown" })
        ),
        h("div", { class: "toggle-row" },
          h("span", { class: "text" }, h("div", { class: "title", text: "Update channel" }),
            h("div", { class: "hint", text: "Release gets new features every month. ESR changes once a year and only receives security fixes." })),
          segmented([["release", "Release"], ["esr", "ESR"]], settings.updateChannel, updateChannel => save({ updateChannel }), "Update channel")
        ),
        switchRow({ title: "Check for engine updates when Keycat starts", checked: settings.checkUpdates, onchange: checkUpdates => save({ checkUpdates }) }),
        h("div", { class: "toggle-row" },
          h("span", { class: "text" }, h("div", { class: "title", text: "Engine status" }), h("div", { class: "hint", text: engineStatus })),
          h("div", { class: "row-buttons" },
            update.staged
              ? h("button", { class: "btn btn-primary btn-sm", type: "button", onclick: restartKeycat }, icon("refresh"), "Restart Keycat")
              : update.available
                ? h("button", { class: "btn btn-primary btn-sm", type: "button", disabled: update.busy, onclick: startUpdate }, icon("download"), "Update")
                : h("button", {
                    class: "btn btn-sm", type: "button", disabled: update.busy,
                    onclick: async event => {
                      event.currentTarget.disabled = true;
                      try {
                        store.state.update = await api("POST", "/api/core/check", {});
                        store.emit({ type: "core" });
                      } catch (err) {
                        toast("bad", "The update check failed", err.message);
                      }
                    },
                  }, icon("refresh"), "Check now"))
        )
      ),
      section("Storage",
        folderRow("Profiles and settings", app.dataDir, "data"),
        folderRow("Open sessions (decrypted while a profile runs)", app.sessionsDir, "sessions"),
        folderRow("Logs", "Diagnostic log without any personal data", "logs")
      ),
      section("About",
        h("div", { class: "about" },
          h("img", { src: "img/keycat-192.png", alt: "", class: "about-logo" }),
          h("div", {},
            h("div", { class: "title", text: `Keycat ${app.version}` }),
            h("div", { class: "hint", text: `Made by ${app.author || "tordev"}.` }),
            inBrowser
              ? h("a", { class: "link-btn hint-link", href: "https://github.com/vulpinepw", target: "_blank", rel: "noopener" }, "github.com/vulpinepw")
              : h("div", { class: "hint", text: "github.com/vulpinepw" }),
            app.portable ? h("div", { class: "hint", text: "Running in portable mode." }) : null
          )
        ),
        h("dl", { class: "credits" },
          h("dt", { text: app.author || "tordev" }), h("dd", { text: "Creator of Keycat: design, encryption, profiles, password manager and the Keycat pages." }),
          h("dt", { text: "Mozilla Firefox" }), h("dd", { text: "The browser engine, by Mozilla and its community (Mozilla Public License 2.0)." }),
          h("dt", { text: "uBlock Origin" }), h("dd", { text: "The optional ad blocker, by Raymond Hill (GPLv3), installed from the add-on store." }),
          h("dt", { text: "Go libraries" }), h("dd", { text: "golang.org/x/crypto, x/net and x/sys (BSD), klauspost/compress (BSD and Apache 2.0)." })
        ),
        h("div", { class: "hint small-print", text: "Keycat is licensed under the Mozilla Public License 2.0. Firefox is a trademark of the Mozilla Foundation; Keycat is not made or endorsed by Mozilla." })
      )
    );
  }

  render();
  checkBrowser();
  unsubscribe = store.on((state, change) => {
    if (change.type !== "status" && change.type !== "links") {
      render();
    }
  });
}
