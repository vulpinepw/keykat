import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { Marionette } from "./marionette.mjs";

const root = path.resolve(import.meta.dirname, "..", "..");
const exe = process.argv[2] || path.join(root, "dist", "Keycat", "Keycat.exe");
const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), "keycat-hub-"));
const hubDir = path.join(dataDir, "Local", "Hub");
const PASSWORD = "correct horse battery staple";
let base, token, launcher;
let exited = false;
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

async function state(id) {
  const { data } = await api("GET", "/api/state");
  return data.profiles.find(p => p.id === id)?.status || {};
}

function hubOpen() {
  try {
    fs.closeSync(fs.openSync(path.join(hubDir, "parent.lock"), "r+"));
    return false;
  } catch (err) {
    return err.code === "EBUSY" || err.code === "EPERM";
  }
}

async function hubSettled(timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let since = 0;
  while (Date.now() < deadline) {
    if (hubOpen()) {
      since ||= Date.now();
      if (Date.now() - since >= 2000) {
        return true;
      }
    } else {
      since = 0;
    }
    await sleep(200);
  }
  return false;
}

function closeHubWindow() {
  const filter = hubDir.replace(/'/g, "''");
  execFileSync("powershell.exe", ["-NoProfile", "-Command",
    `Get-CimInstance Win32_Process -Filter "Name='firefox.exe'" | Where-Object { $_.CommandLine -like '*${filter}*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`]);
}

async function openProfile(id) {
  await api("POST", `/api/profiles/${id}/open`, { method: "password", password: PASSWORD });
  return waitFor(async () => {
    const st = await state(id);
    return st.state === "running" && st.pid > 0;
  }, 90000);
}

async function main() {
  launcher = spawn(exe, ["--headless", "--marionette"], {
    env: { ...process.env, KEYCAT_DATA_DIR: dataDir },
    stdio: "ignore",
  });
  launcher.on("exit", () => { exited = true; });
  const info = await waitFor(async () => {
    const f = path.join(dataDir, "Local", "launcher.json");
    return fs.existsSync(f) ? JSON.parse(fs.readFileSync(f, "utf8")) : undefined;
  }, 60000);
  ({ url: base, token } = info);
  check("profiles window opens with Keycat", await hubSettled(60000));

  const presets = (await api("GET", "/api/state")).data.presets;
  const created = await api("POST", "/api/profiles", {
    name: "Hub test", color: "#8B6CF6", avatar: "cat",
    security: { mode: "encrypted", preset: "standard", password: PASSWORD },
    privacy: presets.privacy.balanced, appearance: { theme: "dark" },
  });
  const id = created.data.profile.id;

  check("profile opens", !!(await openProfile(id)));
  check("profiles window makes way for the browser", !!(await waitFor(async () => !hubOpen(), 30000)));

  await api("POST", `/api/profiles/${id}/lock`, {});
  const back = await waitFor(async () => hubOpen(), 30000);
  const lockedThen = (await state(id)).state;
  check("profiles window comes back when the profile is locked", !!back, `profile ${lockedThen} at that moment`);
  check("profile sealed", !!(await waitFor(async () => (await state(id)).state === "locked", 60000)));
  check("Keycat keeps running", !exited);
  await hubSettled(30000);

  check("profile opens again", !!(await openProfile(id)));
  check("profiles window makes way again", !!(await waitFor(async () => !hubOpen(), 30000)));
  const m = new Marionette();
  await m.connect(90000);
  await m.session();
  await m.exec(`Services.startup.quit(Ci.nsIAppStartup.eAttemptQuit); return true;`).catch(() => {});
  m.close();
  let sawClosing = false;
  const backAgain = await waitFor(async () => {
    if ((await state(id)).state === "closing") {
      sawClosing = true;
    }
    return hubOpen();
  }, 30000);
  check("browser told Keycat it is closing", sawClosing);
  check("profiles window comes back when the browser locks the profile", !!backAgain);
  check("profile sealed again", !!(await waitFor(async () => (await state(id)).state === "locked", 60000)));
  check("Keycat still running", !exited);

  await hubSettled(30000);
  check("profile opens a third time", !!(await openProfile(id)));
  const t0 = Date.now();
  await api("POST", `/api/profiles/${id}/lock`, {});
  const quick = await waitFor(async () => (await state(id)).state === "locked", 60000);
  const seconds = (Date.now() - t0) / 1000;
  check("a profile locked right after opening is sealed quickly", !!quick && seconds < 15, `${seconds.toFixed(1)} s`);
  await hubSettled(30000);

  closeHubWindow();
  check("Keycat exits once its window is closed with no profile open", !!(await waitFor(async () => exited, 30000)));
  const log = fs.readFileSync(path.join(dataDir, "Local", "Logs", "keycat.log"), "utf8");
  check("log records the window coming back each time", (log.match(/no profile open: showing the profiles window/g) || []).length === 3);
  check("log records the browser closing", /browser closing/.test(log),
    log.split("\n").filter(l => /closing|exited|sealed|hub|window|opened/.test(l)).map(l => l.slice(11, 19) + l.slice(26)).join(" | "));
}

try {
  await main();
} catch (err) {
  check("unexpected error", false, err.stack || String(err));
} finally {
  if (!exited) {
    await api("POST", "/api/quit", {}).catch(() => {});
    await sleep(3000);
    launcher?.kill();
  }
  await sleep(1500);
  fs.rmSync(dataDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 500 });
  const failed = results.filter(r => !r.ok).length;
  console.log(`\n${results.length - failed}/${results.length} checks passed`);
  process.exitCode = failed ? 1 : 0;
}
