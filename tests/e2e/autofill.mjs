import { spawn } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { Marionette } from "./marionette.mjs";

const root = path.resolve(import.meta.dirname, "..", "..");
const exe = process.argv[2] || (process.platform === "win32" ? path.join(root, "dist", "Keycat", "Keycat.exe") : path.join(root, "dist", "linux", "keycat", "keycat"));
const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), "keycat-autofill-"));
const PASSWORD = "correct horse battery staple";
const results = [];
let base, token, launcher, site;

function check(name, ok, detail = "") {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  (" + detail + ")" : ""}`);
}

const sleep = ms => new Promise(r => setTimeout(r, ms));

async function waitFor(fn, timeoutMs, label) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await fn().catch(() => undefined);
    if (value) {
      return value;
    }
    if (Date.now() > deadline) {
      throw new Error(`timed out waiting for ${label}`);
    }
    await sleep(200);
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


const PAGES = {
  "/login": `<!doctype html><meta charset="utf-8"><title>Log in</title>
    <form id="f" action="/done" method="post">
      <label>E-mail <input id="user" name="username" autocomplete="username"></label>
      <label>Password <input id="pass" name="password" type="password" autocomplete="current-password"></label>
      <button id="go" type="submit">Log in</button>
    </form>`,
  "/signup": `<!doctype html><meta charset="utf-8"><title>Sign up</title>
    <form id="f" action="/done" method="post">
      <input id="first" name="first_name" placeholder="First name">
      <input id="last" name="last_name" placeholder="Last name">
      <input id="email" name="email" type="email" placeholder="E-mail">
      <input id="phone" name="phone" type="tel" placeholder="Phone">
      <input id="street" name="address" placeholder="Street address">
      <input id="city" name="city" placeholder="City">
      <input id="zip" name="zip" placeholder="ZIP code">
      <select id="country" name="country"><option value="">Country</option><option value="US">United States</option><option value="FR">France</option><option value="GB">United Kingdom</option></select>
      <input id="pw1" name="password" type="password" autocomplete="new-password" placeholder="Password">
      <input id="pw2" name="password2" type="password" autocomplete="new-password" placeholder="Repeat password">
      <button id="go" type="submit">Create account</button>
    </form>`,
  "/identifier": `<!doctype html><meta charset="utf-8"><title>Sign in</title>
    <form id="f" action="/done" method="post">
      <input id="ident" type="email" name="identifier" autocomplete="username" placeholder="Email or phone">
      <button id="next" type="button">Next</button>
    </form>`,
  "/formless": `<!doctype html><meta charset="utf-8"><title>Members</title>
    <div class="news"><input id="newsletter" type="email" placeholder="Newsletter e-mail"><input id="nname" placeholder="Your name"><input id="ncity" placeholder="City"></div>
    <div class="box"><div class="row"><input id="fuser" name="login" placeholder="Login"></div>
      <div class="row"><input id="fpass" type="password" placeholder="Password"></div><button id="fgo">Sign in</button></div>`,
};

