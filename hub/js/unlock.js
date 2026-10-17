import { api } from "./api.js";
import { store } from "./store.js";
import { avatar, fmtBytes, focusFirst, h, icon, modal, mount, toast } from "./dom.js";
import { securityLine } from "./views.js";
import { createSnake, resetProgress } from "./snake.js";

const MODES = {
  open: { title: "Unlock", action: "Unlock and open", path: "open", icon: "unlock" },
  secure: { title: "Secure leftover data", action: "Encrypt now", path: "secure", icon: "lock" },
  verify: { title: "Verify vault", action: "Verify vault", path: "verify", icon: "shieldCheck" },
  keychain: { title: "Unlock passwords", action: "Unlock passwords", path: "keychain/unlock", icon: "key" },
  "restore-backup": {
    title: "Restore the previous vault", action: "Verify and restore", path: "restore", icon: "history",
    body: credentials => ({ credentials, source: "backup" }),
  },
  "restore-rescue": {
    title: "Restore the rescue copy", action: "Verify and restore", path: "restore", icon: "shieldCheck",
    body: credentials => ({ credentials, source: "rescue" }),
  },
};

export function openUnlock(id, mode = "open", extra = {}) {
  return new Promise(resolve => {
    const p = store.profile(id);
    const cfg = MODES[mode];
    const slots = p.security.unlock.filter(s => s !== "windows" || store.state.app.windowsUnlock);
    let method = slots.includes("password") ? "password" : slots.includes("windows") ? "windows" : "recovery";
    let succeeded = false;
    let unsubscribe = () => {};
    const dlg = modal({
      label: `${cfg.title}: ${p.name}`,
      onClose: () => {
        unsubscribe();
        resolve(succeeded);
      },
    });

    const input = h("input", { class: "input", autocomplete: "off", spellcheck: "false" });
    const reveal = h("button", { class: "btn btn-icon", type: "button", "aria-label": "Show password" }, icon("eye"));
    reveal.addEventListener("click", () => {
      const showing = input.type === "text";
      input.type = showing ? "password" : "text";
      reveal.replaceChildren(icon(showing ? "eye" : "eyeOff"));
      reveal.setAttribute("aria-label", showing ? "Show password" : "Hide password");
      input.focus();
    });
    const error = h("div", { class: "error-text", role: "alert" });
    const snake = createSnake(p.color, "lg");
    const progress = h("div", { class: "unlock-progress", hidden: true }, snake.node);
    const submit = h("button", { class: "btn btn-primary btn-block", type: "submit" });

    const intro = {
      secure: "Keycat was interrupted while this profile was open, so its latest data is still decrypted on disk. Unlock it to encrypt that data again.",
      verify: "Decrypts the whole vault in memory and checks every block, without opening the browser.",
      keychain: "Unlock this profile's password manager without opening the browser. It locks again after 10 minutes.",
      "restore-backup": "Keycat checks the previous vault in full, then puts it back. The current vault becomes the previous one, so you can switch back the same way.",
      "restore-rescue": "Keycat checks the rescue copy in full, then puts it back. The current vault is kept as the previous vault.",
    }[mode] || (extra.then === "import" ? "After unlocking, the import window opens in the profile."
      : extra.link ? "After unlocking, the link opens in this profile." : "");

    const altLinks = h("div", { class: "unlock-alt" });

    function renderMethod() {
      const field = h("div", { class: "field" });
      if (method === "password") {
        input.type = "password";
        input.placeholder = "";
        input.classList.remove("input-mono");
        field.append(h("label", { class: "label", for: "kc-unlock-input", text: "Password" }),
          h("div", { class: "input-wrap" }, input, reveal));
      } else if (method === "recovery") {
        input.type = "text";
        input.placeholder = "XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX";
        input.classList.add("input-mono");
        field.append(h("label", { class: "label", for: "kc-unlock-input", text: "Recovery code" }), input);
      } else {
        field.append(h("div", { class: "callout" }, icon("windows"),
          h("span", { text: "Unlocks with the Windows account you are signed in to. No password needed on this computer." })));
      }
      input.id = "kc-unlock-input";
      input.value = "";
      input.removeAttribute("aria-invalid");
      error.textContent = "";
      submit.replaceChildren(icon(cfg.icon), cfg.action);
      mount(fieldHost, field);

      const links = [];
      const link = (label, target) => h("button", { type: "button", class: "link-btn", text: label, onclick: () => { method = target; renderMethod(); focusFirst(dlg.panel); } });
      if (method !== "password" && slots.includes("password")) {
        links.push(link("Use password", "password"));
      }
      if (method !== "recovery" && slots.includes("recovery")) {
        links.push(link("Use recovery code", "recovery"));
      }
      if (method !== "windows" && slots.includes("windows")) {
        links.push(link("Unlock with Windows", "windows"));
      }
      mount(altLinks, ...links.flatMap((l, i) => (i ? [h("span", { class: "dotsep", text: "·" }), l] : [l])));
    }

    const fieldHost = h("div");
    const form = h("form", { class: "modal-body", novalidate: true },
      fieldHost, error, progress,
      h("div", { class: "unlock-actions" }, submit, altLinks)
    );
    form.addEventListener("submit", event => {
      event.preventDefault();
      go();
    });

    mount(dlg.panel,
      h("div", { class: "unlock-head" },
        avatar(p, "lg"),
        h("h2", { text: mode === "open" ? p.name : `${cfg.title}: ${p.name}` }),
        securityLine(p),
        intro ? h("p", { class: "hint unlock-intro", text: intro }) : null
      ),
      form
    );
    renderMethod();
    focusFirst(dlg.panel);

    function setBusy(busy) {
      dlg.setBusy(busy);
      input.disabled = busy;
      fieldHost.hidden = busy;
      submit.disabled = busy;
      for (const b of altLinks.querySelectorAll("button")) {
        b.disabled = busy;
      }
      progress.hidden = !busy;
    }

    function showProgress(status) {
      const current = store.profile(id) || p;
      snake.update(current, status || { profileId: id, state: mode === "secure" ? "sealing" : "unlocking", phase: mode === "secure" ? "encrypt" : "key", progress: 0 });
    }

    async function go() {
      const cred = { method };
      if (method === "password") {
        if (!input.value) {
          input.setAttribute("aria-invalid", "true");
          error.textContent = "Enter the profile password.";
          input.focus();
          return;
        }
        cred.password = input.value;
      } else if (method === "recovery") {
        cred.recoveryCode = input.value;
      }
      const body = cfg.body ? cfg.body(cred) : { ...cred, ...extra };
      error.textContent = "";
      input.removeAttribute("aria-invalid");
      resetProgress(id);
      setBusy(true);
      showProgress(null);
      unsubscribe = store.on(state => {
        const current = state?.profiles.find(x => x.id === id)?.status;
        if (current && current.state !== "locked" && current.state !== "running") {
          showProgress(current);
        }
      });
      try {
        const result = await api("POST", `/api/profiles/${id}/${cfg.path}`, body);
        succeeded = true;
        if (mode === "verify") {
          showVerified(result);
          return;
        }
        if (mode === "secure") {
          toast("good", `${p.name} is encrypted again`, "The leftover decrypted data was sealed and wiped.");
        }
        if (cfg.path === "restore") {
          toast("good", `${p.name} was restored`, result.message || "");
        }
        dlg.close();
      } catch (err) {
        unsubscribe();
        setBusy(false);
        if (err.code === "wrong_password") {
          error.textContent = method === "recovery" ? "That recovery code is not correct." : "That password is not correct.";
          input.setAttribute("aria-invalid", "true");
          input.select();
          input.focus();
        } else {
          error.textContent = err.message;
        }
      }
    }

    function showVerified(result) {
      setBusy(false);
      const text = result.empty
        ? "This profile has not been opened yet, so there is no vault to check."
        : `All ${result.files} files (${fmtBytes(result.bytes)}) decrypted and authenticated in ${result.ms} ms.`;
      mount(form,
        h("div", { class: "callout good" }, icon("shieldCheck"), h("span", {},
          h("strong", { text: "The vault is intact." }), " ", text)),
        h("div", { class: "unlock-actions" },
          h("button", { class: "btn btn-primary btn-block", type: "button", text: "Done", onclick: () => dlg.close() }))
      );
    }
  });
}
