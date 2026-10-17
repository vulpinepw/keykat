import { api } from "./api.js";

const listeners = new Set();

export const store = {
  state: null,

  set(next) {
    this.state = next;
    this.emit({ type: "state" });
  },

  emit(change) {
    for (const fn of listeners) {
      try {
        fn(this.state, change);
      } catch (err) {
        console.error(err);
      }
    }
  },

  on(fn) {
    listeners.add(fn);
    return () => listeners.delete(fn);
  },

  profile(id) {
    return this.state?.profiles.find(p => p.id === id);
  },

  setStatus(status) {
    const p = this.profile(status.profileId);
    if (p) {
      p.status = status;
      this.emit({ type: "status", id: p.id });
    }
  },
};

let refreshing = null;

export function refresh() {
  if (!refreshing) {
    refreshing = api("GET", "/api/state")
      .then(state => store.set(state))
      .finally(() => {
        refreshing = null;
      });
  }
  return refreshing;
}

export function activity(status) {
  const pct = status.progress ? ` ${Math.round(status.progress * 100)}%` : "";
  switch (status.state) {
    case "unlocking":
      return {
        key: "Deriving the key from your password",
        decrypt: `Decrypting the vault${pct}`,
        launch: "Starting the browser",
      }[status.phase] || "Unlocking";
    case "sealing":
      return {
        encrypt: `Encrypting the vault${pct}`,
        verify: `Checking the new vault${pct}`,
        wipe: "Wiping the decrypted copy",
        decrypt: `Re-encrypting${pct}`,
      }[status.phase] || "Encrypting";
    case "closing":
      return "Closing the browser";
  }
  return "";
}

export const isBusy = state => state === "unlocking" || state === "sealing" || state === "closing";