function startSite() {
  return new Promise(resolve => {
    const server = http.createServer((req, res) => {
      const page = req.method === "POST" ? "<!doctype html><title>Done</title><p id=done>Welcome</p>" : PAGES[req.url];
      res.writeHead(page ? 200 : 404, { "Content-Type": "text/html; charset=utf-8" });
      res.end(page || "not found");
    });
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}


let m;

async function inContent(fn) {
  await m.send("Marionette:SetContext", { value: "content" });
  try {
    return await fn();
  } finally {
    await m.send("Marionette:SetContext", { value: "chrome" });
  }
}

const contentScript = script => inContent(async () => (await m.send("WebDriver:ExecuteScript", { script, args: [] })).value);

async function navigate(url) {
  await inContent(() => m.send("WebDriver:Navigate", { url }));
}

async function click(selector) {
  await inContent(async () => {
    const found = await m.send("WebDriver:FindElement", { using: "css selector", value: selector });
    await m.send("WebDriver:ElementClick", { id: Object.values(found.value)[0] });
  });
}

const panelState = () => m.exec(`return document.getElementById("keycat-autofill-panel")?.state || "none";`);

async function panelItems() {
  await waitFor(async () => (await panelState()) === "open", 8000, "autofill panel");
  return m.exec(`return [...document.querySelectorAll("#keycat-autofill-panel .kca-item")]
    .map(b => ({ title: b.querySelector(".kca-title")?.textContent, sub: b.querySelector(".kca-sub")?.textContent || "" }));`);
}

async function pick(title) {
  const ok = await m.exec(`const item = [...document.querySelectorAll("#keycat-autofill-panel .kca-item")]
      .find(b => b.querySelector(".kca-title")?.textContent === ${JSON.stringify(title)});
    if (!item) return false;
    item.click();
    return true;`);
  if (!ok) {
    throw new Error(`no panel item "${title}"`);
  }
  await sleep(600);
}

const values = ids => contentScript(`return Object.fromEntries(${JSON.stringify(ids)}.map(id => [id, document.getElementById(id).value]));`);


async function main() {
  site = await startSite();
  const origin = `http://127.0.0.1:${site.address().port}`;
  console.log("Keycat autofill e2e: exe", exe);
  console.log("test site", origin);

  launcher = spawn(exe, ["--no-hub", "--headless", "--marionette"], {
    env: { ...process.env, KEYCAT_DATA_DIR: dataDir },
    stdio: "ignore",
  });
  const info = await waitFor(async () => {
    const f = path.join(dataDir, "Local", "launcher.json");
    return fs.existsSync(f) ? JSON.parse(fs.readFileSync(f, "utf8")) : undefined;
  }, 60000, "launcher.json");
  ({ url: base, token } = info);

  const { data: state } = await api("GET", "/api/state");
  const created = await api("POST", "/api/profiles", {
    name: "Autofill", color: "#8B6CF6", avatar: "key",
    security: { mode: "encrypted", preset: "standard", password: PASSWORD },
    privacy: state.presets.privacy.balanced,
    appearance: { theme: "dark" },
  });
  const profile = created.data.profile;
  check("profile with Keycat Passwords and fake identity created", created.status === 201 && profile.privacy.passwords === "keycat" && profile.privacy.fakeIdentity === "profile", created.data.error);

  await api("POST", `/api/profiles/${profile.id}/open`, { method: "password", password: PASSWORD });
  await waitFor(async () => (await api("GET", "/api/state")).data.profiles[0].status?.pid > 0, 90000, "browser hello");
  const saved = await api("POST", `/api/profiles/${profile.id}/keychain/entries`, {
    title: "Test site", origins: [origin], username: "alice@example.com", password: "s3cret-Pass!",
  });
  check("login stored in Keycat Passwords", saved.status === 200, saved.data.error);

  m = new Marionette();
  await m.connect(90000);
  await m.session();
  await sleep(1500);

  await navigate(`${origin}/signup`);
  await contentScript(`document.getElementById("first").focus(); return true;`);
  await sleep(1200);
  check("no panel when a page focuses a field other than a login field", (await panelState()) !== "open");

  await navigate(`${origin}/login`);
  await contentScript(`document.getElementById("user").blur(); document.getElementById("user").focus(); return true;`);
  const autoItems = await panelItems().catch(() => []);
  check("saved logins offered when a login page focuses its login field", autoItems.length === 2 && autoItems[0].title === "alice@example.com", JSON.stringify(autoItems.map(i => i.title)));
  await m.exec(`document.getElementById("keycat-autofill-panel")?.hidePopup(); return true;`);
  await sleep(300);

  await click("#user");
  const loginItems = await panelItems();
  check("panel offers the saved login", loginItems.some(i => i.title === "alice@example.com" && i.sub === "Test site"), JSON.stringify(loginItems.map(i => i.title)));
  await pick("alice@example.com");
  const filled = await values(["user", "pass"]);
  check("login filled into the page", filled.user === "alice@example.com" && filled.pass === "s3cret-Pass!", JSON.stringify({ user: filled.user, pass: filled.pass ? "(set)" : "" }));
  check("panel closed after filling", (await panelState()) !== "open");

  await contentScript(`document.getElementById("pass").value = "n3w-Pass!"; return true;`);
  await click("#go");
  const prompt = await waitFor(() => m.exec(`const n = PopupNotifications.getNotification("keycat-password", gBrowser.selectedBrowser);
      return n ? { label: n.mainAction.label, message: n.message } : null;`), 8000, "save prompt");
  check("update prompt shown after logging in with a new password", prompt.label === "Update", JSON.stringify(prompt));
  await m.exec(`PopupNotifications.getNotification("keycat-password", gBrowser.selectedBrowser).mainAction.callback(); return true;`);
  await sleep(1000);
  const entryId = saved.data.id;
  const stored = await api("GET", `/api/profiles/${profile.id}/keychain/entries/${entryId}/password`);
  check("updated password saved in Keycat Passwords", stored.data.password === "n3w-Pass!");

  await navigate(`${origin}/identifier`);
  await click("#ident");
  const identItems = await panelItems().catch(() => []);
  check("saved login offered on an e-mail-first sign-in page", identItems.some(i => i.title === "alice@example.com"), JSON.stringify(identItems.map(i => i.title)));
  if (identItems.some(i => i.title === "alice@example.com")) {
    await pick("alice@example.com");
    const ident = await values(["ident"]);
    check("account filled on the e-mail-first page", ident.ident === "alice@example.com", ident.ident);
  }

  await navigate(`${origin}/formless`);
  await click("#fuser");
  const formlessItems = await panelItems().catch(() => []);
  check("saved login offered on a page without a form element", formlessItems.some(i => i.title === "alice@example.com"), JSON.stringify(formlessItems.map(i => i.title)));
  if (formlessItems.some(i => i.title === "alice@example.com")) {
    await pick("alice@example.com");
    const fl = await values(["fuser", "fpass", "newsletter"]);
    check("login filled on the page without a form, other fields untouched", fl.fuser === "alice@example.com" && !!fl.fpass && !fl.newsletter, JSON.stringify({ ...fl, fpass: fl.fpass ? "(set)" : "" }));
  }

  await navigate(`${origin}/signup`);
  await click("#first");
  const idItems = await panelItems();
  const fillItem = idItems.find(i => i.title === "Fill with fake identity");
  check("panel offers the fake identity", !!fillItem && idItems.some(i => i.title === "Use another random identity"), JSON.stringify(idItems.map(i => i.title)));
  await pick("Fill with fake identity");
  const ids = ["first", "last", "email", "phone", "street", "city", "zip", "country", "pw1", "pw2"];
  const one = await values(ids);
  check("identity filled: name, e-mail, phone, address", !!one.first && !!one.last && /@/.test(one.email) && !!one.phone && !!one.street && !!one.city && !!one.zip && !!one.country, JSON.stringify({ ...one, pw1: undefined, pw2: undefined }));
  check("identity e-mail is a placeholder address", /@example\.(com|org|net)$/.test(one.email), one.email);
  check("sign-up password generated and confirmed", one.pw1.length >= 12 && one.pw1 === one.pw2, `${one.pw1.length} characters`);
  check("identity matches the profile's identity", (fillItem.sub || "").includes(one.email), fillItem.sub);

  await click("#email");
  await panelItems();
  await pick("Use another random identity");
  const two = await values(ids);
  check("another random identity replaces the first", two.email !== one.email && !!two.first, `${one.email} -> ${two.email}`);

  await contentScript(`for (const id of ["pw1", "pw2"]) document.getElementById(id).value = ""; return true;`);
  await click("#pw1");
  const pwItems = await panelItems();
  const strong = pwItems.find(i => i.title === "Use a strong password");
  check("panel offers a strong password on a new-password field", !!strong && strong.sub.length >= 12, strong?.sub);
  await pick("Use a strong password");
  const pw = await values(["pw1", "pw2"]);
  check("generated password filled into both fields", pw.pw1 === strong.sub && pw.pw2 === strong.sub);

  const menuItem = await m.exec(`return !!document.getElementById("keycat-context-autofill");`);
  check("\"Fill with Keycat\" added to the page context menu", menuItem);

  const imp = await api("POST", `/api/profiles/${profile.id}/import`, {});
  check("import request accepted for the open profile", imp.status === 200, imp.data.error);
  const wizard = await waitFor(() => m.exec(`const uri = gBrowser.selectedBrowser.currentURI.spec;
      return uri.startsWith("about:preferences") ? uri : null;`), 10000, "import window").catch(() => "");
  check("the import window opened in the profile", !!wizard, wizard);

  await m.exec(`return (async () => {
    const LoginInfo = Components.Constructor("@mozilla.org/login-manager/loginInfo;1", Ci.nsILoginInfo, "init");
    await Services.logins.addLoginAsync(new LoginInfo("https://imported.example", "https://imported.example", null, "imported-user", "imported-pass-123", "", ""));
    return true;
  })();`);
  const movedOut = await waitFor(() => m.exec(`return (async () => (await Services.logins.getAllLogins()).length === 0)();`), 10000, "login moved").catch(() => false);
  const keychain = await api("GET", `/api/profiles/${profile.id}/keychain`);
  const importedEntry = (keychain.data.entries || []).find(e => e.username === "imported-user");
  check("imported password moved from the built-in store into Keycat Passwords", movedOut && !!importedEntry, importedEntry?.title);
  if (importedEntry) {
    const pw = await api("GET", `/api/profiles/${profile.id}/keychain/entries/${importedEntry.id}/password`);
    check("imported password kept intact", pw.data.password === "imported-pass-123");
  }

  fs.mkdirSync(path.join(root, "tests", "e2e", "output"), { recursive: true });
  m.close();
}

main()
  .catch(err => {
    check("unexpected error", false, err.stack || String(err));
  })
  .finally(async () => {
    if (base) {
      await api("POST", "/api/quit", {}).catch(() => {});
    }
    await sleep(3000);
    launcher?.kill();
    site?.close();
    const failed = results.filter(r => !r.ok).length;
    console.log(`\n${results.length - failed}/${results.length} checks passed`);
    try {
      fs.rmSync(dataDir, { recursive: true, force: true });
    } catch {}
    process.exit(failed ? 1 : 0);
  });
