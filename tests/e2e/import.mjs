import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { Marionette } from "./marionette.mjs";

const root = path.resolve(import.meta.dirname, "..", "..");
const exe = process.argv[2] || path.join(root, "dist", "Keycat", "Keycat.exe");
const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), "keycat-import-"));
const edgeDir = fs.mkdtempSync(path.join(os.tmpdir(), "keycat-fake-edge-"));
const PASSWORD = "correct horse battery staple";
let base, token, launcher;
const results = [];

function check(name, ok, detail = "") {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  (" + detail + ")" : ""}`);
}

const sleep = ms => new Promise(r => setTimeout(r, ms));

async function waitFor(fn, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await fn().catch(() => undefined);
    if (value) {
      return value;
    }
    if (Date.now() > deadline) {
      return undefined;
    }
    await sleep(250);
  }
}

async function api(method, p, body) {
  const res = await fetch(base + p, {
    method,
    headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: res.status, data: await res.json().catch(() => ({})) };
}

async function main() {
  launcher = spawn(exe, ["--no-hub", "--headless", "--marionette"], {
    env: { ...process.env, KEYCAT_DATA_DIR: dataDir },
    stdio: "ignore",
  });
  const info = await waitFor(async () => {
    const f = path.join(dataDir, "Local", "launcher.json");
    return fs.existsSync(f) ? JSON.parse(fs.readFileSync(f, "utf8")) : undefined;
  }, 60000);
  ({ url: base, token } = info);
  const presets = (await api("GET", "/api/state")).data.presets;
  const created = await api("POST", "/api/profiles", {
    name: "Import", color: "#8B6CF6", avatar: "key",
    security: { mode: "encrypted", preset: "standard", password: PASSWORD },
    privacy: { ...presets.privacy.balanced, passwords: "keycat", ublockOrigin: false },
    appearance: { theme: "dark" },
  });
  const id = created.data.profile.id;
  await api("POST", `/api/profiles/${id}/open`, { method: "password", password: PASSWORD });
  check("profile opens", !!(await waitFor(async () => {
    const { data } = await api("GET", "/api/state");
    return data.profiles.find(p => p.id === id)?.status?.pid > 0;
  }, 90000)));
  await api("POST", `/api/profiles/${id}/keychain/entries`, { title: "Mail", origins: ["https://mail.example"], username: "me", password: "newer-in-keycat" });

  const m = new Marionette();
  await m.connect(90000);
  await m.session();
  const run = await m.exec(`return (async () => {
    const folder = ${JSON.stringify(path.join(edgeDir, "Default"))};
    await IOUtils.makeDirectory(folder);
    const { OSCrypto } = ChromeUtils.importESModule("resource://gre/modules/OSCrypto_win.sys.mjs");
    const { ChromeMigrationUtils } = ChromeUtils.importESModule("resource:///modules/ChromeMigrationUtils.sys.mjs");
    const { ChromeWindowsLoginCrypto } = ChromeUtils.importESModule("resource:///modules/ChromeWindowsLoginCrypto.sys.mjs");
    const { ChromiumEdgeMigrator } = ChromeUtils.importESModule("resource:///modules/ChromeProfileMigrator.sys.mjs");
    const { KeycatImport } = ChromeUtils.importESModule("resource://keycat/modules/KeycatImport.sys.mjs");
    const { Sqlite } = ChromeUtils.importESModule("resource://gre/modules/Sqlite.sys.mjs");

    const key = "keycat-test-key-0123456789abcdef".slice(0, 32);
    const os = new OSCrypto();
    const protectedKey = os.encryptData(key);
    os.finalize();
    ChromeMigrationUtils.getLocalState = async () => ({ os_crypt: { encrypted_key: btoa("DPAPI" + protectedKey) } });
    const crypto = new ChromeWindowsLoginCrypto("Edge");
    const blob = s => Uint8Array.from(s, c => c.charCodeAt(0));
    const v10 = async pw => blob(await crypto.encryptData(pw, "v10"));
    const v20 = blob("v20") ;

    const makeDb = async (name, rows) => {
      const db = await Sqlite.openConnection({ path: PathUtils.join(folder, name) });
      await db.execute("CREATE TABLE logins (origin_url TEXT, action_url TEXT, username_element TEXT, username_value TEXT, password_element TEXT, password_value BLOB, signon_realm TEXT, scheme INTEGER, date_created INTEGER, times_used INTEGER, blacklisted_by_user INTEGER)");
      for (const r of rows) {
        await db.execute("INSERT INTO logins VALUES (:origin, :action, '', :user, '', :pw, :realm, :scheme, 13300000000000000, 1, :never)", {
          origin: r.origin, action: r.action || "", user: r.user, pw: r.pw, realm: r.realm || r.origin, scheme: r.scheme || 0, never: r.never ? 1 : 0,
        });
      }
      await db.close();
    };
    await makeDb("Login Data", [
      { origin: "https://mail.example/login", user: "me", pw: await v10("from-edge") },
      { origin: "https://shop.example/", action: "javascript:void(0)", user: "buyer", pw: await v10("shop-pw") },
      { origin: "https://router.example/", realm: "https://router.example/Admin area", scheme: 1, user: "admin", pw: await v10("router-pw") },
      { origin: "https://bank.example/", user: "client", pw: Uint8Array.from([...v20, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]) },
      { origin: "android://hash@com.example.app/", user: "app", pw: await v10("app-pw") },
      { origin: "https://never.example/", user: "", pw: new Uint8Array(0), never: true },
    ]);
    await makeDb("Login Data For Account", [
      { origin: "https://account.example/", user: "synced", pw: await v10("account-pw") },
    ]);
    crypto.finalize();

    const resource = await new ChromiumEdgeMigrator()._GetPasswordsResource(folder);
    const ok = await new Promise(resolve => resource.migrate(resolve));
    const unreadable = KeycatImport.unreadable;
    const from = KeycatImport.unreadableFrom;
    await KeycatImport.finish();
    const note = gNotificationBox.getNotificationWithValue("keycat-import");
    const label = note ? (note.messageText?.textContent || note.label || "") : "";
    const buttons = note ? [...note.querySelectorAll("button")].map(b => b.label || b.textContent).filter(Boolean) : [];
    const leftInFirefox = (await Services.logins.getAllLogins()).length;
    return { ok, unreadable, from, label: String(label), buttons, leftInFirefox };
  })();`);
  check("Edge passwords imported", run.ok === true);
  check("the password only Edge can read is counted, nothing else", run.unreadable === 1 && run.from === "Microsoft Edge", JSON.stringify({ unreadable: run.unreadable, from: run.from }));
  check("Keycat explains how to bring it over", /could not be read: Microsoft Edge/.test(run.label) && /Export passwords/.test(run.label), run.label.slice(0, 120));
  check("notification offers the CSV import", run.buttons.some(b => /CSV/.test(b)), JSON.stringify(run.buttons));
  check("nothing left in the browser's own store", run.leftInFirefox === 0);

  const { data } = await api("GET", `/api/profiles/${id}/keychain`);
  const entries = data.entries || [];
  const by = origin => entries.filter(e => (e.origins || []).includes(origin));
  check("login with an unusual form address imported", by("https://shop.example").length === 1);
  check("login from the second database imported", by("https://account.example").length === 1);
  check("HTTP authentication login imported", by("https://router.example").length === 1);
  check("an import keeps the existing password next to the imported one", by("https://mail.example").length === 2);
  check("no Android app or never-save entries", by("android://hash@com.example.app").length === 0 && by("https://never.example").length === 0);
  check("the unreadable login was not imported", by("https://bank.example").length === 0);
  const shop = by("https://shop.example")[0];
  const pw = shop ? (await api("GET", `/api/profiles/${id}/keychain/entries/${shop.id}/password`)).data.password : "";
  check("imported password decrypted correctly", pw === "shop-pw");

  await m.exec(`return (async () => {
    const info = Cc["@mozilla.org/login-manager/loginInfo;1"].createInstance(Ci.nsILoginInfo);
    info.init("https://csv.example", "", null, "csv-user", "csv-pw", "", "");
    await Services.logins.addLoginAsync(info);
    return true;
  })();`);
  const moved = await waitFor(async () => {
    const { data } = await api("GET", `/api/profiles/${id}/keychain`);
    return (data.entries || []).some(e => (e.origins || []).includes("https://csv.example"));
  }, 15000);
  check("a login added to the browser's store moves to Keycat Passwords", !!moved);
  m.close();
}

try {
  await main();
} catch (err) {
  check("unexpected error", false, err.stack || String(err));
} finally {
  await api("POST", "/api/quit", {}).catch(() => {});
  await sleep(4000);
  launcher?.kill();
  fs.rmSync(dataDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 500 });
  fs.rmSync(edgeDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 500 });
  const failed = results.filter(r => !r.ok).length;
  console.log(`\n${results.length - failed}/${results.length} checks passed`);
  process.exitCode = failed ? 1 : 0;
}
