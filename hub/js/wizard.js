import { api } from "./api.js";
import { refresh, store } from "./store.js";
import { avatar, focusFirst, h, icon, modal, mount, passwordStrength, switchRow, toast } from "./dom.js";
import { choiceCards } from "./views.js";
import { privacyEditor } from "./privacy.js";

const STEPS = ["Identity", "Security", "Privacy", "Appearance"];

export function identityEditor(state, draft, onChange) {
  const presets = state.presets;
  const preview = h("div", { class: "identity-preview" });
  const nameInput = h("input", {
    class: "input", id: "kc-name", maxlength: "40", value: draft.name,
    placeholder: "Work, Banking, Shopping", autocomplete: "off", spellcheck: "false",
  });
  const grid = h("div", { class: "drawing-grid", role: "radiogroup", "aria-label": "Profile picture" });
  const swatches = h("div", { class: "swatches", role: "radiogroup", "aria-label": "Colour" });
  const custom = h("input", { type: "color", value: draft.color, "aria-label": "Custom colour" });
  const customSwatch = h("label", { class: "swatch-custom", title: "Custom colour" }, icon("plus"), custom);

  function renderPreview() {
    mount(preview, avatar(draft, "lg"),
      h("div", {},
        h("div", { class: "profile-name", text: draft.name || "New profile" }),
        h("div", { class: "hint", text: "Shown on the profile card and in the browser toolbar." })
      )
    );
  }

  function renderGrid() {
    const tiles = [...presets.avatars.map(a => [a.id, a.name]), ["initials", "Initials"]];
    mount(grid, ...tiles.map(([id, label]) =>
      h("button", {
        type: "button", role: "radio", title: label, "aria-label": label,
        "aria-checked": String(draft.avatar === id),
        onclick: () => {
          draft.avatar = id;
          onChange();
          renderGrid();
          renderPreview();
        },
      }, avatar({ ...draft, avatar: id }, "tile"))
    ));
  }

  function renderSwatches() {
    const isCustom = !presets.palette.some(c => c.toLowerCase() === draft.color.toLowerCase());
    mount(swatches,
      ...presets.palette.map(color => h("button", {
        type: "button", class: "swatch", role: "radio", vars: { "--sw": color },
        "aria-label": color, "aria-checked": String(color.toLowerCase() === draft.color.toLowerCase()),
        onclick: () => setColor(color),
      })),
      customSwatch
    );
    customSwatch.setAttribute("aria-checked", String(isCustom));
    customSwatch.style.setProperty("--sw", draft.color);
  }

  function setColor(color) {
    draft.color = color;
    custom.value = color;
    onChange();
    renderSwatches();
    renderGrid();
    renderPreview();
  }

  nameInput.addEventListener("input", () => {
    draft.name = nameInput.value;
    onChange();
    renderPreview();
    if (draft.avatar === "initials") {
      renderGrid();
    }
  });
  custom.addEventListener("input", () => setColor(custom.value));

  renderPreview();
  renderGrid();
  renderSwatches();
  return h("div", {},
    preview,
    h("div", { class: "field" }, h("label", { class: "label", for: "kc-name", text: "Name" }), nameInput),
    h("div", { class: "field" }, h("span", { class: "label", text: "Picture" }), grid),
    h("div", { class: "field" }, h("span", { class: "label", text: "Colour" }), swatches)
  );
}

export function passwordFields({ label = "Password", onInput }) {
  const pw = h("input", { class: "input", type: "password", id: "kc-pw", autocomplete: "new-password", spellcheck: "false" });
  const confirm = h("input", { class: "input", type: "password", id: "kc-pw2", autocomplete: "new-password", spellcheck: "false" });
  const meter = h("div", { class: "strength", "aria-hidden": "true" }, h("span"), h("span"), h("span"), h("span"));
  const meterLabel = h("div", { class: "strength-label" });
  const reveal = h("button", { class: "btn btn-icon", type: "button", "aria-label": "Show password" }, icon("eye"));
  reveal.addEventListener("click", () => {
    const show = pw.type === "password";
    pw.type = confirm.type = show ? "text" : "password";
    reveal.replaceChildren(icon(show ? "eyeOff" : "eye"));
    reveal.setAttribute("aria-label", show ? "Hide password" : "Show password");
  });
  const update = () => {
    const s = passwordStrength(pw.value);
    meter.dataset.level = String(s.level);
    mount(meterLabel,
      h("span", { text: pw.value ? `${s.label} · about ${s.bits} bits` : "Use a long passphrase: four or more random words work well." }),
      pw.value.length > 0 && pw.value.length < 8 ? h("span", { class: "error-text", text: "At least 8 characters" }) : null
    );
    onInput?.();
  };
  pw.addEventListener("input", update);
  confirm.addEventListener("input", () => onInput?.());
  update();
  const node = h("div", {},
    h("div", { class: "field" }, h("label", { class: "label", for: "kc-pw", text: label }),
      h("div", { class: "input-wrap" }, pw, reveal), meter, meterLabel),
    h("div", { class: "field" }, h("label", { class: "label", for: "kc-pw2", text: "Confirm password" }), confirm)
  );
  return {
    node,
    get value() {
      return pw.value;
    },
    get confirmed() {
      return pw.value === confirm.value;
    },
    focus: () => pw.focus(),
  };
}

