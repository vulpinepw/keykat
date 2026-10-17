const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  AddonManager: "resource://gre/modules/AddonManager.sys.mjs",
});

const CONFIG_FILE = "keycat-profile.json";
const UBLOCK_ID = "uBlock0@raymondhill.net";
const UBLOCK_URL = "https://addons.mozilla.org/firefox/downloads/latest/ublock-origin/latest.xpi";
const UBLOCK_MANAGED_PREF = "keycat.ublock.managed";

const FALLBACK_ENGINES = {
  DuckDuckGo: "https://duckduckgo.com/?q={searchTerms}",
  Startpage: "https://www.startpage.com/sp/search?query={searchTerms}",
  "Brave Search": "https://search.brave.com/search?q={searchTerms}",
  Mojeek: "https://www.mojeek.com/search?q={searchTerms}",
  Qwant: "https://www.qwant.com/?q={searchTerms}",
  Ecosia: "https://www.ecosia.org/search?q={searchTerms}",
};

async function loadSearchService() {
  for (const url of [
    "moz-src:///toolkit/components/search/SearchService.sys.mjs",
    "resource://gre/modules/SearchService.sys.mjs",
  ]) {
    try {
      const mod = ChromeUtils.importESModule(url);
      if (mod.SearchService) {
        return mod.SearchService;
      }
    } catch (ex) {
    }
  }
  return Services.search;
}

export const KeycatProfile = {
  readConfig() {
    try {
      const file = Services.dirsvc.get("ProfD", Ci.nsIFile);
      file.append(CONFIG_FILE);
      if (!file.exists()) {
        return null;
      }
      return JSON.parse(Cu.readUTF8File(file));
    } catch (ex) {
      console.error("Keycat: cannot read profile configuration", ex);
      return null;
    }
  },

  resetPrefs(names) {
    for (const name of names || []) {
      try {
        if (Services.prefs.prefHasUserValue(name)) {
          Services.prefs.clearUserPref(name);
        }
      } catch (ex) {
        console.error(`Keycat: cannot reset ${name}`, ex);
      }
    }
  },

  async applySearchEngine(name) {
    if (!name) {
      return;
    }
    const search = await loadSearchService();
    await search.init();
    let engine = search.getEngineByName(name);
    if (!engine && FALLBACK_ENGINES[name] && search.addUserEngine) {
      engine = await search.addUserEngine({ name, url: FALLBACK_ENGINES[name], method: "GET" });
    }
    if (!engine) {
      console.warn(`Keycat: search engine ${name} is not available`);
      return;
    }
    const current = await search.getDefault();
    if (current?.name !== engine.name) {
      const reason = search.CHANGE_REASON?.USER ?? Ci.nsISearchService?.CHANGE_REASON_USER;
      await search.setDefault(engine, reason);
    }
  },

  async syncContentBlocker(wanted) {
    const addon = await lazy.AddonManager.getAddonByID(UBLOCK_ID);
    const managed = Services.prefs.getBoolPref(UBLOCK_MANAGED_PREF, false);
    if (addon) {
      if (managed && wanted && addon.userDisabled) {
        await addon.enable();
      } else if (managed && !wanted && !addon.userDisabled) {
        await addon.disable();
      }
      return;
    }
    if (!wanted || managed) {
      return;
    }
    const install = await lazy.AddonManager.getInstallForURL(UBLOCK_URL, {
      telemetryInfo: { source: "enterprise-policy" },
    });
    await new Promise((resolve, reject) => {
      install.addListener({
        onInstallEnded: resolve,
        onDownloadFailed: () => reject(new Error("download failed")),
        onInstallFailed: () => reject(new Error("install failed")),
        onInstallCancelled: () => reject(new Error("install cancelled")),
      });
      install.install();
    });
    Services.prefs.setBoolPref(UBLOCK_MANAGED_PREF, true);
  },
};
