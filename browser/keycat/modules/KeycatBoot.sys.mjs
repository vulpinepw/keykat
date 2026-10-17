import { setTimeout } from "resource://gre/modules/Timer.sys.mjs";

const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  KeycatAbout: "resource://keycat/modules/KeycatAbout.sys.mjs",
  KeycatAutofill: "resource://keycat/modules/KeycatAutofill.sys.mjs",
  KeycatHubWindow: "resource://keycat/modules/KeycatHubWindow.sys.mjs",
  KeycatImport: "resource://keycat/modules/KeycatImport.sys.mjs",
  KeycatLink: "resource://keycat/modules/KeycatLink.sys.mjs",
  KeycatPrefs: "resource://keycat/modules/KeycatPrefs.sys.mjs",
  KeycatProfile: "resource://keycat/modules/KeycatProfile.sys.mjs",
  KeycatSettings: "resource://keycat/modules/KeycatSettings.sys.mjs",
  KeycatThemes: "resource://keycat/modules/KeycatThemes.sys.mjs",
  KeycatUI: "resource://keycat/modules/KeycatUI.sys.mjs",
});

const TOPICS = [
  "profile-after-change",
  "final-ui-startup",
  "browser-delayed-startup-finished",
  "sessionstore-windows-restored",
  "quit-application-granted",
];

