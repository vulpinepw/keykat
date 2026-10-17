import { AppConstants } from "resource://gre/modules/AppConstants.sys.mjs";
import { setTimeout } from "resource://gre/modules/Timer.sys.mjs";

const API = Services.env.get("KEYCAT_API");
const TOKEN = Services.env.get("KEYCAT_TOKEN");
const PROFILE = Services.env.get("KEYCAT_PROFILE");

if (TOKEN) {
  Services.env.set("KEYCAT_TOKEN", "");
  Services.obs.addObserver((subject, topic, data) => {
    if (data === "restart") {
      Services.env.set("KEYCAT_TOKEN", TOKEN);
    }
  }, "quit-application");
}

let stopped = false;
let polling = null;

async function call(method, path, body, signal) {
  const options = {
    method,
    headers: { Authorization: "Bearer " + TOKEN },
    signal,
  };
  if (body !== undefined) {
    options.headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  const response = await fetch(API + path, options);
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `HTTP ${response.status}`);
  }
  return data;
}

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

function allowForeground() {
  if (AppConstants.platform !== "win") {
    return;
  }
  try {
    const { ctypes } = ChromeUtils.importESModule("resource://gre/modules/ctypes.sys.mjs");
    const user32 = ctypes.open("user32.dll");
    try {
      const allow = user32.declare("AllowSetForegroundWindow", ctypes.winapi_abi, ctypes.int, ctypes.uint32_t);
      allow(0xffffffff);
    } finally {
      user32.close();
    }
  } catch (ex) {
    console.error("Keycat: AllowSetForegroundWindow", ex);
  }
}

export const KeycatLink = {
  get available() {
    return !!(API && TOKEN);
  },

  get apiBase() {
    return API || "";
  },
  get token() {
    return TOKEN || "";
  },
  get profileId() {
    return PROFILE || "";
  },

  call,

  async start(onCommand) {
    if (!this.available) {
      return;
    }
    for (let attempt = 0; attempt < 5 && !stopped; attempt++) {
      try {
        await call("POST", "/api/instance/hello", { pid: Services.appinfo.processID });
        break;
      } catch (ex) {
        await sleep(1000 * (attempt + 1));
      }
    }
    while (!stopped) {
      try {
        polling = new AbortController();
        const { commands } = await call("GET", "/api/instance/commands", undefined, polling.signal);
        for (const cmd of commands || []) {
          try {
            onCommand(cmd);
          } catch (ex) {
            console.error("Keycat command failed:", cmd, ex);
          }
        }
      } catch (ex) {
        if (stopped) {
          break;
        }
        await sleep(3000);
      } finally {
        polling = null;
      }
    }
  },

  stop() {
    stopped = true;
    if (polling) {
      polling.abort();
      const deadline = Date.now() + 500;
      Services.tm.spinEventLoopUntil("Keycat: stop", () => !polling || Date.now() > deadline);
    }
  },

  showHub() {
    return call("POST", "/api/instance/hub", {});
  },

  closing() {
    if (!this.available) {
      return;
    }
    allowForeground();
    let done = false;
    call("POST", "/api/instance/closing", {})
      .catch(() => {})
      .finally(() => {
        done = true;
      });
    const deadline = Date.now() + 1000;
    Services.tm.spinEventLoopUntil("Keycat: closing", () => done || Date.now() > deadline);
  },

  saveSettings(patch) {
    return call("POST", "/api/instance/settings", patch);
  },

  openSettings(pane = "keycat") {
    const win = Services.wm.getMostRecentWindow("navigator:browser");
    win?.openPreferences?.(pane);
  },
};
