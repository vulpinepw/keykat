import { api } from "./api.js";
import { store } from "./store.js";
import { avatar, h, icon, modal, mount, toast } from "./dom.js";
import { openUnlock } from "./unlock.js";

let current = null;

export function askForLinks() {
  const link = store.state?.links?.[0];
  if (!link || current) {
    return;
  }
  current = link.id;
  const done = () => {
    current = null;
    setTimeout(askForLinks, 0);
  };
  if (link.profile && store.profile(link.profile)) {
    openUnlock(link.profile, "open", { link: link.id }).then(ok => {
      forget(link);
      if (!ok) {
        dismiss(link);
      }
      done();
    });
    return;
  }
  choose(link, done);
}

function forget(link) {
  store.state.links = (store.state.links || []).filter(l => l.id !== link.id);
}

function dismiss(link) {
  forget(link);
  return api("POST", `/api/links/${link.id}/dismiss`, {}).catch(() => {});
}

function address(url) {
  try {
    const u = new URL(url);
    if (u.protocol === "file:") {
      return h("span", { class: "link-url" }, h("strong", { text: decodeURIComponent(u.pathname.split("/").pop()) }), " on this computer");
    }
    return h("span", { class: "link-url" }, `${u.protocol}//`, h("strong", { text: u.host }), `${u.pathname}${u.search}`);
  } catch {
    return h("span", { class: "link-url", text: url });
  }
}

function choose(link, done) {
  let picked = false;
  const dlg = modal({
    label: "Open a link",
    onClose: () => {
      if (!picked) {
        dismiss(link);
        done();
      }
    },
  });
  const remember = h("input", { type: "checkbox", id: "kc-link-remember" });
  const pick = async p => {
    picked = true;
    dlg.close();
    if (remember.checked) {
      api("PUT", "/api/settings", { ...store.state.settings, openLinks: p.id })
        .then(settings => {
          store.state.settings = settings;
          store.emit({ type: "settings" });
        })
        .catch(err => toast("bad", "Could not save the setting", err.message));
    }
    if (p.status?.state === "running" || p.security.mode !== "encrypted") {
      try {
        await api("POST", `/api/links/${link.id}/open`, { profile: p.id });
        forget(link);
      } catch (err) {
        toast("bad", `The link could not open in ${p.name}`, err.message);
      }
    } else if (await openUnlock(p.id, "open", { link: link.id })) {
      forget(link);
    }
    done();
  };
  const status = p => ({ running: "Open", locked: p.security.mode === "encrypted" ? "Locked" : "Closed" })[p.status?.state || "locked"] || "Busy";
  const profiles = [...store.state.profiles].sort((a, b) => (b.status?.state === "running") - (a.status?.state === "running"));
  mount(dlg.panel,
    h("div", { class: "modal-head" },
      h("div", {}, h("h2", { text: "Open this link in" }), h("p", {}, address(link.url)))),
    h("div", { class: "modal-body" },
      h("div", { class: "link-profiles" }, ...profiles.map(p => {
        const busy = !["running", "locked"].includes(p.status?.state || "locked");
        return h("button", { class: "link-profile", type: "button", disabled: busy, onclick: () => pick(p) },
          avatar(p, "sm"), h("span", { class: "name", text: p.name }), h("span", { class: "state", text: status(p) }));
      })),
      profiles.length ? null : h("div", { class: "callout" }, icon("info"), h("span", { text: "Make a profile first, then open the link again." })),
      h("label", { class: "check-row", for: "kc-link-remember" }, remember, "Always open links in the profile I pick")),
    h("div", { class: "modal-foot" },
      h("button", {
        class: "btn", type: "button",
        onclick: () => navigator.clipboard.writeText(link.url).then(() => toast("info", "Link copied"), () => {}),
      }, "Copy link"),
      h("span", { class: "spacer" }),
      h("button", { class: "btn", type: "button", text: "Don't open", onclick: () => dlg.close() }))
  );
}