export const KeycatBoot = {
  mode: "unmanaged",
  config: null,
  initialized: false,
  watchingQuit: false,
  started: false,
  quitWhenReady: false,

  init() {
    if (this.initialized) {
      return;
    }
    this.initialized = true;
    const mode = Services.env.get("KEYCAT_MODE");
    this.mode = mode === "profile" || mode === "hub" ? mode : "unmanaged";
    void lazy.KeycatLink.available;

    lazy.KeycatPrefs.applyBase(this.mode);
    if (this.mode === "profile") {
      this.config = lazy.KeycatProfile.readConfig();
      if (this.config) {
        lazy.KeycatPrefs.applyProfileDefaults(this.config);
        lazy.KeycatAbout.register();
        lazy.KeycatSettings.init(this);
      }
    } else if (this.mode === "hub") {
      lazy.KeycatPrefs.applyHubDefaults();
    }
    for (const topic of TOPICS) {
      Services.obs.addObserver(this, topic);
    }
  },

  observe(subject, topic) {
    try {
      switch (topic) {
        case "profile-after-change":
          Services.obs.removeObserver(this, topic);
          if (this.config) {
            lazy.KeycatProfile.resetPrefs(this.config.resetPrefs);
            this.registerActors();
          }
          break;
        case "final-ui-startup":
          Services.obs.removeObserver(this, topic);
          this.onUIStartup();
          break;
        case "browser-delayed-startup-finished":
          this.onWindow(subject);
          break;
        case "sessionstore-windows-restored":
          Services.obs.removeObserver(this, topic);
          this.started = true;
          if (this.quitWhenReady) {
            this.quit();
          }
          break;
        case "quit-application-granted":
          lazy.KeycatLink.stop();
          break;
      }
    } catch (ex) {
      console.error("Keycat:", topic, ex);
    }
  },

  onUIStartup() {
    this.registerSheets();
    this.disablePinning();
    this.disableJumpLists();
    if (this.mode !== "unmanaged") {
      lazy.KeycatThemes.activate(this.themeSetting()).catch(ex => console.error("Keycat themes:", ex));
    }

    if (this.mode === "profile" && this.config) {
      lazy.KeycatImport.init(this.config);
      const privacy = this.config.privacy;
      lazy.KeycatProfile.applySearchEngine(privacy.searchEngine).catch(ex =>
        console.error("Keycat search:", ex)
      );
      lazy.KeycatProfile.syncContentBlocker(privacy.ublockOrigin).catch(ex =>
        console.error("Keycat content blocker:", ex)
      );
    }
    if (this.mode !== "unmanaged") {
      lazy.KeycatLink.start(cmd => this.onCommand(cmd));
    }
  },

  disablePinning() {
    try {
      const { ShellService } = ChromeUtils.importESModule(
        "moz-src:///browser/components/shell/ShellService.sys.mjs"
      );
      ShellService.doesAppNeedPin = async () => false;
      ShellService.pinToTaskbar = async () => {};
      ShellService.doesAppNeedStartMenuPin = async () => false;
      ShellService.pinToStartMenu = async () => false;
    } catch (ex) {
      console.error("Keycat: cannot disable pinning", ex);
    }
  },

  disableJumpLists() {
    try {
      const { WinTaskbarJumpList } = ChromeUtils.importESModule("resource:///modules/WindowsJumpLists.sys.mjs");
      WinTaskbarJumpList.startup = async () => {};
    } catch (ex) {
      console.error("Keycat: cannot disable jump lists", ex);
    }
  },

  registerSheets() {
    try {
      const sss = Cc["@mozilla.org/content/style-sheet-service;1"].getService(Ci.nsIStyleSheetService);
      const uri = Services.io.newURI("chrome://keycat/content/about-pages.css");
      if (!sss.sheetRegistered(uri, sss.USER_SHEET)) {
        sss.loadAndRegisterSheet(uri, sss.USER_SHEET);
      }
    } catch (ex) {
      console.error("Keycat: cannot register about-pages.css", ex);
    }
  },

  registerActors() {
    const privacy = this.config.privacy || {};
    const identity = privacy.fakeIdentity && privacy.fakeIdentity !== "off";
    if (privacy.passwords !== "keycat" && !identity) {
      return;
    }
    ChromeUtils.registerWindowActor("KeycatAutofill", {
      parent: { esModuleURI: "resource://keycat/actors/KeycatAutofillParent.sys.mjs" },
      child: {
        esModuleURI: "resource://keycat/actors/KeycatAutofillChild.sys.mjs",
        events: {
          focusin: {},
          click: { capture: true },
          input: {},
          keydown: {},
          contextmenu: { capture: true },
          pagehide: {},
          DOMFormBeforeSubmit: {},
        },
      },
      matches: ["http://*/*", "https://*/*"],
      allFrames: true,
      messageManagerGroups: ["browsers"],
      safeForUntrustedWebProcess: true,
    });
    this.autofill = true;
  },

  themeSetting() {
    if (this.mode === "hub") {
      const theme = Services.env.get("KEYCAT_HUB_THEME");
      return theme === "light" || theme === "system" ? theme : "dark";
    }
    return this.config?.appearance?.theme || "system";
  },

  quit() {
    if (this.started) {
      this.quitWhenReady = false;
      Services.startup.quit(Ci.nsIAppStartup.eAttemptQuit);
      return;
    }
    if (!this.quitWhenReady) {
      this.quitWhenReady = true;
      setTimeout(() => {
        if (this.quitWhenReady) {
          this.started = true;
          this.quit();
        }
      }, 10000);
    }
  },

  onWindow(win) {
    if (!this.watchingQuit && this.mode === "profile") {
      this.watchingQuit = true;
      Services.obs.addObserver(() => {
        if (!Services.startup.restarting) {
          lazy.KeycatLink.closing();
        }
      }, "quit-application-granted");
    }
    if (this.mode === "hub") {
      lazy.KeycatHubWindow.attach(win);
    } else {
      lazy.KeycatUI.attach(win, this);
      if (this.mode === "profile" && this.config) {
        lazy.KeycatSettings.watchWindow(win);
      }
    }
  },

  applySettings(cmd) {
    if (this.mode !== "profile" || !this.config || !cmd.config) {
      return;
    }
    const before = this.config;
    this.config = cmd.config;
    lazy.KeycatPrefs.applyProfileDefaults(cmd.config);
    lazy.KeycatPrefs.applyLive(cmd.prefs || {}, cmd.reset || []);
    const privacy = cmd.config.privacy || {};
    if (privacy.searchEngine !== before.privacy?.searchEngine) {
      lazy.KeycatProfile.applySearchEngine(privacy.searchEngine).catch(ex => console.error("Keycat search:", ex));
    }
    if (privacy.ublockOrigin !== before.privacy?.ublockOrigin) {
      lazy.KeycatProfile.syncContentBlocker(privacy.ublockOrigin).catch(ex => console.error("Keycat content blocker:", ex));
    }
    if (!this.autofill && (privacy.passwords === "keycat" || (privacy.fakeIdentity && privacy.fakeIdentity !== "off"))) {
      this.registerActors();
      for (const win of Services.wm.getEnumerator("navigator:browser")) {
        lazy.KeycatAutofill.attachWindow(win);
      }
    }
    lazy.KeycatUI.refresh(this);
  },

  onCommand(cmd) {
    switch (cmd.type) {
      case "quit":
        this.quit();
        break;
      case "theme":
        if (this.config) {
          this.config.appearance.theme = cmd.theme;
        }
        lazy.KeycatThemes.activate(cmd.theme).catch(console.error);
        break;
      case "focus": {
        const win = Services.wm.getMostRecentWindow("navigator:browser");
        if (win) {
          if (win.windowState === win.STATE_MINIMIZED) {
            win.restore();
          }
          win.focus();
        }
        break;
      }
      case "minimize":
        for (const win of Services.wm.getEnumerator("navigator:browser")) {
          win.minimize();
        }
        break;
      case "import":
        if (this.mode === "profile") {
          lazy.KeycatImport.open().catch(ex => console.error("Keycat import:", ex));
        }
        break;
      case "settings":
        this.applySettings(cmd);
        break;
    }
  },
};
