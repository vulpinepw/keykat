const SVG_NS = "http://www.w3.org/2000/svg";

const ICONS = {
  lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  unlock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.6-1.8"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  more: '<circle cx="5" cy="12" r="1.4" fill="currentColor"/><circle cx="12" cy="12" r="1.4" fill="currentColor"/><circle cx="19" cy="12" r="1.4" fill="currentColor"/>',
  settings: '<path d="M4 6h9M17 6h3M4 12h3M11 12h9M4 18h11M19 18h1"/><circle cx="15" cy="6" r="2"/><circle cx="9" cy="12" r="2"/><circle cx="17" cy="18" r="2"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M4.6 4.6l1.4 1.4M18 18l1.4 1.4M2.5 12h2M19.5 12h2M4.6 19.4 6 18M18 6l1.4-1.4"/>',
  moon: '<path d="M20 14.6A8.2 8.2 0 1 1 9.4 4a6.6 6.6 0 0 0 10.6 10.6Z"/>',
  monitor: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>',
  eye: '<path d="M2.5 12S6 5 12 5s9.5 7 9.5 7S18 19 12 19 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="3"/>',
  eyeOff: '<path d="M3 3l18 18M10.6 5.1C11 5 11.5 5 12 5c6 0 9.5 7 9.5 7a17 17 0 0 1-3 4M6.6 6.6C4 8.3 2.5 12 2.5 12S6 19 12 19c1.9 0 3.6-.6 5-1.5M9.9 9.9a3 3 0 0 0 4.2 4.2"/>',
  x: '<path d="M6 6l12 12M18 6 6 18"/>',
  check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
  alert: '<path d="M12 3.5 2.8 19.5h18.4L12 3.5Z"/><path d="M12 10v4.5M12 17.2v.01"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3.2 2"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>',
  edit: '<path d="M4 20h4L19.5 8.5a2.1 2.1 0 0 0-4-4L4 16v4Z"/><path d="M14 6l4 4"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/>',
  refresh: '<path d="M20 11a8 8 0 0 0-14.4-4.6L4 8M4 4v4h4M4 13a8 8 0 0 0 14.4 4.6L20 16M20 20v-4h-4"/>',
  download: '<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V6a2 2 0 0 1 2-2h8"/>',
  shield: '<path d="M12 3 4.5 6v6c0 4.8 3.2 8 7.5 9 4.3-1 7.5-4.2 7.5-9V6L12 3Z"/>',
  shieldCheck: '<path d="M12 3 4.5 6v6c0 4.8 3.2 8 7.5 9 4.3-1 7.5-4.2 7.5-9V6L12 3Z"/><path d="M8.7 12.2l2.3 2.3 4.3-4.6"/>',
  key: '<circle cx="8" cy="15" r="4"/><path d="M10.9 12.1 19 4M15.5 7.5l2.5 2.5M13.5 9.5l2 2"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.4 2.5 3.7 5.5 3.7 9s-1.3 6.5-3.7 9c-2.4-2.5-3.7-5.5-3.7-9S9.6 5.5 12 3Z"/>',
  cookie: '<path d="M21 12.4A9 9 0 1 1 11.6 3a3 3 0 0 0 3.4 3.6 3 3 0 0 0 3.4 3.4 3 3 0 0 0 2.6 2.4Z"/><path d="M8.5 9.5v.01M12 13v.01M8.5 15.5v.01M15.5 16v.01"/>',
  database: '<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
  history: '<path d="M3.5 12a8.5 8.5 0 1 0 2.6-6.1L3.5 8.5M3.5 4v4.5H8"/><path d="M12 7.5V12l3 2"/>',
  fingerprint: '<path d="M6.5 18.5c1-2 1.5-4 1.5-6.5a4 4 0 0 1 8 0c0 1.3-.1 2.5-.3 3.7M12 12c0 3.5-.8 6.5-2.4 9M16.4 20.2c.6-1.4 1-3 1.3-4.6M3.6 15c.3-1 .4-2 .4-3a8 8 0 0 1 14.6-4.5M20 12c0 .9 0 1.8-.1 2.6"/>',
  network: '<rect x="9" y="3" width="6" height="5" rx="1"/><rect x="3" y="16" width="6" height="5" rx="1"/><rect x="15" y="16" width="6" height="5" rx="1"/><path d="M12 8v4M6 16v-2h12v2"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8v.01"/>',
  chevronRight: '<path d="M9 6l6 6-6 6"/>',
  chevronLeft: '<path d="M15 6l-6 6 6 6"/>',
  power: '<path d="M12 3v8M6.3 6.3a8 8 0 1 0 11.4 0"/>',
  windows: '<path d="M4 5.6 10.6 4.7V11H4V5.6ZM12.4 4.4 20 3.3V11h-7.6V4.4ZM4 13h6.6v6.3L4 18.4V13ZM12.4 13H20v7.7l-7.6-1.1V13Z"/>',
  cpu: '<rect x="6" y="6" width="12" height="12" rx="2"/><rect x="9.5" y="9.5" width="5" height="5" rx="1"/><path d="M9 2.5V6M15 2.5V6M9 18v3.5M15 18v3.5M2.5 9H6M2.5 15H6M18 9h3.5M18 15h3.5"/>',
  external: '<path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
  focus: '<path d="M4 9V5a1 1 0 0 1 1-1h4M15 4h4a1 1 0 0 1 1 1v4M20 15v4a1 1 0 0 1-1 1h-4M9 20H5a1 1 0 0 1-1-1v-4"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4.5 20.5a7.5 7.5 0 0 1 15 0"/>',
  ghost: '<path d="M5 20V10a7 7 0 0 1 14 0v10l-2.4-1.6L14.3 20 12 18.4 9.7 20l-2.3-1.6L5 20Z"/><path d="M9.5 10.5v.01M14.5 10.5v.01"/>',
  bolt: '<path d="M13 3 5 13.5h6L10 21l8-10.5h-6L13 3Z"/>',
  dice: '<rect x="4" y="4" width="16" height="16" rx="3"/><circle cx="9" cy="9" r="1.1" fill="currentColor"/><circle cx="15" cy="9" r="1.1" fill="currentColor"/><circle cx="9" cy="15" r="1.1" fill="currentColor"/><circle cx="15" cy="15" r="1.1" fill="currentColor"/>',
  layers: '<path d="M12 3 3 8l9 5 9-5-9-5Z"/><path d="M3 13l9 5 9-5M3 18l9 5 9-5"/>',
  wipe: '<path d="M8 20h12M5.5 14.5l7.8-7.8a2 2 0 0 1 2.8 0l1.2 1.2a2 2 0 0 1 0 2.8l-7.8 7.8a2 2 0 0 1-2.8 0l-1.2-1.2a2 2 0 0 1 0-2.8ZM10 10l4 4"/>',
};

