const KEY = "keycat-token";

function takeToken() {
  const params = new URLSearchParams(location.hash.slice(1));
  const value = params.get("k") || "";
  if (/^[0-9a-f]{64}$/.test(value)) {
    try {
      sessionStorage.setItem(KEY, value);
    } catch {}
    params.delete("k");
    const rest = params.toString();
    history.replaceState(null, "", location.pathname + (rest ? "#" + rest : ""));
    return value;
  }
  try {
    return sessionStorage.getItem(KEY) || "";
  } catch {
    return "";
  }
}

function browserLink() {
  if (location.protocol !== "about:" && location.protocol !== "chrome:") {
    return null;
  }
  try {
    return ChromeUtils.importESModule("resource://keycat/modules/KeycatLink.sys.mjs").KeycatLink;
  } catch {
    return null;
  }
}

const link = browserLink();

export const android = typeof window.KeycatAndroid === "object";

export const inBrowser = !!link?.available;

export const currentProfileId = inBrowser ? link.profileId : "";

const base = inBrowser ? link.apiBase : "";
const token = inBrowser ? link.token : takeToken();

export class ApiError extends Error {
  constructor(status, data) {
    super(data?.error || `Request failed (${status})`);
    this.status = status;
    this.code = data?.code || "error";
  }
}

export async function api(method, path, body) {
  const options = { method, headers: { Authorization: `Bearer ${token}` } };
  if (body !== undefined) {
    options.headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(base + path, options);
  } catch {
    throw new ApiError(0, { error: "Keycat is not responding.", code: "offline" });
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(res.status, data);
  }
  return data;
}

export const hasToken = () => !!token;

export function listen(onEvent, onReconnect) {
  let stopped = false;
  let delay = 500;
  (async () => {
    let first = true;
    while (!stopped) {
      try {
        const res = await fetch(base + "/api/events", { headers: { Authorization: `Bearer ${token}` } });
        if (!res.ok || !res.body) {
          throw new Error("event stream unavailable");
        }
        if (!first) {
          onReconnect();
        }
        first = false;
        delay = 500;
        const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
        let buffer = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) {
            break;
          }
          buffer += value;
          let index;
          while ((index = buffer.indexOf("\n\n")) >= 0) {
            const chunk = buffer.slice(0, index);
            buffer = buffer.slice(index + 2);
            for (const line of chunk.split("\n")) {
              if (line.startsWith("data: ")) {
                try {
                  onEvent(JSON.parse(line.slice(6)));
                } catch (err) {
                  console.error(err);
                }
              }
            }
          }
        }
      } catch {
      }
      first = false;
      await new Promise(r => setTimeout(r, delay));
      delay = Math.min(delay * 2, 5000);
    }
  })();
  return () => {
    stopped = true;
  };
}

export const browser = {
  openSettings(pane = "keycat") {
    link?.openSettings?.(pane);
  },
};
