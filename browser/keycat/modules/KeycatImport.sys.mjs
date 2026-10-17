import { setTimeout, clearTimeout } from "resource://gre/modules/Timer.sys.mjs";

const lazy = {};
ChromeUtils.defineESModuleGetters(lazy, {
  ChromeMigrationUtils: "resource:///modules/ChromeMigrationUtils.sys.mjs",
  ChromeProfileMigrator: "resource:///modules/ChromeProfileMigrator.sys.mjs",
  ChromeWindowsLoginCrypto: "resource:///modules/ChromeWindowsLoginCrypto.sys.mjs",
  KeycatLink: "resource://keycat/modules/KeycatLink.sys.mjs",
  MigrationUtils: "resource:///modules/MigrationUtils.sys.mjs",
});

const BATCH = 200;

const LOGIN_DATABASES = ["Login Data", "Login Data For Account"];

const BROWSER_NAMES = {
  chrome: "Google Chrome",
  "chrome-beta": "Google Chrome Beta",
  "chrome-dev": "Google Chrome Dev",
  canary: "Google Chrome Canary",
  chromium: "Chromium",
  brave: "Brave",
  "chromium-edge": "Microsoft Edge",
  "chromium-edge-beta": "Microsoft Edge Beta",
  "chromium-360se": "360 Secure Browser",
  opera: "Opera",
  "opera-gx": "Opera GX",
  vivaldi: "Vivaldi",
};

const SCHEME_BASIC = 1;
const SCHEME_DIGEST = 2;

function browserWindow() {
  const win = Services.wm.getMostRecentWindow("navigator:browser");
  if (win?.gBrowserInit?.delayedStartupFinished) {
    return Promise.resolve(win);
  }
  return new Promise(resolve => {
    const observer = subject => {
      Services.obs.removeObserver(observer, "browser-delayed-startup-finished");
      resolve(subject);
    };
    Services.obs.addObserver(observer, "browser-delayed-startup-finished");
  });
}

function webOrigin(url) {
  try {
    const u = new URL(url);
    return u.protocol === "https:" || u.protocol === "http:" ? u.origin : "";
  } catch {
    return "";
  }
}

function countLogins(path) {
  return lazy.MigrationUtils.getRowsFromDBWithoutLocks(
    path,
    "Chrome passwords",
    "SELECT COUNT(*) AS n FROM logins WHERE blacklisted_by_user = 0"
  ).then(rows => rows[0]?.getResultByName("n") || 0, () => 0);
}

async function readChromiumLogins(folder, userDataPathSuffix) {
  const loginCrypto = new lazy.ChromeWindowsLoginCrypto(userDataPathSuffix);
  const logins = [];
  let unreadable = 0;
  const now = new Date();
  try {
    for (const name of LOGIN_DATABASES) {
      const path = PathUtils.join(folder, name);
      if (!(await IOUtils.exists(path))) {
        continue;
      }
      const rows = await lazy.MigrationUtils.getRowsFromDBWithoutLocks(
        path,
        "Chrome passwords",
        `SELECT origin_url, signon_realm, scheme, username_value, password_value,
          date_created, times_used FROM logins WHERE blacklisted_by_user = 0`
      );
      for (const row of rows) {
        const realm = row.getResultByName("signon_realm") || "";
        const origin = webOrigin(row.getResultByName("origin_url")) || webOrigin(realm);
        const value = row.getResultByName("password_value");
        if (!origin || !value?.length) {
          continue;
        }
        let password;
        try {
          password = await loginCrypto.decryptData(value);
        } catch {
          unreadable++;
          continue;
        }
        if (!password) {
          continue;
        }
        const scheme = row.getResultByName("scheme");
        logins.push({
          origin,
          username: row.getResultByName("username_value") || "",
          password,
          httpRealm: scheme === SCHEME_BASIC || scheme === SCHEME_DIGEST ? realm.slice(origin.length + 1) : null,
          timeCreated: lazy.ChromeMigrationUtils.chromeTimeToDate(row.getResultByName("date_created") + 0, now).getTime(),
          timesUsed: row.getResultByName("times_used") + 0,
        });
      }
    }
  } finally {
    loginCrypto.finalize();
  }
  return { logins, unreadable };
}