export function icon(name, cls = "") {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("class", `icon ${cls}`.trim());
  svg.innerHTML = ICONS[name] || ICONS.info;
  return svg;
}

export function h(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) {
      continue;
    }
    if (key === "class") {
      node.className = value;
    } else if (key === "text") {
      node.textContent = value;
    } else if (key === "vars") {
      for (const [k, v] of Object.entries(value)) {
        node.style.setProperty(k, v);
      }
    } else if (key === "dataset") {
      Object.assign(node.dataset, value);
    } else if (key.startsWith("on") && typeof value === "function") {
      node.addEventListener(key.slice(2), value);
    } else if (key === "value" || key === "checked" || key === "disabled" || key === "selected") {
      node[key] = value;
    } else {
      node.setAttribute(key, value === true ? "" : value);
    }
  }
  append(node, children);
  return node;
}

function append(node, children) {
  for (const child of children.flat(Infinity)) {
    if (child === undefined || child === null || child === false) {
      continue;
    }
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
}

export function mount(node, ...children) {
  node.replaceChildren();
  append(node, children);
}


let drawings = {};

export function setDrawings(list) {
  drawings = Object.fromEntries((list || []).map(d => [d.id, d.svg]));
}

const validColor = c => /^#[0-9a-f]{6}$/i.test(c || "");

export function inkFor(hex) {
  const n = parseInt(hex.slice(1), 16);
  const lin = c => {
    c /= 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  const l = 0.2126 * lin((n >> 16) & 255) + 0.7152 * lin((n >> 8) & 255) + 0.0722 * lin(n & 255);
  return l > 0.36 ? "#15120a" : "#ffffff";
}

export function initials(name) {
  const out = [];
  for (const word of (name || "").trim().split(/\s+/)) {
    const m = /[\p{L}\p{N}]/u.exec(word);
    if (m) {
      out.push(m[0].toUpperCase());
    }
    if (out.length === 2) {
      break;
    }
  }
  return out.join("") || "?";
}

export function avatar(profile, size = "") {
  const color = validColor(profile.color) ? profile.color : "#F5A524";
  const ink = inkFor(color);
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("viewBox", "0 0 48 48");
  svg.setAttribute("class", `avatar ${size}`.trim());
  svg.setAttribute("aria-hidden", "true");
  svg.style.setProperty("--pc", color);
  const drawing = drawings[profile.avatar];
  svg.innerHTML =
    `<circle cx="24" cy="24" r="24" fill="${color}"/>` +
    (drawing ? drawing.replaceAll("{ink}", ink).replaceAll("{bg}", color) : "");
  if (!drawing) {
    const letters = initials(profile.name);
    const text = document.createElementNS(SVG_NS, "text");
    const attrs = {
      x: "24", y: "25", "text-anchor": "middle", "dominant-baseline": "middle",
      "font-family": "Segoe UI Variable Display, Segoe UI, system-ui, sans-serif",
      "font-weight": "700", "font-size": letters.length > 1 ? "17" : "20", fill: ink,
    };
    for (const [k, v] of Object.entries(attrs)) {
      text.setAttribute(k, v);
    }
    text.textContent = letters;
    svg.append(text);
  }
  return svg;
}


export function toast(kind, title, text = "") {
  const host = document.getElementById("toasts");
  const iconName = { good: "check", bad: "alert", info: "info" }[kind] || "info";
  const node = h("div", { class: `toast ${kind}` },
    icon(iconName),
    h("div", { class: "text" }, h("strong", { text: title }), text ? h("span", { text }) : null),
    h("button", { class: "btn btn-ghost btn-icon btn-sm", "aria-label": "Dismiss", onclick: () => node.remove() }, icon("x"))
  );
  host.append(node);
  setTimeout(() => node.remove(), kind === "bad" ? 9000 : 5000);
}


const FOCUSABLE = 'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea, [tabindex]:not([tabindex="-1"])';

function overlay({ cls, panelCls, label, onClose }) {
  const panel = h("div", { class: panelCls, role: "dialog", "aria-modal": "true", "aria-label": label });
  const backdrop = h("div", { class: cls }, panel);
  const previous = document.activeElement;
  let closed = false;

  const onKey = event => {
    if (event.key === "Escape" && !backdrop.dataset.locked) {
      event.preventDefault();
      close();
    } else if (event.key === "Tab") {
      const items = [...panel.querySelectorAll(FOCUSABLE)];
      if (!items.length) {
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
  };
  backdrop.addEventListener("mousedown", event => {
    if (event.target === backdrop && !backdrop.dataset.locked) {
      close();
    }
  });
  function close() {
    if (closed) {
      return;
    }
    closed = true;
    document.removeEventListener("keydown", onKey, true);
    backdrop.remove();
    previous?.focus?.();
    onClose?.();
  }
  document.addEventListener("keydown", onKey, true);
  document.body.append(backdrop);
  const setBusy = busy => {
    if (busy) {
      backdrop.dataset.locked = "1";
    } else {
      delete backdrop.dataset.locked;
    }
  };
  return { panel, close, setBusy, isOpen: () => !closed };
}

export function modal({ wide = false, label = "", onClose } = {}) {
  return overlay({ cls: "backdrop", panelCls: `modal ${wide ? "wide" : ""}`.trim(), label, onClose });
}

export function drawer({ label = "", onClose } = {}) {
  return overlay({ cls: "backdrop drawer-backdrop", panelCls: "drawer", label, onClose });
}

export function inlinePanel(host, label = "") {
  const panel = h("div", { class: "drawer inline", role: "region", "aria-label": label });
  mount(host, panel);
  return { panel, close: () => {}, setBusy: () => {}, isOpen: () => panel.isConnected, inline: true };
}

export function focusFirst(container) {
  requestAnimationFrame(() => {
    const target = container.querySelector("[autofocus]") || container.querySelector(FOCUSABLE);
    if (target?.matches("[autofocus], textarea, input:not([type=checkbox]):not([type=radio]):not([type=range]):not([type=color])")) {
      target.focus();
      return;
    }
    if (!container.hasAttribute("tabindex")) {
      container.setAttribute("tabindex", "-1");
    }
    container.focus({ preventScroll: true });
  });
}

export function confirmDialog({ title, text, confirmLabel = "Confirm", danger = false, typeToConfirm = "" }) {
  return new Promise(resolve => {
    let result = false;
    const dlg = modal({ label: title, onClose: () => resolve(result) });
    const input = typeToConfirm
      ? h("input", { class: "input", "aria-label": `Type ${typeToConfirm} to confirm`, placeholder: typeToConfirm, autocomplete: "off", spellcheck: "false" })
      : null;
    const ok = h("button", {
      class: `btn ${danger ? "btn-danger" : "btn-primary"}`,
      text: confirmLabel,
      disabled: !!typeToConfirm,
      onclick: () => {
        result = typeToConfirm ? input.value : true;
        dlg.close();
      },
    });
    input?.addEventListener("input", () => {
      ok.disabled = input.value !== typeToConfirm;
    });
    input?.addEventListener("keydown", event => {
      if (event.key === "Enter" && !ok.disabled) {
        ok.click();
      }
    });
    mount(dlg.panel,
      h("div", { class: "modal-head" },
        h("div", {}, h("h2", { text: title }), text ? h("p", { text }) : null)
      ),
      input ? h("div", { class: "modal-body" }, h("div", { class: "field" },
        h("label", { class: "label", text: `Type ${typeToConfirm} to confirm` }), input)) : h("div", { class: "modal-body" }),
      h("div", { class: "modal-foot" },
        h("span", { class: "spacer" }),
        h("button", { class: "btn btn-ghost", text: "Cancel", onclick: () => dlg.close() }),
        ok
      )
    );
    focusFirst(dlg.panel);
  });
}


let openMenuState = null;

export function closeMenu() {
  openMenuState?.close();
}

export function openMenu(anchor, items) {
  closeMenu();
  const menu = h("div", { class: "menu", role: "menu" });
  for (const item of items) {
    if (item === "-") {
      menu.append(h("hr"));
      continue;
    }
    menu.append(h("button", {
      role: "menuitem",
      class: item.danger ? "danger" : "",
      onclick: () => {
        close();
        item.onclick();
      },
    }, icon(item.icon || "chevronRight"), item.label));
  }
  document.body.append(menu);
  const rect = anchor.getBoundingClientRect();
  const width = menu.offsetWidth;
  const height = menu.offsetHeight;
  let left = Math.min(rect.right - width, window.innerWidth - width - 8);
  let top = rect.bottom + 6;
  if (top + height > window.innerHeight - 8) {
    top = rect.top - height - 6;
  }
  menu.style.left = `${Math.max(8, left)}px`;
  menu.style.top = `${Math.max(8, top)}px`;
  menu.querySelector("button")?.focus();

  const onDown = event => {
    if (!menu.contains(event.target) && event.target !== anchor && !anchor.contains(event.target)) {
      close();
    }
  };
  const onKey = event => {
    const buttons = [...menu.querySelectorAll("button")];
    const index = buttons.indexOf(document.activeElement);
    if (event.key === "Escape") {
      close();
      anchor.focus();
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      buttons[(index + 1) % buttons.length].focus();
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      buttons[(index - 1 + buttons.length) % buttons.length].focus();
    }
  };
  function close() {
    menu.remove();
    document.removeEventListener("mousedown", onDown, true);
    document.removeEventListener("keydown", onKey, true);
    openMenuState = null;
  }
  document.addEventListener("mousedown", onDown, true);
  document.addEventListener("keydown", onKey, true);
  openMenuState = { close };
}


export function fmtBytes(n) {
  if (!n) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(n >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}

export function fmtAgo(iso) {
  if (!iso) {
    return "never";
  }
  const seconds = (Date.now() - new Date(iso).getTime()) / 1000;
  if (seconds < 60) {
    return "just now";
  }
  const steps = [
    [60, "minute"],
    [24, "hour"],
    [7, "day"],
    [4.35, "week"],
    [12, "month"],
    [Infinity, "year"],
  ];
  let value = seconds / 60;
  for (const [size, unit] of steps) {
    if (value < size) {
      const n = Math.floor(value);
      return `${n} ${unit}${n === 1 ? "" : "s"} ago`;
    }
    value /= size;
  }
  return "long ago";
}

export function segmented(options, value, onChange, label = "") {
  const group = h("div", { class: "seg", role: "radiogroup", "aria-label": label });
  const render = current => {
    for (const button of group.children) {
      button.setAttribute("aria-checked", String(button.dataset.value === current));
    }
  };
  for (const [v, text] of options) {
    group.append(h("button", {
      type: "button",
      role: "radio",
      dataset: { value: v },
      text,
      onclick: () => {
        render(v);
        onChange(v);
      },
    }));
  }
  render(value);
  return group;
}

export function switchRow({ title, hint, checked, disabled, onchange }) {
  const input = h("input", { type: "checkbox", class: "switch", checked, disabled, "aria-label": title });
  input.addEventListener("change", () => onchange(input.checked));
  return h("label", { class: "toggle-row" },
    h("span", { class: "text" }, h("div", { class: "title", text: title }), hint ? h("div", { class: "hint", text: hint }) : null),
    input
  );
}

export function selectRow({ title, hint, value, options, onchange }) {
  const select = h("select", { class: "select", "aria-label": title },
    options.map(([v, text]) => h("option", { value: v, text, selected: v === value }))
  );
  select.addEventListener("change", () => onchange(select.value));
  return h("div", { class: "toggle-row" },
    h("span", { class: "text" }, h("div", { class: "title", text: title }), hint ? h("div", { class: "hint", text: hint }) : null),
    select
  );
}

export function passwordStrength(pw) {
  if (!pw) {
    return { level: 0, label: "", bits: 0 };
  }
  let pool = 0;
  if (/[a-z]/.test(pw)) pool += 26;
  if (/[A-Z]/.test(pw)) pool += 26;
  if (/[0-9]/.test(pw)) pool += 10;
  if (/[^A-Za-z0-9]/.test(pw)) pool += 33;
  let bits = pw.length * Math.log2(Math.max(pool, 2));
  if (/(.)\1{2,}/.test(pw)) bits *= 0.75;
  if (/(0123|1234|2345|3456|4567|5678|6789|abcd|bcde|qwer|asdf|zxcv)/i.test(pw)) bits *= 0.8;
  if (/^(password|passw0rd|qwerty|letmein|admin|welcome|iloveyou|monkey|dragon|keycat|123456)/i.test(pw)) {
    bits = Math.min(bits, 18);
  }
  const words = pw.trim().split(/[\s\-_.,]+/).filter(Boolean);
  if (words.length >= 2 && words.every(w => /^\p{L}+$/u.test(w))) {
    bits = Math.min(bits, words.length * 13);
  }
  const level = bits < 30 ? 1 : bits < 48 ? 2 : bits < 70 ? 3 : 4;
  const label = ["", "Weak", "Fair", "Good", "Strong"][level];
  return { level, label, bits: Math.round(bits) };
}
