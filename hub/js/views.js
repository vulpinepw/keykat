import { h, icon } from "./dom.js";

export const CIPHER_SHORT = {
  "aes-256-gcm": "AES-256-GCM",
  "xchacha20-poly1305": "XChaCha20-Poly1305",
  "chacha20-poly1305": "ChaCha20-Poly1305",
  "aes-256-gcm+xchacha20-poly1305": "AES-256 + XChaCha20 cascade",
};

export function securityLine(p) {
  const s = p.security;
  if (s.mode === "encrypted") {
    const kdf = (s.kdfName || "").split(" · ")[0];
    return h("div", { class: "profile-sec enc" }, icon("lock"),
      h("span", { text: [CIPHER_SHORT[s.cipher] || s.cipherName, kdf].filter(Boolean).join(" · ") }));
  }
  if (s.mode === "ephemeral") {
    return h("div", { class: "profile-sec eph" }, icon("clock"), h("span", { text: "Ephemeral · erased when closed" }));
  }
  return h("div", { class: "profile-sec" }, icon("unlock"), h("span", { text: "Not encrypted" }));
}

export function section(title, ...rows) {
  return h("section", { class: "section" },
    h("h3", { class: "section-title", text: title }),
    h("div", { class: "panel" }, ...rows)
  );
}

export function choiceCards(options, value, onChange, label) {
  const group = h("div", { class: "choices", role: "radiogroup", "aria-label": label });
  const render = current => {
    for (const card of group.children) {
      card.setAttribute("aria-checked", String(card.dataset.value === current));
    }
  };
  for (const o of options) {
    group.append(h("button", {
      type: "button",
      class: "choice",
      role: "radio",
      dataset: { value: o.value },
      disabled: o.disabled,
      onclick: () => {
        render(o.value);
        onChange(o.value);
      },
    },
      o.tag ? h("span", { class: "tag", text: o.tag }) : null,
      o.icon ? h("span", { class: `choice-icon ${o.iconCls || ""}` }, icon(o.icon)) : null,
      h("span", { class: "choice-title", text: o.title }),
      o.desc ? h("span", { class: "choice-desc", text: o.desc }) : null
    ));
  }
  render(value);
  group.setValue = render;
  return group;
}