export const KeycatImport = {
  config: null,
  moving: null,
  again: false,
  finishing: null,
  timer: null,
  imported: 0,
  unreadable: 0,
  unreadableFrom: "",

  init(config) {
    this.config = config;
    this.readChromiumPasswords();
    Services.obs.addObserver(this, "Migration:Ended");
    if (this.usesKeycatPasswords) {
      Services.obs.addObserver(this, "passwordmgr-storage-changed");
      this.timer = setTimeout(() => this.finish(), 5000);
    }
  },

  get usesKeycatPasswords() {
    return this.config?.privacy?.passwords === "keycat";
  },

  async open(migratorKey) {
    const win = await browserWindow();
    win.focus();
    const options = { entrypoint: lazy.MigrationUtils.MIGRATION_ENTRYPOINTS.PREFERENCES };
    if (migratorKey) {
      options.migratorKey = migratorKey;
    }
    await lazy.MigrationUtils.showMigrationWizard(win, options);
  },

  readChromiumPasswords() {
    const keycat = this;
    lazy.ChromeProfileMigrator.prototype._GetPasswordsResource = async function (folder) {
      let count = 0;
      for (const name of LOGIN_DATABASES) {
        const path = PathUtils.join(folder, name);
        if (await IOUtils.exists(path)) {
          count += await countLogins(path);
        }
      }
      if (!count) {
        return null;
      }
      const suffix = this._chromeUserDataPathSuffix;
      const browser = BROWSER_NAMES[this.constructor.key] || "that browser";
      return {
        type: lazy.MigrationUtils.resourceTypes.PASSWORDS,
        async migrate(callback) {
          try {
            const { logins, unreadable } = await readChromiumLogins(folder, suffix);
            await keycat.addLogins(logins);
            if (unreadable) {
              keycat.unreadable += unreadable;
              keycat.unreadableFrom = browser;
            }
            callback(true);
          } catch (ex) {
            console.error("Keycat import:", ex);
            callback(false);
          }
        },
      };
    };
  },

  async addLogins(logins) {
    if (!logins.length) {
      return;
    }
    if (!this.usesKeycatPasswords) {
      await lazy.MigrationUtils.insertLoginsWrapper(logins.map(l => ({
        origin: l.origin,
        formActionOrigin: l.httpRealm === null ? "" : null,
        httpRealm: l.httpRealm,
        username: l.username,
        password: l.password,
        timeCreated: l.timeCreated,
        timesUsed: l.timesUsed,
      })));
      return;
    }
    for (let i = 0; i < logins.length; i += BATCH) {
      await lazy.KeycatLink.call("POST", "/api/instance/keychain/import", {
        logins: logins.slice(i, i + BATCH).map(l => ({ origin: l.origin, username: l.username, password: l.password })),
      });
    }
    lazy.MigrationUtils._importQuantities.logins += logins.length;
    this.imported += logins.length;
  },

  observe(subject, topic, data) {
    if (topic === "Migration:Ended" || (topic === "passwordmgr-storage-changed" && (data === "addLogin" || data === "importLogins"))) {
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.finish(), 1500);
    }
  },

  finish() {
    this.finishing = (this.finishing || Promise.resolve())
      .then(async () => {
        if (this.usesKeycatPasswords) {
          this.imported += await this.moveLogins();
        }
        const imported = this.imported;
        const unreadable = this.unreadable;
        const from = this.unreadableFrom;
        this.imported = this.unreadable = 0;
        if (imported || unreadable) {
          await this.notify(imported, unreadable, from);
        }
      })
      .catch(ex => console.error("Keycat import:", ex));
    return this.finishing;
  },

  async moveLogins() {
    if (this.moving) {
      this.again = true;
      return this.moving;
    }
    this.moving = (async () => {
      let moved = 0;
      do {
        this.again = false;
        const logins = (await Services.logins.getAllLogins()).filter(l => /^https?:\/\//.test(l.origin) && l.password);
        for (let i = 0; i < logins.length; i += BATCH) {
          const batch = logins.slice(i, i + BATCH);
          const result = await lazy.KeycatLink.call("POST", "/api/instance/keychain/import", {
            logins: batch.map(l => ({ origin: l.origin, username: l.username, password: l.password })),
          });
          const skipped = new Set(result.skipped || []);
          for (const [index, login] of batch.entries()) {
            if (!skipped.has(index)) {
              await Services.logins.removeLoginAsync(login);
              moved++;
            }
          }
        }
      } while (this.again);
      return moved;
    })();
    try {
      return await this.moving;
    } finally {
      this.moving = null;
    }
  },

  async notify(imported, unreadable, from) {
    const win = await browserWindow();
    const box = win.gNotificationBox;
    if (!box) {
      return;
    }
    const parts = [];
    if (imported && this.usesKeycatPasswords) {
      parts.push(`${imported} password${imported === 1 ? " was" : "s were"} imported into Keycat Passwords, encrypted with this profile.`);
    }
    if (unreadable) {
      parts.push(
        `${unreadable} password${unreadable === 1 ? "" : "s"} could not be read: ${from} encrypts ${unreadable === 1 ? "it" : "them"} so that only ${from} can open ${unreadable === 1 ? "it" : "them"}. ` +
          `In ${from}'s password settings, choose Export passwords, import that CSV file here, then delete the file.`
      );
    } else if (imported) {
      parts.push("If you imported a CSV file, delete it now: it holds your passwords unencrypted.");
    }
    box.getNotificationWithValue?.("keycat-import")?.close?.();
    const buttons = unreadable
      ? [{ label: "Import a CSV file", callback: () => void this.open("file-password-csv") }]
      : [];
    await box.appendNotification("keycat-import", {
      label: parts.join(" "),
      priority: unreadable ? box.PRIORITY_WARNING_MEDIUM : box.PRIORITY_INFO_MEDIUM,
    }, buttons);
  },
};