export function openWizard() {
  const state = store.state;
  const presets = state.presets;
  const draft = {
    name: "",
    color: presets.palette[store.state.profiles.length % presets.palette.length],
    avatar: "cat",
    security: {
      mode: "encrypted", preset: "standard", cipher: "xchacha20-poly1305", kdf: "argon2id-strong",
      password: "", recovery: true, windowsUnlock: false, keepBackup: true, keepCache: false,
    },
    privacy: structuredClone(presets.privacy.balanced),
    appearance: { theme: ["dark", "light"].includes(state.settings.theme) ? state.settings.theme : "system" },
    open: true,
    import: false,
  };
  let step = 0;
  let password = null;
  let creating = false;

  const dlg = modal({ wide: true, label: "New profile" });
  const stepsEl = h("div", { class: "steps" });
  const body = h("div", { class: "modal-body wizard-body" });
  const error = h("div", { class: "error-text", role: "alert" });
  const back = h("button", { class: "btn btn-ghost", type: "button" }, icon("chevronLeft"), "Back");
  const next = h("button", { class: "btn btn-primary", type: "button" });
  back.addEventListener("click", () => go(step - 1));
  next.addEventListener("click", () => (step === STEPS.length - 1 ? create() : go(step + 1)));

  mount(dlg.panel,
    h("div", { class: "modal-head" },
      h("div", {}, h("h2", { text: "New profile" }), h("p", { text: "A separate browser with its own encryption and privacy settings." })),
      h("button", { class: "btn btn-ghost btn-icon close", "aria-label": "Close", onclick: () => dlg.close() }, icon("x"))
    ),
    stepsEl, body,
    h("div", { class: "modal-foot" }, error, h("span", { class: "spacer" }), back, next)
  );

  function validate(index) {
    if (index === 0 && !draft.name.trim()) {
      return "Give the profile a name.";
    }
    if (index === 1 && draft.security.mode === "encrypted") {
      const pw = password?.value || "";
      if (!pw && !draft.security.windowsUnlock) {
        return state.app.windowsUnlock ? "Choose a password, or turn on Windows account unlock." : "Choose a password.";
      }
      if (pw && pw.length < 8) {
        return "The password must be at least 8 characters.";
      }
      if (pw && !password.confirmed) {
        return "The passwords do not match.";
      }
    }
    return "";
  }

  function go(target) {
    if (target > step) {
      const problem = validate(step);
      if (problem) {
        error.textContent = problem;
        return;
      }
    }
    if (step === 1 && password) {
      draft.security.password = password.value;
    }
    step = Math.max(0, Math.min(STEPS.length - 1, target));
    render();
  }

  function render() {
    error.textContent = "";
    mount(stepsEl, ...STEPS.map((name, i) =>
      h("div", { class: `step ${i < step ? "done" : ""} ${i === step ? "current" : ""}`, text: name, "aria-current": i === step ? "step" : undefined })));
    back.hidden = step === 0;
    mount(next, ...(step === STEPS.length - 1 ? [icon("check"), "Create profile"] : ["Continue", icon("chevronRight")]));
    mount(body, [identityStep, securityStep, privacyStep, appearanceStep][step]());
    focusFirst(body);
  }

  const identityStep = () => identityEditor(state, draft, () => (error.textContent = ""));

  function securityStep() {
    const sec = draft.security;
    const details = h("div", { class: "stack" });
    const renderDetails = () => {
      if (sec.mode !== "encrypted") {
        mount(details, h("div", { class: "callout" }, icon("info"), h("span", {
          text: sec.mode === "ephemeral"
            ? "Every time this profile opens it starts empty, and everything is wiped when it closes. Nothing is ever saved."
            : "The profile is stored unencrypted. Keycat's privacy settings still apply, but anyone with access to this computer can read its data.",
        })));
        return;
      }
      const custom = h("div", { class: "custom-crypto", hidden: sec.preset !== "custom" },
        h("div", { class: "field" }, h("label", { class: "label", for: "kc-cipher", text: "Cipher" }),
          select("kc-cipher", presets.ciphers.map(c => [c.id, c.name]), sec.cipher, v => (sec.cipher = v))),
        h("div", { class: "field" }, h("label", { class: "label", for: "kc-kdf", text: "Key derivation" }),
          select("kc-kdf", presets.kdfs.map(k => [k.id, k.name]), sec.kdf, v => (sec.kdf = v)))
      );
      password = passwordFields({ onInput: () => (error.textContent = "") });
      mount(details,
        h("div", { class: "field" }, h("span", { class: "label", text: "Encryption strength" }),
          choiceCards([
            ...presets.security.map(s => ({ value: s.id, title: s.name, desc: s.description, tag: s.id === "standard" ? "Fastest" : s.id === "paranoid" ? "Slowest" : "" })),
            { value: "custom", title: "Custom", desc: "Pick the cipher and key derivation yourself." },
          ], sec.preset, v => {
            sec.preset = v;
            custom.hidden = v !== "custom";
          }, "Encryption strength")),
        custom,
        password.node,
        h("div", { class: "panel" },
          switchRow({ title: "Create a recovery code", hint: "A one-time code that unlocks the profile if you forget the password", checked: sec.recovery, onchange: on => (sec.recovery = on) }),
          state.app.windowsUnlock && switchRow({ title: "Unlock with my Windows account", hint: "Convenient: anyone signed in to your Windows account can open this profile", checked: sec.windowsUnlock, onchange: on => (sec.windowsUnlock = on) }),
          switchRow({ title: "Keep a backup of the previous vault", hint: "One older copy is kept in case the newest one is damaged", checked: sec.keepBackup, onchange: on => (sec.keepBackup = on) })
        )
      );
      if (sec.password) {
        details.querySelector("#kc-pw").value = sec.password;
        details.querySelector("#kc-pw2").value = sec.password;
        details.querySelector("#kc-pw").dispatchEvent(new Event("input"));
      }
    };
    renderDetails();
    return h("div", {},
      h("div", { class: "field" }, h("span", { class: "label", text: "Storage" }),
        choiceCards([
          { value: "encrypted", title: "Encrypted", icon: "lock", tag: "Recommended", desc: "Sealed in an encrypted vault whenever it is closed." },
          { value: "ephemeral", title: "Ephemeral", icon: "clock", iconCls: "info", desc: "Starts empty and is erased every time it closes." },
          { value: "plain", title: "Not encrypted", icon: "unlock", iconCls: "muted", desc: "A regular profile with Keycat's privacy settings." },
        ], sec.mode, v => {
          sec.mode = v;
          if (v !== "encrypted" && draft.privacy.passwords === "keycat") {
            draft.privacy.passwords = v === "ephemeral" ? "off" : "firefox";
          } else if (v === "encrypted" && draft.privacy.preset !== "custom") {
            draft.privacy.passwords = "keycat";
          }
          renderDetails();
        }, "Storage")),
      details
    );
  }

  function privacyStep() {
    return h("div", {},
      h("p", { class: "hint step-intro", text: "Pick a level, then fine-tune anything below. You can change all of this later." }),
      privacyEditor({ value: draft.privacy, presets, mode: draft.security.mode, onChange: v => (draft.privacy = v) })
    );
  }

  function appearanceStep() {
    const group = themePicker(draft.appearance.theme, id => (draft.appearance.theme = id));
    return h("div", {},
      h("div", { class: "field" }, h("span", { class: "label", text: "Browser theme" }), group,
        h("span", { class: "hint", text: "The browser's built-in themes. Others can be installed from the add-ons manager." })),
      h("div", { class: "panel" },
        switchRow({ title: "Open the profile when it is created", checked: draft.open, onchange: on => {
          draft.open = on;
          importRow.querySelector("input").disabled = !on;
        } }),
        importRow
      )
    );
  }

  const importRow = switchRow({
    title: "Import from another browser when it opens",
    hint: "Bookmarks (favorites), history, form data and passwords from Edge, Chrome, Brave, Opera, Vivaldi or another browser on this PC",
    checked: draft.import,
    onchange: on => (draft.import = on),
  });

  async function create() {
    if (creating) {
      return;
    }
    creating = true;
    dlg.setBusy(true);
    next.disabled = back.disabled = true;
    mount(next, "Creating");
    const sec = draft.security;
    const request = {
      name: draft.name.trim(),
      color: draft.color,
      avatar: draft.avatar,
      security: {
        mode: sec.mode,
        preset: sec.mode === "encrypted" ? sec.preset : "",
        cipher: sec.preset === "custom" ? sec.cipher : "",
        kdf: sec.preset === "custom" ? sec.kdf : "",
        password: sec.mode === "encrypted" ? sec.password : "",
        recovery: sec.mode === "encrypted" && sec.recovery,
        windowsUnlock: sec.mode === "encrypted" && sec.windowsUnlock,
        keepBackup: sec.keepBackup,
        keepCache: false,
      },
      privacy: draft.privacy,
      appearance: draft.appearance,
      open: draft.open,
      import: draft.open && draft.import,
    };
    try {
      const result = await api("POST", "/api/profiles", request);
      sec.password = "";
      await refresh().catch(() => {});
      if (result.recoveryCode) {
        showRecovery(result.profile, result.recoveryCode);
      } else {
        dlg.close();
        toast("good", `${result.profile.name} was created`, draft.open ? "Opening it now." : "");
      }
    } catch (err) {
      creating = false;
      dlg.setBusy(false);
      next.disabled = back.disabled = false;
      mount(next, icon("check"), "Create profile");
      error.textContent = err.message;
    }
  }

  function showRecovery(profile, code) {
    dlg.setBusy(true);
    const saved = h("input", { type: "checkbox", class: "switch", id: "kc-saved" });
    const done = h("button", { class: "btn btn-primary", type: "button", disabled: true, onclick: () => { dlg.setBusy(false); dlg.close(); } }, "Done");
    saved.addEventListener("change", () => (done.disabled = !saved.checked));
    const copy = async () => {
      try {
        await navigator.clipboard.writeText(code);
        toast("good", "Recovery code copied", "Paste it somewhere safe, away from this computer.");
      } catch {
        toast("bad", "Could not copy", "Select the code and copy it manually.");
      }
    };
    const download = () => {
      const text = `Keycat recovery code\n\nProfile: ${profile.name}\nCreated: ${new Date().toLocaleString()}\n\n${code}\n\nThis code unlocks the profile if you forget its password. Keep it offline and private.\n`;
      const link = h("a", { href: URL.createObjectURL(new Blob([text], { type: "text/plain" })), download: `Keycat recovery code - ${profile.name}.txt` });
      document.body.append(link);
      link.click();
      link.remove();
    };
    mount(dlg.panel,
      h("div", { class: "modal-head" },
        h("div", {}, h("h2", { text: "Save your recovery code" }),
          h("p", { text: `If you forget the password, this code is the only way to unlock ${profile.name}. Keycat does not keep a copy.` }))),
      h("div", { class: "modal-body" },
        h("div", { class: "recovery-code", "aria-label": "Recovery code" }, ...code.split("-").map(group => h("span", { text: group }))),
        h("div", { class: "row-buttons" },
          h("button", { class: "btn", type: "button", onclick: copy }, icon("copy"), "Copy"),
          h("button", { class: "btn", type: "button", onclick: download }, icon("download"), "Download as text")),
        h("label", { class: "toggle-row", for: "kc-saved" },
          h("span", { class: "text" }, h("div", { class: "title", text: "I saved my recovery code somewhere safe" })), saved)
      ),
      h("div", { class: "modal-foot" }, h("span", { class: "spacer" }), done)
    );
  }

  render();
}

export function themePicker(value, onChange) {
  const themes = [["system", "System", "auto"], ["light", "Light", "light"], ["dark", "Dark", "dark"]];
  const group = h("div", { class: "theme-previews", role: "radiogroup", "aria-label": "Browser theme" });
  const render = () => mount(group, ...themes.map(([id, label, cls]) =>
    h("button", {
      type: "button", class: "theme-preview", role: "radio", "aria-checked": String(value === id),
      onclick: () => {
        value = id;
        onChange(id);
        render();
      },
    }, h("div", { class: `mini ${cls}` }, h("div", { class: "tabs-row" }), h("div", { class: "bar" }, h("i")), h("div", { class: "page" })), label)));
  render();
  return group;
}

function select(id, options, value, onChange) {
  const el = h("select", { class: "select", id }, options.map(([v, label]) => h("option", { value: v, text: label, selected: v === value })));
  el.addEventListener("change", () => onChange(el.value));
  return el;
}
