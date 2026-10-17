const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  AddonManager: "resource://gre/modules/AddonManager.sys.mjs",
  BuiltInThemes: "resource:///modules/BuiltInThemes.sys.mjs",
});

export const THEMES = {
  dark: "firefox-compact-dark@mozilla.org",
  light: "firefox-compact-light@mozilla.org",
  system: "default-theme@mozilla.org",
};

const LEGACY_THEMES = {
  "keycat-dark@keycat.app": "dark",
  "keycat-light@keycat.app": "light",
  "keycat-auto@keycat.app": "system",
};

const LEGACY_KEYS = { "keycat-dark": "dark", "keycat-light": "light", "keycat-auto": "system" };

let cleaning = null;

export const KeycatThemes = {
  removeLegacy() {
    if (!cleaning) {
      cleaning = (async () => {
        for (const [id, key] of Object.entries(LEGACY_THEMES)) {
          const addon = await lazy.AddonManager.getAddonByID(id);
          if (!addon) {
            continue;
          }
          const wasActive = addon.isActive;
          await addon.uninstall();
          if (wasActive) {
            await this.enable(THEMES[key]);
          }
        }
      })();
    }
    return cleaning;
  },

  async activate(key) {
    await this.removeLegacy().catch(ex => console.error("Keycat themes:", ex));
    const id = THEMES[LEGACY_KEYS[key] || key];
    if (id) {
      await this.enable(id);
    }
  },

  async enable(id) {
    let addon = await lazy.AddonManager.getAddonByID(id);
    if (!addon) {
      const info = lazy.BuiltInThemes.builtInThemeMap.get(id);
      if (info) {
        await lazy.AddonManager.maybeInstallBuiltinAddon(id, info.version, info.path);
        addon = await lazy.AddonManager.getAddonByID(id);
      }
    }
    if (addon && !addon.isActive) {
      await addon.enable();
    }
  },

  currentKey() {
    const id = Services.prefs.getStringPref("extensions.activeThemeID", "");
    return Object.keys(THEMES).find(key => THEMES[key] === id) || "firefox";
  },
};
